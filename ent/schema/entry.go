package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Entry struct {
	ent.Schema
}

func (Entry) Fields() []ent.Field {
	return []ent.Field{
		field.String("drive_id").NotEmpty(),
		field.String("file_id").NotEmpty(),
		field.String("parent_file_id").NotEmpty().Default("root"),
		field.String("name").NotEmpty(),
		field.Enum("type").Values("file", "folder"),
		field.Int64("size").NonNegative().Default(0),
		field.String("content_hash").Optional().Nillable(),
		field.String("pre_hash").Optional().Nillable(),
		field.String("upload_id").Optional().Nillable(),
		field.String("revision_id").NotEmpty(),
		field.String("encrypt_mode").NotEmpty().Default("none"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Entry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("drive_id", "file_id").Unique(),
		index.Fields("drive_id", "parent_file_id", "name").Unique(),
		index.Fields("drive_id", "parent_file_id"),
	}
}
