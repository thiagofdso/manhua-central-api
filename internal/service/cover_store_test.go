package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type testCoverStreamer struct{ payload []byte }

func (streamer testCoverStreamer) ProxyImage(ctx context.Context, writer http.ResponseWriter, source string) error {
	_, err := writer.Write(streamer.payload)
	return err
}

func TestCoverStorePersistsDeduplicatesAndServes(t *testing.T) {
	payload, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jE9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := NewCoverStore(root, 1024, testCoverStreamer{payload})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Download(context.Background(), "https://ravenscans.org/cover.png", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Download(context.Background(), "https://ravenscans.org/other.png", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key {
		t.Fatal("identical images must share a file")
	}
	reopened, err := NewCoverStore(root, 1024, testCoverStreamer{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/media/covers/{key...}", reopened)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/media/covers/"+first.Key, nil))
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), payload) {
		t.Fatal("persisted image is not readable after reopening storage")
	}
	if response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatal("missing immutable cache policy")
	}
}

func TestCoverStoreRejectsHTMLAndOversize(t *testing.T) {
	for _, payload := range [][]byte{[]byte("<!doctype html><title>Just a moment</title>"), bytes.Repeat([]byte("x"), 2048)} {
		root := t.TempDir()
		store, err := NewCoverStore(root, 1024, testCoverStreamer{payload})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Download(context.Background(), "https://ravenscans.org/cover", ""); err == nil {
			t.Fatal("invalid download accepted")
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatal("failed download left files behind")
		}
	}
}

func TestCoverStoreBrowserFallback(t *testing.T) {
	payload, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jE9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	worker := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer host-token" {
			t.Error("missing host collector authentication")
		}
		if request.URL.Path != "/collect-cover" || request.Method != "POST" {
			t.Error("wrong worker route")
		}
		writer.Header().Set("X-Cover-Source", "https://ravenscans.org/real-cover.png")
		_, _ = writer.Write(payload)
	}))
	defer worker.Close()
	store, err := NewCoverStore(t.TempDir(), 1024, testCoverStreamer{[]byte("<html>Cloudflare</html>")})
	if err != nil {
		t.Fatal(err)
	}
	store.SetWorkerURL(worker.URL)
	store.SetWorkerToken("host-token")
	cover, err := store.Download(context.Background(), "https://ravenscans.org/cover.png", "https://ravenscans.org/manga/example/")
	if err != nil {
		t.Fatal(err)
	}
	if cover.SourceURL != "https://ravenscans.org/real-cover.png" {
		t.Fatal("original source lost")
	}
	file, err := os.Open(filepath.Join(store.root, cover.Key))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actual, _ := io.ReadAll(file)
	if !bytes.Equal(actual, payload) {
		t.Fatal("challenge response was mixed with image bytes")
	}
}
