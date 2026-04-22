package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UploadSession struct {
	ent.Schema
}

func (UploadSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("drive_id").NotEmpty(),
		field.String("upload_id").NotEmpty().Unique().Immutable(),
		field.String("file_id").NotEmpty(),
		field.Int("part_count").Positive(),
		field.Int64("chunk_size").NonNegative().Default(0),
		field.Enum("status").Values("init", "uploading", "completed", "aborted").Default("init"),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (UploadSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("drive_id", "file_id"),
	}
}
