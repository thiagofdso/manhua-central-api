package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Streamer struct {
	guard              *SSRFGuard
	httpClient         *http.Client
	allowTestHost      string
	scraperWorkerURL   string
	hostCollectorURL   string
	hostCollectorToken string
}

func (s *Streamer) SetWorkerURL(url string) {
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	s.scraperWorkerURL = strings.TrimRight(url, "/")
}

func (s *Streamer) SetHostCollector(url, token string) {
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	s.hostCollectorURL = strings.TrimRight(url, "/")
	s.hostCollectorToken = token
}

func NewStreamer(guard *SSRFGuard) *Streamer {
	if guard == nil {
		guard = NewSSRFGuard(DefaultAllowedDomains)
	}

	streamer := &Streamer{
		guard: guard,
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Allow bypass only for mock local test server in unit tests
			if streamer.allowTestHost != "" && host == streamer.allowTestHost {
				return dialer.DialContext(ctx, network, addr)
			}

			// DNS Rebinding protection: validate actual resolved destination IP at connect time
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve host %s: %w", host, err)
			}

			for _, ip := range ips {
				if guard.IsPrivateOrReservedIP(ip) {
					return nil, fmt.Errorf("connection to %s blocked: %w", ip.String(), ErrSSRFBlocked)
				}
			}

			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	streamer.httpClient = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many image redirects")
			}
			if _, err := guard.ValidateURL(req.Context(), req.URL.String()); err != nil {
				return err
			}
			req.Header.Del("Cookie")
			streamer.applyDomainHeaders(req, req.URL.Hostname())
			return nil
		},
	}

	return streamer
}

func (s *Streamer) ProxyImage(ctx context.Context, w http.ResponseWriter, targetURL string) error {
	var target *url.URL
	var err error

	// If test host is configured for unit test, allow it
	u, parseErr := url.Parse(targetURL)
	if parseErr != nil {
		return fmt.Errorf("invalid url: %w", parseErr)
	}

	if s.allowTestHost != "" && u.Hostname() == s.allowTestHost {
		target = u
	} else {
		target, err = s.guard.ValidateURL(ctx, targetURL)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create upstream request: %w", err)
	}

	// Apply specialized headers per domain according to GEMINI.md Section 4.3 & ADR-0002
	s.applyDomainHeaders(req, target.Hostname())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if s.scraperWorkerURL != "" || s.hostCollectorURL != "" {
			return s.proxyViaScraperWorker(ctx, w, targetURL)
		}
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		if s.scraperWorkerURL != "" || s.hostCollectorURL != "" {
			return s.proxyViaScraperWorker(ctx, w, targetURL)
		}
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	// Forward response headers
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}

	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400") // 24h cache
	}

	if eTag := resp.Header.Get("ETag"); eTag != "" {
		w.Header().Set("ETag", eTag)
	}

	w.WriteHeader(http.StatusOK)

	// Stream directly to client with 32 KB chunk buffer (Zero-Buffer Streaming, ADR-0002)
	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(w, resp.Body, buf)
	if err != nil {
		return fmt.Errorf("streaming failed: %w", err)
	}

	return nil
}

func (s *Streamer) proxyViaScraperWorker(ctx context.Context, w http.ResponseWriter, targetURL string) error {
	workerBase := s.scraperWorkerURL
	token := ""
	if s.hostCollectorURL != "" {
		lower := strings.ToLower(targetURL)
		if strings.Contains(lower, "ravenscans.org") || strings.Contains(lower, "manhuaus.com") {
			workerBase = s.hostCollectorURL
			token = s.hostCollectorToken
		}
	}
	workerURL := workerBase + "/stream-image?url=" + url.QueryEscape(targetURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, workerURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create worker stream request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil && workerBase != s.scraperWorkerURL && s.scraperWorkerURL != "" {
		fallbackURL := s.scraperWorkerURL + "/stream-image?url=" + url.QueryEscape(targetURL)
		if fallbackReq, fbErr := http.NewRequestWithContext(ctx, http.MethodGet, fallbackURL, nil); fbErr == nil {
			if fbResp, fbDoErr := http.DefaultClient.Do(fallbackReq); fbDoErr == nil && fbResp.StatusCode == http.StatusOK {
				resp = fbResp
				err = nil
			}
		}
	}
	if err != nil {
		return fmt.Errorf("worker stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("worker stream returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/webp"
	}
	w.Header().Set("Content-Type", contentType)

	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}

	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.WriteHeader(http.StatusOK)

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(w, resp.Body, buf)
	if err != nil {
		return fmt.Errorf("streaming failed from worker: %w", err)
	}

	return nil
}

func (s *Streamer) applyDomainHeaders(req *http.Request, hostname string) {
	host := strings.ToLower(hostname)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	if strings.Contains(host, "asurascans.com") {
		req.Header.Set("Referer", "https://asurascans.com/")
	} else if strings.Contains(host, "ravenscans.org") {
		req.Header.Set("Referer", "https://ravenscans.org/")
	} else if strings.Contains(host, "mangafire.to") || strings.Contains(host, "mfcdn") {
		req.Header.Set("Referer", "https://mangafire.to/")
	} else if strings.Contains(host, "mangak.io") || strings.Contains(host, "qvzr") || strings.Contains(host, "resmk") {
		req.Header.Set("Referer", "https://mangak.io/")
	} else if strings.Contains(host, "manhuatop.org") || strings.Contains(host, "wp.com") {
		req.Header.Set("Referer", "https://manhuatop.org/")
	} else if strings.Contains(host, "arenascan") {
		req.Header.Set("Referer", "https://arenascan.com/")
	} else if strings.Contains(host, "mgeko") || strings.Contains(host, "imgsrv") {
		req.Header.Set("Referer", "https://www.mgeko.cc/")
	} else if strings.Contains(host, "levelingwithgods") {
		req.Header.Set("Referer", "https://levelingwithgods.com/")
	} else if strings.Contains(host, "pickmeupgacha") || strings.Contains(host, "abyssrift") {
		req.Header.Set("Referer", "https://pickmeupgacha.com/")
	} else if strings.Contains(host, "magicemperor") {
		req.Header.Set("Referer", "https://magicemperor.online/")
	} else if strings.Contains(host, "manhuaus.com") {
		req.Header.Set("Referer", "https://manhuaus.com/")
	} else if strings.Contains(host, "comix.to") || strings.HasSuffix(host, ".site") {
		// ADR-0002 / GEMINI.md: comix.to requires referer = None
		req.Header.Del("Referer")
	} else {
		req.Header.Set("Referer", "https://"+host+"/")
	}
}
