package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GroupPower struct {
	ent.Schema
}

func (GroupPower) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "group_power"},
	}
}

func (GroupPower) Fields() []ent.Field {
	return []ent.Field{
		field.Int("group_id"),
		field.Int("power_id"),
	}
}

func (GroupPower) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("group", Group.Type).Field("group_id").Required().Unique(),
		edge.To("power", Power.Type).Field("power_id").Required().Unique(),
	}
}

func (GroupPower) Indexes() []ent.Index {
	return []ent.Index{
		// 确保同一组内权限不重复
		index.Fields("group_id", "power_id").Unique(),
	}
}
