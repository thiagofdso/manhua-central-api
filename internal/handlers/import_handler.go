package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type ImportHandler struct {
	importSvc  *service.ImportService
	jwtService *auth.JWTService
}

func NewImportHandler(importSvc *service.ImportService, jwtService *auth.JWTService) *ImportHandler {
	return &ImportHandler{
		importSvc:  importSvc,
		jwtService: jwtService,
	}
}

func (h *ImportHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *middleware.AuthMiddleware) {
	mux.HandleFunc("GET /api/import/domains", h.GetDomains)
	mux.HandleFunc("GET /api/import/browse", h.Browse)
	if authMiddleware != nil {
		mux.Handle("POST /api/import", authMiddleware.RequireAuth(http.HandlerFunc(h.ImportWorks)))
	} else {
		mux.HandleFunc("POST /api/import", h.ImportWorks)
	}
}

func (h *ImportHandler) GetDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	domains := h.importSvc.GetDomains()
	sendJSON(w, http.StatusOK, domains)
}

func (h *ImportHandler) Browse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	domain := strings.TrimSpace(q.Get("domain"))
	if domain == "" {
		sendJSONError(w, http.StatusBadRequest, "parâmetro 'domain' é obrigatório")
		return
	}

	query := strings.TrimSpace(q.Get("q"))
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	var userID *string
	// Tenta extrair usuário do contexto (se autenticado via middleware) ou direto do header Authorization
	if uid, ok := middleware.GetUserIDFromContext(r.Context()); ok && uid != "" {
		userID = &uid
	} else if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") && h.jwtService != nil {
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if claims, err := h.jwtService.ValidateAccessToken(tokenStr); err == nil && claims != nil {
			uid := claims.UserID
			userID = &uid
		}
	}

	res, err := h.importSvc.Browse(r.Context(), domain, query, page, userID)
	if err != nil {
		sendJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, res)
}

func (h *ImportHandler) ImportWorks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok || userID == "" {
		// Tenta validar direto pelo token se o middleware não tiver sido executado
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") && h.jwtService != nil {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if claims, err := h.jwtService.ValidateAccessToken(tokenStr); err == nil && claims != nil {
				userID = claims.UserID
				ok = true
			}
		}
	}

	if !ok || userID == "" {
		sendJSONError(w, http.StatusUnauthorized, "Token de autenticação obrigatório")
		return
	}

	var payload struct {
		Items []service.ImportPayloadItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Corpo de requisição JSON inválido")
		return
	}

	res, err := h.importSvc.ImportWorks(r.Context(), userID, payload.Items)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, res)
}
