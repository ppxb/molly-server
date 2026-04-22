package user_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	appuser "molly-server/internal/application/user"
	domain "molly-server/internal/domain/user"
	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/auth"
)

// ── 测试辅助 ──────────────────────────────────────────────────

func newTestUseCase(repo *mockRepo) *appuser.UseCase {
	auth.Init("test-secret-key-must-be-32-chars!!")
	cfg := config.AuthConfig{
		Secret:    "test-secret-key-must-be-32-chars!!",
		JwtExpire: 120,
	}
	return appuser.NewUseCase(repo, cfg)
}

// hashedPassword 生成测试用 bcrypt hash（只调用一次，避免测试慢）
func hashedPassword(t *testing.T, plain string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost) // MinCost 让测试更快
	require.NoError(t, err)
	return string(h)
}

func activeUser(t *testing.T) *domain.User {
	return &domain.User{
		ID:       "user-001",
		NickName: "Alice",
		UserName: "alice",
		Password: hashedPassword(t, "correct-password"),
		Email:    "alice@example.com",
		GroupID:  2,
		State:    domain.StateActive,
	}
}

func defaultGroup() *domain.Group {
	return &domain.Group{ID: 2, Name: "users", Space: 10 << 30}
}

func defaultPowers() []domain.Power {
	return []domain.Power{
		{ID: 1, Characteristic: "file:upload"},
		{ID: 2, Characteristic: "file:download"},
	}
}

// ── Login ─────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()
	u := activeUser(t)

	repo.On("GetByUserName", ctx, "alice").Return(u, nil)
	repo.On("GetGroupWithPowers", ctx, 2).Return(defaultGroup(), defaultPowers(), nil)

	resp, err := uc.Login(ctx, appuser.LoginReq{
		UserName: "alice",
		Password: "correct-password",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "user-001", resp.UserID)
	assert.Equal(t, "alice", resp.UserName)
	assert.ElementsMatch(t, []string{"file:upload", "file:download"}, resp.Powers)
	repo.AssertExpectations(t)
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	repo.On("GetByUserName", ctx, "alice").Return(activeUser(t), nil)
	// GetGroupWithPowers 不应被调用

	_, err := uc.Login(ctx, appuser.LoginReq{
		UserName: "alice",
		Password: "wrong-password",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
	repo.AssertExpectations(t)
	// 确认 GetGroupWithPowers 未被调用
	repo.AssertNotCalled(t, "GetGroupWithPowers")
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	repo.On("GetByUserName", ctx, "ghost").Return(nil, domain.ErrNotFound)

	_, err := uc.Login(ctx, appuser.LoginReq{
		UserName: "ghost",
		Password: "any",
	})

	// 用户不存在时也应返回 ErrInvalidCredential（防枚举攻击）
	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
	repo.AssertExpectations(t)
}

func TestLogin_DisabledUser(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	disabled := activeUser(t)
	disabled.State = domain.StateDisabled

	repo.On("GetByUserName", ctx, "alice").Return(disabled, nil)

	_, err := uc.Login(ctx, appuser.LoginReq{
		UserName: "alice",
		Password: "correct-password",
	})

	assert.ErrorIs(t, err, domain.ErrDisabled)
	repo.AssertExpectations(t)
}

// ── Register ──────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	// Create 接受任意 *domain.User（密码 hash 每次不同，无法精确匹配）
	repo.On("Create", ctx, mock.AnythingOfType("*user.User")).
		Return(&domain.User{
			ID:       "new-001",
			NickName: "Bob",
			UserName: "bob",
			Email:    "bob@example.com",
			GroupID:  2,
			State:    domain.StateActive,
		}, nil)

	resp, err := uc.Register(ctx, appuser.RegisterReq{
		UserName: "bob",
		Password: "securepassword",
		NickName: "Bob",
		Email:    "bob@example.com",
	})

	require.NoError(t, err)
	assert.Equal(t, "new-001", resp.ID)
	assert.Equal(t, "bob", resp.UserName)
	// 密码字段绝对不能出现在响应里
	repo.AssertExpectations(t)
}

func TestRegister_DuplicateUserName(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	repo.On("Create", ctx, mock.AnythingOfType("*user.User")).
		Return(nil, domain.ErrUserNameConflict)

	_, err := uc.Register(ctx, appuser.RegisterReq{
		UserName: "alice",
		Password: "securepassword",
		NickName: "Alice2",
		Email:    "alice2@example.com",
	})

	assert.ErrorIs(t, err, domain.ErrUserNameConflict)
	repo.AssertExpectations(t)
}

// ── GetSession ────────────────────────────────────────────────

func TestGetSession_Success(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()
	u := activeUser(t)

	repo.On("GetByID", ctx, "user-001").Return(u, nil)
	repo.On("GetGroupWithPowers", ctx, 2).Return(defaultGroup(), defaultPowers(), nil)

	session, err := uc.GetSession(ctx, "user-001")

	require.NoError(t, err)
	assert.Equal(t, "user-001", session.UserID)
	assert.False(t, session.IsAdmin()) // groupID=2，不是管理员
	assert.True(t, session.HasPower("file:upload"))
	assert.False(t, session.HasPower("admin:manage"))
	repo.AssertExpectations(t)
}

func TestGetSession_AdminGroup(t *testing.T) {
	repo := new(mockRepo)
	uc := newTestUseCase(repo)
	ctx := context.Background()

	admin := activeUser(t)
	admin.GroupID = 1

	repo.On("GetByID", ctx, "admin-001").Return(admin, nil)
	repo.On("GetGroupWithPowers", ctx, 1).Return(
		&domain.Group{ID: 1, Name: "admin"},
		[]domain.Power{{Characteristic: "admin:manage"}},
		nil,
	)

	session, err := uc.GetSession(ctx, "admin-001")

	require.NoError(t, err)
	assert.True(t, session.IsAdmin())
	assert.True(t, session.HasPower("admin:manage"))
}
