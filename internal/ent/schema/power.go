package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Power struct {
	ent.Schema
}

func (Power) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "power"},
	}
}

func (Power) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Comment("权限名称"),
		field.String("description").Comment("权限描述"),
		field.String("characteristic").Unique().Comment("权限标识符（file:upload、admin:manage）"),
		field.Time("created_at").Default(time.Now).Immutable().Comment("创建时间"),
	}
}

func (Power) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("groups", Group.Type).Ref("powers").Through("group_powers", GroupPower.Type),
	}
}
