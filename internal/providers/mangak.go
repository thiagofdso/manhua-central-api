package providers

import (
	"context"
	"encoding/json"
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
	nextDataRegex      = regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)
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
		targetURL = fmt.Sprintf("%s/latest?page=%d&sort=latest", s.baseURL, page)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://mangak.io/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

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

	htmlContent, _ := doc.Html()

	// 1. Tentar extrair de __NEXT_DATA__
	if m := nextDataRegex.FindStringSubmatch(htmlContent); len(m) > 1 {
		var nextPayload struct {
			Props struct {
				PageProps struct {
					SsrItems []map[string]interface{} `json:"ssrItems"`
					Items    []map[string]interface{} `json:"items"`
				} `json:"pageProps"`
			} `json:"props"`
		}

		if err := json.Unmarshal([]byte(m[1]), &nextPayload); err == nil {
			rawList := nextPayload.Props.PageProps.SsrItems
			if len(rawList) == 0 {
				rawList = nextPayload.Props.PageProps.Items
			}

			if len(rawList) > 0 {
				var items []domain.ScrapedItem
				for _, it := range rawList {
					name, _ := it["name"].(string)
					slug, _ := it["slug"].(string)
					if slug == "" {
						slug, _ = it["url"].(string)
					}
					slug = strings.Trim(slug, "/")
					if name == "" || slug == "" {
						continue
					}

					cover, _ := it["cover"].(string)
					if cover == "" {
						cover, _ = it["poster"].(string)
					}

					var lastChapter *int
					if latestChs, ok := it["latestChapters"].([]interface{}); ok && len(latestChs) > 0 {
						if firstCh, ok := latestChs[0].(map[string]interface{}); ok {
							chName, _ := firstCh["name"].(string)
							if chName == "" {
								chName, _ = firstCh["slug"].(string)
							}
							if matches := mangakChapterRegex.FindStringSubmatch(chName); len(matches) > 1 {
								if ch, err := strconv.Atoi(matches[1]); err == nil {
									lastChapter = &ch
								}
							}
						}
					}

					if lastChapter == nil {
						if stats, ok := it["stats"].(map[string]interface{}); ok {
							if countFloat, ok := stats["chaptersCount"].(float64); ok && countFloat > 0 {
								c := int(countFloat)
								lastChapter = &c
							}
						}
					}

					items = append(items, domain.ScrapedItem{
						Title:         html.UnescapeString(strings.TrimSpace(name)),
						Slug:          slug,
						URLTemplate:   fmt.Sprintf("https://mangak.io/%s/chapter-{chapter}", slug),
						CoverURL:      cover,
						LastChapter:   lastChapter,
						PrimaryDomain: "mangak.io",
					})
				}

				if len(items) > 0 {
					return &domain.ScrapeResult{
						Items:      items,
						Page:       page,
						TotalPages: 10,
					}, nil
				}
			}
		}
	}

	// 2. Fallback para DOM HTML
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

		coverURL := coverFromImage(sel.Find("img").First(), s.baseURL)
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
