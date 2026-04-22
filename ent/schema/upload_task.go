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

type UploadTask struct {
	ent.Schema
}

func (UploadTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upload_task"},
	}
}

// UploadStatus 上传任务状态枚举
const (
	UploadStatusPending   = "pending"
	UploadStatusUploading = "uploading"
	UploadStatusCompleted = "completed"
	UploadStatusFailed    = "failed"
	UploadStatusAborted   = "aborted"
)

func (UploadTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable().Comment("预检 ID（precheck_id），前端用于断点续传标识"),
		field.String("user_id"),
		field.String("file_name"),
		field.Int64("file_size"),
		field.Int64("chunk_size").Default(5 << 20).Comment("分片大小，默认 5MB"),
		field.Int("total_chunks"),
		field.Int("uploaded_chunks").Default(0),
		field.String("chunk_signature").Optional().Default("").Comment("分片签名，用于秒传检测"),
		field.String("path_id").Comment("目标虚拟路径 ID"),
		field.String("temp_dir").Comment("服务端临时分片目录"),
		field.String("status").Default(UploadStatusPending).Comment("pending | uploading | completed | failed | aborted"),
		field.String("error_message").Optional().Default(""),
		field.Time("created_at").Default(time.Now).Immutable().StorageKey("create_time"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).StorageKey("update_time"),
		field.Time("expires_at").StorageKey("expire_time").Comment("任务过期时间，默认 7 天后自动清理"),
	}
}

func (UploadTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
