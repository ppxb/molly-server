package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"molly-server/pkg/auth"
)

const testSecret = "test-secret-key-must-be-32-chars!!"

func init() {
	auth.Init(testSecret)
}

func TestGenerateAndParseToken(t *testing.T) {
	token, err := auth.GenerateJWT("user-001", "session-abc", "alice", 120)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := auth.ParseToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-001", claims.UserID)
	assert.Equal(t, "session-abc", claims.SessionID)
	assert.Equal(t, "alice", claims.UserName)
	assert.True(t, claims.ExpiresAt.Time.After(time.Now()))
}

func TestParseToken_InvalidSignature(t *testing.T) {
	// 用不同 secret 签发的 token
	auth.Init("another-secret-key-32-chars-long!!")
	token, _ := auth.GenerateJWT("u", "s", "n", 120)

	// 恢复正确 secret 后验证
	auth.Init(testSecret)
	_, err := auth.ParseToken(token)
	assert.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := auth.ParseToken("not.a.jwt")
	assert.ErrorIs(t, err, auth.ErrTokenInvalid)
}

func TestCookieDomain(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"localhost:8080", "localhost"},
		{"api.example.com", "api.example.com"},
		{"api.example.com:443", "api.example.com"},
		{"192.168.1.1:9000", "192.168.1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, auth.CookieDomain(tt.host))
		})
	}
}
