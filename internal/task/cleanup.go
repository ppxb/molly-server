// internal/task/cleanup.go

package task

import (
	"context"
	"time"

	apprecycled "molly-server/internal/application/recycled"
	domainfile "molly-server/internal/domain/file"
	"molly-server/pkg/logger"
)

// RecycledCleanup 定期清理超期回收站文件。
type RecycledCleanup struct {
	uc            *apprecycled.UseCase
	retentionDays int
	log           *logger.Logger
}

func NewRecycledCleanup(uc *apprecycled.UseCase, retentionDays int, log *logger.Logger) *RecycledCleanup {
	return &RecycledCleanup{uc: uc, retentionDays: retentionDays, log: log}
}

// Start 启动定时任务，interval 为执行间隔（推荐 24h）。
// 返回 stop 函数，graceful shutdown 时调用。
func (c *RecycledCleanup) Start(interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		c.log.Info("task: recycled cleanup started",
			"interval", interval, "retention_days", c.retentionDays)
		for {
			select {
			case <-ticker.C:
				n, err := c.uc.CleanExpired(ctx, c.retentionDays)
				if err != nil {
					c.log.Error("task: recycled cleanup failed", "error", err)
				} else if n > 0 {
					c.log.Info("task: recycled cleanup done", "deleted", n)
				}
			case <-ctx.Done():
				c.log.Info("task: recycled cleanup stopped")
				return
			}
		}
	}()
	return cancel
}

// UploadCleanup 定期清理过期上传任务及其临时文件。
type UploadCleanup struct {
	repo domainfile.UploadTaskRepository
	log  *logger.Logger
}

func NewUploadCleanup(repo domainfile.UploadTaskRepository, log *logger.Logger) *UploadCleanup {
	return &UploadCleanup{repo: repo, log: log}
}

func (c *UploadCleanup) Start(interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		c.log.Info("task: upload cleanup started", "interval", interval)
		for {
			select {
			case <-ticker.C:
				n, err := c.repo.DeleteExpired(ctx, time.Now())
				if err != nil {
					c.log.Error("task: upload cleanup failed", "error", err)
				} else if n > 0 {
					c.log.Info("task: upload cleanup done", "deleted", n)
				}
			case <-ctx.Done():
				c.log.Info("task: upload cleanup stopped")
				return
			}
		}
	}()
	return cancel
}
