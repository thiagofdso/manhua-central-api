package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

func TestExtractorService_AsuraScans(t *testing.T) {
	mockAsuraHTML := `
<!DOCTYPE html>
<html>
<body>
  <script type="application/ld+json">{"name":"Overgeared"}</script>
  <div>
    "pages":[[0,{"url":"https://cdn.asurascans.com/asura-images/chapters/overgeared/344/page1.webp?v=1"}],[0,{"url":"https://cdn.asurascans.com/asura-images/chapters/overgeared/344/page2.webp?v=1"}]]
  </div>
</body>
</html>`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(mockAsuraHTML))
	}))
	defer mockServer.Close()

	extractor := NewExtractorService("", mockServer.Client())

	manhua := &domain.CanonicalManhua{
		Slug:          "overgeared",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   mockServer.URL + "/comics/overgeared/chapter/{chapter}",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 344)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	expected1 := "https://cdn.asurascans.com/asura-images/chapters/overgeared/344/page1.webp?v=1"
	if images[0] != expected1 {
		t.Errorf("expected image 0 to be %q, got %q", expected1, images[0])
	}
}

func TestExtractorService_RavenScans(t *testing.T) {
	mockRavenHTML := `
<!DOCTYPE html>
<html>
<body>
  <script>
    ts_reader.run({
      "sources": [
        {
          "images": [
            "https://ravenscans.org/wp-content/uploads/hua/ch1/01.webp",
            "https://ravenscans.org/wp-content/uploads/hua/ch1/02.webp"
          ]
        }
      ]
    });
  </script>
</body>
</html>`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(mockRavenHTML))
	}))
	defer mockServer.Close()

	extractor := NewExtractorService("", mockServer.Client())

	manhua := &domain.CanonicalManhua{
		Slug:          "mount-hua",
		PrimaryDomain: "ravenscans.org",
		URLTemplate:   mockServer.URL + "/mount-hua-chapter-{chapter}/",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
}

func TestExtractorService_ScraperWorkerDelegation(t *testing.T) {
	// Mock scraping worker daemon
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/extract-chapter" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"success": true,
			"images": []map[string]interface{}{
				{"index": 1, "url": "https://mfcdn.net/page1.webp"},
				{"index": 2, "url": "https://mfcdn.net/page2.webp"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer workerServer.Close()

	extractor := NewExtractorService(workerServer.URL, workerServer.Client())

	manhua := &domain.CanonicalManhua{
		Slug:          "solo-fire",
		PrimaryDomain: "mangafire.to",
		URLTemplate:   "https://mangafire.to/manga/solo-fire/chapter/{chapter}",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	if images[0] != "https://mfcdn.net/page1.webp" {
		t.Errorf("expected https://mfcdn.net/page1.webp, got %q", images[0])
	}
}

func TestExtractorService_PickMeUpGacha(t *testing.T) {
	mockHTML := `<html><body>
		<img src="https://w19.pickmeupgacha.com/wp-content/plugins/a3-lazy-load/assets/images/lazy_placeholder.gif" data-src="https://abyssrift.com/Gacha/219/01.webp">
		<img src="https://w19.pickmeupgacha.com/wp-content/plugins/a3-lazy-load/assets/images/lazy_placeholder.gif" data-src="https://abyssrift.com/Gacha/219/02.webp">
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	extractor := NewExtractorService("", server.Client())
	manhua := &domain.CanonicalManhua{
		Slug:          "pick-me-up-infinite-gacha",
		PrimaryDomain: "pickmeupgacha.com",
		URLTemplate:   server.URL + "/chapter-{chapter}",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 219)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0] != "https://abyssrift.com/Gacha/219/01.webp" {
		t.Errorf("expected abyssrift image, got %s", images[0])
	}
}

func TestExtractorService_MagicEmperor(t *testing.T) {
	mockHTML := `<html><body>
		<div class="entry-content">
			<img src="https://cdn.black-clover.org/file/flanbox/magic-emperor/chapter-900/1.webp">
			<img src="https://cdn.black-clover.org/file/flanbox/magic-emperor/chapter-900/2.webp">
		</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	extractor := NewExtractorService("", server.Client())
	manhua := &domain.CanonicalManhua{
		Slug:          "magic-emperor-online",
		PrimaryDomain: "magicemperor.online",
		URLTemplate:   server.URL + "/chapter-{chapter}",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 900)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0] != "https://cdn.black-clover.org/file/flanbox/magic-emperor/chapter-900/1.webp" {
		t.Errorf("expected black-clover image, got %s", images[0])
	}
}

func TestExtractorService_LevelingWithGods(t *testing.T) {
	mockHTML := `<html><body>
		<div class="entry-content">
			<img src="lazy_placeholder.gif" data-src="https://blogger.googleusercontent.com/img/b/001.png">
			<img src="lazy_placeholder.gif" data-src="https://blogger.googleusercontent.com/img/b/002.png">
		</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	extractor := NewExtractorService("", server.Client())
	manhua := &domain.CanonicalManhua{
		Slug:          "leveling-with-the-gods-online",
		PrimaryDomain: "levelingwithgods.com",
		URLTemplate:   server.URL + "/chapter-{chapter}",
	}

	images, err := extractor.ExtractChapterImages(context.Background(), manhua, 169)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0] != "https://blogger.googleusercontent.com/img/b/001.png" {
		t.Errorf("expected blogger image, got %s", images[0])
	}
}
