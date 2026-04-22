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

type Recycled struct {
	ent.Schema
}

func (Recycled) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "recycled"},
	}
}

func (Recycled) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable(),
		field.String("user_id"),
		field.String("file_id"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Recycled) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("file_id"),
	}
}
