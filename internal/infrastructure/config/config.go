package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Log      LogConfig      `mapstructure:"log"`
	Database DatabaseConfig `mapstructure:"database"`
	Storage  StorageConfig  `mapstructure:"storage"`
	File     FileConfig     `mapstructure:"file"`
	Cors     CorsConfig     `mapstructure:"cors"`
	Cache    CacheConfig    `mapstructure:"cache"`
	WebDAV   WebDAVConfig   `mapstructure:"webdav"`
	S3       S3Config       `mapstructure:"s3"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	SSL          bool          `mapstructure:"ssl"`
	SSLKey       string        `mapstructure:"ssl_key"`
	SSLCert      string        `mapstructure:"ssl_cert"`
}

type AuthConfig struct {
	Secret    string `mapstructure:"secret"`
	ApiKey    bool   `mapstructure:"api_key"`
	JwtExpire int    `mapstructure:"jwt_expire"`
}

type LogConfig struct {
	Level   string `mapstructure:"level"`
	LogPath string `mapstructure:"log_path"`
	MaxSize int    `mapstructure:"max_size"`
	MaxAge  int    `mapstructure:"max_age"`
}

type DatabaseConfig struct {
	Type        string        `mapstructure:"type"` // mysql | postgres
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port"`
	User        string        `mapstructure:"user"`
	Password    string        `mapstructure:"password"`
	DBName      string        `mapstructure:"db_name"`
	SSLMode     string        `mapstructure:"ssl_mode"` // postgres only: disable | require | verify-full
	MaxOpen     int           `mapstructure:"max_open"`
	MaxIdle     int           `mapstructure:"max_idle"`
	MaxLife     time.Duration `mapstructure:"max_life"`
	MaxIdleLife time.Duration `mapstructure:"max_idle_life"`
}

type StorageConfig struct {
	Driver string    `mapstructure:"driver"`
	Local  LocalDisk `mapstructure:"local"`
	MinIO  MinIODisk `mapstructure:"minio"`
}

type LocalDisk struct {
	DataDir string `mapstructure:"data_dir"`
	TempDir string `mapstructure:"temp_dir"`
	BaseURL string `mapstructure:"base_url"`
}

type MinIODisk struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
}

type FileConfig struct {
	Thumbnail        bool  `mapstructure:"thumbnail"`
	BigFileThreshold int64 `mapstructure:"big_file_threshold"`
	BigChunkSize     int64 `mapstructure:"big_chunk_size"`
}

type CorsConfig struct {
	Enable           bool   `mapstructure:"enable"`
	AllowOrigins     string `mapstructure:"allow_origins"`
	AllowMethods     string `mapstructure:"allow_methods"`
	AllowHeaders     string `mapstructure:"allow_headers"`
	AllowCredentials bool   `mapstructure:"allow_credentials"`
	ExposeHeaders    string `mapstructure:"expose_headers"`
}

type CacheConfig struct {
	Type  string        `mapstructure:"type"`
	TTL   time.Duration `mapstructure:"ttl"`
	Redis RedisConfig   `mapstructure:"redis"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type WebDAVConfig struct {
	Enable bool   `mapstructure:"enable"`
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
	Prefix string `mapstructure:"prefix"`
}

type S3Config struct {
	Enable              bool   `mapstructure:"enable"`
	SharePort           bool   `mapstructure:"share_port"`
	Port                int    `mapstructure:"port"`
	Region              string `mapstructure:"region"`
	PathPrefix          string `mapstructure:"path_prefix"`
	EncryptionMasterKey string `mapstructure:"encryption_master_key"`
	OperationTimeout    int    `mapstructure:"operation_timeout"`
}

var (
	global *Config
	once   sync.Once
)

// MustLoad 解析指定路径的 TOML 配置文件并返回配置实例，失败则直接 panic
func MustLoad(path string) *Config {
	once.Do(func() {
		cfg, err := load(path)
		if err != nil {
			panic(fmt.Sprintf("[INITIALIZE] load config err: %v", err))
		}
		global = cfg
	})
	return global
}

// Get 返回全局配置，MustLoad 之前调用会 panic
func Get() *Config {
	if global == nil {
		panic("config: not initialized — call MustLoad first")
	}
	return global
}

func load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	v.SetEnvPrefix("MOLLY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 9527)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")

	// Auth
	v.SetDefault("auth.jwt_expire", 120) // 2h

	// Log
	v.SetDefault("log.level", "info")
	v.SetDefault("log.log_path", "stdout")
	v.SetDefault("log.max_size", 100) // MB
	v.SetDefault("log.max_age", 30)   // days

	// Database
	v.SetDefault("database.type", "sqlite")
	v.SetDefault("database.max_open", 25)
	v.SetDefault("database.max_idle", 10)
	v.SetDefault("database.max_life", "5m")
	v.SetDefault("database.max_idle_life", "10m")

	// Storage
	v.SetDefault("storage.driver", "local")
	v.SetDefault("storage.local.data_dir", "./data/uploads")
	v.SetDefault("storage.local.temp_dir", "./data/tmp")
	v.SetDefault("storage.minio.use_ssl", true)

	// File
	v.SetDefault("file.thumbnail", true)
	v.SetDefault("file.big_file_threshold", 1<<30) // 1 GiB
	v.SetDefault("file.big_chunk_size", 64<<20)    // 64 MiB

	// Cache
	v.SetDefault("cache.type", "memory")
	v.SetDefault("cache.ttl", "10m")
	v.SetDefault("cache.redis.db", 0)
	v.SetDefault("cache.redis.pool_size", 10)

	// WebDAV
	v.SetDefault("webdav.enable", false)
	v.SetDefault("webdav.port", 8081)
	v.SetDefault("webdav.prefix", "/dav")

	// S3 service
	v.SetDefault("s3.enable", false)
	v.SetDefault("s3.share_port", true)
	v.SetDefault("s3.operation_timeout", 30)

}

func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port %d is invalid", cfg.Server.Port)
	}
	if cfg.Auth.Secret == "" {
		return fmt.Errorf("auth.secret must not be empty")
	}
	if len(cfg.Auth.Secret) < 32 {
		return fmt.Errorf("auth.secret must be at least 32 characters")
	}
	if cfg.Database.Type != "mysql" && cfg.Database.Type != "postgres" {
		return fmt.Errorf("database.type %q is not supported (mysql | postgres)", cfg.Database.Type)
	}
	if cfg.Storage.Driver == "minio" {
		m := cfg.Storage.MinIO
		if m.Endpoint == "" || m.Bucket == "" || m.AccessKeyID == "" || m.SecretAccessKey == "" {
			return fmt.Errorf("storage.minio: endpoint, bucket, access_key_id and secret_access_key are required")
		}
	}
	return nil
}
