package providers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPartialSourceCacheDoesNotInventPages(t *testing.T) {
	for _, source := range []struct {
		domain, file string
		total        int
	}{
		{"manhuaus.com", "import_cache_manhuaus_com.json", 112},
		{"ravenscans.org", "import_cache_ravenscans_org.json", 137},
	} {
		t.Run(source.domain, func(t *testing.T) {
			root := t.TempDir()
			payload := `{"catalog":{"":{"1":[{"title":"First work","urlTemplate":"https://example.com/first/{chapter}"}]}},"totalPages":{"":{"1":` + strconv.Itoa(source.total) + `}}}`
			if err := os.WriteFile(filepath.Join(root, source.file), []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			first, err := LoadCachedDomainItems(root, source.domain, "", 1)
			if err != nil || first.TotalPages != source.total {
				t.Fatalf("lost source page count: %+v %v", first, err)
			}
			scraper := NewCachedDomainScraper(source.domain, source.domain, root, nil)
			for _, page := range []int{2, 5, 6} {
				result, err := scraper.SearchOrBrowse(context.Background(), "", page)
				if result != nil || !errors.Is(err, ErrCachedPageMissing) {
					t.Fatalf("page %d was invented from partial cache: %+v %v", page, result, err)
				}
			}
			if _, err := LoadCachedDomainItems(root, source.domain, "unknown query", 1); !errors.Is(err, ErrCachedPageMissing) {
				t.Fatalf("uncached search treated as authoritative: %v", err)
			}
		})
	}
}
