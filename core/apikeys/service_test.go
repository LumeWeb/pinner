package apikeys

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// newTestToken mints a JWT with the given subject and jti claims.
func newTestToken(t *testing.T, subject, id, audience string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Subject:  subject,
		ID:       id,
		Audience: jwt.ClaimStrings{audience},
	})

	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// TestGetCurrentAPIKeyUUIDReturnsJTIPinsRegression guards against returning the
// subject claim instead of the jti claim: for api-purpose tokens the Subject
// holds the user ID, while the API key UUID lives in the jti (ID) claim.
func TestGetCurrentAPIKeyUUIDReturnsJTI(t *testing.T) {
	const (
		userID = "user-42"
		keyID  = "key-uuid-abc"
	)

	svc := New(nil, newTestToken(t, userID, keyID, "api"))

	got := svc.GetCurrentAPIKeyUUID()
	if got != keyID {
		t.Fatalf("GetCurrentAPIKeyUUID() = %q, want %q (jti claim)", got, keyID)
	}
	if got == userID {
		t.Fatalf("GetCurrentAPIKeyUUID() returned the subject (user ID) %q; must return the jti (API key UUID)", got)
	}
}

func TestGetCurrentAPIKeyUUIDNonAPITokenReturnsEmpty(t *testing.T) {
	svc := New(nil, newTestToken(t, "user-42", "key-uuid-abc", "login"))

	if got := svc.GetCurrentAPIKeyUUID(); got != "" {
		t.Fatalf("GetCurrentAPIKeyUUID() = %q for non-api token, want empty", got)
	}
}

func TestGetCurrentAPIKeyUUIDEmptyTokenReturnsEmpty(t *testing.T) {
	svc := New(nil, "")

	if got := svc.GetCurrentAPIKeyUUID(); got != "" {
		t.Fatalf("GetCurrentAPIKeyUUID() = %q for empty token, want empty", got)
	}
}
