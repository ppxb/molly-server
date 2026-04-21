package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"molly-server/ent"
	"molly-server/internal/infrastructure/config"
	"molly-server/pkg/logger"
)

type DB struct {
	Client *ent.Client
}

func Open(cfg config.DatabaseConfig, log *logger.Logger) (*DB, error) {
	driverName, dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}

	// 1. 用标准库建立底层连接池
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("persistence: open sql: %w", err)
	}

	// 2. 连接池调优（直接使用 config 里的 time.Duration，无需二次换算）
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.MaxLife)
	sqlDB.SetConnMaxIdleTime(cfg.MaxIdleLife)

	// 3. 验证连通性（Open 是懒连接，Ping 才真正拨号）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("persistence: ping %s: %w", cfg.Type, err)
	}

	// 4. 把 *sql.DB 交给 ent Driver
	drv := entsql.OpenDB(dialect.Postgres, sqlDB) // postgres / mysql 见 buildDSN
	if cfg.Type == "mysql" {
		drv = entsql.OpenDB(dialect.MySQL, sqlDB)
	}

	client := ent.NewClient(
		ent.Driver(drv),
		ent.Log(newEntLogger(log)), // SQL 日志桥接到 slog
	)

	log.Info("database connected",
		"type", cfg.Type,
		"max_open", cfg.MaxOpen,
		"max_idle", cfg.MaxIdle,
	)

	return &DB{Client: client}, nil
}

func (db *DB) Close() error {
	return db.Client.Close()
}

func buildDSN(cfg config.DatabaseConfig) (driverName, dsn string, err error) {
	switch cfg.Type {
	case "mysql":
		// charset=utf8mb4 防止 emoji 乱码；parseTime 让 GORM/ent 正确处理 time.Time
		return "mysql", fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName,
		), nil

	case "postgres":
		return "postgres", fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, sslMode(cfg),
		), nil

	default:
		return "", "", fmt.Errorf("persistence: unsupported database type %q (mysql | postgres)", cfg.Type)
	}
}

// sslMode 根据配置返回 PostgreSQL sslmode 参数。
// 默认 disable，生产环境建议通过 MYOBJ_DATABASE_SSLMODE=require 覆盖。
func sslMode(cfg config.DatabaseConfig) string {
	if cfg.SSLMode != "" {
		return cfg.SSLMode
	}
	return "disable"
}

// newEntLogger 返回一个符合 ent.Option Log 签名的函数。
// ent 的日志回调签名是 func(...any)，我们把它路由到 slog.Debug。
func newEntLogger(log *logger.Logger) func(...any) {
	return func(args ...any) {
		// ent 传入的通常是格式化好的 SQL 字符串，直接作为 msg 记录
		if len(args) == 0 {
			return
		}
		msg := fmt.Sprint(args...)
		log.Debug("ent", slog.String("sql", msg))
	}
}
