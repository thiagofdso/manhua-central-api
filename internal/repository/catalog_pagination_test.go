package repository_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func TestCollectionPreservesSourcePaginationForCompletedPages(t *testing.T) {
	connection, cleanup := setupTestPostgres(t)
	defer cleanup()
	db, err := repository.NewPostgresDB(connection)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.RunMigrationsFS(migrations.FS, "."))
	ctx := context.Background()
	_, err = db.DB.ExecContext(ctx, `INSERT INTO catalog_discovery_queries(domain,query,page,state,payload,refreshed_at) VALUES
	('ravenscans.org','',9,'ready','{"page":9,"totalPages":137,"items":[]}',NOW()),
	('ravenscans.org','',10,'ready','{"page":10,"totalPages":10,"items":[]}',NOW()),
	('ravenscans.org','nano machine',1,'ready','{"page":1,"totalPages":1,"items":[]}',NOW()),
	('manhuaus.com','',1,'ready','{"page":1,"totalPages":112,"items":[]}',NOW())`)
	require.NoError(t, err)
	collection := service.NewCatalogCollection(db.DB, providers.NewDefaultProviderRegistry(""), repository.NewPostgresCanonicalRepository(db.DB), nil)
	for _, state := range []string{"ready", "pending", "processing", "failed"} {
		t.Run(state, func(t *testing.T) {
			_, err := db.DB.ExecContext(ctx, `UPDATE catalog_discovery_queries SET state=$1 WHERE domain='ravenscans.org' AND query='' AND page=10`, state)
			require.NoError(t, err)
			page, _, err := collection.Browse(ctx, "ravenscans.org", "", 10)
			require.NoError(t, err)
			require.Equal(t, 10, page.Page)
			require.Equal(t, 137, page.TotalPages, "page 10 must not disable navigation to page 11")
		})
	}
	search, _, err := collection.Browse(ctx, "ravenscans.org", "nano machine", 1)
	require.NoError(t, err)
	require.Equal(t, 1, search.TotalPages, "totals must not leak across queries")
	other, _, err := collection.Browse(ctx, "manhuaus.com", "", 1)
	require.NoError(t, err)
	require.Equal(t, 112, other.TotalPages, "totals must not leak across domains")

	t.Run("requested page precedes older queued pages", func(t *testing.T) {
		_, err := db.DB.ExecContext(ctx, `DELETE FROM catalog_discovery_queries;
		INSERT INTO catalog_discovery_queries(domain,query,page,state,payload) VALUES
		('ravenscans.org','',39,'pending','{"page":39,"totalPages":137,"items":[]}'),
		('ravenscans.org','',114,'pending','{"page":114,"totalPages":137,"items":[]}')`)
		require.NoError(t, err)
		calls := make(chan int, 2)
		registry := providers.NewProviderRegistry()
		registry.Register(paginationQueueProvider{calls: calls})
		worker := service.NewCatalogCollection(db.DB, registry, repository.NewPostgresCanonicalRepository(db.DB), nil)
		_, _, err = worker.Browse(ctx, "ravenscans.org", "", 114)
		require.NoError(t, err)
		workerCtx, cancel := context.WithCancel(ctx)
		worker.Start(workerCtx)
		defer func() { cancel(); worker.Wait() }()
		select {
		case page := <-calls:
			require.Equal(t, 114, page)
		case <-time.After(5 * time.Second):
			t.Fatal("requested page was not processed")
		}
	})

	t.Run("cover queue recovers expired jobs and alternates supported domains", func(t *testing.T) {
		_, err := db.DB.ExecContext(ctx, `DELETE FROM catalog_discovery_queries;
		INSERT INTO canonical_manhuas(id,slug,title,primary_domain,url_template) VALUES
		(90001,'recover','Recover','ravenscans.org','https://ravenscans.org/manga/recover/'),
		(90002,'priority','Priority','ravenscans.org','https://ravenscans.org/manga/priority/'),
		(90003,'fair','Fair','manhuaus.com','https://manhuaus.com/manga/fair/'),
		(90004,'outside','Outside','other.example','https://other.example/manga/outside/');
		INSERT INTO catalog_sources(canonical_id,domain,source_url,original_cover_url,state,next_attempt_at,last_requested_at) VALUES
		(90001,'ravenscans.org','https://ravenscans.org/manga/recover/','https://ravenscans.org/recover.png','processing',NOW()-INTERVAL '1 hour','-infinity'),
		(90002,'ravenscans.org','https://ravenscans.org/manga/priority/','https://ravenscans.org/priority.png','pending',NOW(),NOW()),
		(90003,'manhuaus.com','https://manhuaus.com/manga/fair/','https://manhuaus.com/fair.png','pending',NOW(),'-infinity'),
		(90004,'other.example','https://other.example/manga/outside/','https://other.example/outside.png','pending',NOW(),NOW())`)
		require.NoError(t, err)
		image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jE9sAAAAASUVORK5CYII=")
		require.NoError(t, err)
		calls := make(chan string, 4)
		store, err := service.NewCoverStore(t.TempDir(), 1024, paginationCoverStreamer{calls, image})
		require.NoError(t, err)
		worker := service.NewCatalogCollection(db.DB, providers.NewProviderRegistry(), repository.NewPostgresCanonicalRepository(db.DB), store)
		workerCtx, cancel := context.WithCancel(ctx)
		worker.Start(workerCtx)
		defer func() { cancel(); worker.Wait() }()
		for _, expected := range []string{"https://ravenscans.org/recover.png", "https://manhuaus.com/fair.png", "https://ravenscans.org/priority.png"} {
			select {
			case source := <-calls:
				require.Equal(t, expected, source)
			case <-time.After(5 * time.Second):
				t.Fatal("cover queue stopped progressing")
			}
		}
		require.Eventually(t, func() bool {
			var count int
			return db.DB.QueryRowContext(ctx, `SELECT count(*) FROM catalog_sources WHERE state='ready'`).Scan(&count) == nil && count == 3
		}, 5*time.Second, 10*time.Millisecond)
		var outside string
		require.NoError(t, db.DB.QueryRowContext(ctx, `SELECT state FROM catalog_sources WHERE canonical_id=90004`).Scan(&outside))
		require.Equal(t, "pending", outside, "other providers must not be downloaded")
	})
}

type paginationCoverStreamer struct {
	calls chan string
	image []byte
}

func (streamer paginationCoverStreamer) ProxyImage(ctx context.Context, w http.ResponseWriter, source string) error {
	streamer.calls <- source
	_, err := w.Write(streamer.image)
	return err
}

type paginationQueueProvider struct{ calls chan int }

func (paginationQueueProvider) Domain() string { return "ravenscans.org" }
func (paginationQueueProvider) Name() string   { return "Test source" }
func (provider paginationQueueProvider) SearchOrBrowse(ctx context.Context, query string, page int) (*domain.ScrapeResult, error) {
	provider.calls <- page
	return &domain.ScrapeResult{Page: page, TotalPages: 137, Items: []domain.ScrapedItem{}}, nil
}
