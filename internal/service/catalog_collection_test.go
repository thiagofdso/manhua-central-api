package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostCatalogCollectorAuthenticationAndErrors(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collect-catalog" || r.Header.Get("Authorization") != "Bearer host-token" {
			t.Error("catalog request did not use authenticated collector")
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"Source did not release content: HTTP 403"}`))
	}))
	defer worker.Close()
	collection := &CatalogCollection{store: &CoverStore{workerURL: worker.URL, workerToken: "host-token"}}
	_, err := collection.collectSourcePage(context.Background(), "manhuaus.com", "", 31)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("collector error details lost: %v", err)
	}
}

func TestCollectionPublishesOnlyStoredCovers(t *testing.T) {
	valid := "/api/media/covers/aa/" + strings.Repeat("a", 64) + ".webp"
	if got := localCoverURL(valid); got != valid {
		t.Fatalf("stored cover rejected: %q", got)
	}
	for _, external := range []string{"https://manhuaus.com/wp-content/uploads/cover.webp", "https://ravenscans.org/cover.jpg", "/api/reader/proxy-image?url=https://manhuaus.com/cover.webp", "/api/media/covers/../../private", ""} {
		if got := localCoverURL(external); got != "" {
			t.Fatalf("unpublished cover leaked to client: %q", got)
		}
	}
}
