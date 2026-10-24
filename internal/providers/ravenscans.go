package providers

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	ravenChapterRegex = regexp.MustCompile(`(?i)(?:Chapter|Ch\.)?\s*(\d+)`)
	ravenPageRegex    = regexp.MustCompile(`(?:page[=/]|page=)(\d+)`)
)

type RavenScansScraper struct {
	baseURL    string
	httpClient *http.Client
}

func NewRavenScansScraper() *RavenScansScraper {
	return NewRavenScansScraperWithURL("https://ravenscans.org")
}

func NewRavenScansScraperWithURL(baseURL string) *RavenScansScraper {
	return &RavenScansScraper{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *RavenScansScraper) Domain() string {
	return "ravenscans.org"
}

func (s *RavenScansScraper) Name() string {
	return "RavenScans"
}

func (s *RavenScansScraper) SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error) {
	reqURL, err := url.Parse(s.baseURL + "/manga/")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := reqURL.Query()
	qKey := strings.TrimSpace(query)
	if qKey != "" {
		q.Set("s", qKey)
	} else {
		q.Set("order", "update")
	}

	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://ravenscans.org/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to ravenscans failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ravenscans returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var items []domain.ScrapedItem

	doc.Find("div.bsx").Each(func(i int, sel *goquery.Selection) {
		link := sel.Find("a").First()
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		// Slug extraction from /manga/slug/ or https://ravenscans.org/manga/slug/
		parsed, err := url.Parse(href)
		path := href
		if err == nil && parsed.Path != "" {
			path = parsed.Path
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		var slug string
		for idx, p := range parts {
			if p == "manga" && idx+1 < len(parts) {
				slug = parts[idx+1]
				break
			}
		}
		if slug == "" && len(parts) > 0 {
			slug = parts[len(parts)-1]
		}

		rawTitle := strings.TrimSpace(sel.Find(".tt").First().Text())
		if rawTitle == "" {
			rawTitle = strings.TrimSpace(link.AttrOr("title", slug))
		}
		title := html.UnescapeString(rawTitle)

		coverURL := sel.Find("img").First().AttrOr("src", "")

		var lastChapter *int
		epxs := strings.TrimSpace(sel.Find(".epxs").First().Text())
		if matches := ravenChapterRegex.FindStringSubmatch(epxs); len(matches) > 1 {
			if ch, err := strconv.Atoi(matches[1]); err == nil {
				lastChapter = &ch
			}
		}

		urlTemplate := fmt.Sprintf("https://ravenscans.org/%s-chapter-{chapter}/", slug)

		items = append(items, domain.ScrapedItem{
			Title:         title,
			Slug:          slug,
			URLTemplate:   urlTemplate,
			CoverURL:      coverURL,
			LastChapter:   lastChapter,
			PrimaryDomain: "ravenscans.org",
		})
	})

	// Pagination
	totalPages := 1
	doc.Find(".pagination a, .pagination span, .nav-links a").Each(func(i int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		if m := ravenPageRegex.FindStringSubmatch(href); len(m) > 1 {
			if p, err := strconv.Atoi(m[1]); err == nil && p > totalPages {
				totalPages = p
			}
		}
		if textP, err := strconv.Atoi(strings.TrimSpace(sel.Text())); err == nil && textP > totalPages {
			totalPages = textP
		}
	})

	return &domain.ScrapeResult{
		Items:      items,
		Page:       page,
		TotalPages: totalPages,
	}, nil
}
