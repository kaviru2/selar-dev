// Package recaptcha verifies Google reCAPTCHA Enterprise (score-based) tokens
// for account creation.
//
// It is disabled unless RECAPTCHA_PROJECT_ID, RECAPTCHA_SITE_KEY and
// RECAPTCHA_API_KEY are all set, so local development and tests never call
// Google. When enabled, Check returns:
//
//   - nil              the token is valid, for the expected action, and scores
//     at or above the threshold;
//   - ErrMissingToken  no token was sent;
//   - ErrRejected      Google answered and the token is invalid, for another
//     action or site key, or scores below the threshold;
//   - ErrUnavailable   Google could not be reached (network error, timeout,
//     429 or 5xx). Callers fail OPEN on this one, with a logged warning, so an
//     outage cannot lock study participants out.
//
// Assessments use the REST createAssessment endpoint with an API key that is
// restricted to the reCAPTCHA Enterprise API. Free tier: 10,000
// assessments/month per organisation.
package recaptcha

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultMinScore is deliberately conservative: only traffic Google scores as
// very likely automated (0.0–0.2) is blocked, so real participants on unusual
// networks are not turned away.
const DefaultMinScore = 0.3

// DefaultTimeout bounds the call to Google; past it the request fails open.
const DefaultTimeout = 3 * time.Second

const defaultEndpoint = "https://recaptchaenterprise.googleapis.com/v1"

var (
	ErrMissingToken = errors.New("recaptcha: token missing")
	ErrRejected     = errors.New("recaptcha: token rejected")
	ErrUnavailable  = errors.New("recaptcha: verification service unavailable")
)

// Verifier is what the HTTP handlers depend on, so tests can use a fake.
type Verifier interface {
	Enabled() bool
	Check(ctx context.Context, token, action string) error
}

// Client calls the reCAPTCHA Enterprise REST API.
type Client struct {
	ProjectID string
	SiteKey   string
	APIKey    string
	MinScore  float64
	Endpoint  string
	HTTP      *http.Client
}

// FromEnv builds a Client from RECAPTCHA_* variables. It returns a disabled
// client when any required variable is missing.
func FromEnv(getenv func(string) string) *Client {
	c := &Client{
		ProjectID: strings.TrimSpace(getenv("RECAPTCHA_PROJECT_ID")),
		SiteKey:   strings.TrimSpace(getenv("RECAPTCHA_SITE_KEY")),
		APIKey:    strings.TrimSpace(getenv("RECAPTCHA_API_KEY")),
		MinScore:  DefaultMinScore,
		Endpoint:  defaultEndpoint,
		HTTP:      &http.Client{Timeout: DefaultTimeout},
	}
	if raw := strings.TrimSpace(getenv("RECAPTCHA_MIN_SCORE")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v >= 0 && v <= 1 {
			c.MinScore = v
		}
	}
	return c
}

// Enabled reports whether verification is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.ProjectID != "" && c.SiteKey != "" && c.APIKey != ""
}

type assessmentRequest struct {
	Event struct {
		Token          string `json:"token"`
		SiteKey        string `json:"siteKey"`
		ExpectedAction string `json:"expectedAction"`
	} `json:"event"`
}

type assessmentResponse struct {
	TokenProperties struct {
		Valid         bool   `json:"valid"`
		InvalidReason string `json:"invalidReason"`
		Action        string `json:"action"`
	} `json:"tokenProperties"`
	RiskAnalysis struct {
		Score   float64  `json:"score"`
		Reasons []string `json:"reasons"`
	} `json:"riskAnalysis"`
	Event struct {
		SiteKey string `json:"siteKey"`
	} `json:"event"`
}

// Check verifies token for action. See the package doc for the error contract.
// A disabled client accepts everything.
func (c *Client) Check(ctx context.Context, token, action string) error {
	if !c.Enabled() {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrMissingToken
	}

	var body assessmentRequest
	body.Event.Token = token
	body.Event.SiteKey = c.SiteKey
	body.Event.ExpectedAction = action
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	endpoint := strings.TrimRight(c.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	u := fmt.Sprintf("%s/projects/%s/assessments", endpoint, url.PathEscape(c.ProjectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The API key goes in a header, not the URL, so it never lands in logs.
	req.Header.Set("X-Goog-Api-Key", c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, redact(err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		// 400/403: misconfiguration (bad key, wrong project) or a request
		// Google refuses. Not an outage, so it fails closed.
		return fmt.Errorf("%w: assessment HTTP %d", ErrRejected, resp.StatusCode)
	}

	var out assessmentResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("%w: unreadable assessment: %v", ErrUnavailable, err)
	}
	if !out.TokenProperties.Valid {
		return fmt.Errorf("%w: invalid token (%s)", ErrRejected, out.TokenProperties.InvalidReason)
	}
	if out.TokenProperties.Action != action {
		return fmt.Errorf("%w: action %q, want %q", ErrRejected, out.TokenProperties.Action, action)
	}
	if out.RiskAnalysis.Score < c.MinScore {
		return fmt.Errorf("%w: score %.1f below %.1f", ErrRejected, out.RiskAnalysis.Score, c.MinScore)
	}
	return nil
}

// redact strips the query string from URL errors (defence in depth: the key
// is sent as a header, but never risk echoing a URL with secrets).
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if i := strings.IndexByte(ue.URL, '?'); i >= 0 {
			ue.URL = ue.URL[:i]
		}
		return ue
	}
	return err
}
