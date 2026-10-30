package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/proxy"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type ChapterImage struct {
	Index    int    `json:"index"`
	URL      string `json:"url"`
	ProxyURL string `json:"proxyUrl"`
}

type ChapterImagesResponse struct {
	CanonicalManhuaID int64          `json:"canonicalManhuaId"`
	Slug              string         `json:"slug"`
	Title             string         `json:"title"`
	Chapter           int            `json:"chapter"`
	LastChapter       *int           `json:"lastChapter,omitempty"`
	TotalImages       int            `json:"totalImages"`
	Images            []ChapterImage `json:"images"`
}

type ReaderHandler struct {
	streamer         *proxy.Streamer
	canonicalRepo    repository.CanonicalRepository
	extractorService *service.ExtractorService
}

func NewReaderHandler(streamer *proxy.Streamer, canonicalRepo repository.CanonicalRepository, extractorService ...*service.ExtractorService) *ReaderHandler {
	var ext *service.ExtractorService
	if len(extractorService) > 0 && extractorService[0] != nil {
		ext = extractorService[0]
	} else {
		ext = service.NewExtractorService("", nil)
	}
	return &ReaderHandler{
		streamer:         streamer,
		canonicalRepo:    canonicalRepo,
		extractorService: ext,
	}
}

func (h *ReaderHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/reader/proxy-image", h.ProxyImage)
	mux.HandleFunc("/api/reader/chapter-images", h.GetChapterImages)
}

func (h *ReaderHandler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		sendJSONError(w, http.StatusBadRequest, "missing url query parameter")
		return
	}

	err := h.streamer.ProxyImage(r.Context(), w, targetURL)
	if err != nil {
		if errors.Is(err, proxy.ErrForbiddenDomain) || errors.Is(err, proxy.ErrSSRFBlocked) {
			sendJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if errors.Is(err, proxy.ErrInvalidScheme) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		sendJSONError(w, http.StatusBadGateway, "failed to stream image: "+err.Error())
		return
	}
}

func (h *ReaderHandler) GetChapterImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.URL.Query().Get("slug")
	chapterStr := r.URL.Query().Get("chapter")

	if slug == "" {
		sendJSONError(w, http.StatusBadRequest, "slug query parameter is required")
		return
	}

	chapter := 1
	if chapterStr != "" {
		c, err := strconv.Atoi(chapterStr)
		if err != nil || c < 1 {
			sendJSONError(w, http.StatusBadRequest, "invalid chapter number")
			return
		}
		chapter = c
	}

	manhua, err := h.canonicalRepo.GetBySlug(r.Context(), slug)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to lookup canonical manhua: "+err.Error())
		return
	}

	if manhua == nil {
		sendJSONError(w, http.StatusNotFound, "canonical manhua not found")
		return
	}

	// RN-03.2: Teto inviolável por lastChapter
	if manhua.LastChapter != nil && chapter > *manhua.LastChapter {
		sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("chapter %d exceeds last available chapter (%d)", chapter, *manhua.LastChapter))
		return
	}

	// Increment view count in canonical catalog if needed
	manhua.TotalViews++
	_ = h.canonicalRepo.Update(r.Context(), manhua)

	// Extract real chapter images from upstream domain
	var images []ChapterImage
	if h.extractorService != nil && manhua.URLTemplate != "" {
		extractedURLs, err := h.extractorService.ExtractChapterImages(r.Context(), manhua, chapter)
		if err != nil {
			sendJSONError(w, http.StatusBadGateway, fmt.Sprintf("failed to extract chapter images from upstream provider: %v", err))
			return
		}
		if len(extractedURLs) == 0 {
			sendJSONError(w, http.StatusNotFound, fmt.Sprintf("no images found for chapter %d", chapter))
			return
		}
		images = make([]ChapterImage, 0, len(extractedURLs))
		for i, imgURL := range extractedURLs {
			proxyURL := "/api/reader/proxy-image?url=" + url.QueryEscape(imgURL)
			images = append(images, ChapterImage{
				Index:    i + 1,
				URL:      imgURL,
				ProxyURL: proxyURL,
			})
		}
	} else {
		sendJSONError(w, http.StatusBadRequest, "manhua does not have a URL template configured")
		return
	}

	resp := ChapterImagesResponse{
		CanonicalManhuaID: manhua.ID,
		Slug:              manhua.Slug,
		Title:             manhua.Title,
		Chapter:           chapter,
		LastChapter:       manhua.LastChapter,
		TotalImages:       len(images),
		Images:            images,
	}

	sendJSON(w, http.StatusOK, resp)
}
