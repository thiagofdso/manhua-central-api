package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type ImportAnnotatedItem struct {
	CoverState       string `json:"coverState,omitempty"`
	Title            string `json:"title"`
	Slug             string `json:"slug"`
	URLTemplate      string `json:"urlTemplate"`
	CoverURL         string `json:"coverUrl"`
	LastChapter      *int   `json:"lastChapter,omitempty"`
	PrimaryDomain    string `json:"primaryDomain"`
	Synopsis         string `json:"synopsis,omitempty"`
	AlreadyInCatalog bool   `json:"alreadyInCatalog"`
	AlreadyInLibrary bool   `json:"alreadyInLibrary"`
}

type ImportBrowseResult struct {
	Syncing    bool                  `json:"syncing"`
	Warning    string                `json:"warning,omitempty"`
	Domain     string                `json:"domain"`
	Page       int                   `json:"page"`
	TotalPages int                   `json:"totalPages"`
	Items      []ImportAnnotatedItem `json:"items"`
}

type ImportPayloadItem struct {
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	URLTemplate   string `json:"urlTemplate"`
	CoverURL      string `json:"coverUrl"`
	LastChapter   *int   `json:"lastChapter,omitempty"`
	PrimaryDomain string `json:"primaryDomain"`
	Synopsis      string `json:"synopsis,omitempty"`
}

type ImportResult struct {
	Status              string `json:"status"`
	ImportedCount       int    `json:"importedCount"`
	AddedToLibraryCount int    `json:"addedToLibraryCount"`
}

type ImportService struct {
	collection    *CatalogCollection
	registry      *providers.ProviderRegistry
	redisClient   *repository.RedisClient
	canonicalRepo repository.CanonicalRepository
	librarySvc    *LibraryService
}

func (s *ImportService) SetCollection(collection *CatalogCollection) { s.collection = collection }

func NewImportService(
	registry *providers.ProviderRegistry,
	redisClient *repository.RedisClient,
	canonicalRepo repository.CanonicalRepository,
	librarySvc *LibraryService,
) *ImportService {
	return &ImportService{
		registry:      registry,
		redisClient:   redisClient,
		canonicalRepo: canonicalRepo,
		librarySvc:    librarySvc,
	}
}

func (s *ImportService) GetDomains() []providers.DomainInfo {
	return s.registry.ListDomains()
}

func (s *ImportService) Browse(ctx context.Context, domainName, query string, page int, userID *string) (*ImportBrowseResult, error) {
	cleanDomain := strings.ToLower(strings.TrimSpace(domainName))
	qClean := strings.ToLower(strings.TrimSpace(query))
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("import:cache:v2:%s:%s:%d", cleanDomain, qClean, page)

	var scrapeRes domain.ScrapeResult
	syncing := false
	useCollection := s.collection != nil && (cleanDomain == "manhuaus.com" || cleanDomain == "ravenscans.org")
	if useCollection {
		result, pending, err := s.collection.Browse(ctx, cleanDomain, qClean, page)
		if err != nil {
			return nil, err
		}
		scrapeRes = *result
		syncing = pending
	}

	// 1. Tentar ler do Redis
	if !useCollection && s.redisClient != nil && s.redisClient.Client != nil {
		cachedStr, err := s.redisClient.Client.Get(ctx, cacheKey).Result()
		if err == nil && cachedStr != "" {
			_ = json.Unmarshal([]byte(cachedStr), &scrapeRes)
		}
	}

	// 2. Se não estiver no cache, chamar o scraper
	if !useCollection && len(scrapeRes.Items) == 0 {
		scraper, exists := s.registry.Get(cleanDomain)
		if !exists {
			return nil, fmt.Errorf("domínio '%s' não suportado para importação", domainName)
		}

		res, err := scraper.SearchOrBrowse(ctx, qClean, page)
		if err != nil {
			return nil, fmt.Errorf("falha ao buscar obras em %s: %w", domainName, err)
		}
		scrapeRes = *res

		// Salvar no Redis com TTL de 7 dias
		if s.redisClient != nil && s.redisClient.Client != nil && len(scrapeRes.Items) > 0 {
			if bytes, err := json.Marshal(scrapeRes); err == nil {
				ttl := 7 * 24 * time.Hour
				for _, item := range scrapeRes.Items {
					if item.CoverURL == "" {
						ttl = 10 * time.Minute
						break
					}
				}
				_ = s.redisClient.Client.Set(ctx, cacheKey, string(bytes), ttl).Err()
			}
		}
	}

	// 3. Anotar itens com status local
	annotated := make([]ImportAnnotatedItem, 0, len(scrapeRes.Items))

	// Cache de biblioteca do usuário se logado
	userLibMap := make(map[int64]bool)
	if userID != nil && *userID != "" && s.librarySvc != nil {
		if libItems, err := s.librarySvc.GetUserLibrary(ctx, *userID); err == nil {
			for _, libItem := range libItems {
				userLibMap[libItem.CanonicalManhuaID] = true
			}
		}
	}

	for _, it := range scrapeRes.Items {
		persistentCover := it.PrimaryDomain == "manhuaus.com" || it.PrimaryDomain == "ravenscans.org"
		if persistentCover {
			it.CoverURL = localCoverURL(it.CoverURL)
		}
		slug := it.Slug
		if slug == "" {
			slug = strings.ToLower(strings.ReplaceAll(it.Title, " ", "-"))
		}

		inCatalog := false
		inLibrary := false

		if s.canonicalRepo != nil {
			if existing, _ := s.canonicalRepo.GetBySlug(ctx, slug); existing != nil {
				if persistentCover && existing.PrimaryDomain == it.PrimaryDomain && existing.URLTemplate == it.URLTemplate && existing.CoverURL != nil {
					it.CoverURL = localCoverURL(*existing.CoverURL)
				}
				inCatalog = true
				if userLibMap[existing.ID] {
					inLibrary = true
				}
			}
		}

		annotated = append(annotated, ImportAnnotatedItem{
			CoverState:       it.CoverState,
			Title:            it.Title,
			Slug:             slug,
			URLTemplate:      it.URLTemplate,
			CoverURL:         it.CoverURL,
			LastChapter:      it.LastChapter,
			PrimaryDomain:    it.PrimaryDomain,
			AlreadyInCatalog: inCatalog,
			AlreadyInLibrary: inLibrary,
		})
	}

	return &ImportBrowseResult{
		Syncing:    syncing,
		Warning:    scrapeRes.Warning,
		Domain:     domainName,
		Page:       scrapeRes.Page,
		TotalPages: scrapeRes.TotalPages,
		Items:      annotated,
	}, nil
}

func (s *ImportService) ImportWorks(ctx context.Context, userID string, items []ImportPayloadItem) (*ImportResult, error) {
	if len(items) == 0 {
		return &ImportResult{Status: "ok", ImportedCount: 0, AddedToLibraryCount: 0}, nil
	}

	canonicalItems := make([]*domain.CanonicalManhua, 0, len(items))
	for _, it := range items {
		slug := it.Slug
		if slug == "" {
			slug = strings.ToLower(strings.ReplaceAll(it.Title, " ", "-"))
		}
		if s.canonicalRepo != nil && (it.PrimaryDomain == "manhuaus.com" || it.PrimaryDomain == "ravenscans.org") {
			if existing, err := s.canonicalRepo.GetBySlug(ctx, slug); err == nil && existing != nil && (existing.PrimaryDomain != it.PrimaryDomain || existing.URLTemplate != it.URLTemplate) {
				return nil, fmt.Errorf("source does not match catalog entry: %s", slug)
			}
		}

		syn := it.Synopsis
		cov := it.CoverURL
		if it.PrimaryDomain == "manhuaus.com" || it.PrimaryDomain == "ravenscans.org" {
			cov = localCoverURL(cov)
		}

		canonicalItems = append(canonicalItems, &domain.CanonicalManhua{
			Slug:          slug,
			Title:         it.Title,
			Synopsis:      &syn,
			CoverURL:      &cov,
			PrimaryDomain: it.PrimaryDomain,
			URLTemplate:   it.URLTemplate,
			LastChapter:   it.LastChapter,
		})
	}

	if s.canonicalRepo != nil {
		if err := s.canonicalRepo.UpsertBatch(ctx, canonicalItems); err != nil {
			return nil, fmt.Errorf("failed to upsert canonical batch: %w", err)
		}
	}

	addedToLibrary := 0
	if userID != "" && s.librarySvc != nil && s.canonicalRepo != nil {
		for _, it := range canonicalItems {
			fetched, err := s.canonicalRepo.GetBySlug(ctx, it.Slug)
			if err != nil || fetched == nil {
				continue
			}

			dto := domain.AddToLibraryDTO{
				CanonicalManhuaID: fetched.ID,
				CurrentChapter:    1,
				IsFavorite:        false,
			}
			_, err = s.librarySvc.AddToLibrary(ctx, userID, dto)
			if err == nil {
				addedToLibrary++
			}
		}
	}

	return &ImportResult{
		Status:              "ok",
		ImportedCount:       len(canonicalItems),
		AddedToLibraryCount: addedToLibrary,
	}, nil
}
