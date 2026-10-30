package providers

import (
	"context"
	"errors"
	"time"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

type CachedDomainScraper struct {
	domain   string
	name     string
	cacheDir string
	fallback DomainScraper
}

func NewCachedDomainScraper(domain, name, cacheDir string, fallback DomainScraper) *CachedDomainScraper {
	return &CachedDomainScraper{
		domain:   domain,
		name:     name,
		cacheDir: cacheDir,
		fallback: fallback,
	}
}

func (s *CachedDomainScraper) Domain() string {
	return s.domain
}

func (s *CachedDomainScraper) Name() string {
	return s.name
}

func (s *CachedDomainScraper) SearchOrBrowse(ctx context.Context, query string, page int) (result *domain.ScrapeResult, resultErr error) {
	defer func() {
		if result != nil && s.domain != "manhuaus.com" && s.domain != "ravenscans.org" {
			coverCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			fillMissingCovers(coverCtx, result.Items)
		}
	}()
	cachedRes, cacheErr := LoadCachedDomainItems(s.cacheDir, s.domain, query, page)
	if errors.Is(cacheErr, ErrCachedPageMissing) {
		return nil, cacheErr
	}

	// Se tiver um scraper nativo/ao vivo, tenta executar primeiro
	if s.fallback != nil {
		res, err := s.fallback.SearchOrBrowse(ctx, query, page)
		if err == nil && res != nil && len(res.Items) > 0 {
			if cachedRes != nil && cachedRes.TotalPages > res.TotalPages {
				res.TotalPages = cachedRes.TotalPages
			}
			return res, nil
		}
	}

	// Caso falhe ou não tenha fallback nativo, consulta o cache consolidado
	if cachedRes != nil && len(cachedRes.Items) > 0 {
		return cachedRes, nil
	}

	return LoadCachedDomainItems(s.cacheDir, s.domain, query, page)
}
