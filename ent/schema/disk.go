package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Disk struct {
	ent.Schema
}

func (Disk) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "disk"}}
}

func (Disk) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.Int64("size").Comment("磁盘总大小（字节）"),
		field.String("disk_path").Comment("磁盘挂载路径"),
		field.String("data_path").Comment("数据存储子路径"),
	}
}

func (Disk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("disk_path"),
	}
}
