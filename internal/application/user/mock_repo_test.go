package user_test

import (
	"context"

	"github.com/stretchr/testify/mock"

	domain "molly-server/internal/domain/user"
)

// mockRepo 实现 domain.Repository，用于单元测试中隔离数据库。
// 使用 testify/mock，无需外部代码生成工具。
type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) GetByUserName(ctx context.Context, userName string) (*domain.User, error) {
	args := m.Called(ctx, userName)
	u, _ := args.Get(0).(*domain.User)
	return u, args.Error(1)
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	args := m.Called(ctx, id)
	u, _ := args.Get(0).(*domain.User)
	return u, args.Error(1)
}

func (m *mockRepo) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	args := m.Called(ctx, u)
	created, _ := args.Get(0).(*domain.User)
	return created, args.Error(1)
}

func (m *mockRepo) Update(ctx context.Context, u *domain.User) error {
	return m.Called(ctx, u).Error(0)
}

func (m *mockRepo) GetGroupWithPowers(ctx context.Context, groupID int) (*domain.Group, []domain.Power, error) {
	args := m.Called(ctx, groupID)
	g, _ := args.Get(0).(*domain.Group)
	powers, _ := args.Get(1).([]domain.Power)
	return g, powers, args.Error(2)
}

func (m *mockRepo) GetAPIKey(ctx context.Context, key string) (*domain.APIKey, error) {
	args := m.Called(ctx, key)
	k, _ := args.Get(0).(*domain.APIKey)
	return k, args.Error(1)
}

// 编译期检查：mock 是否完整实现了 domain.Repository
var _ domain.Repository = (*mockRepo)(nil)
