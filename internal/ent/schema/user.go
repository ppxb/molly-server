package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable().Comment("UUID"),
		field.String("nick_name").Comment("昵称"),
		field.String("user_name").Unique().Comment("用户名"),
		field.String("password").Comment("密码"),
		field.String("email").Comment("邮箱"),
		field.String("phone").Default("").Comment("电话号码"),
		field.Int("group_id").Comment("组 ID"),
		field.Int64("space").Default(0).Comment("总存储配额（字节）"),
		field.Int64("free_space").Default(0).Comment("剩余存储空间（字节）"),
		field.String("file_password").Optional().Default("").Comment("文件访问密码"),
		field.Int("state").Default(0).Comment("0=正常 1=禁用"),
		field.Time("created_at").Default(time.Now).Immutable().Comment("创建时间"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).Ref("users").Field("group_id").Required().Unique(),
		edge.To("user_files", UserFile.Type),
		edge.To("api_keys", APIKey.Type),
	}
}
