package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UploadPart struct {
	ent.Schema
}

func (UploadPart) Fields() []ent.Field {
	return []ent.Field{
		field.String("upload_id").NotEmpty(),
		field.Int("part_number").Positive(),
		field.Int64("size").NonNegative().Default(0),
		field.String("etag").Optional().Nillable(),
		field.Enum("status").Values("pending", "uploaded").Default("pending"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (UploadPart) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upload_id", "part_number").Unique(),
	}
}
