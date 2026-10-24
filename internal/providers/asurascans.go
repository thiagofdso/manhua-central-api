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
	asuraChapterRegex = regexp.MustCompile(`(\d+)\s*(?:<!-- -->)?\s*(?:Chs\.|Chapters)`)
	asuraPageRegex    = regexp.MustCompile(`page=(\d+)`)
)

type AsuraScansScraper struct {
	baseURL    string
	httpClient *http.Client
}

func NewAsuraScansScraper() *AsuraScansScraper {
	return NewAsuraScansScraperWithURL("https://asurascans.com")
}

func NewAsuraScansScraperWithURL(baseURL string) *AsuraScansScraper {
	return &AsuraScansScraper{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *AsuraScansScraper) Domain() string {
	return "asurascans.com"
}

func (s *AsuraScansScraper) Name() string {
	return "Asura Scans"
}

func (s *AsuraScansScraper) SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error) {
	reqURL, err := url.Parse(s.baseURL + "/browse/comics")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	q := reqURL.Query()
	qKey := strings.TrimSpace(query)
	if qKey != "" {
		q.Set("q", qKey)
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
	req.Header.Set("Referer", "https://asurascans.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to asurascans failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asurascans returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var items []domain.ScrapedItem

	doc.Find("div.series-card").Each(func(i int, s *goquery.Selection) {
		link := s.Find("a[href*='/comics/']").First()
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		parts := strings.Split(strings.Trim(href, "/"), "/")
		var slug string
		if len(parts) >= 2 && parts[0] == "comics" {
			slug = parts[1]
		} else {
			slug = parts[len(parts)-1]
		}

		title := strings.TrimSpace(s.Find("h3").First().Text())
		if title == "" {
			title = strings.TrimSpace(link.AttrOr("title", slug))
		}
		title = html.UnescapeString(title)

		coverURL := s.Find("img").First().AttrOr("src", "")

		// Extract chapter count
		cardText := s.Text()
		var lastChapter *int
		if matches := asuraChapterRegex.FindStringSubmatch(cardText); len(matches) > 1 {
			if ch, err := strconv.Atoi(matches[1]); err == nil {
				lastChapter = &ch
			}
		}

		urlTemplate := fmt.Sprintf("https://asurascans.com/comics/%s/chapter/{chapter}", slug)

		items = append(items, domain.ScrapedItem{
			Title:         title,
			Slug:          slug,
			URLTemplate:   urlTemplate,
			CoverURL:      coverURL,
			LastChapter:   lastChapter,
			PrimaryDomain: "asurascans.com",
		})
	})

	// Pagination
	totalPages := 1
	doc.Find("nav a, .pagination a").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if m := asuraPageRegex.FindStringSubmatch(href); len(m) > 1 {
			if p, err := strconv.Atoi(m[1]); err == nil && p > totalPages {
				totalPages = p
			}
		}
		if textP, err := strconv.Atoi(strings.TrimSpace(s.Text())); err == nil && textP > totalPages {
			totalPages = textP
		}
	})

	return &domain.ScrapeResult{
		Items:      items,
		Page:       page,
		TotalPages: totalPages,
	}, nil
}
