package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"molly-server/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type CompletedPart struct {
	PartNumber int32
	ETag       string
}

type UploadedPart struct {
	PartNumber int32
	ETag       string
	Size       int64
}

type Client interface {
	CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error)
	PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error)
	ListUploadedParts(ctx context.Context, key, uploadID string) ([]UploadedPart, error)
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error
	DeleteObject(ctx context.Context, key string) error
	OpenObject(ctx context.Context, key string) (io.ReadCloser, error)
	PresignGetObject(ctx context.Context, key, disposition string, expires time.Duration) (string, error)
	PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error)
}

type client struct {
	bucket   string
	s3Client *s3.Client
	presign  *s3.PresignClient
}

func New(cfg config.ObjectStorageConfig) (Client, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("object storage endpoint is required")
	}

	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("object storage bucket is required")
	}

	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	secretAccessKey := strings.TrimSpace(cfg.SecretAccessKey)
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, fmt.Errorf("object storage access key id and secret access key are required")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	awsConfig, err := awscfg.LoadDefaultConfig(
		context.Background(),
		awscfg.WithRegion(region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		awscfg.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = cfg.ForcePathStyle
		options.BaseEndpoint = aws.String(endpoint)
	})

	return &client{
		bucket:   bucket,
		s3Client: s3Client,
		presign:  s3.NewPresignClient(s3Client),
	}, nil
}

func (c *client) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(strings.TrimSpace(key)),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(strings.TrimSpace(contentType))
	}

	output, err := c.s3Client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}
	if strings.TrimSpace(aws.ToString(output.UploadId)) == "" {
		return "", fmt.Errorf("create multipart upload: empty upload id")
	}

	return aws.ToString(output.UploadId), nil
}

func (c *client) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	if strings.TrimSpace(uploadID) == "" {
		return "", fmt.Errorf("upload id is required")
	}
	if partNumber <= 0 {
		return "", fmt.Errorf("part number must be positive")
	}

	request, err := c.presign.PresignUploadPart(
		ctx,
		&s3.UploadPartInput{
			Bucket:     aws.String(c.bucket),
			Key:        aws.String(strings.TrimSpace(key)),
			UploadId:   aws.String(strings.TrimSpace(uploadID)),
			PartNumber: aws.Int32(partNumber),
		},
		func(options *s3.PresignOptions) {
			options.Expires = normalizePresignExpiry(expires)
		},
	)
	if err != nil {
		return "", fmt.Errorf("presign upload part: %w", err)
	}

	return request.URL, nil
}

func (c *client) ListUploadedParts(ctx context.Context, key, uploadID string) ([]UploadedPart, error) {
	if strings.TrimSpace(uploadID) == "" {
		return nil, fmt.Errorf("upload id is required")
	}

	input := &s3.ListPartsInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(strings.TrimSpace(key)),
		UploadId: aws.String(strings.TrimSpace(uploadID)),
		MaxParts: aws.Int32(1000),
	}

	parts := make([]UploadedPart, 0, 16)
	for {
		output, err := c.s3Client.ListParts(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list uploaded parts: %w", err)
		}

		for _, part := range output.Parts {
			eTag := strings.Trim(aws.ToString(part.ETag), "\"")
			if aws.ToInt32(part.PartNumber) <= 0 || eTag == "" {
				continue
			}
			parts = append(parts, UploadedPart{
				PartNumber: aws.ToInt32(part.PartNumber),
				ETag:       eTag,
				Size:       aws.ToInt64(part.Size),
			})
		}

		nextMarker, err := strconv.ParseInt(strings.TrimSpace(aws.ToString(output.NextPartNumberMarker)), 10, 32)
		if err != nil || !aws.ToBool(output.IsTruncated) || nextMarker <= 0 {
			break
		}
		nextMarkerString := strconv.FormatInt(nextMarker, 10)
		input.PartNumberMarker = aws.String(nextMarkerString)
	}

	return parts, nil
}

func (c *client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	if strings.TrimSpace(uploadID) == "" {
		return fmt.Errorf("upload id is required")
	}
	if len(parts) == 0 {
		return fmt.Errorf("completed parts are required")
	}

	completedParts := make([]s3types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		eTag := strings.TrimSpace(part.ETag)
		if eTag == "" {
			return fmt.Errorf("part %d has empty etag", part.PartNumber)
		}
		completedParts = append(completedParts, s3types.CompletedPart{
			ETag:       aws.String(eTag),
			PartNumber: aws.Int32(part.PartNumber),
		})
	}

	_, err := c.s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(strings.TrimSpace(key)),
		UploadId: aws.String(strings.TrimSpace(uploadID)),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}

	return nil
}

func (c *client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	if strings.TrimSpace(uploadID) == "" {
		return fmt.Errorf("upload id is required")
	}

	_, err := c.s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(strings.TrimSpace(key)),
		UploadId: aws.String(strings.TrimSpace(uploadID)),
	})
	if err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}

	return nil
}

func (c *client) DeleteObject(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("object key is required")
	}

	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(strings.TrimSpace(key)),
	})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}

	return nil
}

func (c *client) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(strings.TrimSpace(key)),
	})
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}

	return output.Body, nil
}

func (c *client) PresignGetObject(ctx context.Context, key, disposition string, expires time.Duration) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(strings.TrimSpace(key)),
	}
	if strings.TrimSpace(disposition) != "" {
		input.ResponseContentDisposition = aws.String(strings.TrimSpace(disposition))
	}

	request, err := c.presign.PresignGetObject(ctx, input, func(options *s3.PresignOptions) {
		options.Expires = normalizePresignExpiry(expires)
	})
	if err != nil {
		return "", fmt.Errorf("presign get object: %w", err)
	}

	return request.URL, nil
}

func (c *client) PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(strings.TrimSpace(key)),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(strings.TrimSpace(contentType))
	}

	request, err := c.presign.PresignPutObject(ctx, input, func(options *s3.PresignOptions) {
		options.Expires = normalizePresignExpiry(expires)
	})
	if err != nil {
		return "", fmt.Errorf("presign put object: %w", err)
	}

	return request.URL, nil
}

func normalizePresignExpiry(expires time.Duration) time.Duration {
	if expires <= 0 {
		return 15 * time.Minute
	}
	if expires > 7*24*time.Hour {
		return 7 * 24 * time.Hour
	}
	return expires
}

func IsLikelyObjectStorageUploadURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func BuildContentDisposition(mode, fileName string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "preview" {
		return "inline"
	}
	if fileName == "" {
		return "attachment"
	}
	return fmt.Sprintf("attachment; filename=\"%s\"", fileName)
}

func BuildCORSHeaders(w http.ResponseWriter, origin string) {
	if strings.TrimSpace(origin) == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-Id")
	w.Header().Set("Access-Control-Expose-Headers", "ETag")
}
