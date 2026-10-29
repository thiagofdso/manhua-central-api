package providers

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

func TestExtractCover(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"lazy cover", `<div class="summary_image"><img src="data:image/gif;base64,AA" data-src="//cdn.example/cover.webp"></div>`, "https://cdn.example/cover.webp"},
		{"relative cover", `<div class="thumb"><img src="/covers/book.jpg"></div>`, "https://example.org/covers/book.jpg"},
		{"srcset", `<div class="thumb"><img src="/placeholder.gif" data-srcset="/book.webp 400w, /large.webp 800w"></div>`, "https://example.org/book.webp"},
		{"comix detail", `<script id="initial-data" type="application/json">{"queries":{"[\"manga\",\"detail\",\"j1ye\"]":{"poster":{"large":"https://static.comix.to/cover.jpg"}}}}</script>`, "https://static.comix.to/cover.jpg"},
		{"metadata", `<meta property="og:image" content="/covers/og.jpg">`, "https://example.org/covers/og.jpg"},
		{"challenge", `<title>Just a moment...</title>`, ""},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(example.html))
			if err != nil {
				t.Fatal(err)
			}
			if got := extractCover(doc, "https://example.org/"); got != example.want {
				t.Fatalf("got %q, want %q", got, example.want)
			}
		})
	}
}

func TestCoverPage(t *testing.T) {
	cases := []struct {
		item domain.ScrapedItem
		want string
	}{
		{domain.ScrapedItem{PrimaryDomain: "comix.to", URLTemplate: "https://comix.to/title/j1ye-mr-zombie/{chapter}"}, "https://comix.to/title/j1ye-mr-zombie"},
		{domain.ScrapedItem{PrimaryDomain: "mgeko.cc", Slug: "1000000000000-game"}, "https://www.mgeko.cc/manga/1000000000000-game/"},
		{domain.ScrapedItem{PrimaryDomain: "manhuaus.com", URLTemplate: "https://manhuaus.com/manga/magic-emperor-0/chapter-{chapter}/"}, "https://manhuaus.com/manga/magic-emperor-0/"},
	}
	for _, example := range cases {
		if got := coverPage(example.item); got != example.want {
			t.Fatalf("got %q, want %q", got, example.want)
		}
	}
}
