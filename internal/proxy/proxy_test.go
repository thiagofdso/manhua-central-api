package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSSRFGuard_PrivateIPs(t *testing.T) {
	guard := NewSSRFGuard([]string{"allowed.com"})

	blockedIPs := []string{
		"127.0.0.1",
		"127.0.1.1",
		"::1",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.1.100",
		"169.254.169.254", // AWS/GCP/Azure metadata
		"169.254.1.1",
		"0.0.0.0",
		"100.64.0.1", // CGNAT
	}

	for _, ipStr := range blockedIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed to parse IP %s", ipStr)
		}
		if !guard.IsPrivateOrReservedIP(ip) {
			t.Errorf("expected IP %s to be blocked as private/reserved, but was allowed", ipStr)
		}
	}

	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"104.26.1.1",
		"151.101.1.69",
	}

	for _, ipStr := range publicIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed to parse IP %s", ipStr)
		}
		if guard.IsPrivateOrReservedIP(ip) {
			t.Errorf("expected IP %s to be allowed as public, but was blocked", ipStr)
		}
	}
}

func TestSSRFGuard_DomainAllowlist(t *testing.T) {
	guard := NewSSRFGuard(nil) // Use DefaultAllowedDomains

	allowedDomains := []string{
		"asurascans.com",
		"cdn.asurascans.com",
		"img.sub.asurascans.com",
		"ravenscans.org",
		"cdn.ravenscans.org",
		"cdn1.ravenscans.org",
		"mangafire.to",
		"q73.mfcdn2.xyz",
		"l1n.mfcdn1.xyz",
		"s.mfcdn.nl",
		"mangak.io",
		"rx.qvzrc.org",
		"rx.qvzrd.org",
		"cdn.resmk.org",
		"mgeko.cc",
		"www.mgeko.cc",
		"imgsrv5.com",
		"cdn.imgsrv1.com",
		"arenascan.com",
		"cdn.arenascan.com",
		"w14.levelingwithgods.com",
		"abyssrift.com",
		"magicemperor.online",
		"manhuaus.com",
		"comix.to",
		"black-clover.org",
	}

	for _, d := range allowedDomains {
		if !guard.IsDomainAllowed(d) {
			t.Errorf("expected domain %s to be allowed, but was blocked", d)
		}
	}

	blockedDomains := []string{
		"localhost",
		"evil.com",
		"asurascans.com.evil.com",
		"attacker-ravenscans.org",
		"169.254.169.254",
		"internal.service",
	}

	for _, d := range blockedDomains {
		if guard.IsDomainAllowed(d) {
			t.Errorf("expected domain %s to be blocked, but was allowed", d)
		}
	}
}

func TestStreamer_ProxyImage_StreamingAndHeaders(t *testing.T) {
	// Create mock upstream CDN server
	var receivedReferer string
	var receivedUserAgent string
	mockCDN := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedReferer = r.Header.Get("Referer")
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "image/webp")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock-image-binary-stream-data"))
	}))
	defer mockCDN.Close()

	u, err := url.Parse(mockCDN.URL)
	if err != nil {
		t.Fatalf("failed to parse mockCDN URL: %v", err)
	}

	// Create guard allowing the mock CDN host
	hostWithoutPort := u.Hostname()
	guard := NewSSRFGuard([]string{hostWithoutPort, "asurascans.com", "cdn.asurascans.com"})
	// For testing mockCDN on loopback, we create a test streamer that bypasses IP check only for mockCDN host
	streamer := NewStreamer(guard)
	streamer.allowTestHost = hostWithoutPort

	rec := httptest.NewRecorder()
	err = streamer.ProxyImage(context.Background(), rec, mockCDN.URL+"/cover.webp")
	if err != nil {
		t.Fatalf("expected successful proxy streaming, got err: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", rec.Header().Get("Content-Type"))
	}

	body, _ := io.ReadAll(rec.Body)
	if string(body) != "mock-image-binary-stream-data" {
		t.Errorf("unexpected body streamed: %s", string(body))
	}

	if receivedUserAgent == "" {
		t.Errorf("expected User-Agent to be forwarded")
	}
	_ = receivedReferer
}

func TestStreamer_ProxyImage_BlocksSSRF(t *testing.T) {
	guard := NewSSRFGuard([]string{"asurascans.com"})
	streamer := NewStreamer(guard)

	rec := httptest.NewRecorder()
	// Attempt private IP
	err := streamer.ProxyImage(context.Background(), rec, "http://127.0.0.1:8080/admin")
	if err == nil {
		t.Fatalf("expected SSRF attack to 127.0.0.1 to be blocked, but err was nil")
	}

	// Attempt non-allowed domain
	err = streamer.ProxyImage(context.Background(), rec, "http://malicious-site.com/exploit.jpg")
	if err == nil {
		t.Fatalf("expected non-allowed domain to be blocked, but err was nil")
	}
}
