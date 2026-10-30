package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type CatalogCollection struct {
	db              *sql.DB
	registry        *providers.ProviderRegistry
	repo            repository.CanonicalRepository
	store           *CoverStore
	workers         sync.WaitGroup
	lastCoverDomain string
}

func NewCatalogCollection(db *sql.DB, registry *providers.ProviderRegistry, repo repository.CanonicalRepository, store *CoverStore) *CatalogCollection {
	return &CatalogCollection{db: db, registry: registry, repo: repo, store: store}
}

func (collection *CatalogCollection) Browse(ctx context.Context, source, query string, page int) (*domain.ScrapeResult, bool, error) {
	if source != "manhuaus.com" && source != "ravenscans.org" {
		return nil, false, fmt.Errorf("collection is disabled for domain: %s", source)
	}
	if _, ok := collection.registry.Get(source); !ok {
		return nil, false, fmt.Errorf("unsupported domain: %s", source)
	}
	_, err := collection.db.ExecContext(ctx, `INSERT INTO catalog_discovery_queries(domain, query, page) VALUES($1,$2,$3)
	ON CONFLICT(domain, query, page) DO UPDATE SET state='pending', next_attempt_at=NOW()
	WHERE catalog_discovery_queries.state='ready' AND catalog_discovery_queries.refreshed_at < NOW()-INTERVAL '1 day'`, source, query, page)
	if err != nil {
		return nil, false, err
	}
	_, err = collection.db.ExecContext(ctx, `UPDATE catalog_discovery_queries SET last_requested_at=NOW()
	WHERE domain=$1 AND query=$2 AND page=$3 AND state IN ('pending','failed') AND last_requested_at<NOW()-INTERVAL '10 seconds'`, source, query, page)
	if err != nil {
		return nil, false, err
	}
	var payload []byte
	var state string
	var lastError sql.NullString
	if err := collection.db.QueryRowContext(ctx, `SELECT payload,state,last_error FROM catalog_discovery_queries WHERE domain=$1 AND query=$2 AND page=$3`, source, query, page).Scan(&payload, &state, &lastError); err != nil {
		return nil, false, err
	}
	result := &domain.ScrapeResult{Items: []domain.ScrapedItem{}, Page: page, TotalPages: page}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, result); err != nil {
			return nil, false, err
		}
	} else {
		filterDomain := source
		if source == "all" {
			filterDomain = ""
		}
		local, err := collection.repo.List(ctx, domain.CatalogFilter{Query: query, Domain: filterDomain, Page: page, PageSize: 40})
		if err != nil {
			return nil, false, err
		}
		result.TotalPages = local.TotalPages
		for _, work := range local.Items {
			cover := ""
			if work.CoverURL != nil {
				cover = *work.CoverURL
			}
			result.Items = append(result.Items, domain.ScrapedItem{Title: work.Title, Slug: work.Slug, PrimaryDomain: work.PrimaryDomain, URLTemplate: work.URLTemplate, CoverURL: cover, LastChapter: work.LastChapter})
		}
	}
	// A completed snapshot can still have incomplete pagination metadata.
	// Preserve the known source total for the same query in every state.
	var knownPages int
	if err := collection.db.QueryRowContext(ctx, `SELECT COALESCE(MAX((payload->>'totalPages')::INTEGER),0) FROM catalog_discovery_queries WHERE domain=$1 AND query=$2`, source, query).Scan(&knownPages); err != nil {
		return nil, false, err
	}
	result.TotalPages = max(page, result.TotalPages, knownPages)
	positions := make(map[string]int)
	slugs := make([]string, 0, len(result.Items))
	for index, item := range result.Items {
		positions[item.Slug] = index
		slugs = append(slugs, item.Slug)
		result.Items[index].CoverURL = localCoverURL(item.CoverURL)
	}
	if _, err := collection.db.ExecContext(ctx, `UPDATE catalog_sources SET next_attempt_at=CASE WHEN next_attempt_at='infinity'::TIMESTAMPTZ THEN NOW() ELSE next_attempt_at END,last_requested_at=NOW() WHERE domain=$1 AND state IN ('pending','failed') AND last_requested_at < NOW()-INTERVAL '10 seconds' AND canonical_id IN (SELECT id FROM canonical_manhuas WHERE slug=ANY($2))`, source, pq.Array(slugs)); err != nil {
		return nil, false, err
	}
	rows, err := collection.db.QueryContext(ctx, `SELECT m.slug,m.cover_url,COALESCE(s.state,'') FROM canonical_manhuas m LEFT JOIN catalog_sources s ON s.canonical_id=m.id AND s.domain=$2 WHERE m.slug=ANY($1)`, pq.Array(slugs), source)
	if err != nil {
		return nil, false, err
	}
	collecting := false
	for rows.Next() {
		var slug, coverState string
		var cover sql.NullString
		if err := rows.Scan(&slug, &cover, &coverState); err != nil {
			rows.Close()
			return nil, false, err
		}
		result.Items[positions[slug]].CoverURL = localCoverURL(cover.String)
		result.Items[positions[slug]].CoverState = coverState
		collecting = collecting || coverState == "pending" || coverState == "processing"
		if coverState == "failed" && (!cover.Valid || !strings.HasPrefix(cover.String, "/api/media/covers/")) {
			result.Warning = "Algumas capas aguardam uma nova tentativa de acesso à plataforma."
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	if state == "failed" {
		result.Warning = "Não foi possível atualizar a fonte. Os dados disponíveis foram preservados; uma nova tentativa será feita automaticamente."
		if strings.Contains(lastError.String, "Source requires manual verification") {
			result.Warning = "A fonte exige validação no navegador do coletor local. Conclua a verificação para retomar a coleta; uma nova tentativa será feita automaticamente."
		}
	}
	return result, collecting || state == "pending" || state == "processing", nil
}

func localCoverURL(candidate string) string {
	if strings.HasPrefix(candidate, "/api/media/covers/") && coverKeyPattern.MatchString(strings.TrimPrefix(candidate, "/api/media/covers/")) {
		return candidate
	}
	return ""
}

func (collection *CatalogCollection) Start(ctx context.Context) {
	collection.reconcile(ctx)
	for _, task := range []func(context.Context) bool{collection.discover, collection.download} {
		collection.workers.Add(1)
		go func(task func(context.Context) bool) {
			defer collection.workers.Done()
			for ctx.Err() == nil {
				if task(ctx) {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}(task)
	}
}

func (collection *CatalogCollection) reconcile(ctx context.Context) {
	rows, err := collection.db.QueryContext(ctx, `SELECT id,file_key FROM catalog_sources WHERE state='ready' AND domain IN ('manhuaus.com','ravenscans.org')`)
	if err != nil {
		log.Printf("cover reconciliation: %v", err)
		return
	}
	var missing []int64
	for rows.Next() {
		var id int64
		var key string
		if rows.Scan(&id, &key) == nil {
			if _, err := os.Stat(filepath.Join(collection.store.root, filepath.FromSlash(key))); os.IsNotExist(err) {
				missing = append(missing, id)
			}
		}
	}
	rows.Close()
	if len(missing) > 0 {
		if _, err := collection.db.ExecContext(ctx, `UPDATE catalog_sources SET state='pending',next_attempt_at=NOW() WHERE id=ANY($1)`, pq.Array(missing)); err != nil {
			log.Printf("cover reconciliation update: %v", err)
		}
	}
}

func (collection *CatalogCollection) Wait() { collection.workers.Wait() }

func (collection *CatalogCollection) PrioritizeCovers(ctx context.Context, works []*domain.CanonicalManhua) {
	ids := make([]int64, 0, len(works))
	for _, work := range works {
		if work.PrimaryDomain == "manhuaus.com" || work.PrimaryDomain == "ravenscans.org" {
			ids = append(ids, work.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if _, err := collection.db.ExecContext(ctx, `UPDATE catalog_sources SET next_attempt_at=CASE WHEN next_attempt_at='infinity'::TIMESTAMPTZ THEN NOW() ELSE next_attempt_at END,last_requested_at=NOW() WHERE canonical_id=ANY($1) AND state IN ('pending','failed') AND last_requested_at < NOW()-INTERVAL '10 seconds'`, pq.Array(ids)); err != nil {
		log.Printf("cover page priority: %v", err)
	}
}

func retryDelay(attempts int) time.Duration {
	if attempts > 10 {
		attempts = 10
	}
	return time.Minute * time.Duration(1<<attempts)
}

func (collection *CatalogCollection) discover(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	var source, query string
	var page, attempts int
	err := collection.db.QueryRowContext(ctx, `UPDATE catalog_discovery_queries SET state='processing',next_attempt_at=NOW()+INTERVAL '3 minutes',attempts=attempts+1
	WHERE (domain,query,page)=(SELECT domain,query,page FROM catalog_discovery_queries WHERE state<>'ready' AND next_attempt_at<=NOW() ORDER BY last_requested_at DESC,next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1)
	RETURNING domain,query,page,attempts`).Scan(&source, &query, &page, &attempts)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		log.Printf("catalog discovery claim: %v", err)
		return false
	}
	scraper, ok := collection.registry.Get(source)
	var result *domain.ScrapeResult
	if !ok {
		err = fmt.Errorf("unknown domain %s", source)
	} else {
		result, err = scraper.SearchOrBrowse(ctx, query, page)
	}
	if errors.Is(err, providers.ErrCachedPageMissing) {
		result, err = collection.collectSourcePage(ctx, source, query, page)
	}
	if err == nil && result != nil {
		for index := range result.Items {
			item := &result.Items[index]
			if err = collection.persist(ctx, item); err != nil {
				break
			}
		}
	} else if err == nil {
		err = fmt.Errorf("empty provider response")
	}
	if err != nil {
		_, saveErr := collection.db.ExecContext(parent, `UPDATE catalog_discovery_queries SET state='failed',last_error=$4,next_attempt_at=$5 WHERE domain=$1 AND query=$2 AND page=$3`, source, query, page, err.Error(), time.Now().Add(retryDelay(attempts)))
		if saveErr != nil {
			log.Printf("catalog discovery retry: %v", saveErr)
		}
		return true
	}
	payload, err := json.Marshal(result)
	if err == nil {
		_, err = collection.db.ExecContext(parent, `UPDATE catalog_discovery_queries SET state='ready',payload=$4,refreshed_at=NOW(),attempts=0,last_error=NULL WHERE domain=$1 AND query=$2 AND page=$3`, source, query, page, string(payload))
	}
	if err != nil {
		log.Printf("catalog discovery completion: %v", err)
	}
	return true
}

func (collection *CatalogCollection) collectSourcePage(ctx context.Context, source, query string, page int) (*domain.ScrapeResult, error) {
	payload, err := json.Marshal(map[string]interface{}{"domain": source, "query": query, "page": page})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, collection.store.workerURL+"/collect-catalog", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if collection.store.workerToken != "" {
		req.Header.Set("Authorization", "Bearer "+collection.store.workerToken)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
		return nil, fmt.Errorf("catalog browser returned %d: %s", response.StatusCode, failure.Error)
	}
	var result domain.ScrapeResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Page != page || result.TotalPages < page {
		return nil, fmt.Errorf("invalid source pagination")
	}
	return &result, nil
}

func (collection *CatalogCollection) persist(ctx context.Context, item *domain.ScrapedItem) error {
	if item.Title == "" || item.PrimaryDomain == "" || item.URLTemplate == "" {
		return fmt.Errorf("incomplete source metadata")
	}
	var slug string
	err := collection.db.QueryRowContext(ctx, `SELECT slug FROM canonical_manhuas WHERE primary_domain=$1 AND url_template=$2 ORDER BY id LIMIT 1`, item.PrimaryDomain, item.URLTemplate).Scan(&slug)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if slug == "" {
		base := item.Slug
		if base == "" {
			base = providers.GenerateSlug(item.Title)
		}
		if len(base) > 220 {
			base = base[:220]
		}
		identity := sha256.Sum256([]byte(item.PrimaryDomain + "\n" + item.URLTemplate))
		slug = fmt.Sprintf("%s-%x", base, identity[:8])
	}
	item.Slug = slug
	work := &domain.CanonicalManhua{Slug: slug, Title: item.Title, PrimaryDomain: item.PrimaryDomain, URLTemplate: item.URLTemplate, LastChapter: item.LastChapter}
	if err := collection.db.QueryRowContext(ctx, `SELECT nextval('catalog_collection_ids')`).Scan(&work.ID); err != nil {
		return err
	}
	if err := collection.repo.UpsertBatch(ctx, []*domain.CanonicalManhua{work}); err != nil {
		return err
	}
	stored, err := collection.repo.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}
	original := item.CoverURL
	if strings.HasPrefix(original, "/api/media/covers/") {
		original = ""
	}
	_, err = collection.db.ExecContext(ctx, `INSERT INTO catalog_sources(canonical_id,domain,source_url,original_cover_url) VALUES($1,$2,$3,$4)
	ON CONFLICT(domain,source_url) DO UPDATE SET original_cover_url=CASE WHEN EXCLUDED.original_cover_url<>'' THEN EXCLUDED.original_cover_url ELSE catalog_sources.original_cover_url END,
	state=CASE WHEN EXCLUDED.original_cover_url<>'' AND EXCLUDED.original_cover_url<>catalog_sources.original_cover_url THEN 'pending' ELSE catalog_sources.state END,
	next_attempt_at=CASE WHEN EXCLUDED.original_cover_url<>'' AND EXCLUDED.original_cover_url<>catalog_sources.original_cover_url THEN NOW() ELSE catalog_sources.next_attempt_at END, updated_at=NOW()`, stored.ID, item.PrimaryDomain, item.URLTemplate, original)
	return err
}

func (collection *CatalogCollection) download(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	var id, canonicalID int64
	var original, source, sourceURL string
	var attempts int
	err := collection.db.QueryRowContext(ctx, `UPDATE catalog_sources SET state='processing',attempts=attempts+1,next_attempt_at=NOW()+INTERVAL '2 minutes'
	WHERE id=(SELECT id FROM catalog_sources WHERE domain IN ('manhuaus.com','ravenscans.org') AND state<>'ready' AND next_attempt_at<=NOW() ORDER BY (state='processing') DESC,(domain<>$1) DESC,last_requested_at DESC,next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1)
	RETURNING id,canonical_id,original_cover_url,domain,source_url,attempts`, collection.lastCoverDomain).Scan(&id, &canonicalID, &original, &source, &sourceURL, &attempts)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		log.Printf("cover claim: %v", err)
		return false
	}
	if collection.store == nil {
		return false
	}
	collection.lastCoverDomain = source
	if original == "" {
		item := domain.ScrapedItem{PrimaryDomain: source, URLTemplate: sourceURL}
		original = providers.ResolveCover(ctx, item)
	}
	var stored *StoredCover
	stored, err = collection.store.Download(ctx, original, providers.CoverPage(domain.ScrapedItem{PrimaryDomain: source, URLTemplate: sourceURL}))
	if err != nil {
		_, saveErr := collection.db.ExecContext(parent, `UPDATE catalog_sources SET state='failed',last_error=$2,next_attempt_at=$3 WHERE id=$1`, id, err.Error(), time.Now().Add(retryDelay(attempts)))
		if saveErr != nil {
			log.Printf("cover retry: %v", saveErr)
		}
		return true
	}
	tx, err := collection.db.BeginTx(ctx, nil)
	if err != nil {
		return true
	}
	defer tx.Rollback()
	original = stored.SourceURL
	_, err = tx.ExecContext(ctx, `UPDATE catalog_sources SET original_cover_url=$2,file_key=$3,content_hash=$4,content_type=$5,byte_size=$6,downloaded_at=NOW(),state='ready',attempts=0,last_error=NULL WHERE id=$1`, id, original, stored.Key, stored.Hash, stored.ContentType, stored.Size)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE canonical_manhuas SET cover_url=$2,updated_at=NOW() WHERE id=$1`, canonicalID, "/api/media/covers/"+stored.Key)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		log.Printf("cover completion: %v", err)
	}
	return true
}
