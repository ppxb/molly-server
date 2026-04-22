package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type UserFile struct {
	ent.Schema
}

func (UserFile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_files"},
	}
}

func (UserFile) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable().Comment("用户文件 UUID"),
		field.String("user_id"),
		field.String("file_id"),
		field.String("file_name"),
		field.String("virtual_path").Comment("用户虚拟路径（如 /documents/report.pdf）"),
		field.Bool("is_public").Default(false).StorageKey("public").Comment("是否公开访问"),
		field.Time("created_at").Default(time.Now).Immutable(),
		// 软删除：nil 表示未删除，有值表示已移入回收站
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (UserFile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("user_files").Field("user_id").Required().Unique(),
	}
}

func (UserFile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("file_id"),
		index.Fields("user_id", "virtual_path"),
		index.Fields("user_id", "deleted_at"),
	}
}
