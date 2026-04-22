package auth

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrTokenInvalid = errors.New("auth: token is invalid or expired")

var secret []byte

// Init 在 cmd.go / server 启动时调用一次，注入签名密钥。
// 密钥来自 config.AuthConfig.Secret（至少 32 字节）。
func Init(signingSecret string) {
	secret = []byte(signingSecret)
}

func GenerateJWT(userID, sessionID, userName string, expireMinutes int) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("auth: secret not initialized, call auth.Init first")
	}

	now := time.Now()
	claims := Claims{
		UserID:    userID,
		SessionID: sessionID,
		UserName:  userName,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireMinutes) * time.Minute)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}

	return signed, nil
}

func ParseToken(tokenStr string) (*Claims, error) {
	if len(secret) == 0 {
		return nil, errors.New("auth: secret not initialized, call auth.Init first")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		// 防止 alg=none 攻击：强制要求 HMAC 算法族
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", t.Header["alg"])
		}

		return secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

func CookieDomain(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		// 没有端口号，原样返回
		return host
	}
	return h
}
