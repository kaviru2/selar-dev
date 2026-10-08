package handler

// notifications.go — optional email study notices (issue #114).
//
// Learners opt in per notice family in Settings (default off). The
// notification service (internal/notify) decides whether a message is really
// sent or only recorded as dry-run; with the default configuration nothing
// ever leaves the server. A scheduled caller (Modal, see
// services/selar-worker/modal_app.py) runs the quiz notices through
// POST /internal/notifications/run, authenticated with the worker secret.

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/notify"
	"github.com/selar-dev/selar-api/internal/store"
)

// SetNotifier installs the notification service and the shared secret the
// scheduler must present (WORKER_TRIGGER_SECRET; empty disables the route).
func (h *Handler) SetNotifier(n *notify.Notifier, runSecret string) {
	h.notifier = n
	h.notifyRunSecret = runSecret
}

// MountNotificationRoutes registers the authenticated Settings routes.
func (h *Handler) MountNotificationRoutes(r chi.Router) {
	r.Get("/users/me/notifications", h.GetNotificationSettings)
	r.Put("/users/me/notifications", h.UpdateNotificationSettings)
}

// MountNotificationAdmin registers admin-only audit routes (mount inside the
// /api/admin group, which checks the role).
func (h *Handler) MountNotificationAdmin(r chi.Router) {
	r.Get("/notifications/log", h.AdminNotificationLog)
}

// MountNotificationPublic registers the token and secret authenticated routes.
func (h *Handler) MountNotificationPublic(r chi.Router) {
	r.Get("/notifications/unsubscribe/{token}", h.UnsubscribePage)
	r.Post("/notifications/unsubscribe/{token}", h.Unsubscribe)
	r.Post("/internal/notifications/run", h.RunNotifications)
}

type notificationSettingsView struct {
	// Available is false unless the research team allowlisted this account.
	Available      bool `json:"available"`
	QuizEmails     bool `json:"quiz_emails"`
	SecurityEmails bool `json:"security_emails"`
	// Live is true only when a message to this account would really be
	// sent; otherwise notices are recorded as dry-run and nothing is sent.
	Live bool `json:"live"`
}

func (h *Handler) notificationView(r notify.Recipient) notificationSettingsView {
	v := notificationSettingsView{Available: h.notifier.Available(r.Email)}
	if v.Available {
		v.QuizEmails, v.SecurityEmails, v.Live = r.QuizEmails, r.SecurityEmails, h.notifier.LiveFor(r.Email)
	}
	return v
}

// GetNotificationSettings returns the learner's email opt-ins.
func (h *Handler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	rec, err := h.store.NotificationRecipient(r.Context(), middleware.GetUserID(r.Context()))
	if errors.Is(err, store.ErrUserNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load email settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.notificationView(rec))
}

// UpdateNotificationSettings sets {"quiz_emails": bool, "security_emails": bool}
// (either may be omitted). Opting in requires an allowlisted account.
func (h *Handler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuizEmails     *bool `json:"quiz_emails"`
		SecurityEmails *bool `json:"security_emails"`
	}
	if err := decodeStrict(r, &req); err != nil || (req.QuizEmails == nil && req.SecurityEmails == nil) {
		badRequest(w, "quiz_emails and/or security_emails (true or false) are required")
		return
	}
	userID := middleware.GetUserID(r.Context())
	if (req.QuizEmails != nil && *req.QuizEmails) || (req.SecurityEmails != nil && *req.SecurityEmails) {
		cur, err := h.store.NotificationRecipient(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update email settings"})
			return
		}
		if !h.notifier.Available(cur.Email) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "email notices are not switched on for this study"})
			return
		}
	}
	rec, err := h.store.SetNotificationPrefs(r.Context(), userID, req.QuizEmails, req.SecurityEmails)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update email settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.notificationView(rec))
}

const unsubscribeHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex"><title>SELAR emails</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#222}button{font:inherit;padding:.5rem 1rem}</style>
</head><body><h1>SELAR emails</h1>%s</body></html>`

func writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, unsubscribeHTML, body)
}

func (h *Handler) unsubscribeToken(r *http.Request) (string, int, bool) {
	if h.notifier == nil || h.calendarCfg().Signer == nil {
		return "", 0, false
	}
	userID, version, err := h.calendarCfg().Signer.Verify(linktoken.Unsubscribe, chi.URLParam(r, "token"))
	return userID, version, err == nil
}

// UnsubscribePage shows a confirmation button. GET never changes anything,
// because mail scanners open links automatically.
func (h *Handler) UnsubscribePage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.unsubscribeToken(r); !ok {
		writeHTML(w, http.StatusNotFound, "<p>This link is not valid.</p>")
		return
	}
	writeHTML(w, http.StatusOK, `<p>Stop all SELAR emails (quiz and account notices) for your account?</p>
<form method="post"><button type="submit">Stop SELAR emails</button></form>
<p>You can turn them back on later in SELAR Settings.</p>`)
}

// Unsubscribe turns every email notice off. It also serves RFC 8058
// one-click POSTs from mail clients. Repeating it is harmless.
func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, version, ok := h.unsubscribeToken(r)
	if !ok {
		writeHTML(w, http.StatusNotFound, "<p>This link is not valid.</p>")
		return
	}
	if _, err := h.store.UnsubscribeAll(r.Context(), userID, version); err != nil {
		log.Printf("unsubscribe failed: %v", err)
		writeHTML(w, http.StatusInternalServerError, "<p>Something went wrong. Please try again, or turn emails off in SELAR Settings.</p>")
		return
	}
	writeHTML(w, http.StatusOK, "<p>Done. SELAR will not email you. You can turn emails back on in SELAR Settings.</p>")
}

// RunNotifications runs one round of quiz notices. Called by the scheduler
// with the worker secret; reports counts and the transport mode only.
func (h *Handler) RunNotifications(w http.ResponseWriter, r *http.Request) {
	provided := r.Header.Get("X-Selar-Worker-Secret")
	if h.notifyRunSecret == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(h.notifyRunSecret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.notifier == nil {
		writeJSON(w, http.StatusOK, notify.RunReport{Mode: "off", Transport: notify.ModeDryRun, Outcomes: map[string]int{}})
		return
	}
	rep, err := h.notifier.RunQuizNotices(r.Context())
	if err != nil {
		log.Printf("notification run failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "notification run failed"})
		return
	}
	log.Printf("notification run: mode=%s transport=%s recipients=%d outcomes=%v", rep.Mode, rep.Transport, rep.Recipients, rep.Outcomes)
	writeJSON(w, http.StatusOK, rep)
}

// AdminNotificationLog lists recent audit rows (no addresses).
func (h *Handler) AdminNotificationLog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.store.ListNotificationLog(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load notification log"})
		return
	}
	mode := "off"
	transport := notify.ModeDryRun
	if h.notifier != nil {
		mode, transport = h.notifier.Mode(), h.notifier.TransportMode()
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": mode, "transport": transport, "entries": rows})
}

// sendSecurityNotice is called after a successful password or email change.
// It never affects the response.
func (h *Handler) sendSecurityNotice(r *http.Request, userID, email string, kind notify.Kind, eventKey string) {
	if h.notifier == nil {
		return
	}
	h.notifier.Security(r.Context(), userID, email, kind, eventKey)
}
