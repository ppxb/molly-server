package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Drive struct {
	ent.Schema
}

func (Drive) Fields() []ent.Field {
	return []ent.Field{
		field.String("drive_id").NotEmpty().Unique().Immutable(),
		field.String("name").NotEmpty().Default("default"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
