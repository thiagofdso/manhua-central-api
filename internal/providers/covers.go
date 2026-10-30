package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

func imageURL(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.Contains(raw, "placeholder") || strings.Contains(raw, "loading") {
		return ""
	}
	root, err := url.Parse(base)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	resolved := root.ResolveReference(ref)
	if resolved.Scheme != "https" && resolved.Scheme != "http" {
		return ""
	}
	return resolved.String()
}

func coverFromImage(img *goquery.Selection, base string) string {
	for _, attr := range []string{"data-src", "data-lazy-src", "data-original", "src"} {
		if cover := imageURL(img.AttrOr(attr, ""), base); cover != "" {
			return cover
		}
	}
	for _, attr := range []string{"data-srcset", "srcset"} {
		for _, candidate := range strings.Split(img.AttrOr(attr, ""), ",") {
			fields := strings.Fields(candidate)
			if len(fields) > 0 {
				if cover := imageURL(fields[0], base); cover != "" {
					return cover
				}
			}
		}
	}
	return ""
}

func extractCover(doc *goquery.Document, base string) string {
	var initial struct {
		Queries map[string]json.RawMessage `json:"queries"`
	}
	if json.Unmarshal([]byte(doc.Find("script#initial-data").Text()), &initial) == nil {
		for key, raw := range initial.Queries {
			if !strings.Contains(key, "detail") {
				continue
			}
			var detail struct {
				Poster struct {
					Large  string `json:"large"`
					Medium string `json:"medium"`
				} `json:"poster"`
			}
			if json.Unmarshal(raw, &detail) == nil {
				for _, candidate := range []string{detail.Poster.Large, detail.Poster.Medium} {
					if cover := imageURL(candidate, base); cover != "" {
						return cover
					}
				}
			}
		}
	}
	for _, selector := range []string{".summary_image img", ".info-image img", ".thumb img", ".manga-info-pic img", ".series-cover img", "img[itemprop='image']"} {
		if cover := coverFromImage(doc.Find(selector).First(), base); cover != "" {
			return cover
		}
	}
	for _, selector := range []string{"meta[property='og:image']", "meta[name='twitter:image']"} {
		if cover := imageURL(doc.Find(selector).AttrOr("content", ""), base); cover != "" && !strings.HasSuffix(cover, ".ico") {
			return cover
		}
	}
	return ""
}

var chapterSuffix = regexp.MustCompile(`(?i)(?:/chapter[-/]|-chapter-)\{chapter\}.*$`)

func coverPage(item domain.ScrapedItem) string {
	slug := item.Slug
	if source, err := url.Parse(chapterSuffix.ReplaceAllString(item.URLTemplate, "")); err == nil && source.Host != "" {
		parts := strings.Split(strings.Trim(source.Path, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			slug = parts[len(parts)-1]
		}
	}
	switch item.PrimaryDomain {
	case "ravenscans.org", "arenascan.com":
		return "https://" + item.PrimaryDomain + "/manga/" + slug + "/"
	case "mgeko.cc":
		return "https://www.mgeko.cc/manga/" + slug + "/"
	}
	page := chapterSuffix.ReplaceAllString(item.URLTemplate, "/")
	if item.PrimaryDomain == "comix.to" {
		page = strings.TrimSuffix(page, "/{chapter}")
	}
	if strings.Contains(page, "{") {
		return ""
	}
	return page
}

type coverEntry struct {
	url     string
	expires time.Time
}

var coverCache sync.Map
var coverClient = &http.Client{Timeout: 4 * time.Second}

func ResolveCover(ctx context.Context, item domain.ScrapedItem) string {
	items := []domain.ScrapedItem{item}
	fillMissingCovers(ctx, items)
	return items[0].CoverURL
}

func CoverPage(item domain.ScrapedItem) string { return coverPage(item) }

func fillMissingCovers(ctx context.Context, items []domain.ScrapedItem) {
	var group sync.WaitGroup
	slots := make(chan struct{}, 6)
	for index := range items {
		if items[index].CoverURL != "" {
			continue
		}
		group.Add(1)
		go func(index int) {
			defer group.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			page := coverPage(items[index])
			if page == "" {
				return
			}
			if cached, ok := coverCache.Load(page); ok && time.Now().Before(cached.(coverEntry).expires) {
				items[index].CoverURL = cached.(coverEntry).url
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")
			response, err := coverClient.Do(req)
			if err != nil {
				return
			}
			defer response.Body.Close()
			cover := ""
			if response.StatusCode == http.StatusOK {
				if doc, err := goquery.NewDocumentFromReader(response.Body); err == nil {
					cover = extractCover(doc, response.Request.URL.String())
				}
			}
			ttl := 10 * time.Minute
			if cover != "" {
				ttl = 24 * time.Hour
			}
			coverCache.Store(page, coverEntry{cover, time.Now().Add(ttl)})
			items[index].CoverURL = cover
		}(index)
	}
	group.Wait()
}
