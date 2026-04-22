package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/auth"

	domainuser "molly-server/internal/domain/user"
)

// UseCase 用户应用层，编排 domain 操作与基础设施调用。
// 不包含业务规则（规则在 domain entity / domain service 中）。
type UseCase struct {
	repo domainuser.Repository
	cfg  config.AuthConfig
}

func NewUseCase(repo domainuser.Repository, cfg config.AuthConfig) *UseCase {
	return &UseCase{
		repo: repo,
		cfg:  cfg,
	}
}

// Login 用户名密码登录，成功返回 JWT 及用户摘要。
func (uc *UseCase) Login(ctx context.Context, req LoginReq) (*LoginResp, error) {
	u, err := uc.repo.GetByUserName(ctx, req.UserName)
	if err != nil {
		// 统一返回"凭证无效"，避免用户名枚举攻击
		return nil, domainuser.ErrInvalidCredential
	}
	if u.IsDisabled() {
		return nil, domainuser.ErrDisabled
	}
	if !u.ValidatePassword(req.Password) {
		return nil, domainuser.ErrInvalidCredential
	}

	_, powers, err := uc.repo.GetGroupWithPowers(ctx, u.GroupID)
	if err != nil {
		return nil, fmt.Errorf("login: load powers: %w", err)
	}
	chars := powerCharacteristics(powers)

	sessionID := uuid.NewString()
	token, err := auth.GenerateJWT(u.ID, sessionID, u.UserName, uc.cfg.JwtExpire)
	if err != nil {
		return nil, fmt.Errorf("login: generate token: %w", err)
	}

	return &LoginResp{
		Token:    token,
		UserID:   u.ID,
		UserName: u.UserName,
		NickName: u.NickName,
		GroupID:  u.GroupID,
		Powers:   chars,
		Space:    u.Space,
	}, nil
}

// Register 注册新用户，返回脱敏的用户信息。
func (uc *UseCase) Register(ctx context.Context, req RegisterReq) (*UserResp, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	u := &domainuser.User{
		ID:        uuid.NewString(),
		NickName:  req.NickName,
		UserName:  req.UserName,
		Password:  string(hash),
		Email:     req.Email,
		Phone:     req.Phone,
		GroupID:   defaultGroupID,
		State:     domainuser.StateActive,
		CreatedAt: time.Now(),
	}

	created, err := uc.repo.Create(ctx, u)
	if err != nil {
		return nil, err // 保留 domain 错误（ErrUserNameConflict 等）
	}
	return toUserResp(created), nil
}

// GetSession 按 UserID 构建鉴权 Session（供 middleware 调用）。
// 每次请求都重新从 DB 加载，确保权限变更即时生效。
func (uc *UseCase) GetSession(ctx context.Context, userID string) (*domainuser.Session, error) {
	u, err := uc.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, powers, err := uc.repo.GetGroupWithPowers(ctx, u.GroupID)
	if err != nil {
		return nil, fmt.Errorf("get session: load powers: %w", err)
	}
	return &domainuser.Session{
		UserID:   u.ID,
		UserName: u.UserName,
		GroupID:  u.GroupID,
		Powers:   powerCharacteristics(powers),
	}, nil
}

// GetAPIKey 查询 API Key 记录（供 middleware.verifyAPIKey 调用）。
func (uc *UseCase) GetAPIKey(ctx context.Context, key string) (*domainuser.APIKey, error) {
	return uc.repo.GetAPIKey(ctx, key)
}

// GetByID 返回脱敏用户信息（密码字段不对外暴露）。
func (uc *UseCase) GetByID(ctx context.Context, userID string) (*UserResp, error) {
	u, err := uc.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toUserResp(u), nil
}

// TODO: 普通用户组，可设置为 ent 的默认值
const defaultGroupID = 2

func powerCharacteristics(powers []domainuser.Power) []string {
	chars := make([]string, len(powers))
	for i, p := range powers {
		chars[i] = p.Characteristic
	}
	return chars
}

func toUserResp(u *domainuser.User) *UserResp {
	return &UserResp{
		ID:        u.ID,
		NickName:  u.NickName,
		UserName:  u.UserName,
		Email:     u.Email,
		Phone:     u.Phone,
		GroupID:   u.GroupID,
		Space:     u.Space,
		FreeSpace: u.FreeSpace,
		State:     int(u.State),
	}
}
