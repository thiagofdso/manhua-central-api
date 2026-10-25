package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	// AsuraScans chapter image regex matching CDN webp URLs
	asuraChapterImageRegex = regexp.MustCompile(`https://cdn\.asurascans\.com/asura-images/chapters(?:-merged)?/[^\s"\'<>&]+(?:\?[^\s"\'<>&]*)?`)
	// RavenScans ts_reader.run JSON payload regex
	ravenTsReaderRegex = regexp.MustCompile(`ts_reader\.run\(\s*({.*?})\s*\);`)
	// RavenScans fallback direct URL regex
	ravenDirectImageRegex = regexp.MustCompile(`https?://[^\s"\'<>]*(?:ravenscans\.org)[^\s"\'<>]*\.(?:webp|jpg|jpeg|png)(?:\?[^\s"\'<>]*)?`)
	// MangaK __NEXT_DATA__ regex
	mangaKNextDataRegex = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)
	// MangaK fallback regex
	mangaKQvzrRegex = regexp.MustCompile(`https?://[^\s"\'<>]*qvzr[^\s"\'<>]*\.(?:webp|jpg|jpeg|png)`)
	// ManhuaTop wp-manga-chapter-img regex
	manhuaTopImageRegex = regexp.MustCompile(`(?i)<img[^>]+(?:src|data-src)=["']([^"']+)["'][^>]*class=["'][^"']*wp-manga-chapter-img[^"']*["']`)
)

type ExtractorService struct {
	scraperWorkerURL string
	httpClient       *http.Client
}

func NewExtractorService(scraperWorkerURL string, httpClient *http.Client) *ExtractorService {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 20 * time.Second,
		}
	}
	if scraperWorkerURL == "" {
		scraperWorkerURL = "http://localhost:50051"
	}
	return &ExtractorService{
		scraperWorkerURL: strings.TrimRight(scraperWorkerURL, "/"),
		httpClient:       httpClient,
	}
}

func (s *ExtractorService) ExtractChapterImages(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]string, error) {
	if m == nil {
		return nil, fmt.Errorf("canonical manhua cannot be nil")
	}

	targetURL := strings.ReplaceAll(m.URLTemplate, "{chapter}", strconv.Itoa(chapter))
	domainLower := strings.ToLower(m.PrimaryDomain)

	// Headless-only domains that require Puppeteer / Turnstile bypass
	if strings.Contains(domainLower, "mangafire.to") ||
		strings.Contains(domainLower, "comix.to") ||
		strings.Contains(domainLower, "manhuaus.com") {
		return s.extractViaScraperWorker(ctx, targetURL, domainLower, chapter)
	}

	// 1. Attempt fast static extraction
	images, err := s.extractStatic(ctx, targetURL, domainLower)
	if err == nil && len(images) > 0 {
		return images, nil
	}

	// 2. If static extraction failed or was blocked by Cloudflare (403/503), delegate to scraper worker pool
	workerImages, workerErr := s.extractViaScraperWorker(ctx, targetURL, domainLower, chapter)
	if workerErr == nil && len(workerImages) > 0 {
		return workerImages, nil
	}

	// Return error if both static and worker failed
	if err != nil {
		return nil, fmt.Errorf("extraction failed for %s (chapter %d): %w", m.Slug, chapter, err)
	}

	return nil, fmt.Errorf("no images found for %s (chapter %d)", m.Slug, chapter)
}

func (s *ExtractorService) extractStatic(ctx context.Context, targetURL string, domainLower string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	// Domain-specific Referer
	if strings.Contains(domainLower, "asura") {
		req.Header.Set("Referer", "https://asurascans.com/")
	} else if strings.Contains(domainLower, "ravenscans") {
		req.Header.Set("Referer", "https://ravenscans.org/")
	} else if strings.Contains(domainLower, "mangak.io") {
		req.Header.Set("Referer", "https://mangak.io/")
	} else if strings.Contains(domainLower, "manhuatop") {
		req.Header.Set("Referer", "https://manhuatop.org/")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	htmlContent := string(bodyBytes)

	// Domain-specific parsing logic
	if strings.Contains(domainLower, "asura") {
		return s.parseAsuraScansHTML(htmlContent)
	}

	if strings.Contains(domainLower, "ravenscans") {
		return s.parseRavenScansHTML(htmlContent)
	}

	if strings.Contains(domainLower, "mangak.io") {
		return s.parseMangaKHTML(htmlContent)
	}

	if strings.Contains(domainLower, "manhuatop") {
		return s.parseManhuaTopHTML(htmlContent)
	}

	// Generic reader HTML parsing
	return s.parseGenericHTML(htmlContent)
}

func (s *ExtractorService) parseAsuraScansHTML(html string) ([]string, error) {
	rawMatches := asuraChapterImageRegex.FindAllString(html, -1)
	if len(rawMatches) > 0 {
		return dedupPreserveOrder(rawMatches), nil
	}

	// Fallback to DOM parsing of reader container
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var images []string
	doc.Find("div.py-8 img, #readerarea img, .reading-content img, div[data-astro-cid] img").Each(func(_ int, sel *goquery.Selection) {
		src := sel.AttrOr("src", sel.AttrOr("data-src", ""))
		if src != "" && !strings.Contains(src, "logo") && !strings.Contains(src, "avatar") && !strings.HasPrefix(src, "data:") {
			images = append(images, src)
		}
	})

	if len(images) > 0 {
		return dedupPreserveOrder(images), nil
	}

	return nil, fmt.Errorf("no asurascans chapter images found")
}

func (s *ExtractorService) parseRavenScansHTML(html string) ([]string, error) {
	// Strategy 1: ts_reader.run JSON payload
	if match := ravenTsReaderRegex.FindStringSubmatch(html); len(match) > 1 {
		var payload struct {
			Sources []struct {
				Images []string `json:"images"`
			} `json:"sources"`
		}
		cleanJSON := strings.ReplaceAll(match[1], `\/`, `/`)
		if err := json.Unmarshal([]byte(cleanJSON), &payload); err == nil {
			if len(payload.Sources) > 0 && len(payload.Sources[0].Images) > 0 {
				return dedupPreserveOrder(payload.Sources[0].Images), nil
			}
		}
	}

	// Strategy 2: #readerarea container
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err == nil {
		var images []string
		doc.Find("#readerarea img").Each(func(_ int, sel *goquery.Selection) {
			src := sel.AttrOr("data-src", sel.AttrOr("src", ""))
			if src != "" && !strings.Contains(src, "readerarea.svg") && !strings.HasPrefix(src, "data:") {
				images = append(images, src)
			}
		})
		if len(images) > 0 {
			return dedupPreserveOrder(images), nil
		}
	}

	// Strategy 3: Regex
	raw := ravenDirectImageRegex.FindAllString(html, -1)
	var filtered []string
	for _, u := range raw {
		if !strings.Contains(u, "/wp-content/themes/") && !strings.Contains(u, "logo") && !strings.Contains(u, "favicon") {
			filtered = append(filtered, u)
		}
	}

	if len(filtered) > 0 {
		return dedupPreserveOrder(filtered), nil
	}

	return nil, fmt.Errorf("no ravenscans chapter images found")
}

func (s *ExtractorService) parseMangaKHTML(html string) ([]string, error) {
	if match := mangaKNextDataRegex.FindStringSubmatch(html); len(match) > 1 {
		var nextData struct {
			Props struct {
				PageProps struct {
					InitialChapter struct {
						Images []string `json:"images"`
					} `json:"initialChapter"`
				} `json:"pageProps"`
			} `json:"props"`
		}
		if err := json.Unmarshal([]byte(match[1]), &nextData); err == nil {
			imgs := nextData.Props.PageProps.InitialChapter.Images
			if len(imgs) > 0 {
				return dedupPreserveOrder(imgs), nil
			}
		}
	}

	raw := mangaKQvzrRegex.FindAllString(html, -1)
	if len(raw) > 0 {
		return dedupPreserveOrder(raw), nil
	}

	return nil, fmt.Errorf("no mangak chapter images found")
}

func (s *ExtractorService) parseManhuaTopHTML(html string) ([]string, error) {
	matches := manhuaTopImageRegex.FindAllStringSubmatch(html, -1)
	var images []string
	for _, m := range matches {
		if len(m) > 1 {
			u := strings.TrimSpace(m[1])
			if !strings.Contains(u, "about_manhuatop") && !strings.HasPrefix(u, "data:") {
				images = append(images, u)
			}
		}
	}

	if len(images) > 0 {
		return dedupPreserveOrder(images), nil
	}

	return nil, fmt.Errorf("no manhuatop chapter images found")
}

func (s *ExtractorService) parseGenericHTML(html string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var images []string
	doc.Find("#readerarea img, #chapter-reader img, .entry-content img, .reading-content img").Each(func(_ int, sel *goquery.Selection) {
		src := sel.AttrOr("data-src", sel.AttrOr("src", ""))
		src = strings.TrimSpace(src)
		if src != "" && !strings.HasPrefix(src, "data:") {
			lower := strings.ToLower(src)
			if !strings.Contains(lower, "logo") && !strings.Contains(lower, "avatar") && !strings.Contains(lower, "banner") {
				images = append(images, src)
			}
		}
	})

	if len(images) > 0 {
		return dedupPreserveOrder(images), nil
	}

	return nil, fmt.Errorf("no generic chapter images found in html")
}

func (s *ExtractorService) extractViaScraperWorker(ctx context.Context, targetURL, domain string, chapter int) ([]string, error) {
	payload := map[string]interface{}{
		"url":     targetURL,
		"domain":  domain,
		"chapter": chapter,
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	workerURL := s.scraperWorkerURL + "/extract-chapter"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, workerURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper worker request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper worker returned HTTP %d", resp.StatusCode)
	}

	var res struct {
		Success bool `json:"success"`
		Images  []struct {
			URL string `json:"url"`
		} `json:"images"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode worker response: %w", err)
	}

	if !res.Success || len(res.Images) == 0 {
		return nil, fmt.Errorf("scraper worker failed: %s", res.Error)
	}

	var result []string
	for _, img := range res.Images {
		if img.URL != "" {
			result = append(result, img.URL)
		}
	}

	return dedupPreserveOrder(result), nil
}

func dedupPreserveOrder(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))

	for _, item := range items {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		// Decode HTML entities if any (e.g. &amp;)
		clean = strings.ReplaceAll(clean, "&amp;", "&")
		if _, exists := seen[clean]; !exists {
			seen[clean] = struct{}{}
			result = append(result, clean)
		}
	}

	return result
}
