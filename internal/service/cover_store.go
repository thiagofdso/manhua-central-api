package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

type CoverStore struct {
	workerURL   string
	workerToken string
	root        string
	maxBytes    int64
	streamer    coverStreamer
}

type coverStreamer interface {
	ProxyImage(context.Context, http.ResponseWriter, string) error
}

type StoredCover struct {
	SourceURL   string
	Key         string
	Hash        string
	ContentType string
	Size        int64
}

func (store *CoverStore) SetWorkerURL(workerURL string) { store.workerURL = workerURL }

func (store *CoverStore) SetWorkerToken(token string) { store.workerToken = token }

func (store *CoverStore) downloadWithBrowser(ctx context.Context, writer *coverWriter, source, pageURL string) error {
	if store.workerURL == "" {
		return fmt.Errorf("cover browser is not configured")
	}
	payload, err := json.Marshal(map[string]string{"pageURL": pageURL, "imageURL": source})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, store.workerURL+"/collect-cover", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if store.workerToken != "" {
		req.Header.Set("Authorization", "Bearer "+store.workerToken)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
		return fmt.Errorf("cover browser returned %d: %s", response.StatusCode, failure.Error)
	}
	writer.header.Set("X-Cover-Source", response.Header.Get("X-Cover-Source"))
	_, err = io.Copy(writer, response.Body)
	return err
}

type coverWriter struct {
	file      *os.File
	header    http.Header
	remaining int64
}

func (writer *coverWriter) Header() http.Header    { return writer.header }
func (writer *coverWriter) WriteHeader(status int) {}
func (writer *coverWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		return 0, fmt.Errorf("cover exceeds download limit")
	}
	count, err := writer.file.Write(data)
	writer.remaining -= int64(count)
	return count, err
}

func NewCoverStore(root string, maxBytes int64, streamer coverStreamer) (*CoverStore, error) {
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &CoverStore{root: root, maxBytes: maxBytes, streamer: streamer}, nil
}

func (store *CoverStore) Download(ctx context.Context, source, pageURL string) (*StoredCover, error) {
	file, err := os.CreateTemp(store.root, ".cover-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	writer := &coverWriter{file: file, header: make(http.Header), remaining: store.maxBytes}
	var contentType string
	var extension string
	for attempt := 0; attempt < 2; attempt++ {
		if err := file.Truncate(0); err != nil {
			return nil, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		writer.remaining = store.maxBytes
		if attempt == 0 && source != "" {
			err = store.streamer.ProxyImage(ctx, writer, source)
		} else {
			err = store.downloadWithBrowser(ctx, writer, source, pageURL)
		}
		if err != nil {
			if attempt == 0 && source != "" {
				continue
			}
			return nil, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		probe := make([]byte, 512)
		count, err := file.Read(probe)
		if err != nil {
			return nil, err
		}
		contentType = http.DetectContentType(probe[:count])
		extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}
		var ok bool
		extension, ok = extensions[contentType]
		if !ok {
			if attempt == 0 {
				continue
			}
			return nil, fmt.Errorf("unsupported cover content: %s", contentType)
		}
		break
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return nil, err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	key := hash[:2] + "/" + hash + extension
	target := filepath.Join(store.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	if err := file.Chmod(0644); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(file.Name(), target); err != nil {
		return nil, err
	}
	if resolved := writer.header.Get("X-Cover-Source"); resolved != "" {
		source = resolved
	}
	return &StoredCover{Key: key, Hash: hash, ContentType: contentType, Size: size, SourceURL: source}, nil
}

var coverKeyPattern = regexp.MustCompile(`^[a-f0-9]{2}/[a-f0-9]{64}\.(jpg|png|webp|gif)$`)

func (store *CoverStore) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	key := request.PathValue("key")
	if !coverKeyPattern.MatchString(key) || key[:2] != key[3:5] {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(writer, request, filepath.Join(store.root, filepath.FromSlash(key)))
}
