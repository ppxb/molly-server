package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UploadChunk struct {
	ent.Schema
}

func (UploadChunk) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upload_chunk"},
	}
}

func (UploadChunk) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.String("file_name"),
		field.Int64("file_size"),
		field.String("md5").Comment("分片 MD5，用于完整性校验"),
		field.String("path_id").Comment("关联的上传任务路径 ID"),
	}
}

func (UploadChunk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("path_id"),
	}
}
