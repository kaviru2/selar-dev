package handler

// analytics.go — first-party usage analytics, research consent and the admin API.
//
// Track is the one server-side entry point other features use:
//
//	h.Track(r.Context(), userID, analytics.QuizSubmitted, analytics.Props{"quiz_id": id, "score_pct": 80})
//
// It never fails the caller's request: it validates the event against the
// allow-list in internal/analytics, writes it synchronously with a short
// deadline (serverless functions freeze after responding, so a background
// goroutine could be lost) and only logs errors.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
)

const trackTimeout = 2 * time.Second

// SetAnalyticsEnabled turns event recording on or off (ANALYTICS_ENABLED).
func (h *Handler) SetAnalyticsEnabled(enabled bool) { h.analyticsOff = !enabled }

// AnalyticsEnabled reports whether events are recorded.
func (h *Handler) AnalyticsEnabled() bool { return !h.analyticsOff }

// SetAdminEmails sets the ADMIN_EMAILS bootstrap list.
func (h *Handler) SetAdminEmails(set analytics.EmailSet) { h.adminEmails = set }

// RoleLookup adapts the store for middleware.RequireAdmin.
func (h *Handler) RoleLookup() middleware.RoleLookup {
	return func(ctx context.Context, userID string) (string, error) {
		if h.store == nil {
			return "", errors.New("no store")
		}
		return h.store.GetUserRole(ctx, userID)
	}
}

// Track records one server-side analytics event for userID. Per-user rows
// are only written for users with research consent; everyone else adds to
// anonymous daily counts. Safe to call with analytics disabled or no store.
func (h *Handler) Track(ctx context.Context, userID string, event analytics.Event, props analytics.Props) {
	h.trackEvents(ctx, userID, analytics.SourceServer, []store.EventInput{{Event: event, Props: props, OccurredAt: time.Now()}})
}

func (h *Handler) trackEvents(ctx context.Context, userID string, source analytics.Source, events []store.EventInput) {
	if h.analyticsOff || h.store == nil || userID == "" || len(events) == 0 {
		return
	}
	if _, err := uuid.Parse(userID); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), trackTimeout)
	defer cancel()
	if _, err := h.store.RecordEvents(ctx, userID, source, events); err != nil {
		log.Printf("analytics: failed to record %d event(s): %v", len(events), err)
	}
}

// decisionAction maps review actions from the various endpoints onto the
// dashboard's keep / change / reject vocabulary.
func decisionAction(action string) string {
	switch action {
	case "confirmed", "confirm":
		return "keep"
	case "relabeled":
		return "change"
	case "rejected", "reject":
		return "reject"
	case "retracted":
		return "retract"
	case "rolled_back":
		return "undo"
	}
	return ""
}

// ============================================================
// User-facing analytics endpoints
// ============================================================

// MountUserAnalytics registers the consent, beacon and delete-my-data routes
// on an authenticated router.
func (h *Handler) MountUserAnalytics(r chi.Router) {
	r.Get("/analytics/config", h.AnalyticsConfig)
	r.Post("/analytics/events", h.CollectEvents)
	r.Put("/users/me/research-consent", h.SetResearchConsent)
	r.Delete("/users/me/analytics", h.DeleteMyAnalytics)
}

// AnalyticsConfig tells the console whether to ask for consent and send events.
func (h *Handler) AnalyticsConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         h.AnalyticsEnabled(),
		"consent_version": analytics.ConsentVersion,
		"max_batch":       analytics.MaxBatch,
		"max_dwell_ms":    analytics.MaxDwellMs,
	})
}

type collectRequest struct {
	Events []struct {
		Event string          `json:"event"`
		Props analytics.Props `json:"props"`
		At    *time.Time      `json:"at"`
	} `json:"events"`
}

// CollectEvents is the console's batched beacon endpoint. It accepts only
// client-side events from the allow-list and always answers 202 for valid
// requests, so the browser never retries or learns consent state from it.
func (h *Handler) CollectEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var req collectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized analytics batch"})
		return
	}
	if len(req.Events) > analytics.MaxBatch {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": fmt.Sprintf("at most %d events per batch", analytics.MaxBatch)})
		return
	}
	if h.analyticsOff {
		writeJSON(w, http.StatusAccepted, map[string]any{"enabled": false})
		return
	}
	now := time.Now()
	events := make([]store.EventInput, 0, len(req.Events))
	for _, e := range req.Events {
		at := now
		if e.At != nil {
			at = analytics.ClampClientTime(*e.At, now)
		}
		events = append(events, store.EventInput{Event: analytics.Event(e.Event), Props: e.Props, OccurredAt: at})
	}
	h.trackEvents(r.Context(), userID, analytics.SourceClient, events)
	writeJSON(w, http.StatusAccepted, map[string]any{"enabled": true})
}

// SetResearchConsent records the user's optional opt-in or withdrawal.
func (h *Handler) SetResearchConsent(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Granted *bool  `json:"granted"`
		Via     string `json:"via"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || req.Granted == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "granted (true or false) is required"})
		return
	}
	if err := h.store.SetResearchConsent(r.Context(), userID, *req.Granted, analytics.ConsentVersion); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save consent"})
		return
	}
	if *req.Granted {
		via := req.Via
		if via != "settings" {
			via = "prompt"
		}
		h.Track(r.Context(), userID, analytics.ConsentGranted, analytics.Props{"via": via})
	}
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// DeleteMyAnalytics deletes every analytics event of the current user.
func (h *Handler) DeleteMyAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	deleted, err := h.store.DeleteUserAnalytics(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete analytics data"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_events": deleted})
}

// ============================================================
// Admin API (mounted behind middleware.RequireAdmin)
// ============================================================

// MountAdmin registers admin-only routes. The caller must wrap the router in
// auth.Verify and middleware.RequireAdmin.
func (h *Handler) MountAdmin(r chi.Router) {
	r.Use(noStore)
	r.Get("/events", h.AdminEventDictionary)
	r.Get("/overview", h.AdminOverview)
	r.Get("/users", h.AdminListUsers)
	r.Get("/users/{id}", h.AdminGetUser)
	r.Patch("/users/{id}", h.AdminUpdateUser)
	r.Get("/users/{id}/timeline", h.AdminUserTimeline)
	r.Get("/export/events.csv", h.AdminExportEvents)
	r.Get("/export/users.csv", h.AdminExportUsers)
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

const maxAdminRange = 366 * 24 * time.Hour

// parseAdminFilter reads ?from=YYYY-MM-DD&to=YYYY-MM-DD&group=… (UTC days,
// both inclusive; default: the last 30 days).
func parseAdminFilter(r *http.Request) (store.AdminFilter, error) {
	q := r.URL.Query()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	to := today.Add(24 * time.Hour)
	from := to.Add(-30 * 24 * time.Hour)
	if v := q.Get("to"); v != "" {
		day, err := time.Parse("2006-01-02", v)
		if err != nil {
			return store.AdminFilter{}, errors.New("to must be YYYY-MM-DD")
		}
		to = day.Add(24 * time.Hour)
	}
	if v := q.Get("from"); v != "" {
		day, err := time.Parse("2006-01-02", v)
		if err != nil {
			return store.AdminFilter{}, errors.New("from must be YYYY-MM-DD")
		}
		from = day
	}
	if !from.Before(to) {
		return store.AdminFilter{}, errors.New("from must not be after to")
	}
	if to.Sub(from) > maxAdminRange {
		return store.AdminFilter{}, errors.New("date range may span at most 366 days")
	}
	group := strings.TrimSpace(q.Get("group"))
	if len(group) > 64 {
		return store.AdminFilter{}, errors.New("group too long")
	}
	return store.AdminFilter{From: from, To: to, Group: group}, nil
}

func (h *Handler) adminFilter(w http.ResponseWriter, r *http.Request) (store.AdminFilter, bool) {
	f, err := parseAdminFilter(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return f, false
	}
	return f, true
}

// AdminEventDictionary returns the allow-listed events.
func (h *Handler) AdminEventDictionary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": h.AnalyticsEnabled(), "consent_version": analytics.ConsentVersion, "events": analytics.Dictionary()})
}

// AdminOverview returns dashboard KPIs.
func (h *Handler) AdminOverview(w http.ResponseWriter, r *http.Request) {
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	overview, err := h.store.AdminOverview(r.Context(), f)
	if err != nil {
		log.Printf("admin overview: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute overview"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"analytics_enabled": h.AnalyticsEnabled(), "overview": overview})
}

// AdminListUsers returns the users table.
func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	users, err := h.store.AdminListUsers(r.Context(), f)
	if err != nil {
		log.Printf("admin users: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func validUserParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return "", false
	}
	return id, true
}

// AdminGetUser returns one user's summary row.
func (h *Handler) AdminGetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := validUserParam(w, r)
	if !ok {
		return
	}
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	user, err := h.store.AdminGetUser(r.Context(), id, f)
	if errors.Is(err, store.ErrUserNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// AdminUpdateUser edits the neutral group label.
func (h *Handler) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := validUserParam(w, r)
	if !ok {
		return
	}
	var req struct {
		GroupLabel *string `json:"group_label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || req.GroupLabel == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "group_label is required"})
		return
	}
	err := h.store.SetUserGroupLabel(r.Context(), id, *req.GroupLabel)
	switch {
	case errors.Is(err, store.ErrInvalidGroupLabel):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
	default:
		label, _ := store.NormalizeGroupLabel(*req.GroupLabel)
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "group_label": label})
	}
}

// AdminUserTimeline returns a user's recent events.
func (h *Handler) AdminUserTimeline(w http.ResponseWriter, r *http.Request) {
	id, ok := validUserParam(w, r)
	if !ok {
		return
	}
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := h.store.AdminUserTimeline(r.Context(), id, f, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load timeline"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// csvCell neutralises spreadsheet formula injection in exported text.
func csvCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

func startCSV(w http.ResponseWriter, name string) *csv.Writer {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	return csv.NewWriter(w)
}

func includeEmail(r *http.Request) bool { return r.URL.Query().Get("include_email") == "true" }

// AdminExportEvents streams analytics events as CSV. Emails are omitted
// unless ?include_email=true; password hashes are never selected.
func (h *Handler) AdminExportEvents(w http.ResponseWriter, r *http.Request) {
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	withEmail := includeEmail(r)
	out := startCSV(w, fmt.Sprintf("selar-events-%s-%s.csv", f.From.Format("20060102"), f.To.Add(-time.Second).Format("20060102")))
	header := []string{"occurred_at_utc", "user_id", "group_label", "event", "source", "props_json"}
	if withEmail {
		header = append(header[:2], append([]string{"email"}, header[2:]...)...)
	}
	_ = out.Write(header)
	err := h.store.AdminExportEvents(r.Context(), f, withEmail, func(row store.ExportEventRow) error {
		record := []string{row.OccurredAt.UTC().Format(time.RFC3339), row.UserID, csvCell(row.GroupLabel), row.Event, row.Source, csvCell(row.Props)}
		if withEmail {
			record = append(record[:2], append([]string{csvCell(row.Email)}, record[2:]...)...)
		}
		return out.Write(record)
	})
	if err != nil {
		log.Printf("admin export events: %v", err)
	}
	out.Flush()
}

// AdminExportUsers exports the per-user summary table as CSV.
func (h *Handler) AdminExportUsers(w http.ResponseWriter, r *http.Request) {
	f, ok := h.adminFilter(w, r)
	if !ok {
		return
	}
	users, err := h.store.AdminListUsers(r.Context(), f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to export users"})
		return
	}
	withEmail := includeEmail(r)
	out := startCSV(w, fmt.Sprintf("selar-users-%s-%s.csv", f.From.Format("20060102"), f.To.Add(-time.Second).Format("20060102")))
	header := []string{"user_id", "role", "group_label", "created_at_utc", "consented", "consent_version", "last_seen_utc",
		"active_days", "events", "documents", "reading_minutes", "suggestions_shown", "decisions", "keep", "change", "reject",
		"chat_questions", "quiz_attempts", "quiz_avg_pct"}
	if withEmail {
		header = append(header[:1], append([]string{"email"}, header[1:]...)...)
	}
	_ = out.Write(header)
	for _, u := range users {
		lastSeen, version, quiz := "", "", ""
		if u.LastSeenAt != nil {
			lastSeen = u.LastSeenAt.UTC().Format(time.RFC3339)
		}
		if u.ConsentVersion != nil {
			version = *u.ConsentVersion
		}
		if u.QuizAvgPct != nil {
			quiz = strconv.FormatFloat(*u.QuizAvgPct, 'f', 1, 64)
		}
		record := []string{u.ID, u.Role, csvCell(u.GroupLabel), u.CreatedAt.UTC().Format(time.RFC3339),
			strconv.FormatBool(u.ConsentedAt != nil), version, lastSeen, itoa(u.ActiveDays), itoa(u.Events), itoa(u.Documents),
			strconv.FormatFloat(float64(u.ReadingMs)/60000, 'f', 1, 64), itoa(u.SuggestionsSeen), itoa(u.Decisions),
			itoa(u.Keep), itoa(u.Change), itoa(u.Reject), itoa(u.ChatQuestions), itoa(u.QuizAttempts), quiz}
		if withEmail {
			record = append(record[:1], append([]string{csvCell(u.Email)}, record[1:]...)...)
		}
		_ = out.Write(record)
	}
	out.Flush()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// textLength counts characters (not bytes) for length-only events.
func textLength(s string) int64 { return int64(utf8.RuneCountInString(s)) }

// roleAdminChecker adapts the users.role lookup to the quiz AdminChecker so
// every /api/admin route shares one source of truth.
type roleAdminChecker struct{ lookup middleware.RoleLookup }

func (c roleAdminChecker) IsAdmin(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	role, err := c.lookup(ctx, userID)
	if err != nil {
		return false, err
	}
	return role == middleware.RoleAdmin, nil
}

// RoleAdminChecker returns an AdminChecker backed by users.role.
func (h *Handler) RoleAdminChecker() AdminChecker { return roleAdminChecker{lookup: h.RoleLookup()} }

// trackQuizAttempt records quiz_started / quiz_submitted from the attempt row
// (server truth: score and duration come from the database, not the client).
func (h *Handler) trackQuizAttempt(ctx context.Context, userID, attemptID string, event analytics.Event) {
	if h.store == nil || h.analyticsOff || attemptID == "" {
		return
	}
	f, err := h.store.QuizAttemptFactsFor(ctx, userID, attemptID)
	if err != nil {
		log.Printf("analytics: quiz facts for %s: %v", event, err)
		return
	}
	props := analytics.Props{"quiz_id": f.QuizID, "attempt_id": attemptID, "phase": f.Kind, "question_count": f.QuestionCount}
	if event == analytics.QuizSubmitted {
		if !f.Submitted {
			return
		}
		props["duration_ms"] = f.DurationMs
		if f.ScorePct != nil {
			props["score_pct"] = *f.ScorePct
		}
	}
	h.Track(ctx, userID, event, props)
}

func (h *Handler) quizAttemptSubmitted(ctx context.Context, userID, attemptID string) bool {
	if h.store == nil || h.analyticsOff {
		return false
	}
	f, err := h.store.QuizAttemptFactsFor(ctx, userID, attemptID)
	return err == nil && f.Submitted
}
