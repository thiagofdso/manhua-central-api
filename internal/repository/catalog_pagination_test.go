package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
}
