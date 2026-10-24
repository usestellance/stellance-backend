package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const googleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

var (
	errInvalidGoogleToken = errors.New("invalid google token")
	googleHTTP            = &http.Client{Timeout: 5 * time.Second}
	googleKeys            = &keyCache{}
)

type keyCache struct {
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (c *keyCache) get(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok && time.Since(c.fetched) < time.Hour {
		return k, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleCertsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := googleHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google certs: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	c.keys, c.fetched = keys, time.Now()
	k, ok := keys[kid]
	if !ok {
		return nil, errInvalidGoogleToken
	}
	return k, nil
}

type googleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Exp           int64  `json:"exp"`
}

// verifyGoogleIDToken checks signature (RS256 against Google's keys), issuer, audience and expiry.
func verifyGoogleIDToken(ctx context.Context, token string, now time.Time) (*googleClaims, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID is not set")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidGoogleToken
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodePart(parts[0], &header); err != nil || header.Alg != "RS256" {
		return nil, errInvalidGoogleToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errInvalidGoogleToken
	}
	key, err := googleKeys.get(ctx, header.Kid)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return nil, errInvalidGoogleToken
	}
	var c googleClaims
	if err := decodePart(parts[1], &c); err != nil {
		return nil, errInvalidGoogleToken
	}
	return &c, checkGoogleClaims(&c, clientID, now)
}

func checkGoogleClaims(c *googleClaims, clientID string, now time.Time) error {
	switch {
	case c.Iss != "accounts.google.com" && c.Iss != "https://accounts.google.com":
		return errInvalidGoogleToken
	case c.Aud != clientID:
		return errInvalidGoogleToken
	case c.Exp <= now.Unix():
		return errInvalidGoogleToken
	case c.Sub == "" || c.Email == "" || !c.EmailVerified:
		return errInvalidGoogleToken
	}
	return nil
}

func decodePart(p string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
