// Package handler provides HTTP handlers for all SELAR API endpoints.
// Includes health check, JWT auth (register/login with bcrypt), user profile,
// document CRUD, link suggestions, annotations, concepts, graph, and sessions.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// Handler holds the store and auth for HTTP handlers.
type Handler struct {
	store *store.Store
	auth  *middleware.Auth
}

// New creates a new Handler with the given store.
func New(st *store.Store) *Handler {
	return &Handler{store: st}
}

// SetAuth sets the auth middleware reference for token generation.
func (h *Handler) SetAuth(a *middleware.Auth) {
	h.auth = a
}

// ============================================================
// Health
// ============================================================

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "selar-api"})
}

// ============================================================
// Auth — Register / Login
// ============================================================

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	// Random cohort assignment via block randomization
	cohort := model.CohortTreatmentHITL // TODO: implement proper randomization

	user, err := h.store.CreateUser(r.Context(), req.Email, string(hash), cohort)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use"})
		return
	}

	// Auto-login: generate token immediately
	token := ""
	if h.auth != nil {
		token, _ = h.auth.GenerateToken(user.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if h.auth == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}

	token, err := h.auth.GenerateToken(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

// ============================================================
// Users
// ============================================================

func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var prefs map[string]any
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.store.UpdateUserPreferences(r.Context(), userID, prefs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update preferences"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Documents
// ============================================================

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	docs, err := h.store.ListDocuments(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list documents"})
		return
	}
	if docs == nil {
		docs = []model.Document{}
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	doc, err := h.store.GetDocument(r.Context(), id, userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var doc model.Document
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	doc.UserID = userID
	doc.Status = model.DocStatusUploaded
	if err := h.store.CreateDocument(r.Context(), &doc); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create document"})
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteDocument(r.Context(), id, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete document"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) GetDocumentStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	stats, err := h.store.GetDocumentStats(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get stats"})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ============================================================
// Link Suggestions
// ============================================================

func (h *Handler) ListSuggestions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	docID := chi.URLParam(r, "id")
	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			page = v
		}
	}

	suggestions, err := h.store.ListSuggestions(r.Context(), userID, docID, page)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list suggestions"})
		return
	}
	if suggestions == nil {
		suggestions = []model.LinkSuggestion{}
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *Handler) RespondToSuggestion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req model.SuggestionResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	status := model.SuggestionStatus(req.Action)
	if status != model.SuggestionConfirmed && status != model.SuggestionRejected && status != model.SuggestionRelabeled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be 'confirmed', 'rejected', or 'relabeled'"})
		return
	}

	if err := h.store.RespondToSuggestion(r.Context(), id, status, req.Label, req.TimeToRespondMs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to respond"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Annotations
// ============================================================

func (h *Handler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			page = v
		}
	}

	anns, err := h.store.ListAnnotations(r.Context(), docID, page)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list annotations"})
		return
	}
	if anns == nil {
		anns = []model.Annotation{}
	}
	writeJSON(w, http.StatusOK, anns)
}

func (h *Handler) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var a model.Annotation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	a.UserID = userID
	if err := h.store.CreateAnnotation(r.Context(), &a); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create annotation"})
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) DeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteAnnotation(r.Context(), id, userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete annotation"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ============================================================
// Concepts & Graph
// ============================================================

func (h *Handler) ListConcepts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	concepts, err := h.store.ListConcepts(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list concepts"})
		return
	}
	if concepts == nil {
		concepts = []model.Concept{}
	}
	writeJSON(w, http.StatusOK, concepts)
}

func (h *Handler) GetGraph(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	concepts, err := h.store.ListConcepts(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list concepts"})
		return
	}
	edges, err := h.store.ListConceptEdges(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list edges"})
		return
	}

	if concepts == nil {
		concepts = []model.Concept{}
	}
	if edges == nil {
		edges = []model.ConceptEdge{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": concepts,
		"edges": edges,
	})
}

// ============================================================
// Reading Sessions
// ============================================================

func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var sess model.ReadingSession
	if err := json.NewDecoder(r.Body).Decode(&sess); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sess.UserID = userID
	if err := h.store.CreateReadingSession(r.Context(), &sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (h *Handler) EndSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		PagesViewed    []int `json:"pages_viewed"`
		MaxScrollDepth int   `json:"max_scroll_depth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.store.EndReadingSession(r.Context(), id, body.PagesViewed, body.MaxScrollDepth); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to end session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Helpers
// ============================================================

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
