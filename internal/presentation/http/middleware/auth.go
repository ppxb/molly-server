package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"molly-server/pkg/cache"
	"molly-server/pkg/util"

	appuser "molly-server/internal/application/user"
	domainuser "molly-server/internal/domain/user"
	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/auth"
)

// ctxKey 用自定义类型避免 gin.Context 中 string key 碰撞。
type ctxKey string

const (
	KeyUserSession ctxKey = "userSession" // 值类型：*domainuser.Session
	KeyUserID      ctxKey = "userID"      // 值类型：string
)

// AuthMiddleware 鉴权中间件，所有依赖通过构造函数注入，无全局变量。
type AuthMiddleware struct {
	cfg    config.AuthConfig
	cache  cache.Cache
	userUC *appuser.UseCase
}

func NewAuthMiddleware(cfg config.AuthConfig, c cache.Cache, uc *appuser.UseCase) *AuthMiddleware {
	return &AuthMiddleware{cfg: cfg, cache: c, userUC: uc}
}

// Verify 强制鉴权：无有效凭证返回 401。
func (m *AuthMiddleware) Verify() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := m.authenticate(c)
		if err != nil {
			abortUnauthorized(c, err.Error())
			return
		}
		setSession(c, session)
		c.Next()
	}
}

// VerifyOptional 可选鉴权：有凭证则验证，无凭证则放行（匿名访问）。
// 凭证存在但非法时仍返回 401。
func (m *AuthMiddleware) VerifyOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !hasAnyCredential(c) {
			c.Next()
			return
		}
		session, err := m.authenticate(c)
		if err != nil {
			abortUnauthorized(c, err.Error())
			return
		}
		setSession(c, session)
		c.Next()
	}
}

// ------------------------------------------------------------
// 核心认证逻辑
// ------------------------------------------------------------

// authenticate 按优先级尝试各种凭证，返回 Session 或错误。
// 优先级：Authorization Header → Cookie → API Key
func (m *AuthMiddleware) authenticate(c *gin.Context) (*domainuser.Session, error) {
	if token := extractBearerToken(c.GetHeader("Authorization")); token != "" {
		return m.verifyJWT(c, token)
	}
	if cookie, err := c.Cookie("Authorization"); err == nil && cookie != "" {
		return m.verifyJWT(c, cookie)
	}
	if m.cfg.ApiKey {
		return m.verifyAPIKey(c)
	}
	return nil, fmt.Errorf("missing credentials")
}

// verifyJWT 验证 JWT session。
// token 作为 session key 存入缓存，值为实际 JWT 字符串。
// 剩余有效期不足 5 分钟时自动续签（sliding window）。
func (m *AuthMiddleware) verifyJWT(c *gin.Context, token string) (*domainuser.Session, error) {
	// 从缓存取实际 JWT（未登录 / 已登出时 cache miss）
	raw, err := m.cache.Get(token)
	if err != nil {
		return nil, fmt.Errorf("session not found or expired")
	}

	claims, err := auth.ParseToken(raw.(string))
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// 剩余不足 5 分钟：续签并刷新缓存 & Cookie
	if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) < 5*time.Minute {
		m.renewToken(c, token, claims)
	}

	// 从数据库（经 use-case）加载最新权限快照
	session, err := m.userUC.GetSession(c.Request.Context(), claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return session, nil
}

// renewToken 重新签发 token 并刷新缓存与 Cookie，失败时静默忽略（不中断请求）。
func (m *AuthMiddleware) renewToken(c *gin.Context, oldToken string, claims *auth.Claims) {
	newToken, err := auth.GenerateJWT(
		claims.UserID, claims.SessionID, claims.UserName,
		m.cfg.JwtExpire, // 单位：分钟，与首次签发一致
	)
	if err != nil {
		return
	}
	ttlSeconds := m.cfg.JwtExpire * 60
	_ = m.cache.Set(oldToken, newToken, ttlSeconds)
	c.SetCookie("Authorization", newToken, ttlSeconds, "/",
		auth.CookieDomain(c.Request.Host), false, true)
}

// verifyAPIKey 验证 HMAC 签名的 API Key。
// 签名格式：RSA 加密的 query string "apikey=...&timestamp=...&nonce=..."
func (m *AuthMiddleware) verifyAPIKey(c *gin.Context) (*domainuser.Session, error) {
	apiKey := c.GetHeader("X-API-Key")
	signature := c.GetHeader("X-Signature")
	timestampStr := c.GetHeader("X-Timestamp")
	nonce := c.GetHeader("X-Nonce")

	if apiKey == "" || signature == "" || timestampStr == "" || nonce == "" {
		return nil, fmt.Errorf("incomplete API Key parameters")
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp format")
	}
	if math.Abs(time.Since(time.UnixMilli(ts)).Minutes()) > 5 {
		return nil, fmt.Errorf("timestamp expired (>5 min)")
	}

	ctx := context.Background()
	record, err := m.userUC.GetAPIKey(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("API Key not found")
	}
	if record.ExpiresAt != nil && record.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API Key expired")
	}

	// RSA 解密签名内容，校验各字段一致性
	decrypted, err := util.DecryptToString(record.PrivateKey, signature)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed")
	}
	vals, err := url.ParseQuery(decrypted)
	if err != nil ||
		vals.Get("apikey") != apiKey ||
		vals.Get("timestamp") != timestampStr ||
		vals.Get("nonce") != nonce {
		return nil, fmt.Errorf("signature content mismatch")
	}

	session, err := m.userUC.GetSession(ctx, record.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return session, nil
}

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

func extractBearerToken(header string) string {
	trimmed := strings.TrimSpace(header)
	if after, ok := strings.CutPrefix(trimmed, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(trimmed)
}

func hasAnyCredential(c *gin.Context) bool {
	if c.GetHeader("Authorization") != "" {
		return true
	}
	v, err := c.Cookie("Authorization")
	return err == nil && v != ""
}

func setSession(c *gin.Context, s *domainuser.Session) {
	c.Set(string(KeyUserSession), s)
	c.Set(string(KeyUserID), s.UserID)
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":    http.StatusUnauthorized,
		"message": msg,
	})
}
