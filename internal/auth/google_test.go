package auth

import (
	"testing"
	"time"
)

func TestCheckGoogleClaims(t *testing.T) {
	now := time.Unix(1000, 0)
	ok := googleClaims{Sub: "s", Email: "a@b.c", EmailVerified: true, Iss: "https://accounts.google.com", Aud: "cid", Exp: 2000}
	tests := map[string]func(c *googleClaims){
		"wrong audience":   func(c *googleClaims) { c.Aud = "other" },
		"expired":          func(c *googleClaims) { c.Exp = 1000 },
		"wrong issuer":     func(c *googleClaims) { c.Iss = "evil.com" },
		"unverified email": func(c *googleClaims) { c.EmailVerified = false },
		"no subject":       func(c *googleClaims) { c.Sub = "" },
	}
	if err := checkGoogleClaims(&ok, "cid", now); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}
	for name, mutate := range tests {
		c := ok
		mutate(&c)
		if err := checkGoogleClaims(&c, "cid", now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVerifyRejectsMalformedAndForgedTokens(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "cid")
	for _, tok := range []string{"", "a.b", "a.b.c", "eyJhbGciOiJub25lIn0.e30."} {
		if _, err := verifyGoogleIDToken(t.Context(), tok, time.Now()); err == nil {
			t.Errorf("token %q accepted", tok)
		}
	}
}
