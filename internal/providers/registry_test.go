package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
)

func TestProviderRegistry_RegistrationAndLookup(t *testing.T) {
	reg := providers.NewProviderRegistry()

	asura := providers.NewAsuraScansScraper()
	raven := providers.NewRavenScansScraper()
	mangak := providers.NewMangaKScraper()
	top := providers.NewManhuaTopScraper()

	reg.Register(asura)
	reg.Register(raven)
	reg.Register(mangak)
	reg.Register(top)

	domains := reg.ListDomains()
	if len(domains) < 4 {
		t.Fatalf("expected at least 4 domains registered, got %d", len(domains))
	}

	sc, exists := reg.Get("asurascans.com")
	if !exists || sc == nil {
		t.Fatalf("expected asurascans.com to be registered")
	}

	if sc.Name() != "Asura Scans" {
		t.Errorf("expected name 'Asura Scans', got '%s'", sc.Name())
	}
}

func TestMangaKScraper_SearchOrBrowse(t *testing.T) {
	mockHTML := `
	<div class="story-item">
		<a href="https://mangak.io/manga/solo-leveling" title="Solo Leveling">
			<img src="https://mangak.io/cover.jpg" alt="Solo Leveling" />
		</a>
		<h3 class="story-title"><a href="https://mangak.io/manga/solo-leveling">Solo Leveling</a></h3>
		<span class="chapter-item">Chapter 200</span>
	</div>
	`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	scraper := providers.NewMangaKScraperWithURL(srv.URL)
	res, err := scraper.SearchOrBrowse(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	if res.Items[0].Title != "Solo Leveling" {
		t.Errorf("expected title 'Solo Leveling', got '%s'", res.Items[0].Title)
	}
	if res.Items[0].LastChapter == nil || *res.Items[0].LastChapter != 200 {
		t.Errorf("expected lastChapter 200, got %v", res.Items[0].LastChapter)
	}
}
