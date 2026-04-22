package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Share struct {
	ent.Schema
}

func (Share) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "shares"},
	}
}

func (Share) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.String("file_id"),
		field.String("token").Unique(),
		field.String("password_hash").Default("").Comment("访问密码 bcrypt 哈希，空字符串表示无密码"),
		field.Int("download_count").Default(0),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Share) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("token"),
	}
}
