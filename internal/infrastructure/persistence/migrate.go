package persistence

import (
	"context"
	"fmt"

	"molly-server/pkg/logger"
)

// Migrate 执行全量 schema 迁移（AutoMigrate 等价）。
//
// Ent 的 migrate.Schema.Create 会对比现有表结构，仅执行增量 DDL，
// 不会删除已有字段（除非显式传入 migrate.WithDropColumn(true)）。
//
// 推荐在 `molly migrate` 子命令中调用，而不是随服务启动自动执行，
// 避免生产环境意外变更表结构。
func Migrate(ctx context.Context, db *DB, log *logger.Logger) error {
	log.Info("persistence: running schema migration...")

	if err := db.Client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("persistence: schema migration failed: %w", err)
	}

	log.Info("persistence: schema migration completed")
	return nil
}
