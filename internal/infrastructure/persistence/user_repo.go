package persistence

import (
	"context"
	"errors"

	ent "molly-server/internal/ent/gen"
	"molly-server/internal/ent/gen/apikey"
	"molly-server/internal/ent/gen/group"
	"molly-server/internal/ent/gen/power"
	"molly-server/internal/ent/gen/user"

	domain "molly-server/internal/domain/user"
)

// userRepo 实现 domain/user.Repository 接口。
// 它只知道 ent，不知道 HTTP、不知道 JWT，不知道任何上层概念。
type userRepo struct {
	client *ent.Client
}

// NewUserRepo 构造函数，返回 domain 接口类型（不暴露具体实现）。
func NewUserRepo(client *ent.Client) domain.Repository {
	return &userRepo{
		client: client,
	}
}

func (r *userRepo) GetByUserName(ctx context.Context, userName string) (*domain.User, error) {
	u, err := r.client.User.
		Query().
		Where(user.UserName(userName)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return toDomainUser(u), nil
}

func (r *userRepo) GetAPIKey(ctx context.Context, key string) (*domain.APIKey, error) {
	k, err := r.client.APIKey.
		Query().
		Where(apikey.Key(key)).
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toDomainAPIKey(k), nil
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	u, err := r.client.User.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return toDomainUser(u), nil
}

func (r *userRepo) Create(ctx context.Context, u *domain.User) (*domain.User, error) {
	created, err := r.client.User.
		Create().
		SetID(u.ID).
		SetNickName(u.NickName).
		SetUserName(u.UserName).
		SetPassword(u.Password).
		SetEmail(u.Email).
		SetPhone(u.Phone).
		SetGroupID(u.GroupID).
		SetSpace(u.Space).
		SetFreeSpace(u.FreeSpace).
		SetState(int(u.State)).
		Save(ctx)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, domain.ErrUserNameConflict
		}
		return nil, err
	}
	return toDomainUser(created), nil
}

func (r *userRepo) Update(ctx context.Context, u *domain.User) error {
	err := r.client.User.
		UpdateOneID(u.ID).
		SetNickName(u.NickName).
		SetEmail(u.Email).
		SetPhone(u.Phone).
		SetSpace(u.Space).
		SetFreeSpace(u.FreeSpace).
		SetState(int(u.State)).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *userRepo) GetGroupWithPowers(ctx context.Context, groupID int) (*domain.Group, []domain.Power, error) {
	g, err := r.client.Group.
		Query().
		Where(group.ID(groupID)).
		WithPowers(func(q *ent.PowerQuery) {
			// 通过穿透表 group_power 联查 power
			q.Order(ent.Asc(power.FieldID))
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}

	domainGroup := &domain.Group{
		ID:        g.ID,
		Name:      g.Name,
		IsDefault: g.IsDefault,
		Space:     g.Space,
	}

	powers := make([]domain.Power, len(g.Edges.Powers))
	for i, p := range g.Edges.Powers {
		powers[i] = domain.Power{
			ID:             p.ID,
			Name:           p.Name,
			Description:    p.Description,
			Characteristic: p.Characteristic,
		}
	}

	return domainGroup, powers, nil
}

// ── 映射函数（ent model → domain entity）
func toDomainUser(u *ent.User) *domain.User {
	return &domain.User{
		ID:           u.ID,
		NickName:     u.NickName,
		UserName:     u.UserName,
		Password:     u.Password,
		Email:        u.Email,
		Phone:        u.Phone,
		GroupID:      u.GroupID,
		Space:        u.Space,
		FreeSpace:    u.FreeSpace,
		FilePassword: u.FilePassword,
		State:        domain.State(u.State),
		CreatedAt:    u.CreatedAt,
	}
}

func toDomainAPIKey(k *ent.APIKey) *domain.APIKey {
	ak := &domain.APIKey{
		ID:          k.ID,
		UserID:      k.UserID,
		Key:         k.Key,
		PrivateKey:  k.PrivateKey,
		S3SecretKey: k.S3SecretKey,
		CreatedAt:   k.CreatedAt,
	}
	if k.ExpiresAt != nil {
		ak.ExpiresAt = new(*k.ExpiresAt)
	}
	return ak
}

// isUniqueConstraintErr 判断是否为唯一约束冲突（MySQL / PostgreSQL 均适用）。
func isUniqueConstraintErr(err error) bool {
	var entErr *ent.ConstraintError
	return errors.As(err, &entErr)
}

func mapEntErr(err error, notFoundErr error) error {
	if err == nil {
		return nil
	}
	if ent.IsNotFound(err) {
		return notFoundErr
	}
	return err
}

var _ domain.Repository = (*userRepo)(nil)
