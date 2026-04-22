package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VirtualPath struct {
	ent.Schema
}

func (VirtualPath) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "virtual_path"},
	}
}

func (VirtualPath) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.String("path"),
		field.Bool("is_file").Default(false),
		field.Bool("is_dir").Default(true),
		field.String("parent_level").Optional().Default("").Comment("父级路径层级，用于快速查询子树"),
		field.Time("created_at").Default(time.Now).Immutable().StorageKey("created_time"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).StorageKey("update_time"),
	}
}

func (VirtualPath) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("user_id", "path"),
	}
}
