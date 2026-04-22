package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type FileChunk struct {
	ent.Schema
}

func (FileChunk) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "file_chunk"},
	}
}

func (FileChunk) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).Immutable(),
		field.String("file_id"),
		field.String("chunk_path"),
		field.Uint64("chunk_size"),
		field.String("chunk_hash"),
		field.Uint32("chunk_index").Comment("分块序号，从 0 开始"),
	}
}

func (FileChunk) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", FileInfo.Type).Ref("chunks").Field("file_id").Required().Unique(),
	}
}

func (FileChunk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_id"),
	}
}
