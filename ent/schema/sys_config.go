package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SysConfig struct {
	ent.Schema
}

func (SysConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "sys_config"}}
}

func (SysConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique(),
		field.String("value"),
	}
}

func (SysConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key"),
	}
}
