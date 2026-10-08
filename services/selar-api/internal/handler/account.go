package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/notify"
	"github.com/selar-dev/selar-api/internal/settings"
	"github.com/selar-dev/selar-api/internal/store"
)

// SessionVersionLookup adapts the store for middleware.Auth so a password
// change (which increments users.session_version) ends other sessions.
func SessionVersionLookup(st *store.Store) middleware.SessionVersionLookup {
	return func(ctx context.Context, userID string) (int, error) {
		version, err := st.SessionVersion(ctx, userID)
		if errors.Is(err, store.ErrUserNotFound) {
			return 0, middleware.ErrSessionUserGone
		}
		return version, err
	}
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}

// validationMessage strips the internal sentinel prefix from a settings error.
func validationMessage(err error) string {
	return strings.ReplaceAll(err.Error(), settings.ErrInvalid.Error()+": ", "")
}

// decodeStrict decodes a JSON object body, rejecting unknown fields.
func decodeStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// ============================================================
// Settings (appearance, reader defaults, suggestion preferences)
// ============================================================

type settingsResponse struct {
	Values map[string]any `json:"values"`
	Locked []string       `json:"locked"`
}

// GetSettings returns the effective settings (defaults, then the learner's
// values, then cohort locks) and which keys are locked.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	stored, _, locks, err := h.store.GetUserSettings(r.Context(), userID)
	if errors.Is(err, store.ErrUserNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err != nil {
		log.Printf("get settings: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	values, locked := settings.Effective(stored, locks)
	writeJSON(w, http.StatusOK, settingsResponse{Values: values, Locked: locked})
}

// UpdateSettings validates and merges a partial settings update. Locked keys
// are refused with 409 so the learner sees why the change did not apply.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&patch); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	clean, err := settings.ValidatePatch(patch)
	if err != nil {
		badRequest(w, validationMessage(err))
		return
	}
	userID := middleware.GetUserID(r.Context())
	stored, _, locks, err := h.store.GetUserSettings(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load settings"})
		return
	}
	_, locked := settings.Effective(nil, locks)
	for _, key := range locked {
		if _, ok := clean[key]; ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("%s is locked for your study group", key)})
			return
		}
	}
	if patch, ok := clean["reader"].(map[string]any); ok {
		// Reader fields merge individually so a partial update keeps the rest.
		clean["reader"] = settings.MergeReader(stored["reader"], patch)
	}
	if err := h.store.MergeUserSettings(r.Context(), userID, clean); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save settings"})
		return
	}
	for k, v := range clean {
		stored[k] = v
	}
	values, lockedKeys := settings.Effective(stored, locks)
	writeJSON(w, http.StatusOK, settingsResponse{Values: values, Locked: lockedKeys})
}

// ============================================================
// Profile, email and password
// ============================================================

// UpdateProfile sets the display name (empty clears it).
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName *string `json:"display_name"`
	}
	if err := decodeStrict(r, &req); err != nil || req.DisplayName == nil {
		badRequest(w, "display_name is required")
		return
	}
	name, err := settings.NormalizeDisplayName(*req.DisplayName)
	if err != nil {
		badRequest(w, validationMessage(err))
		return
	}
	userID := middleware.GetUserID(r.Context())
	if err := h.store.UpdateDisplayName(r.Context(), userID, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save profile"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"display_name": name})
}

// verifyCurrentPassword returns the user's email and session version when
// password matches, writing the error response otherwise.
func (h *Handler) verifyCurrentPassword(w http.ResponseWriter, r *http.Request, password string) (string, bool) {
	userID := middleware.GetUserID(r.Context())
	email, hash, _, err := h.store.GetUserAuth(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return "", false
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "current password is incorrect"})
		return "", false
	}
	return email, true
}

// ChangeEmail changes the sign-in address after re-checking the password.
// There is no outbound email in SELAR, so the new address is not verified by
// a link; the password confirmation is the safeguard.
func (h *Handler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email           string `json:"email"`
		CurrentPassword string `json:"current_password"`
	}
	if err := decodeStrict(r, &req); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	parsed, parseErr := mail.ParseAddress(req.Email)
	if req.Email == "" || parseErr != nil || parsed.Address != req.Email || len(req.Email) > 254 {
		badRequest(w, "a valid email address is required")
		return
	}
	if req.CurrentPassword == "" {
		badRequest(w, "current_password is required")
		return
	}
	previous, ok := h.verifyCurrentPassword(w, r, req.CurrentPassword)
	if !ok {
		return
	}
	err := h.store.UpdateEmail(r.Context(), middleware.GetUserID(r.Context()), req.Email)
	if errors.Is(err, store.ErrEmailTaken) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "that email is already in use"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to change email"})
		return
	}
	// Optional security notice to the previous address (opt-in, dry-run by default).
	h.sendSecurityNotice(r, middleware.GetUserID(r.Context()), previous, notify.SecurityEmail, req.Email+"@"+time.Now().UTC().Format(time.RFC3339))
	writeJSON(w, http.StatusOK, map[string]string{"email": req.Email})
}

// ChangePassword verifies the current password, enforces the strength rules,
// stores the new hash and increments the session version. Every other
// session's token is then rejected by the auth middleware; this session gets
// a replacement token in the response.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeStrict(r, &req); err != nil || req.CurrentPassword == "" || req.NewPassword == "" {
		badRequest(w, "current_password and new_password are required")
		return
	}
	email, ok := h.verifyCurrentPassword(w, r, req.CurrentPassword)
	if !ok {
		return
	}
	if err := settings.CheckPasswordStrength(req.NewPassword, email); err != nil {
		badRequest(w, validationMessage(err))
		return
	}
	if req.NewPassword == req.CurrentPassword {
		badRequest(w, "the new password must differ from the current one")
		return
	}
	if h.auth == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}
	userID := middleware.GetUserID(r.Context())
	version, err := h.store.UpdatePassword(r.Context(), userID, string(hash))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to change password"})
		return
	}
	token, err := h.auth.GenerateSessionToken(userID, version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue a new session"})
		return
	}
	h.sendSecurityNotice(r, userID, email, notify.SecurityPassword, strconv.Itoa(version))
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "other_sessions_ended": true})
}
