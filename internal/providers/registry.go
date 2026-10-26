package providers

import (
	"context"
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
	return list
}
