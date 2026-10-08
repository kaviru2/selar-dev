package allowlist

import "testing"

func TestParseAndAllows(t *testing.T) {
	l := Parse(" Ana@Example.com, @study.test;bad, @, x@ ,\n")
	for email, want := range map[string]bool{
		"ana@example.com":    true,
		"ANA@EXAMPLE.COM":    true,
		"bob@example.com":    false,
		"zoe@study.test":     true,
		"zoe@sub.study.test": false,
		"":                   false,
		"nobody":             false,
	} {
		if got := l.Allows(email); got != want {
			t.Errorf("Allows(%q) = %v, want %v", email, got, want)
		}
	}
	if l.Size() != 2 || l.Everyone() {
		t.Fatalf("size=%d everyone=%v", l.Size(), l.Everyone())
	}
}

func TestZeroAndStar(t *testing.T) {
	if (List{}).Allows("a@b.c") || Parse("").Allows("a@b.c") {
		t.Fatal("empty list must match nothing")
	}
	if !Parse("*").Allows("a@b.c") || Parse("*").Allows("not-an-email") {
		t.Fatal("* matches every valid address only")
	}
}

func TestGateNeedsBothFlagAndList(t *testing.T) {
	cases := []struct {
		flag, list string
		want       bool
	}{
		{"", "*", false},
		{"false", "a@b.c", false},
		{"ture", "a@b.c", false},
		{"true", "", false},
		{"true", "other@b.c", false},
		{"true", "a@b.c", true},
		{"TRUE", "@b.c", true},
	}
	for _, c := range cases {
		if got := FromEnv(c.flag, c.list).Allows("a@b.c"); got != c.want {
			t.Errorf("flag=%q list=%q: %v want %v", c.flag, c.list, got, c.want)
		}
	}
	if FromEnv("", "").Describe() != "off" {
		t.Fatal("describe")
	}
}
