package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Group struct {
	ent.Schema
}

func (Group) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "groups"},
	}
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Bool("is_default").Default(false).Comment("是否为注册默认组"),
		field.Int64("space").Default(0).Comment("组默认存储配额（字节）"),
		field.Time("created_at").Default(time.Now).Immutable().Comment("创建时间"),
	}
}

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
		edge.To("powers", Power.Type).Through("group_powers", GroupPower.Type),
	}
}
