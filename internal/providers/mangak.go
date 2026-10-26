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
	mangakChapterRegex = regexp.MustCompile(`(?i)(?:chapter|ch\.?)\s*(\d+)`)
)

type MangaKScraper struct {
	baseURL    string
	httpClient *http.Client
}

func NewMangaKScraper() *MangaKScraper {
	return NewMangaKScraperWithURL("https://mangak.io")
}

func NewMangaKScraperWithURL(baseURL string) *MangaKScraper {
	return &MangaKScraper{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *MangaKScraper) Domain() string {
	return "mangak.io"
}

func (s *MangaKScraper) Name() string {
	return "MangaK"
}

func (s *MangaKScraper) SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error) {
	var targetURL string
	qKey := strings.TrimSpace(query)
	if qKey != "" {
		targetURL = fmt.Sprintf("%s/search?q=%s", s.baseURL, url.QueryEscape(qKey))
		if page > 1 {
			targetURL = fmt.Sprintf("%s&page=%d", targetURL, page)
		}
	} else {
		targetURL = fmt.Sprintf("%s/filter?page=%d", s.baseURL, page)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://mangak.io/")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mangak request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mangak returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var items []domain.ScrapedItem
	doc.Find("div.story-item, div.manga-item, .list-story-item").Each(func(i int, sel *goquery.Selection) {
		link := sel.Find("a[href*='/manga/']").First()
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		parts := strings.Split(strings.Trim(href, "/"), "/")
		slug := parts[len(parts)-1]

		title := strings.TrimSpace(sel.Find("h3, .story-title, .title").First().Text())
		if title == "" {
			title = strings.TrimSpace(link.AttrOr("title", slug))
		}
		title = html.UnescapeString(title)

		coverURL := sel.Find("img").First().AttrOr("src", "")
		if strings.HasPrefix(coverURL, "//") {
			coverURL = "https:" + coverURL
		}

		cardText := sel.Text()
		var lastChapter *int
		if matches := mangakChapterRegex.FindStringSubmatch(cardText); len(matches) > 1 {
			if ch, err := strconv.Atoi(matches[1]); err == nil {
				lastChapter = &ch
			}
		}

		urlTemplate := fmt.Sprintf("https://mangak.io/manga/%s/chapter-{chapter}", slug)

		items = append(items, domain.ScrapedItem{
			Title:         title,
			Slug:          slug,
			URLTemplate:   urlTemplate,
			CoverURL:      coverURL,
			LastChapter:   lastChapter,
			PrimaryDomain: "mangak.io",
		})
	})

	return &domain.ScrapeResult{
		Items:      items,
		Page:       page,
		TotalPages: 10,
	}, nil
}
