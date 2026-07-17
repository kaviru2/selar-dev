// Package handler provides HTTP handlers for all SELAR API endpoints.
// Includes health check, JWT auth (register/login with bcrypt), user profile,
// document CRUD, link suggestions, annotations, concepts, graph, and sessions.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
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

	// Clean up local temp file storage to save space
	_ = os.Remove("/tmp/selar_uploads/" + id + ".pdf")

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
	userID := middleware.GetUserID(r.Context())
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

	if err := h.store.RespondToSuggestion(r.Context(), userID, id, status, req.Label, req.TimeToRespondMs); err != nil {
		log.Printf("failed to respond to suggestion %s: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to respond"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Annotations
// ============================================================

func (h *Handler) ListAnnotations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	docID := chi.URLParam(r, "id")
	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			page = v
		}
	}

	anns, err := h.store.ListAnnotations(r.Context(), userID, docID, page)
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

	mentalModels, err := h.store.ListMentalModels(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list mental models"})
		return
	}
	mentalLinks, err := h.store.ListMentalModelLinks(r.Context(), userID, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list mental-model links"})
		return
	}
	documentConceptEdges, err := h.store.ListDocumentConceptEdges(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect mental models to concepts"})
		return
	}
	reviewedSuggestionEdges, err := h.store.ListReviewedSuggestionEdges(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to project reviewed passage links"})
		return
	}

	graphNodes := make([]model.GraphNode, 0, len(concepts)+len(mentalModels)*4)
	graphEdges := make([]model.GraphEdge, 0, len(edges)+len(mentalLinks)+len(documentConceptEdges)+len(reviewedSuggestionEdges)+len(mentalModels)*4)
	for _, concept := range concepts {
		graphNodes = append(graphNodes, model.GraphNode{
			ID: concept.ID, Name: concept.Name, Description: concept.Description,
			NodeType: "concept", State: concept.State, CreatedAt: concept.CreatedAt,
		})
	}
	for _, edge := range edges {
		explanation := ""
		if edge.SupportCount > 0 {
			explanation = fmt.Sprintf("Adapted deterministically from %d grounded chat answer(s) across %d document(s).", edge.SupportCount, edge.DocumentCount)
		}
		validFrom := edge.ValidFrom
		observedAt := edge.ObservedAt
		graphEdges = append(graphEdges, model.GraphEdge{
			ID: edge.ID, Source: edge.SourceConceptID, Target: edge.TargetConceptID,
			Relation: string(edge.Relation), State: edge.State, Confidence: edge.Confidence,
			CreatedVia: string(edge.CreatedVia), Explanation: explanation,
			ValidFrom: &validFrom, ValidTo: edge.ValidTo, ObservedAt: &observedAt, SupersededBy: edge.SupersededBy,
		})
	}
	for _, mentalModel := range mentalModels {
		modelNodeID := "model:" + mentalModel.ID
		claimNodeID := "claim:" + mentalModel.ID
		graphNodes = append(graphNodes,
			model.GraphNode{ID: modelNodeID, Name: mentalModel.DocumentTitle, Description: mentalModel.Domain,
				NodeType: "document", State: string(mentalModel.Status), DocumentID: mentalModel.DocumentID,
				DocumentTitle: mentalModel.DocumentTitle, CreatedAt: mentalModel.GeneratedAt},
			model.GraphNode{ID: claimNodeID, Name: "Main claim", Description: mentalModel.MainClaim,
				NodeType: "claim", State: "supported", DocumentID: mentalModel.DocumentID,
				DocumentTitle: mentalModel.DocumentTitle, CreatedAt: mentalModel.GeneratedAt},
		)
		graphEdges = append(graphEdges, model.GraphEdge{ID: "has-claim:" + mentalModel.ID,
			Source: modelNodeID, Target: claimNodeID, Relation: "has_claim", State: "confirmed", CreatedVia: "system"})
		for index, assumption := range mentalModel.Assumptions {
			nodeID := "assumption:" + mentalModel.ID + ":" + strconv.Itoa(index)
			graphNodes = append(graphNodes, model.GraphNode{ID: nodeID, Name: "Assumption", Description: assumption,
				NodeType: "assumption", State: "supported", DocumentID: mentalModel.DocumentID,
				DocumentTitle: mentalModel.DocumentTitle, CreatedAt: mentalModel.GeneratedAt})
			graphEdges = append(graphEdges, model.GraphEdge{ID: "has-assumption:" + mentalModel.ID + ":" + strconv.Itoa(index),
				Source: modelNodeID, Target: nodeID, Relation: "has_assumption", State: "confirmed", CreatedVia: "system"})
		}
		for index, question := range mentalModel.OpenQuestions {
			nodeID := "question:" + mentalModel.ID + ":" + strconv.Itoa(index)
			graphNodes = append(graphNodes, model.GraphNode{ID: nodeID, Name: "Open question", Description: question,
				NodeType: "question", State: "supported", DocumentID: mentalModel.DocumentID,
				DocumentTitle: mentalModel.DocumentTitle, CreatedAt: mentalModel.GeneratedAt})
			graphEdges = append(graphEdges, model.GraphEdge{ID: "raises:" + mentalModel.ID + ":" + strconv.Itoa(index),
				Source: modelNodeID, Target: nodeID, Relation: "raises", State: "confirmed", CreatedVia: "system"})
		}
	}
	for _, link := range mentalLinks {
		graphEdges = append(graphEdges, model.GraphEdge{
			ID: link.ID, Source: "model:" + link.SourceModelID, Target: "model:" + link.TargetModelID,
			Relation: string(link.LinkType), State: string(link.Status), Confidence: link.Confidence,
			CreatedVia: string(link.CreatedVia), Explanation: link.BridgeExplanation,
		})
	}
	graphEdges = append(graphEdges, documentConceptEdges...)
	graphEdges = append(graphEdges, reviewedSuggestionEdges...)

	writeJSON(w, http.StatusOK, map[string]any{"nodes": graphNodes, "edges": graphEdges})
}

// ============================================================
// Runtime Mental Models
// ============================================================

func (h *Handler) GetDocumentMentalModel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mentalModel, err := h.store.GetDocumentMentalModel(r.Context(), userID, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mental model not found"})
		return
	}
	writeJSON(w, http.StatusOK, mentalModel)
}

func (h *Handler) ListMentalModelLinks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	links, err := h.store.ListMentalModelLinks(r.Context(), userID, r.URL.Query().Get("document_id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list mental-model links"})
		return
	}
	if links == nil {
		links = []model.MentalModelLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

func (h *Handler) RespondToMentalModelLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var response model.MentalModelLinkResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if response.Action != model.MentalLinkConfirmed && response.Action != model.MentalLinkRejected && response.Action != model.MentalLinkRelabeled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be confirmed, rejected, or relabeled"})
		return
	}
	if err := h.store.RespondToMentalModelLink(r.Context(), userID, chi.URLParam(r, "id"), response); err != nil {
		if errors.Is(err, store.ErrMentalModelLinkNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mental-model link not found"})
			return
		}
		log.Printf("failed to respond to mental-model link %s: %v", chi.URLParam(r, "id"), err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save mental-model response"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ListLearnerConceptState(w http.ResponseWriter, r *http.Request) {
	states, err := h.store.ListLearnerConceptState(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list learner state"})
		return
	}
	if states == nil {
		states = []model.LearnerConceptState{}
	}
	writeJSON(w, http.StatusOK, states)
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
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	var body struct {
		PagesViewed    []int `json:"pages_viewed"`
		MaxScrollDepth int   `json:"max_scroll_depth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.store.EndReadingSession(r.Context(), userID, id, body.PagesViewed, body.MaxScrollDepth); err != nil {
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
