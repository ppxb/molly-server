package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type APIKey struct {
	ent.Schema
}

func (APIKey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_key"},
	}
}

func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id").Comment("用户 ID"),
		field.String("key").Unique().Comment("API 访问密钥"),
		field.String("private_key").Comment("RSA 私钥"),
		field.String("s3_secret_key").Comment("S3 Secret Key"),
		field.Time("expires_at").Optional().Nillable().Comment("过期时间（nil 表示永不过期）"),
		field.Time("created_at").Default(time.Now).Immutable().Comment("创建时间"),
	}
}

func (APIKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("api_keys").Field("user_id").Required().Unique(),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
