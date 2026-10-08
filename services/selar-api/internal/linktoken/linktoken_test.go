package linktoken

import (
	"strings"
	"testing"
)

const user = "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11"

func TestRoundTripAndPurposeSeparation(t *testing.T) {
	s := New("server-secret")
	tok, err := s.Issue(Calendar, user, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(tok, "+/=") || len(tok) != 48 {
		t.Fatalf("token not URL-safe or unexpected length: %q", tok)
	}
	id, v, err := s.Verify(Calendar, tok)
	if err != nil || id != user || v != 3 {
		t.Fatalf("verify = %q %d %v", id, v, err)
	}
	if _, _, err := s.Verify(Unsubscribe, tok); err == nil {
		t.Fatal("a calendar token must not verify as an unsubscribe token")
	}
	if _, _, err := New("other-secret").Verify(Calendar, tok); err == nil {
		t.Fatal("a different secret must reject the token")
	}
}

func TestTamperingIsRejected(t *testing.T) {
	s := New("server-secret")
	tok, _ := s.Issue(Calendar, user, 0)
	for _, bad := range []string{"", "x", tok[:47], tok + "A", strings.Repeat("A", 48), "../../etc/passwd"} {
		if _, _, err := s.Verify(Calendar, bad); err == nil {
			t.Errorf("Verify(%q) must fail", bad)
		}
	}
	b := []byte(tok)
	b[5] ^= 1
	if _, _, err := s.Verify(Calendar, string(b)); err == nil {
		t.Fatal("flipped bit must fail")
	}
}

func TestEmptySecretRejects(t *testing.T) {
	if _, err := New("").Issue(Calendar, user, 0); err == nil {
		t.Fatal("issue without secret")
	}
	if _, _, err := New("").Verify(Calendar, "abc"); err == nil {
		t.Fatal("verify without secret")
	}
	if _, err := New("k").Issue(Calendar, "not-a-uuid", 0); err == nil {
		t.Fatal("bad user id")
	}
}
