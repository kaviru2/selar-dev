// Package handler provides HTTP handlers for all SELAR API endpoints.
// Includes health check, JWT auth (register/login with bcrypt), user profile,
// document CRUD, link suggestions, annotations, concepts, graph, and sessions.
package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/recaptcha"
	"github.com/selar-dev/selar-api/internal/settings"
	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/store"
	"github.com/selar-dev/selar-api/internal/workertrigger"
	"golang.org/x/crypto/bcrypt"
)

// Handler holds the store and auth for HTTP handlers.
type Handler struct {
	store   *store.Store
	auth    *middleware.Auth
	storage storage.Store
	worker  *workertrigger.Client
	admins  AdminChecker
	google  GoogleExchanger
	// driveAPIBase overrides the Google Drive endpoint in tests.
	driveAPIBase string

	// adminEmails (ADMIN_EMAILS) are promoted to admin on login/registration.
	adminEmails analytics.EmailSet
	// analyticsOff disables all event recording (ANALYTICS_ENABLED=false).
	analyticsOff bool
	// captcha verifies reCAPTCHA tokens on account creation (nil = off).
	captcha recaptcha.Verifier

	// calendar configures the optional quiz-window calendar feed (calendar.go).
	calendar *CalendarConfig
}

// New creates a new Handler with the given store. Upload storage defaults to
// the local /tmp/selar_uploads directory until SetStorage is called.
func New(st *store.Store) *Handler {
	return &Handler{store: st, storage: &storage.Local{Root: "/tmp/selar_uploads"}, worker: &workertrigger.Client{}}
}

// SetStorage selects the upload storage backend.
func (h *Handler) SetStorage(s storage.Store) {
	if s != nil {
		h.storage = s
	}
}

// SetWorkerTrigger configures the serverless worker notification client.
func (h *Handler) SetWorkerTrigger(client *workertrigger.Client) {
	if client != nil {
		h.worker = client
	}
}

// notifyWorker tells a serverless worker a job is ready. It waits at most
// WORKER_TRIGGER_TIMEOUT (serverless functions are frozen once they respond,
// so a background goroutine would never deliver it) and failures are
// tolerated: the job is durable and the scheduled sweep claims it.
func (h *Handler) notifyWorker(r *http.Request, jobID string) {
	h.worker.NotifyBestEffort(r.Context(), jobID)
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
	// ResearchConsent is the optional analytics opt-in checkbox shown at
	// registration. Omitted = not asked (the console asks after login).
	ResearchConsent *bool `json:"research_consent,omitempty"`
	// RecaptchaToken is required on registration when RECAPTCHA_* is set.
	RecaptchaToken string `json:"recaptcha_token,omitempty"`
}

// roleFor returns the role a user should be created with.
func (h *Handler) roleFor(email string) string {
	if h.adminEmails.Contains(email) {
		return middleware.RoleAdmin
	}
	return middleware.RoleUser
}

// applyAdminBootstrap promotes a user listed in ADMIN_EMAILS. It never
// demotes: removing an email from the list does not revoke an existing
// admin (use `go run ./cmd/admin demote <email>`).
func (h *Handler) applyAdminBootstrap(r *http.Request, email string, user *model.User) {
	if user.Role == middleware.RoleAdmin || !h.adminEmails.Contains(email) {
		return
	}
	if err := h.store.SetUserRole(r.Context(), user.ID, middleware.RoleAdmin); err != nil {
		log.Printf("ADMIN_EMAILS promotion failed: %v", err)
		return
	}
	user.Role = middleware.RoleAdmin
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	parsedEmail, emailErr := mail.ParseAddress(req.Email)
	if req.Email == "" || emailErr != nil || parsedEmail.Address != req.Email || len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a valid email and a password of at least 8 characters are required"})
		return
	}
	if !h.requireHuman(w, r, req.RecaptchaToken, RecaptchaActionRegister) {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user, err := h.store.CreateUserWith(r.Context(), h.newRegistration(req.Email, string(hash), req.ResearchConsent, ""))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use"})
		return
	}

	// Auto-login: generate token immediately
	token := ""
	if h.auth != nil {
		token, _ = h.auth.GenerateToken(user.ID)
	}
	if user.ConsentedAt != nil {
		h.Track(r.Context(), user.ID, analytics.ConsentGranted, analytics.Props{"via": "register"})
	}
	h.Track(r.Context(), user.ID, analytics.SignedIn, analytics.Props{"method": "register"})

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

	// Bind the token to the current session version so it survives only
	// until the next password change.
	_, _, sessionVersion, err := h.store.GetUserAuth(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	token, err := h.auth.GenerateSessionToken(user.ID, sessionVersion)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	h.applyAdminBootstrap(r, req.Email, user)
	h.Track(r.Context(), user.ID, analytics.SignedIn, analytics.Props{"method": "login"})

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

// UpdatePreferences is the legacy route used by older consoles, which send
// their whole preferences object. It merges (never replaces) and silently
// drops unknown, invalid or cohort-locked keys. New clients use
// PATCH /api/users/me/settings, which rejects them explicitly.
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var prefs map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&prefs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	_, _, locks, err := h.store.GetUserSettings(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update preferences"})
		return
	}
	_, locked := settings.Effective(nil, locks)
	stored, _, _, _ := h.store.GetUserSettings(r.Context(), userID)
	if clean := settings.SanitizeLegacy(prefs, locked); len(clean) > 0 {
		if patch, ok := clean["reader"].(map[string]any); ok {
			clean["reader"] = settings.MergeReader(stored["reader"], patch)
		}
		if err := h.store.MergeUserSettings(r.Context(), userID, clean); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update preferences"})
			return
		}
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
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid document id"})
		return
	}
	// Only the owner may delete; resolve the stored PDF locator before the
	// rows (and their ingestion jobs) disappear.
	if _, err := h.store.GetDocument(r.Context(), id, userID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	pdfLocator := ""
	if job, err := h.store.GetLatestIngestionJob(r.Context(), id, userID); err == nil && job.SourceType == "pdf" {
		pdfLocator = job.FilePath
	}
	if err := h.store.DeleteDocument(r.Context(), id, userID); err != nil {
		log.Printf("failed to delete document %s: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete document"})
		return
	}
	if err := h.storage.DeleteDocument(r.Context(), id, pdfLocator); err != nil {
		log.Printf("storage cleanup for deleted document %s failed: %v", id, err)
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
		switch {
		case errors.Is(err, store.ErrUnclassifiedNotConfirmable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		case errors.Is(err, store.ErrSuggestionNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "suggestion not found"})
			return
		}
		log.Printf("failed to respond to suggestion %s: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to respond"})
		return
	}
	h.Track(r.Context(), userID, analytics.DecisionMade, analytics.Props{"kind": "passage_link",
		"action": decisionAction(string(status)), "link_id": id, "response_ms": req.TimeToRespondMs})
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
	h.Track(r.Context(), userID, analytics.HighlightCreated, analytics.Props{"document_id": a.DocumentID, "page": a.Page, "type": string(a.Type)})
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
	mentalLinks, err := h.store.ListReviewedMentalLinkEdges(r.Context(), userID)
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
		if edge.CreatedVia == model.EdgeDeterministicChat {
			explanation = "Explicitly confirmed by the owner; earlier chat co-citations are not relationship proof."
		} else if edge.SupportCount > 0 {
			explanation = "Earlier chat co-citations were recorded as observations, not relationship proof."
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
	graphEdges = append(graphEdges, mentalLinks...)
	graphEdges = append(graphEdges, documentConceptEdges...)
	graphEdges = append(graphEdges, reviewedSuggestionEdges...)

	assertions, err := h.store.ListConfirmedResearchAssertionEdges(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to project research assertions"})
		return
	}
	assertionNodes, assertionEdges := researchAssertionGraph(assertions)
	graphNodes = append(graphNodes, assertionNodes...)
	graphEdges = append(graphEdges, assertionEdges...)

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
	if response.Revision == nil || *response.Revision < 0 || uuid.Validate(chi.URLParam(r, "id")) != nil ||
		(response.Action != model.MentalLinkConfirmed && response.Action != model.MentalLinkRejected && response.Action != model.MentalLinkRelabeled && response.Action != "retracted" && response.Action != "rolled_back") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid id, explicit revision and supported action required"})
		return
	}
	if err := h.store.RespondToMentalModelLink(r.Context(), userID, chi.URLParam(r, "id"), response); err != nil {
		if errors.Is(err, store.ErrMentalModelLinkNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mental-model link not found"})
			return
		}
		if errors.Is(err, store.ErrReviewStale) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "stale review revision; refresh preview"})
			return
		}
		if errors.Is(err, store.ErrReviewInvalid) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid transition or unsupported relabel"})
			return
		}
		log.Printf("failed to respond to mental-model link %s: %v", chi.URLParam(r, "id"), err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save mental-model response"})
		return
	}
	h.Track(r.Context(), userID, analytics.DecisionMade, analytics.Props{"kind": "mental_link",
		"action": decisionAction(string(response.Action)), "link_id": chi.URLParam(r, "id")})
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
	sess.DocumentID = strings.TrimSpace(sess.DocumentID)
	if sess.DocumentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "document_id required"})
		return
	}
	sess.StartedAt = time.Now()
	if err := h.store.CreateReadingSession(r.Context(), &sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create session"})
		return
	}
	h.Track(r.Context(), userID, analytics.ReadingSessionStart, analytics.Props{"document_id": sess.DocumentID, "session_id": sess.ID})
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
	sess, err := h.store.EndReadingSession(r.Context(), userID, id, body.PagesViewed, body.MaxScrollDepth)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown, foreign or already-ended session: idempotent no-op.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to end session"})
		return
	}
	h.Track(r.Context(), userID, analytics.ReadingSessionEnd, analytics.Props{"document_id": sess.DocumentID, "session_id": sess.ID,
		"duration_ms": sess.EndedAt.Sub(sess.StartedAt).Milliseconds(), "pages_viewed": len(body.PagesViewed), "max_scroll_depth": body.MaxScrollDepth})
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
