package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/googleauth"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
)

// GoogleExchanger redeems an authorization code for a verified identity.
// *googleauth.Client implements it; tests substitute a fake.
type GoogleExchanger interface {
	Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (*googleauth.Identity, error)
}

// SetGoogle enables Sign in with Google. A nil exchanger disables it.
func (h *Handler) SetGoogle(g GoogleExchanger) {
	h.google = g
}

// GoogleEnabled reports whether Sign in with Google is configured.
func (h *Handler) GoogleEnabled() bool { return h.google != nil }

// googleAction is what a verified Google identity resolves to.
type googleAction int

const (
	googleSignIn   googleAction = iota // an account is already linked to this Google subject
	googleLink                         // link the verified email's existing account, then sign in
	googleCreate                       // no account: create one through the registration path
	googleConflict                     // the email's account is linked to a different Google account
)

// resolveGoogleAccount applies the linking rules. The identity has already
// passed ID-token verification, which refuses unverified emails, so an
// existing account is linked only when Google says the address is verified
// AND it equals the account's email.
func resolveGoogleAccount(id *googleauth.Identity, bySub *model.User, byEmail *store.GoogleAccount) googleAction {
	if bySub != nil {
		return googleSignIn
	}
	if byEmail == nil || !strings.EqualFold(byEmail.User.Email, id.Email) {
		return googleCreate
	}
	if byEmail.GoogleSub != "" && byEmail.GoogleSub != id.Subject {
		return googleConflict
	}
	return googleLink
}

type googleAuthRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	Nonce        string `json:"nonce"`
	RedirectURI  string `json:"redirect_uri"`
}

// GoogleSignIn completes Sign in with Google. The console's callback route
// has already checked the OAuth state; this exchanges the code (with the
// PKCE verifier) at Google, verifies the ID token and issues the same
// session token as password login.
func (h *Handler) GoogleSignIn(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Google sign-in is not enabled"})
		return
	}
	if h.auth == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	var req googleAuthRequest
	if err := decodeStrict(r, &req); err != nil || req.Code == "" || req.CodeVerifier == "" || req.Nonce == "" || req.RedirectURI == "" {
		badRequest(w, "code, code_verifier, nonce and redirect_uri are required")
		return
	}
	if len(req.CodeVerifier) < 43 || len(req.CodeVerifier) > 128 {
		badRequest(w, "invalid code_verifier")
		return
	}
	id, err := h.google.Exchange(r.Context(), req.Code, req.CodeVerifier, req.RedirectURI, req.Nonce)
	if errors.Is(err, googleauth.ErrEmailNotVerified) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Your Google account's email address is not verified, so it can't be used to sign in."})
		return
	}
	if err != nil {
		log.Printf("google sign-in: %v", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Google sign-in failed. Please try again."})
		return
	}

	ctx := r.Context()
	bySub, err := h.store.GetUserByGoogleSub(ctx, id.Subject)
	if err != nil && !errors.Is(err, store.ErrUserNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Google sign-in failed"})
		return
	}
	var byEmail *store.GoogleAccount
	if bySub == nil {
		byEmail, err = h.store.GetGoogleAccountByEmail(ctx, id.Email)
		if err != nil && !errors.Is(err, store.ErrUserNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Google sign-in failed"})
			return
		}
	}

	var user *model.User
	created := false
	switch resolveGoogleAccount(id, bySub, byEmail) {
	case googleConflict:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This email's SELAR account is linked to a different Google account. Sign in with your password instead."})
		return
	case googleSignIn:
		user = bySub
	case googleLink:
		user = byEmail.User
		if byEmail.GoogleSub == "" {
			if _, err := h.store.LinkGoogleSub(ctx, user.ID, id.Subject); err != nil {
				log.Printf("google sign-in: link %s: %v", user.ID, err)
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Google sign-in failed. Please try again."})
				return
			}
		}
	case googleCreate:
		user, err = h.createGoogleUser(r, id)
		if err != nil {
			log.Printf("google sign-in: create: %v", err)
			writeJSON(w, http.StatusConflict, map[string]string{"error": "We couldn't create an account for that Google address."})
			return
		}
		created = true
	}

	_, _, sessionVersion, err := h.store.GetUserAuth(ctx, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	token, err := h.auth.GenerateSessionToken(user.ID, sessionVersion)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	if !created {
		h.applyAdminBootstrap(r, user.Email, user)
	}
	h.Track(ctx, user.ID, analytics.SignedIn, analytics.Props{"method": "google"})

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"token": token, "user": user, "created": created})
}

// createGoogleUser creates an account the same way Register does (same
// cohort assignment, ADMIN_EMAILS role bootstrap, and research consent left
// unasked so the console's post-login prompt runs), linked to the Google
// subject. The password hash is of 32 random bytes nobody knows, so password
// sign-in is impossible for the account.
func (h *Handler) createGoogleUser(r *http.Request, id *googleauth.Identity) (*model.User, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	// bcrypt accepts at most 72 bytes; 64 hex characters fit.
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(secret)), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return h.store.CreateUserWith(r.Context(), h.newRegistration(id.Email, string(hash), nil, id.Subject))
}

// newRegistration builds the account a registration creates. Register and
// Sign in with Google share it so both assign the study cohort, role and
// consent state identically.
func (h *Handler) newRegistration(email, passwordHash string, researchConsent *bool, googleSub string) store.NewUser {
	// Random cohort assignment via block randomization
	cohort := model.CohortTreatmentHITL // TODO: implement proper randomization

	newUser := store.NewUser{Email: email, PasswordHash: passwordHash, Cohort: cohort, Role: h.roleFor(email), GoogleSub: googleSub}
	if researchConsent != nil {
		newUser.Consent = researchConsent
		newUser.ConsentVersion = analytics.ConsentVersion
	}
	return newUser
}
