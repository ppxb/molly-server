package persistence

import (
	"context"
	"time"

	entrecycled "molly-server/internal/ent/gen/recycled"
	entuserfile "molly-server/internal/ent/gen/userfile"

	domain "molly-server/internal/domain/recycled"
	ent "molly-server/internal/ent/gen"
)

type recycledRepo struct{ client *ent.Client }

func NewRecycledRepo(client *ent.Client) domain.Repository {
	return &recycledRepo{client: client}
}

func (r *recycledRepo) Create(ctx context.Context, rec *domain.Recycled) error {
	_, err := r.client.Recycled.Create().
		SetID(rec.ID).
		SetUserID(rec.UserID).
		SetFileID(rec.UserFileID). // schema 里 file_id 存的是 uf_id（原 FileID 字段语义）
		Save(ctx)
	return err
}

func (r *recycledRepo) GetByID(ctx context.Context, id string) (*domain.Recycled, error) {
	e, err := r.client.Recycled.Get(ctx, id)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toRecycled(e), nil
}

func (r *recycledRepo) GetByUserFileID(ctx context.Context, userFileID string) (*domain.Recycled, error) {
	e, err := r.client.Recycled.Query().
		Where(entrecycled.FileID(userFileID)). // schema 的 file_id 实际存 uf_id
		Only(ctx)
	if err != nil {
		return nil, mapEntErr(err, domain.ErrNotFound)
	}
	return toRecycled(e), nil
}

func (r *recycledRepo) ListByUser(ctx context.Context, userID string, offset, limit int) ([]*domain.Recycled, error) {
	es, err := r.client.Recycled.Query().
		Where(entrecycled.UserID(userID)).
		Order(ent.Desc(entrecycled.FieldCreatedAt)).
		Offset(offset).Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapRecycleds(es), nil
}

func (r *recycledRepo) CountByUser(ctx context.Context, userID string) (int64, error) {
	n, err := r.client.Recycled.Query().
		Where(entrecycled.UserID(userID)).Count(ctx)
	return int64(n), err
}

func (r *recycledRepo) Delete(ctx context.Context, id string) error {
	return r.client.Recycled.DeleteOneID(id).Exec(ctx)
}

func (r *recycledRepo) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.client.Recycled.Delete().
		Where(entrecycled.UserID(userID)).Exec(ctx)
	return err
}

func (r *recycledRepo) ListExpired(ctx context.Context, before time.Time) ([]*domain.Recycled, error) {
	es, err := r.client.Recycled.Query().
		Where(entrecycled.CreatedAtLT(before)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mapRecycleds(es), nil
}

// CountActiveUserFiles 统计指定 fileID 对应的活跃（未软删除）UserFile 数量。
// 用于判断物理文件是否仍被其他用户引用。
func (r *recycledRepo) CountActiveUserFiles(ctx context.Context, fileID string) (int64, error) {
	n, err := r.client.UserFile.Query().
		Where(
			entuserfile.FileID(fileID),
			entuserfile.DeletedAtIsNil(), // 只统计未软删除的
		).Count(ctx)
	return int64(n), err
}

// ── 映射 ────────────────────────────────────────────────────

func toRecycled(e *ent.Recycled) *domain.Recycled {
	return &domain.Recycled{
		ID:         e.ID,
		UserID:     e.UserID,
		UserFileID: e.FileID, // schema 的 file_id 存的是 uf_id
		FileID:     e.FileID, // 需要从 UserFile 关联获取真实 file_id，此处暂存 uf_id
		CreatedAt:  e.CreatedAt,
	}
}

func mapRecycleds(es []*ent.Recycled) []*domain.Recycled {
	out := make([]*domain.Recycled, len(es))
	for i, e := range es {
		out[i] = toRecycled(e)
	}
	return out
}

var _ domain.Repository = (*recycledRepo)(nil)
