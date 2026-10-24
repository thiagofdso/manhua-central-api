package providers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
)

const sampleAsuraHTML = `
<!DOCTYPE html>
<html>
<body>
  <div class="grid">
    <div class="series-card bg-surface rounded-xl">
      <a href="/comics/solo-leveling-ragnarok-11fe7da3">
        <img src="https://cdn.asurascans.com/covers/sl-rag.webp" alt="Solo Leveling: Ragnarok" />
        <h3 class="text-sm font-bold">Solo Leveling: Ragnarok</h3>
      </a>
      <div class="flex items-center">
        48 <!-- --> <span class="text-xs">Chapters</span>
      </div>
    </div>
    <div class="series-card bg-surface rounded-xl">
      <a href="/comics/the-extras-academy-survival-guide-11fe7da3">
        <img src="https://cdn.asurascans.com/covers/extras.webp" alt="The Extra&#39;s Academy Survival Guide" />
        <h3 class="text-sm font-bold">The Extra&#39;s Academy Survival Guide</h3>
      </a>
      <div class="flex items-center">
        74 <span>Chs.</span>
      </div>
    </div>
  </div>
  <nav id="pagination-nav">
    <a href="?page=1">1</a>
    <a href="?page=2">2</a>
    <a href="?page=7">7</a>
  </nav>
</body>
</html>
`

const sampleRavenHTML = `
<!DOCTYPE html>
<html>
<body>
  <div class="listupd">
    <div class="bs">
      <div class="bsx">
        <a href="https://ravenscans.org/manga/almighty-pig-slaughtering-knife/" title="Almighty Pig Slaughtering Knife">
          <div class="limit">
            <img src="https://ravenscans.org/wp-content/uploads/knife.jpg" class="ts-post-image" />
          </div>
          <div class="bigor">
            <div class="tt">Almighty Pig Slaughtering Knife</div>
            <div class="epxs">Chapter 66</div>
          </div>
        </a>
      </div>
    </div>
    <div class="bs">
      <div class="bsx">
        <a href="/manga/return-of-the-mount-hua-sect/" title="Return of the Mount Hua Sect">
          <div class="limit">
            <img src="https://ravenscans.org/wp-content/uploads/hua.jpg" class="ts-post-image" />
          </div>
          <div class="bigor">
            <div class="tt">Return of the Mount Hua Sect</div>
            <div class="epxs">Ch. 142</div>
          </div>
        </a>
      </div>
    </div>
  </div>
  <div class="pagination">
    <span class="page-numbers current">1</span>
    <a class="page-numbers" href="https://ravenscans.org/manga/?page=2">2</a>
    <a class="page-numbers" href="https://ravenscans.org/manga/?page=4">4</a>
  </div>
</body>
</html>
`

func TestAsuraScansScraper_ParseHTML(t *testing.T) {
	var capturedReferer string
	var capturedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReferer = r.Header.Get("Referer")
		capturedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleAsuraHTML))
	}))
	defer server.Close()

	scraper := providers.NewAsuraScansScraperWithURL(server.URL)
	assert.Equal(t, "asurascans.com", scraper.Domain())
	assert.Equal(t, "Asura Scans", scraper.Name())

	res, err := scraper.SearchOrBrowse(context.Background(), "solo", 1)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "https://asurascans.com/", capturedReferer)
	assert.NotEmpty(t, capturedUserAgent)

	require.Len(t, res.Items, 2)
	assert.Equal(t, 7, res.TotalPages)
	assert.Equal(t, 1, res.Page)

	item1 := res.Items[0]
	assert.Equal(t, "Solo Leveling: Ragnarok", item1.Title)
	assert.Equal(t, "solo-leveling-ragnarok-11fe7da3", item1.Slug)
	assert.Equal(t, "https://asurascans.com/comics/solo-leveling-ragnarok-11fe7da3/chapter/{chapter}", item1.URLTemplate)
	assert.Equal(t, "https://cdn.asurascans.com/covers/sl-rag.webp", item1.CoverURL)
	require.NotNil(t, item1.LastChapter)
	assert.Equal(t, 48, *item1.LastChapter)
	assert.Equal(t, "asurascans.com", item1.PrimaryDomain)

	item2 := res.Items[1]
	assert.Equal(t, "The Extra's Academy Survival Guide", item2.Title)
	assert.Equal(t, "the-extras-academy-survival-guide-11fe7da3", item2.Slug)
	assert.Equal(t, "https://asurascans.com/comics/the-extras-academy-survival-guide-11fe7da3/chapter/{chapter}", item2.URLTemplate)
	require.NotNil(t, item2.LastChapter)
	assert.Equal(t, 74, *item2.LastChapter)
}

func TestRavenScansScraper_ParseHTML(t *testing.T) {
	var capturedReferer string
	var capturedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReferer = r.Header.Get("Referer")
		capturedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleRavenHTML))
	}))
	defer server.Close()

	scraper := providers.NewRavenScansScraperWithURL(server.URL)
	assert.Equal(t, "ravenscans.org", scraper.Domain())
	assert.Equal(t, "RavenScans", scraper.Name())

	res, err := scraper.SearchOrBrowse(context.Background(), "", 1)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "https://ravenscans.org/", capturedReferer)
	assert.NotEmpty(t, capturedUserAgent)

	require.Len(t, res.Items, 2)
	assert.Equal(t, 4, res.TotalPages)
	assert.Equal(t, 1, res.Page)

	item1 := res.Items[0]
	assert.Equal(t, "Almighty Pig Slaughtering Knife", item1.Title)
	assert.Equal(t, "almighty-pig-slaughtering-knife", item1.Slug)
	assert.Equal(t, "https://ravenscans.org/almighty-pig-slaughtering-knife-chapter-{chapter}/", item1.URLTemplate)
	assert.Equal(t, "https://ravenscans.org/wp-content/uploads/knife.jpg", item1.CoverURL)
	require.NotNil(t, item1.LastChapter)
	assert.Equal(t, 66, *item1.LastChapter)
	assert.Equal(t, "ravenscans.org", item1.PrimaryDomain)

	item2 := res.Items[1]
	assert.Equal(t, "Return of the Mount Hua Sect", item2.Title)
	assert.Equal(t, "return-of-the-mount-hua-sect", item2.Slug)
	assert.Equal(t, "https://ravenscans.org/return-of-the-mount-hua-sect-chapter-{chapter}/", item2.URLTemplate)
	require.NotNil(t, item2.LastChapter)
	assert.Equal(t, 142, *item2.LastChapter)
}
