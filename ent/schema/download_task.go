package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type DownloadTask struct {
	ent.Schema
}

func (DownloadTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "download_task"},
	}
}

// DownloadTaskType 下载任务类型
const (
	DownloadTypeHTTP    = 1
	DownloadTypeTorrent = 2
	DownloadTypeMagnet  = 3
)

// DownloadTaskState 下载任务状态
const (
	DownloadStatePending   = 0
	DownloadStateRunning   = 1
	DownloadStateCompleted = 2
	DownloadStateFailed    = 3
	DownloadStatePaused    = 4
)

func (DownloadTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable(),
		field.String("user_id"),
		field.String("file_id").Optional().Default(""),
		field.String("file_name"),
		field.Int64("file_size").Default(0),
		field.Int64("downloaded_size").Default(0),
		field.Int("progress").Default(0).Comment("0-100"),
		field.Int64("speed").Default(0).Comment("下载速度，字节/秒"),
		field.Int("type").Comment("1=HTTP 2=Torrent 3=Magnet"),
		field.String("url").Optional().Default(""),
		field.String("path").Optional().Default("").Comment("保存路径"),
		field.String("virtual_path").Optional().Default(""),
		field.Int("state").Default(DownloadStatePending),
		field.String("error_msg").Optional().Default(""),
		field.String("target_dir").Optional().Default(""),
		field.Bool("support_range").Default(false).Comment("是否支持断点续传"),
		field.Bool("enable_encryption").Default(false),

		// BT / 磁力链专用字段
		field.String("info_hash").Optional().Default("").Comment("种子 InfoHash"),
		field.Int("file_index").Default(0).Comment("种子内文件索引"),
		field.String("torrent_name").Optional().Default(""),

		field.Time("created_at").Default(time.Now).Immutable().StorageKey("create_time"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).StorageKey("update_time"),
		field.Time("finished_at").Optional().Nillable().StorageKey("finish_time"),
	}
}

func (DownloadTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("info_hash"),
	}
}
