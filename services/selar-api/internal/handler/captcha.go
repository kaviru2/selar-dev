package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/selar-dev/selar-api/internal/recaptcha"
)

// RecaptchaActionRegister is the reCAPTCHA action the console uses for every
// account-creating request (password registration and Google sign-up).
const RecaptchaActionRegister = "register"

// SetCaptcha configures the reCAPTCHA verifier for account creation. A nil or
// disabled verifier turns the check off (local development, tests).
func (h *Handler) SetCaptcha(v recaptcha.Verifier) {
	h.captcha = v
}

// CaptchaEnabled reports whether account creation requires a reCAPTCHA token.
func (h *Handler) CaptchaEnabled() bool {
	return h.captcha != nil && h.captcha.Enabled()
}

// requireHuman runs the reCAPTCHA check for an account-creating request. It
// writes the error response and returns false when the request must stop.
//
// Fail-open is limited to ErrUnavailable (Google unreachable, timeout, 429 or
// 5xx), logged as a warning, so an outage cannot lock participants out. A
// missing, invalid, wrong-action or low-score token is refused. Use it from
// any path that creates a user (e.g. Google sign-up) with the same action.
func (h *Handler) requireHuman(w http.ResponseWriter, r *http.Request, token, action string) bool {
	if !h.CaptchaEnabled() {
		return true
	}
	err := h.captcha.Check(r.Context(), token, action)
	switch {
	case err == nil:
		return true
	case errors.Is(err, recaptcha.ErrUnavailable):
		log.Printf("WARNING recaptcha unavailable, allowing %s without verification: %v", action, err)
		return true
	default:
		log.Printf("recaptcha refused %s: %v", action, err)
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "We couldn't confirm this sign-up came from a person. Reload the page and try again.",
			"code":  "recaptcha_failed",
		})
		return false
	}
}
