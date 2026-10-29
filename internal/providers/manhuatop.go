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
	manhuatopChapterRegex = regexp.MustCompile(`(?i)(?:chapter|ch\.?)\s*(\d+)`)
)

type ManhuaTopScraper struct {
	baseURL    string
	httpClient *http.Client
}

func NewManhuaTopScraper() *ManhuaTopScraper {
	return NewManhuaTopScraperWithURL("https://manhuatop.org")
}

func NewManhuaTopScraperWithURL(baseURL string) *ManhuaTopScraper {
	return &ManhuaTopScraper{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *ManhuaTopScraper) Domain() string {
	return "manhuatop.org"
}

func (s *ManhuaTopScraper) Name() string {
	return "ManhuaTop"
}

func (s *ManhuaTopScraper) SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error) {
	var targetURL string
	qKey := strings.TrimSpace(query)
	if qKey != "" {
		targetURL = fmt.Sprintf("%s/page/%d/?s=%s&post_type=wp-manga", s.baseURL, page, url.QueryEscape(qKey))
	} else {
		targetURL = fmt.Sprintf("%s/manhua-genre/manhua/page/%d/?m_orderby=alphabet", s.baseURL, page)
		if page <= 1 {
			targetURL = s.baseURL + "/manhua-genre/manhua/?m_orderby=alphabet"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://manhuatop.org/")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manhuatop request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manhuatop returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var items []domain.ScrapedItem
	doc.Find("div.page-item-detail, div.c-tabs-item__content, .comic_post__inner").Each(func(i int, sel *goquery.Selection) {
		link := sel.Find("a[href*='/manhua/']").First()
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		parts := strings.Split(strings.Trim(href, "/"), "/")
		slug := parts[len(parts)-1]

		title := strings.TrimSpace(sel.Find(".comic_post__title, .post-title, h3, h4").First().Text())
		if title == "" {
			title = strings.TrimSpace(link.AttrOr("title", slug))
		}
		title = html.UnescapeString(title)

		coverURL := coverFromImage(sel.Find("img").First(), s.baseURL)
		if strings.HasPrefix(coverURL, "//") {
			coverURL = "https:" + coverURL
		}

		cardText := sel.Text()
		var lastChapter *int
		if matches := manhuatopChapterRegex.FindStringSubmatch(cardText); len(matches) > 1 {
			if ch, err := strconv.Atoi(matches[1]); err == nil {
				lastChapter = &ch
			}
		}

		urlTemplate := fmt.Sprintf("https://manhuatop.org/manhua/%s/chapter-{chapter}", slug)

		items = append(items, domain.ScrapedItem{
			Title:         title,
			Slug:          slug,
			URLTemplate:   urlTemplate,
			CoverURL:      coverURL,
			LastChapter:   lastChapter,
			PrimaryDomain: "manhuatop.org",
		})
	})

	return &domain.ScrapeResult{
		Items:      items,
		Page:       page,
		TotalPages: 10,
	}, nil
}
