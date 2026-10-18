package services

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FlightStrips/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Exercise the real JWKS client and JWT parser rather than the replay-mode bypass.
func TestAuthenticationServiceValidatesJWKSClaims(t *testing.T) {
	require.False(t, config.IsTestMode(), "this regression test requires real authentication")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwks := map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "kid": "auth-test", "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes()),
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwks); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	service, err := NewAuthenticationService("RS256", server.URL, "flightstrips")
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		change func(jwt.MapClaims)
		method jwt.SigningMethod
		valid  bool
	}{
		{name: "valid", valid: true},
		{name: "wrong audience", change: func(c jwt.MapClaims) { c["aud"] = "another-service" }},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{name: "missing CID", change: func(c jwt.MapClaims) { delete(c, "vatsim/cid") }},
		{name: "missing rating", change: func(c jwt.MapClaims) { delete(c, "vatsim/rating") }},
		{name: "unapproved algorithm", method: jwt.SigningMethodRS512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{
				"aud": "flightstrips", "exp": time.Now().Add(time.Hour).Unix(),
				"vatsim/cid": "1234567", "vatsim/rating": 5,
			}
			if tc.change != nil {
				tc.change(claims)
			}
			method := tc.method
			if method == nil {
				method = jwt.SigningMethodRS256
			}
			token := jwt.NewWithClaims(method, claims)
			token.Header["kid"] = "auth-test"
			signed, err := token.SignedString(privateKey)
			require.NoError(t, err)
			user, err := service.Validate(signed)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "1234567", user.GetCid())
			require.Equal(t, 5, user.GetRating())
			require.True(t, user.IsValid())
		})
	}
}
