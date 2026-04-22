package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ─────────────────────────────────────────────
// S3Bucket  存储桶（对应用户虚拟目录）
// ─────────────────────────────────────────────

type S3Bucket struct{ ent.Schema }

func (S3Bucket) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_buckets"}}
}

func (S3Bucket) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("user_id").MaxLen(36),
		field.String("region").Default("us-east-1").MaxLen(32),
		field.Int("virtual_path_id"),
		field.String("versioning").Default("Disabled").MaxLen(16).
			Comment("Enabled | Suspended | Disabled"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (S3Bucket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bucket_name", "user_id").Unique(),
		index.Fields("user_id"),
	}
}

// ─────────────────────────────────────────────
// S3Object  对象元数据（扩展 FileInfo）
// ─────────────────────────────────────────────

type S3Object struct{ ent.Schema }

func (S3Object) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_object_metadata"}}
}

func (S3Object) Fields() []ent.Field {
	return []ent.Field{
		field.String("file_id").MaxLen(36).Optional().Default("").
			Comment("关联 FileInfo.id；DeleteMarker 时为空"),
		field.String("bucket_name").MaxLen(63),
		field.String("object_key").MaxLen(1024),
		field.String("user_id").MaxLen(36),
		field.String("etag").MaxLen(64).Optional().Default(""),
		field.String("storage_class").Default("STANDARD").MaxLen(32),
		field.String("content_type").MaxLen(256).Optional().Default(""),
		field.String("user_metadata").Optional().Default("").
			Comment("JSON: x-amz-meta-* 自定义元数据"),
		field.String("tags").Optional().Default("").
			Comment("JSON: 对象标签"),
		field.String("version_id").MaxLen(36).Optional().Default(""),
		field.Bool("is_latest").Default(true),
		field.Bool("is_delete_marker").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (S3Object) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bucket_name", "object_key"),
		index.Fields("user_id"),
		index.Fields("version_id"),
		index.Fields("is_latest"),
		index.Fields("is_delete_marker"),
	}
}

// ─────────────────────────────────────────────
// S3MultipartUpload  分片上传会话
// ─────────────────────────────────────────────

type S3MultipartUpload struct{ ent.Schema }

func (S3MultipartUpload) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_multipart_uploads"}}
}

func (S3MultipartUpload) Fields() []ent.Field {
	return []ent.Field{
		field.String("upload_id").MaxLen(64).Unique().Immutable(),
		field.String("bucket_name").MaxLen(63),
		field.String("object_key").MaxLen(1024),
		field.String("user_id").MaxLen(36),
		field.String("metadata").Optional().Default("").Comment("JSON 元数据"),
		field.String("status").Default("in-progress").MaxLen(32).
			Comment("in-progress | completed | aborted"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (S3MultipartUpload) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bucket_name"),
		index.Fields("object_key"),
		index.Fields("user_id"),
	}
}

// ─────────────────────────────────────────────
// S3MultipartPart  分片信息
// ─────────────────────────────────────────────

type S3MultipartPart struct{ ent.Schema }

func (S3MultipartPart) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_multipart_parts"}}
}

func (S3MultipartPart) Fields() []ent.Field {
	return []ent.Field{
		field.String("upload_id").MaxLen(64),
		field.Int("part_number"),
		field.String("etag").MaxLen(64),
		field.Int64("size"),
		field.String("chunk_path").MaxLen(512).Optional().Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (S3MultipartPart) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upload_id", "part_number").Unique(),
	}
}

// ─────────────────────────────────────────────
// S3BucketCORS / ACL / Policy / Lifecycle
// 均为单桶单配置（唯一约束在 bucket_name+user_id）
// ─────────────────────────────────────────────

type S3BucketCORS struct{ ent.Schema }

func (S3BucketCORS) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_bucket_cors"}}
}
func (S3BucketCORS) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("user_id").MaxLen(36),
		field.String("cors_config").Comment("JSON CORS 规则数组"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3BucketCORS) Indexes() []ent.Index {
	return []ent.Index{index.Fields("bucket_name", "user_id").Unique()}
}

// ---

type S3BucketACL struct{ ent.Schema }

func (S3BucketACL) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_bucket_acl"}}
}
func (S3BucketACL) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("user_id").MaxLen(36),
		field.String("acl_config").Comment("JSON ACL 配置"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3BucketACL) Indexes() []ent.Index {
	return []ent.Index{index.Fields("bucket_name", "user_id").Unique()}
}

// ---

type S3ObjectACL struct{ ent.Schema }

func (S3ObjectACL) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_object_acl"}}
}
func (S3ObjectACL) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("object_key").MaxLen(1024),
		field.String("version_id").MaxLen(36).Optional().Default(""),
		field.String("user_id").MaxLen(36),
		field.String("acl_config").Comment("JSON ACL 配置"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3ObjectACL) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bucket_name", "object_key", "version_id", "user_id"),
	}
}

// ---

type S3BucketPolicy struct{ ent.Schema }

func (S3BucketPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_bucket_policy"}}
}
func (S3BucketPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("user_id").MaxLen(36),
		field.String("policy_json"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3BucketPolicy) Indexes() []ent.Index {
	return []ent.Index{index.Fields("bucket_name", "user_id").Unique()}
}

// ---

type S3BucketLifecycle struct{ ent.Schema }

func (S3BucketLifecycle) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_bucket_lifecycle"}}
}
func (S3BucketLifecycle) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("user_id").MaxLen(36),
		field.String("lifecycle_json"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3BucketLifecycle) Indexes() []ent.Index {
	return []ent.Index{index.Fields("bucket_name", "user_id").Unique()}
}

// ─────────────────────────────────────────────
// S3EncryptionKey / S3ObjectEncryption
// ─────────────────────────────────────────────

type S3EncryptionKey struct{ ent.Schema }

func (S3EncryptionKey) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_encryption_keys"}}
}
func (S3EncryptionKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("key_id").MaxLen(64).Unique(),
		field.String("key_data").Comment("加密后的密钥（base64）"),
		field.String("algorithm").Default("AES256").MaxLen(32),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// ---

type S3ObjectEncryption struct{ ent.Schema }

func (S3ObjectEncryption) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "s3_object_encryption"}}
}
func (S3ObjectEncryption) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket_name").MaxLen(63),
		field.String("object_key").MaxLen(1024),
		field.String("version_id").MaxLen(36).Optional().Default(""),
		field.String("user_id").MaxLen(36),
		field.String("encryption_type").MaxLen(32).Comment("SSE-S3 | SSE-C | SSE-KMS"),
		field.String("algorithm").Default("AES256").MaxLen(32),
		field.String("key_id").MaxLen(64).Optional().Default(""),
		field.String("encrypted_key").Optional().Default("").Comment("SSE-C 加密密钥"),
		field.String("iv").MaxLen(64).Optional().Default("").Comment("初始化向量（base64）"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
func (S3ObjectEncryption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bucket_name", "object_key", "version_id"),
		index.Fields("user_id"),
	}
}
