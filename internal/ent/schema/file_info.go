package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// FileInfo 文件元信息表（物理文件，与用户无关）
type FileInfo struct {
	ent.Schema
}

func (FileInfo) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "file_info"},
	}
}

func (FileInfo) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable().Comment("UUID"),
		field.String("name").Comment("文件原始名"),
		field.String("random_name").Comment("磁盘存储名（随机生成）"),
		field.Int64("size").Comment("大小"),
		field.String("mime").Comment("MIME"),
		field.String("path").Optional().Default("").Comment("物理存储路径"),
		field.String("enc_path").Default("").Comment("加密文件路径"),
		field.String("thumbnail_img").Optional().Default("").Comment("缩略图"),

		// 哈希与去重
		field.String("file_hash").Comment("全量文件哈希（用于秒传）"),
		field.String("file_enc_hash").Optional().Default("").Comment("加密后文件哈希"),
		field.String("chunk_signature").Optional().Default("").Comment("分片签名（快速预检，非全量）"),
		field.String("first_chunk_hash").Optional().Default(""),
		field.String("second_chunk_hash").Optional().Default(""),
		field.String("third_chunk_hash").Optional().Default(""),
		field.Bool("has_full_hash").Default(false).Comment("是否已完成全量哈希计算"),

		// 加密
		field.Bool("is_enc").Default(false),

		// 分块
		field.Bool("is_chunk").Default(false),
		field.Int("chunk_count").Default(0),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (FileInfo) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("chunks", FileChunk.Type),
	}
}

func (FileInfo) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_hash"),
		index.Fields("chunk_signature"),
		index.Fields("mime"),
		index.Fields("name"),
	}
}
