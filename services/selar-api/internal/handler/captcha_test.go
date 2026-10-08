package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/handler"
	"github.com/selar-dev/selar-api/internal/recaptcha"
)

type fakeVerifier struct {
	enabled bool
	err     error
	token   string
	action  string
	calls   int
}

func (f *fakeVerifier) Enabled() bool { return f.enabled }
func (f *fakeVerifier) Check(_ context.Context, token, action string) error {
	f.calls++
	f.token, f.action = token, action
	return f.err
}

const validRegister = `{"email":"a@b.com","password":"eightchars","recaptcha_token":"tok-123"}`

// registerWithCaptcha runs Register with a nil store. Reaching persistence
// panics on the nil store, which the test reports as "passed the captcha".
func registerWithCaptcha(t *testing.T, v recaptcha.Verifier, body string) (code int, reachedStore bool, resp string) {
	t.Helper()
	h := handler.New(nil)
	h.SetCaptcha(v)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	func() {
		defer func() {
			if recover() != nil {
				reachedStore = true
			}
		}()
		h.Register(w, req)
	}()
	return w.Code, reachedStore, w.Body.String()
}

func TestRegisterCaptchaRejectsMissingOrInvalidToken(t *testing.T) {
	for _, err := range []error{recaptcha.ErrMissingToken, recaptcha.ErrRejected} {
		v := &fakeVerifier{enabled: true, err: err}
		code, reached, resp := registerWithCaptcha(t, v, validRegister)
		if code != http.StatusForbidden || reached {
			t.Fatalf("%v: code=%d reachedStore=%v", err, code, reached)
		}
		if !strings.Contains(resp, `"code":"recaptcha_failed"`) {
			t.Fatalf("%v: body %s", err, resp)
		}
		if v.token != "tok-123" || v.action != handler.RecaptchaActionRegister {
			t.Fatalf("verifier got token=%q action=%q", v.token, v.action)
		}
	}
}

func TestRegisterCaptchaPassesAndFailsOpenOnOutage(t *testing.T) {
	for _, err := range []error{nil, errors.Join(recaptcha.ErrUnavailable, errors.New("dial tcp: timeout"))} {
		v := &fakeVerifier{enabled: true, err: err}
		_, reached, _ := registerWithCaptcha(t, v, validRegister)
		if !reached {
			t.Fatalf("err=%v: registration should continue to account creation", err)
		}
	}
}

func TestRegisterCaptchaDisabledIsNotCalled(t *testing.T) {
	for _, v := range []recaptcha.Verifier{nil, &fakeVerifier{enabled: false, err: recaptcha.ErrRejected}} {
		_, reached, _ := registerWithCaptcha(t, v, `{"email":"a@b.com","password":"eightchars"}`)
		if !reached {
			t.Fatal("disabled captcha must not block registration")
		}
		if f, ok := v.(*fakeVerifier); ok && f.calls != 0 {
			t.Fatal("disabled verifier must not be called")
		}
	}
}

func TestRegisterValidatesInputBeforeCaptcha(t *testing.T) {
	v := &fakeVerifier{enabled: true}
	code, _, _ := registerWithCaptcha(t, v, `{"email":"bad","password":"x"}`)
	if code != http.StatusBadRequest || v.calls != 0 {
		t.Fatalf("code=%d calls=%d: invalid input must not spend an assessment", code, v.calls)
	}
}
