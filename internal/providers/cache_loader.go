package providers

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var DomainFilenameMap = map[string]DomainInfo{
	"import_cache_asurascans_com.json": {Domain: "asurascans.com", Name: "Asura Scans", Supported: true},
	"import_cache_ravenscans_org.json": {Domain: "ravenscans.org", Name: "RavenScans", Supported: true},
	"import_cache_comix_to.json":       {Domain: "comix.to", Name: "Comix", Supported: true},
	"import_cache_manhuaus_com.json":   {Domain: "manhuaus.com", Name: "ManhuaUS", Supported: true},
	"import_cache_arenascan_com.json":  {Domain: "arenascan.com", Name: "ArenaScan", Supported: true},
	"import_cache_manhuatop_org.json":  {Domain: "manhuatop.org", Name: "ManhuaTop", Supported: true},
	"import_cache_mgeko_cc.json":       {Domain: "mgeko.cc", Name: "MGeko", Supported: true},
	"import_cache_mangak_io.json":      {Domain: "mangak.io", Name: "MangaK", Supported: true},
	"import_cache_mangafire_to.json":   {Domain: "mangafire.to", Name: "MangaFire", Supported: true},
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

func GenerateSlug(title string) string {
	clean := strings.ToLower(strings.TrimSpace(title))
	clean = nonAlphanumericRegex.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "untitled"
	}
	return clean
}

type rawCachedItem struct {
	Title         string      `json:"title"`
	Slug          string      `json:"slug"`
	URLTemplate   string      `json:"urlTemplate"`
	CoverURL      string      `json:"coverUrl"`
	LastChapter   interface{} `json:"lastChapter"`
	PrimaryDomain string      `json:"primaryDomain"`
	Synopsis      string      `json:"synopsis"`
}

func (r *rawCachedItem) toScrapedItem(defaultDomain string) domain.ScrapedItem {
	var lastCh *int
	if r.LastChapter != nil {
		switch v := r.LastChapter.(type) {
		case float64:
			c := int(v)
			lastCh = &c
		case int:
			lastCh = &v
		case string:
			if iv, err := strconv.Atoi(v); err == nil {
				lastCh = &iv
			} else if fv, err := strconv.ParseFloat(v, 64); err == nil {
				c := int(fv)
				lastCh = &c
			}
		}
	}
	slug := r.Slug
	if slug == "" {
		if strings.Contains(r.URLTemplate, "/comics/") {
			parts := strings.Split(r.URLTemplate, "/comics/")
			if len(parts) > 1 {
				sub := strings.Split(parts[1], "/")[0]
				if sub != "" {
					slug = sub
				}
			}
		} else if strings.Contains(r.URLTemplate, "mangak.io/") {
			parts := strings.Split(r.URLTemplate, "mangak.io/")
			if len(parts) > 1 {
				sub := strings.Split(parts[1], "/")[0]
				if sub != "" {
					slug = sub
				}
			}
		}
		if slug == "" {
			slug = GenerateSlug(r.Title)
		}
	}
	domainName := r.PrimaryDomain
	if domainName == "" {
		domainName = defaultDomain
	}
	return domain.ScrapedItem{
		Title:         r.Title,
		Slug:          slug,
		URLTemplate:   r.URLTemplate,
		CoverURL:      r.CoverURL,
		LastChapter:   lastCh,
		PrimaryDomain: domainName,
	}
}

type CachedFilePayload struct {
	Timestamp  float64                               `json:"timestamp"`
	Catalog    map[string]map[string][]rawCachedItem `json:"catalog"`
	TotalPages map[string]map[string]int             `json:"totalPages"`
}

func GetDefaultCacheDir() string {
	if custom := os.Getenv("CACHE_DIR"); custom != "" {
		return custom
	}
	for _, candidate := range []string{
		"/.cache",
		".cache",
		"/app/.cache",
		"/home/thiagofdso/Projetos/manhua-manager-pro/apps/api-gateway/.cache",
		"/home/thiagofdso/Projetos/manhua-manager/.cache",
	} {
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
	}
	return ".cache"
}

func LoadCachedDomainItems(cacheDir, domainName, query string, page int) (*domain.ScrapeResult, error) {
	if cacheDir == "" {
		cacheDir = GetDefaultCacheDir()
	}

	cleanDomain := strings.ToLower(strings.TrimSpace(domainName))
	qClean := strings.ToLower(strings.TrimSpace(query))
	pKey := strconv.Itoa(page)

	if cleanDomain == "all" {
		return loadAllAggregatedCache(cacheDir, qClean, page)
	}

	var targetFilename string
	for fname, info := range DomainFilenameMap {
		if info.Domain == cleanDomain {
			targetFilename = fname
			break
		}
	}

	if targetFilename == "" {
		sanitized := strings.ReplaceAll(cleanDomain, ".", "_")
		targetFilename = fmt.Sprintf("import_cache_%s.json", sanitized)
	}

	filePath := filepath.Join(cacheDir, targetFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("arquivo de cache não encontrado para %s: %w", domainName, err)
	}

	var payload CachedFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("falha ao decodificar JSON de cache: %w", err)
	}

	if pagesMap, ok := payload.Catalog[qClean]; ok {
		if rawItems, ok := pagesMap[pKey]; ok {
			totalP := len(pagesMap)
			for pStr := range pagesMap {
				if pInt, err := strconv.Atoi(pStr); err == nil && pInt > totalP {
					totalP = pInt
				}
			}
			if tpMap, ok := payload.TotalPages[qClean]; ok {
				for _, tp := range tpMap {
					if tp > totalP {
						totalP = tp
					}
				}
			}
			items := make([]domain.ScrapedItem, len(rawItems))
			for i, raw := range rawItems {
				items[i] = raw.toScrapedItem(cleanDomain)
			}
			return &domain.ScrapeResult{
				Items:      items,
				Page:       page,
				TotalPages: totalP,
			}, nil
		}
	}

	var allItems []domain.ScrapedItem
	for _, pages := range payload.Catalog {
		for _, list := range pages {
			for _, raw := range list {
				it := raw.toScrapedItem(cleanDomain)
				if qClean == "" || strings.Contains(strings.ToLower(it.Title), qClean) {
					allItems = append(allItems, it)
				}
			}
		}
	}

	return paginateItems(allItems, cleanDomain, page, 20), nil
}

func loadAllAggregatedCache(cacheDir, query string, page int) (*domain.ScrapeResult, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return &domain.ScrapeResult{Items: []domain.ScrapedItem{}, Page: 1, TotalPages: 1}, nil
	}

	uniqueWorks := make(map[string]domain.ScrapedItem)

	for _, entry := range entries {
		fname := entry.Name()
		if !strings.HasPrefix(fname, "import_cache_") || !strings.HasSuffix(fname, ".json") || fname == "import_cache_all.json" {
			continue
		}

		info, ok := DomainFilenameMap[fname]
		dName := fname
		if ok {
			dName = info.Domain
		} else {
			dName = strings.TrimSuffix(strings.TrimPrefix(fname, "import_cache_"), ".json")
			dName = strings.ReplaceAll(dName, "_", ".")
		}

		filePath := filepath.Join(cacheDir, fname)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var payload CachedFilePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}

		for _, pages := range payload.Catalog {
			for _, list := range pages {
				for _, raw := range list {
					it := raw.toScrapedItem(dName)
					tmpl := strings.TrimSpace(it.URLTemplate)
					title := strings.TrimSpace(it.Title)
					if tmpl == "" || title == "" {
						continue
					}

					if query != "" && !strings.Contains(strings.ToLower(title), query) {
						continue
					}

					if _, exists := uniqueWorks[tmpl]; !exists {
						uniqueWorks[tmpl] = it
					}
				}
			}
		}
	}

	var all []domain.ScrapedItem
	for _, it := range uniqueWorks {
		all = append(all, it)
	}

	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].Title) < strings.ToLower(all[j].Title)
	})

	return paginateItems(all, "all", page, 40), nil
}

func paginateItems(items []domain.ScrapedItem, domainName string, page, limit int) *domain.ScrapeResult {
	if limit <= 0 {
		limit = 20
	}
	totalItems := len(items)
	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages && totalItems > 0 {
		page = totalPages
	}

	start := (page - 1) * limit
	if start > totalItems {
		start = totalItems
	}
	end := start + limit
	if end > totalItems {
		end = totalItems
	}

	paged := items[start:end]
	return &domain.ScrapeResult{
		Items:      paged,
		Page:       page,
		TotalPages: totalPages,
	}
}
