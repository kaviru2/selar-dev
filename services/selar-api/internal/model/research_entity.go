package model

import (
	"strings"
	"unicode"
)

// Entity alias normalization for research assertions (issue #9).
//
// Symbol variants of one name collapse to one key: "τ²-bench", "𝜏2-bench",
// "tau2-bench", "Tau^2 Benchmark" -> "tau2bench". Distinct names stay
// distinct: "TAU Benchmark"/"τ-bench" -> "taubench", which is NOT tau2bench,
// because they may be different benchmarks and merging them would invent a
// claim. Version qualifiers ("modified", "original", "re-implemented" ...)
// are split off so "modified SagaLLM" and "original SagaLLM" share the key
// "sagallm" but never the same assertion identity.

var greekNames = map[rune]string{
	'α': "alpha", 'β': "beta", 'γ': "gamma", 'δ': "delta", 'ε': "epsilon",
	'θ': "theta", 'λ': "lambda", 'μ': "mu", 'π': "pi", 'σ': "sigma",
	'τ': "tau", 'φ': "phi", 'χ': "chi", 'ψ': "psi", 'ω': "omega",
}

var superscripts = map[rune]rune{
	'⁰': '0', '¹': '1', '²': '2', '³': '3', '⁴': '4',
	'⁵': '5', '⁶': '6', '⁷': '7', '⁸': '8', '⁹': '9',
}

// Qualifiers that change which version of an entity a claim is about.
var entityQualifiers = map[string]string{
	"modified": "modified", "reimplemented": "reimplemented", "re-implemented": "reimplemented",
	"reproduced": "reimplemented", "adapted": "modified", "original": "original",
	"official": "original", "baseline": "baseline",
}

// foldRune maps mathematical alphanumerics (e.g. 𝜏 U+1D70F) to their base letters.
func foldRune(r rune) rune {
	switch {
	case r >= 0x1D6A8 && r <= 0x1D7C9: // mathematical Greek, 5 styles of 58
		offset := (r - 0x1D6A8) % 58
		if offset < 25 { // capital Alpha..Omega (with theta symbol gap)
			return unicode.ToLower(0x0391 + offset)
		}
		if offset >= 26 && offset < 51 {
			return 0x03B1 + (offset - 26)
		}
		return r
	case r >= 0x1D400 && r <= 0x1D6A3: // mathematical Latin, 13 styles of 52
		offset := (r - 0x1D400) % 52
		if offset < 26 {
			return 'a' + offset
		}
		return 'a' + offset - 26
	}
	if d, ok := superscripts[r]; ok {
		return d
	}
	return unicode.ToLower(r)
}

// NormalizeEntity returns a stable comparison key and any version qualifier.
func NormalizeEntity(name string) (key, qualifier string) {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return unicode.IsSpace(r) })
	var kept []string
	var qualifiers []string
	for _, word := range words {
		trimmed := strings.Trim(word, "()[]{},.;:\"'“”‘’")
		if q, ok := entityQualifiers[trimmed]; ok && len(words) > 1 {
			qualifiers = append(qualifiers, q)
			continue
		}
		kept = append(kept, word)
	}
	var b strings.Builder
	for _, r := range strings.Join(kept, " ") {
		r = foldRune(r)
		if g, ok := greekNames[r]; ok {
			b.WriteString(g)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	key = b.String()
	key = strings.TrimSuffix(key, "benchmark") + map[bool]string{true: "bench", false: ""}[strings.HasSuffix(key, "benchmark")]
	return key, strings.Join(qualifiers, " ")
}
