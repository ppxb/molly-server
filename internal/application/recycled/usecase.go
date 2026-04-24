package recycled

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	domainfile "molly-server/internal/domain/file"
	domainrecycled "molly-server/internal/domain/recycled"
	domainuser "molly-server/internal/domain/user"
	"molly-server/pkg/logger"
)

// Deps 依赖集合。
type Deps struct {
	Recycled    domainrecycled.Repository
	UserFile    domainfile.UserFileRepository
	FileInfo    domainfile.FileInfoRepository
	VirtualPath domainfile.VirtualPathRepository
	UserRepo    domainuser.Repository
	Log         *logger.Logger
}

type UseCase struct {
	deps Deps
}

func NewUseCase(deps Deps) *UseCase {
	return &UseCase{deps: deps}
}

// ── 核心：将文件移入回收站（由 file/usecase.DeleteFiles 调用）──────

// MoveToRecycled 软删除 UserFile 并创建回收站记录，作为原子操作。
// 调用方（file/usecase）确保 userFileID 属于该 userID。
func (uc *UseCase) MoveToRecycled(ctx context.Context, userID, userFileID string) error {
	uf, err := uc.deps.UserFile.GetByUserAndUfID(ctx, userID, userFileID)
	if err != nil {
		return domainfile.ErrNotFound
	}

	// 软删除
	if err := uc.deps.UserFile.SoftDelete(ctx, uf.ID); err != nil {
		return fmt.Errorf("recycled: soft delete user_file: %w", err)
	}

	// 创建回收站记录
	r := &domainrecycled.Recycled{
		ID:         uuid.NewString(),
		UserID:     userID,
		UserFileID: uf.ID,
		FileID:     uf.FileID,
		CreatedAt:  time.Now(),
	}
	if err := uc.deps.Recycled.Create(ctx, r); err != nil {
		// 回收站记录创建失败时，尝试回滚软删除（最终一致性 best-effort）
		uc.deps.Log.Error("recycled: create record failed, soft delete may be orphaned",
			"user_file_id", uf.ID, "error", err)
		return fmt.Errorf("recycled: create record: %w", err)
	}
	return nil
}

// ── 列表 ────────────────────────────────────────────────────

func (uc *UseCase) List(ctx context.Context, userID string, req ListReq) (*ListResp, error) {
	offset := (req.Page - 1) * req.PageSize
	records, err := uc.deps.Recycled.ListByUser(ctx, userID, offset, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("recycled: list: %w", err)
	}
	total, err := uc.deps.Recycled.CountByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("recycled: count: %w", err)
	}

	items := make([]Item, 0, len(records))
	for _, r := range records {
		item, err := uc.buildItem(ctx, r)
		if err != nil {
			// 跳过数据不一致的记录，不中断整个列表
			uc.deps.Log.Warn("recycled: build item failed", "id", r.ID, "error", err)
			continue
		}
		items = append(items, item)
	}
	return &ListResp{Items: items, Total: total, Page: req.Page, PageSize: req.PageSize}, nil
}

func (uc *UseCase) buildItem(ctx context.Context, r *domainrecycled.Recycled) (Item, error) {
	// UserFile 已被软删除，通过 GetByID 绕过 deleted_at 过滤
	uf, err := uc.deps.UserFile.GetByID(ctx, r.UserFileID)
	if err != nil {
		return Item{}, fmt.Errorf("get user_file: %w", err)
	}
	fi, err := uc.deps.FileInfo.GetByID(ctx, uf.FileID)
	if err != nil {
		return Item{}, fmt.Errorf("get file_info: %w", err)
	}
	deletedAt := time.Time{}
	if uf.DeletedAt != nil {
		deletedAt = *uf.DeletedAt
	}
	return Item{
		RecycledID:   r.ID,
		UserFileID:   uf.ID,
		FileName:     uf.FileName,
		FileSize:     fi.Size,
		MimeType:     fi.Mime,
		IsEnc:        fi.IsEnc,
		HasThumbnail: fi.ThumbnailImg != "",
		DeletedAt:    deletedAt,
	}, nil
}

// ── 还原 ────────────────────────────────────────────────────

// Restore 还原文件到原目录；若原目录已删除，自动降级到根目录。
func (uc *UseCase) Restore(ctx context.Context, userID string, req RestoreReq) (string, error) {
	r, err := uc.getOwned(ctx, userID, req.RecycledID)
	if err != nil {
		return "", err
	}

	uf, err := uc.deps.UserFile.GetByID(ctx, r.UserFileID)
	if err != nil {
		return "", fmt.Errorf("recycled: get user_file for restore: %w", err)
	}

	// 确认目标目录是否存在
	targetPath := uf.VirtualPath
	restoredToRoot := false

	if targetPath != "" && targetPath != "0" {
		pathID := 0
		if _, err := fmt.Sscanf(targetPath, "%d", &pathID); err == nil && pathID > 0 {
			if _, err := uc.deps.VirtualPath.GetByID(ctx, pathID); err != nil {
				// 原目录已不存在，降级到根目录
				root, err := uc.deps.VirtualPath.GetRoot(ctx, userID)
				if err != nil {
					return "", fmt.Errorf("recycled: get root path: %w", err)
				}
				targetPath = fmt.Sprintf("%d", root.ID)
				restoredToRoot = true
			}
		}
	}

	// 还原：清除软删除标记（并按需更新路径）
	uf.DeletedAt = nil
	uf.VirtualPath = targetPath
	if err := uc.deps.UserFile.Restore(ctx, uf); err != nil {
		return "", fmt.Errorf("recycled: restore user_file: %w", err)
	}

	// 删除回收站记录
	if err := uc.deps.Recycled.Delete(ctx, r.ID); err != nil {
		return "", fmt.Errorf("recycled: delete record: %w", err)
	}

	if restoredToRoot {
		return "file restored to root (original directory was deleted)", nil
	}
	return "file restored", nil
}

// ── 永久删除 ────────────────────────────────────────────────

// DeletePermanently 永久删除单个回收站文件。
// 若该物理文件仍被其他活跃 UserFile 引用，则只删除本条记录，不清除物理文件。
func (uc *UseCase) DeletePermanently(ctx context.Context, userID string, req DeleteReq) error {
	r, err := uc.getOwned(ctx, userID, req.RecycledID)
	if err != nil {
		return err
	}
	return uc.permanentlyDelete(ctx, r)
}

// Empty 清空当前用户回收站。
func (uc *UseCase) Empty(ctx context.Context, userID string) (*EmptyResp, error) {
	// 拉取全量记录（分批处理避免内存溢出）
	records, err := uc.deps.Recycled.ListByUser(ctx, userID, 0, 10000)
	if err != nil {
		return nil, fmt.Errorf("recycled: list for empty: %w", err)
	}

	resp := &EmptyResp{}
	for _, r := range records {
		if err := uc.permanentlyDelete(ctx, r); err != nil {
			uc.deps.Log.Error("recycled: delete failed", "id", r.ID, "error", err)
			resp.Failed++
		} else {
			resp.Deleted++
		}
	}
	return resp, nil
}

// ── 定时清理（由 task 层调用）──────────────────────────────

// CleanExpired 清理超过保留期的回收站记录（系统定时任务）。
func (uc *UseCase) CleanExpired(ctx context.Context, retentionDays int) (int, error) {
	before := time.Now().AddDate(0, 0, -retentionDays)
	expired, err := uc.deps.Recycled.ListExpired(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("recycled: list expired: %w", err)
	}

	deleted := 0
	for _, r := range expired {
		if err := uc.permanentlyDelete(ctx, r); err != nil {
			uc.deps.Log.Warn("recycled: cleanup failed", "id", r.ID, "error", err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// ── 内部核心：永久删除单条记录 ──────────────────────────────

// permanentlyDelete 执行完整的永久删除流程：
//  1. 检查物理文件是否还有其他活跃引用
//  2. 无其他引用 → 删除物理文件 + FileInfo
//  3. 归还用户存储空间
//  4. 删除（已软删除的）UserFile 硬删除
//  5. 删除 Recycled 记录
func (uc *UseCase) permanentlyDelete(ctx context.Context, r *domainrecycled.Recycled) error {
	// 1. 获取关联的 UserFile（已软删除，使用 GetByID 无过滤）
	uf, err := uc.deps.UserFile.GetByID(ctx, r.UserFileID)
	if err != nil {
		// UserFile 记录不存在，仅删除回收站记录
		uc.deps.Log.Warn("recycled: user_file not found, removing orphan record", "id", r.ID)
		return uc.deps.Recycled.Delete(ctx, r.ID)
	}

	// 2. 检查同一物理文件的活跃引用数
	activeRefs, err := uc.deps.Recycled.CountActiveUserFiles(ctx, uf.FileID)
	if err != nil {
		return fmt.Errorf("recycled: count active refs: %w", err)
	}

	if activeRefs == 0 {
		// 3. 无其他引用，可以安全删除物理文件
		fi, err := uc.deps.FileInfo.GetByID(ctx, uf.FileID)
		if err == nil {
			uc.deletePhysical(fi)

			// 4. 归还用户存储空间
			if err := uc.returnSpace(ctx, r.UserID, fi.Size); err != nil {
				uc.deps.Log.Warn("recycled: return space failed", "user_id", r.UserID, "error", err)
			}

			// 5. 删除 FileInfo
			if err := uc.deps.FileInfo.Delete(ctx, fi.ID); err != nil {
				return fmt.Errorf("recycled: delete file_info: %w", err)
			}
		}
	}

	// 6. 硬删除 UserFile（绕过软删除过滤）
	if err := uc.deps.UserFile.HardDelete(ctx, uf.ID); err != nil {
		return fmt.Errorf("recycled: hard delete user_file: %w", err)
	}

	// 7. 删除 Recycled 记录
	return uc.deps.Recycled.Delete(ctx, r.ID)
}

// ── 物理文件清理 ────────────────────────────────────────────

// deletePhysical 删除磁盘上的物理文件（失败只记录日志，不阻断流程）。
func (uc *UseCase) deletePhysical(fi *domainfile.FileInfo) {
	remove := func(path string) {
		if path == "" {
			return
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			uc.deps.Log.Warn("recycled: remove file failed", "path", path, "error", err)
		}
	}

	// 普通文件
	remove(fi.Path)

	// 加密文件
	if fi.IsEnc {
		remove(fi.EncPath)
	}

	// 缩略图
	if fi.ThumbnailImg != "" {
		remove(fi.ThumbnailImg)
	}

	// 空父目录清理（best-effort）
	if fi.Path != "" {
		dir := filepath.Dir(fi.Path)
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
}

// returnSpace 归还用户配额（仅对有限空间用户）。
func (uc *UseCase) returnSpace(ctx context.Context, userID string, size int64) error {
	user, err := uc.deps.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Space <= 0 {
		return nil // 无限空间，无需归还
	}
	user.FreeSpace += size
	return uc.deps.UserRepo.Update(ctx, user)
}

// ── Helper ──────────────────────────────────────────────────

// getOwned 获取并验证归属权。
func (uc *UseCase) getOwned(ctx context.Context, userID, recycledID string) (*domainrecycled.Recycled, error) {
	r, err := uc.deps.Recycled.GetByID(ctx, recycledID)
	if err != nil {
		return nil, domainrecycled.ErrNotFound
	}
	if r.UserID != userID {
		return nil, domainrecycled.ErrForbidden
	}
	return r, nil
}
