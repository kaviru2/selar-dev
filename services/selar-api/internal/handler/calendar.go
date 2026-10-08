package handler

// calendar.go — optional quiz-window calendar feed (issue #113).
//
// A learner who opts in gets a private feed URL that Google Calendar (or any
// iCalendar client) can subscribe to with "From URL". The feed is computed on
// every fetch from the server-enforced quiz windows, so window changes reach
// subscribed calendars on the client's next refresh. Off by default: the
// server needs CALENDAR_FEED_ENABLED=true and the learner's address in
// CALENDAR_FEED_ALLOWLIST before the Settings control can be switched on or a
// feed shows any event.

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/calendar"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
)

// CalendarConfig configures the feed.
type CalendarConfig struct {
	Gate allowlist.Gate
	// Signer signs feed URLs (derived from LINK_TOKEN_SECRET or JWT_SECRET).
	Signer *linktoken.Signer
	// PublicAPIURL is the externally reachable API origin used in feed URLs
	// (PUBLIC_API_URL). Empty = derived from the request.
	PublicAPIURL string
	// ConsoleURL is the console origin used in event links (PUBLIC_CONSOLE_URL).
	ConsoleURL string
	// Now is overridable in tests.
	Now func() time.Time
}

// SetCalendarConfig installs the calendar feed configuration.
func (h *Handler) SetCalendarConfig(c CalendarConfig) {
	if c.Now == nil {
		c.Now = time.Now
	}
	h.calendar = &c
}

func (h *Handler) calendarCfg() *CalendarConfig {
	if h.calendar == nil {
		return &CalendarConfig{Now: time.Now}
	}
	return h.calendar
}

// MountCalendarRoutes registers the authenticated Settings routes.
func (h *Handler) MountCalendarRoutes(r chi.Router) {
	r.Get("/users/me/calendar", h.GetCalendarSettings)
	r.Put("/users/me/calendar", h.UpdateCalendarSettings)
	r.Post("/users/me/calendar/reset", h.ResetCalendarFeed)
}

type calendarEventView struct {
	Summary   string    `json:"summary"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	GoogleURL string    `json:"google_url"`
}

type calendarSettingsView struct {
	// Available is false when the research team has not switched the feature
	// on for this account; the console then explains that instead of
	// offering the toggle.
	Available          bool                `json:"available"`
	Enabled            bool                `json:"enabled"`
	FeedURL            string              `json:"feed_url,omitempty"`
	GoogleSubscribeURL string              `json:"google_subscribe_url,omitempty"`
	Events             []calendarEventView `json:"events"`
}

func (h *Handler) apiOrigin(r *http.Request) string {
	if base := strings.TrimRight(h.calendarCfg().PublicAPIURL, "/"); base != "" {
		return base
	}
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") &&
		(strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

func (h *Handler) calendarView(r *http.Request, userID string, st store.CalendarFeedState) (calendarSettingsView, error) {
	cfg := h.calendarCfg()
	v := calendarSettingsView{Available: cfg.Gate.Allows(st.Email) && cfg.Signer != nil, Events: []calendarEventView{}}
	if !v.Available {
		return v, nil
	}
	v.Enabled = st.Enabled
	if !st.Enabled {
		return v, nil
	}
	token, err := cfg.Signer.Issue(linktoken.Calendar, userID, st.Version)
	if err != nil {
		return v, err
	}
	v.FeedURL = h.apiOrigin(r) + "/calendar/" + token + ".ics"
	v.GoogleSubscribeURL = calendar.GoogleSubscribeURL(v.FeedURL)
	windows, err := h.store.CalendarWindows(r.Context(), userID, cfg.Now())
	if err != nil {
		return v, err
	}
	for _, w := range windows {
		e := calendar.ToEvent(w, userID, cfg.ConsoleURL)
		v.Events = append(v.Events, calendarEventView{Summary: e.Summary, Start: e.Start, End: e.End, GoogleURL: calendar.GoogleTemplateURL(e)})
	}
	return v, nil
}

func (h *Handler) writeCalendarView(w http.ResponseWriter, r *http.Request, st store.CalendarFeedState) {
	v, err := h.calendarView(r, middleware.GetUserID(r.Context()), st)
	if err != nil {
		log.Printf("calendar settings: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load calendar settings"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, v)
}

// GetCalendarSettings returns whether the feed is available and, when on,
// the private feed URL and the upcoming windows with per-window links.
func (h *Handler) GetCalendarSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.GetCalendarFeedState(r.Context(), middleware.GetUserID(r.Context()))
	if errors.Is(err, store.ErrUserNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load calendar settings"})
		return
	}
	h.writeCalendarView(w, r, st)
}

// UpdateCalendarSettings turns the feed on or off ({"enabled": bool}).
// Turning it off revokes the current URL.
func (h *Handler) UpdateCalendarSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil || req.Enabled == nil {
		badRequest(w, "enabled (true or false) is required")
		return
	}
	userID := middleware.GetUserID(r.Context())
	if *req.Enabled {
		st, err := h.store.GetCalendarFeedState(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update calendar settings"})
			return
		}
		if !h.calendarCfg().Gate.Allows(st.Email) || h.calendarCfg().Signer == nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "calendar reminders are not switched on for this study"})
			return
		}
	}
	st, err := h.store.SetCalendarFeed(r.Context(), userID, *req.Enabled, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update calendar settings"})
		return
	}
	h.writeCalendarView(w, r, st)
}

// ResetCalendarFeed issues a new feed URL; the old one stops showing events.
func (h *Handler) ResetCalendarFeed(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	cur, err := h.store.GetCalendarFeedState(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset calendar link"})
		return
	}
	st, err := h.store.SetCalendarFeed(r.Context(), userID, cur.Enabled, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset calendar link"})
		return
	}
	h.writeCalendarView(w, r, st)
}

// ServeCalendarFeed is the public, token-authenticated feed:
// GET /calendar/{token}.ics. A forged token gets 404. A genuine token that
// was revoked, belongs to a deleted account, or whose account is no longer
// allowed gets a valid but empty calendar so subscribed clients remove the
// old events instead of keeping them forever.
func (h *Handler) ServeCalendarFeed(w http.ResponseWriter, r *http.Request) {
	cfg := h.calendarCfg()
	name := chi.URLParam(r, "token")
	token, ok := strings.CutSuffix(name, ".ics")
	if !ok || cfg.Signer == nil {
		http.NotFound(w, r)
		return
	}
	userID, version, err := cfg.Signer.Verify(linktoken.Calendar, token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	now := cfg.Now()
	events := []calendar.Event{}
	st, err := h.store.GetCalendarFeedState(r.Context(), userID)
	switch {
	case errors.Is(err, store.ErrUserNotFound):
	case err != nil:
		log.Printf("calendar feed: state lookup failed")
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		return
	case st.Enabled && st.Version == version && cfg.Gate.Allows(st.Email):
		windows, err := h.store.CalendarWindows(r.Context(), userID, now)
		if err != nil {
			log.Printf("calendar feed: windows failed: %v", err)
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		for _, win := range windows {
			events = append(events, calendar.ToEvent(win, userID, cfg.ConsoleURL))
		}
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="selar-quiz-windows.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(calendar.Feed(events, now)))
}
