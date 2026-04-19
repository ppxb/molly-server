package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App           AppConfig           `mapstructure:"app"`
	HTTP          HTTPConfig          `mapstructure:"http"`
	Log           LogConfig           `mapstructure:"log"`
	Database      DatabaseConfig      `mapstructure:"database"`
	RateLimit     RateLimitConfig     `mapstructure:"rate_limit"`
	Upload        UploadConfig        `mapstructure:"upload"`
	ObjectStorage ObjectStorageConfig `mapstructure:"object_storage"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type HTTPConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	ReadTimeoutSeconds     int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds    int    `mapstructure:"write_timeout_seconds"`
	IdleTimeoutSeconds     int    `mapstructure:"idle_timeout_seconds"`
	ShutdownTimeoutSeconds int    `mapstructure:"shutdown_timeout_seconds"`
}

func (c HTTPConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type DatabaseConfig struct {
	DSN                    string `mapstructure:"dsn"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `mapstructure:"conn_max_lifetime_minutes"`
	AutoMigrate            bool   `mapstructure:"auto_migrate"`
}

type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerSecond int  `mapstructure:"requests_per_second"`
	Burst             int  `mapstructure:"burst"`
}

type UploadConfig struct {
	DomainID             string `mapstructure:"domain_id"`
	Location             string `mapstructure:"location"`
	DefaultDriveID       string `mapstructure:"default_drive_id"`
	UploadURLTTLSecs     int    `mapstructure:"upload_url_ttl_secs"`
	DownloadURLTTLSecs   int    `mapstructure:"download_url_ttl_secs"`
	RecycleRetentionDays int    `mapstructure:"recycle_retention_days"`
	SinglePutMaxSize     int64  `mapstructure:"single_put_max_size"`
}

type ObjectStorageConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	Bucket          string `mapstructure:"bucket"`
	ForcePathStyle  bool   `mapstructure:"force_path_style"`
}

func Load() (Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")

	v.SetEnvPrefix("MOLLY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindExternalEnv(v)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}

func bindExternalEnv(v *viper.Viper) {
	_ = v.BindEnv("database.dsn", "DATABASE_URL")
	_ = v.BindEnv("object_storage.endpoint", "S3_ENDPOINT")
	_ = v.BindEnv("object_storage.region", "S3_REGION")
	_ = v.BindEnv("object_storage.access_key_id", "S3_ACCESS_KEY_ID")
	_ = v.BindEnv("object_storage.secret_access_key", "S3_SECRET_ACCESS_KEY")
	_ = v.BindEnv("object_storage.bucket", "S3_BUCKET")

	forcePathStyleRaw := strings.TrimSpace(v.GetString("S3_FORCE_PATH_STYLE"))
	if forcePathStyleRaw == "" {
		return
	}
	parsed, err := strconv.ParseBool(forcePathStyleRaw)
	if err == nil {
		v.Set("object_storage.force_path_style", parsed)
	}
}
