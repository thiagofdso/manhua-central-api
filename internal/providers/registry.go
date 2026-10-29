package providers

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

type DomainInfo struct {
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Supported bool   `json:"supported"`
}

type DomainScraper interface {
	Domain() string
	Name() string
	SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error)
}

type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]DomainScraper
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]DomainScraper),
	}
}

func NewDefaultProviderRegistry(cacheDir string) *ProviderRegistry {
	if cacheDir == "" {
		cacheDir = GetDefaultCacheDir()
	}

	reg := NewProviderRegistry()

	// 1. Agregador Geral "all"
	reg.Register(NewCachedDomainScraper("all", "Todos (Geral)", cacheDir, nil))

	// 2. Provedores com Scraper Nativo + Fallback Cache
	reg.Register(NewCachedDomainScraper("asurascans.com", "Asura Scans", cacheDir, NewAsuraScansScraper()))
	reg.Register(NewCachedDomainScraper("mangak.io", "MangaK", cacheDir, NewMangaKScraper()))
	reg.Register(NewCachedDomainScraper("ravenscans.org", "RavenScans", cacheDir, NewRavenScansScraper()))
	reg.Register(NewCachedDomainScraper("manhuatop.org", "ManhuaTop", cacheDir, NewManhuaTopScraper()))

	// 3. Provedores com Cache Consolidado (Comix, MangaFire, ArenaScan, MGeko, ManhuaUS)
	reg.Register(NewCachedDomainScraper("comix.to", "Comix", cacheDir, nil))
	reg.Register(NewCachedDomainScraper("mangafire.to", "MangaFire", cacheDir, nil))
	reg.Register(NewCachedDomainScraper("arenascan.com", "ArenaScan", cacheDir, nil))
	reg.Register(NewCachedDomainScraper("mgeko.cc", "MGeko", cacheDir, nil))
	reg.Register(NewCachedDomainScraper("manhuaus.com", "ManhuaUS", cacheDir, nil))

	return reg
}

func (r *ProviderRegistry) Register(scraper DomainScraper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clean := strings.ToLower(strings.TrimSpace(scraper.Domain()))
	r.providers[clean] = scraper
}

func (r *ProviderRegistry) Get(domain string) (DomainScraper, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clean := strings.ToLower(strings.TrimSpace(domain))
	clean = strings.TrimPrefix(clean, "www.")
	sc, ok := r.providers[clean]
	return sc, ok
}

func (r *ProviderRegistry) ListDomains() []DomainInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []DomainInfo
	for _, p := range r.providers {
		list = append(list, DomainInfo{
			Domain:    p.Domain(),
			Name:      p.Name(),
			Supported: true,
		})
	}

	// Ordena: "all" sempre no topo, depois alfabético por nome
	sort.Slice(list, func(i, j int) bool {
		if list[i].Domain == "all" {
			return true
		}
		if list[j].Domain == "all" {
			return false
		}
		return list[i].Name < list[j].Name
	})

	return list
}
