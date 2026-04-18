package objectstorage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"molly-server/internal/config"
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
	PresignGetObject(ctx context.Context, key, disposition string, expires time.Duration) (string, error)
	PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error)
}

type client struct {
	bucket          string
	endpoint        *url.URL
	region          string
	accessKeyID     string
	secretAccessKey string
	forcePathStyle  bool
	httpClient      *http.Client
}

type initiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	UploadID string   `xml:"UploadId"`
}

type listPartsResult struct {
	XMLName              xml.Name              `xml:"ListPartsResult"`
	IsTruncated          bool                  `xml:"IsTruncated"`
	NextPartNumberMarker int                   `xml:"NextPartNumberMarker"`
	Parts                []listPartsResultPart `xml:"Part"`
}

type listPartsResultPart struct {
	PartNumber int32  `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
	Size       int64  `xml:"Size"`
}

type completeMultipartUploadRequest struct {
	XMLName xml.Name                      `xml:"CompleteMultipartUpload"`
	Parts   []completeMultipartUploadPart `xml:"Part"`
}

type completeMultipartUploadPart struct {
	PartNumber int32  `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

func New(cfg config.ObjectStorageConfig) (Client, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("object storage endpoint is required")
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse object storage endpoint: %w", err)
	}
	if parsedEndpoint.Scheme == "" || parsedEndpoint.Host == "" {
		return nil, fmt.Errorf("invalid object storage endpoint")
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

	return &client{
		bucket:          bucket,
		endpoint:        parsedEndpoint,
		region:          region,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		forcePathStyle:  cfg.ForcePathStyle,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (c *client) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	query := url.Values{}
	query.Set("uploads", "")

	headers := map[string]string{}
	if strings.TrimSpace(contentType) != "" {
		headers["content-type"] = strings.TrimSpace(contentType)
	}

	body, _, err := c.doSignedRequest(ctx, http.MethodPost, key, query, nil, headers)
	if err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}

	var result initiateMultipartUploadResult
	if err := xml.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("create multipart upload: decode response: %w", err)
	}
	if strings.TrimSpace(result.UploadID) == "" {
		return "", fmt.Errorf("create multipart upload: empty upload id")
	}

	return strings.TrimSpace(result.UploadID), nil
}

func (c *client) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	_ = ctx
	if strings.TrimSpace(uploadID) == "" {
		return "", fmt.Errorf("upload id is required")
	}
	if partNumber <= 0 {
		return "", fmt.Errorf("part number must be positive")
	}

	query := url.Values{}
	query.Set("uploadId", strings.TrimSpace(uploadID))
	query.Set("partNumber", strconv.FormatInt(int64(partNumber), 10))
	return c.presignURL(http.MethodPut, key, query, expires)
}

func (c *client) ListUploadedParts(ctx context.Context, key, uploadID string) ([]UploadedPart, error) {
	if strings.TrimSpace(uploadID) == "" {
		return nil, fmt.Errorf("upload id is required")
	}

	var (
		allParts   []UploadedPart
		nextMarker int
	)

	for {
		query := url.Values{}
		query.Set("uploadId", strings.TrimSpace(uploadID))
		query.Set("max-parts", "1000")
		if nextMarker > 0 {
			query.Set("part-number-marker", strconv.Itoa(nextMarker))
		}

		body, _, err := c.doSignedRequest(ctx, http.MethodGet, key, query, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("list uploaded parts: %w", err)
		}

		var result listPartsResult
		if err := xml.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("list uploaded parts: decode response: %w", err)
		}

		for _, item := range result.Parts {
			eTag := strings.TrimSpace(item.ETag)
			eTag = strings.Trim(eTag, "\"")
			allParts = append(allParts, UploadedPart{
				PartNumber: item.PartNumber,
				ETag:       eTag,
				Size:       item.Size,
			})
		}

		if !result.IsTruncated || result.NextPartNumberMarker <= 0 {
			break
		}
		nextMarker = result.NextPartNumberMarker
	}

	sort.SliceStable(allParts, func(i, j int) bool {
		return allParts[i].PartNumber < allParts[j].PartNumber
	})

	return allParts, nil
}

func (c *client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	if strings.TrimSpace(uploadID) == "" {
		return fmt.Errorf("upload id is required")
	}
	if len(parts) == 0 {
		return fmt.Errorf("completed parts are required")
	}

	sort.SliceStable(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	payload := completeMultipartUploadRequest{
		Parts: make([]completeMultipartUploadPart, 0, len(parts)),
	}
	for _, part := range parts {
		eTag := strings.TrimSpace(part.ETag)
		if eTag == "" {
			return fmt.Errorf("part %d has empty etag", part.PartNumber)
		}
		if !strings.HasPrefix(eTag, "\"") {
			eTag = `"` + eTag
		}
		if !strings.HasSuffix(eTag, "\"") {
			eTag = eTag + `"`
		}
		payload.Parts = append(payload.Parts, completeMultipartUploadPart{
			PartNumber: part.PartNumber,
			ETag:       eTag,
		})
	}

	xmlBody, err := xml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("complete multipart upload: encode body: %w", err)
	}

	query := url.Values{}
	query.Set("uploadId", strings.TrimSpace(uploadID))
	headers := map[string]string{
		"content-type": "application/xml",
	}
	if _, _, err := c.doSignedRequest(ctx, http.MethodPost, key, query, xmlBody, headers); err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}

	return nil
}

func (c *client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	if strings.TrimSpace(uploadID) == "" {
		return fmt.Errorf("upload id is required")
	}

	query := url.Values{}
	query.Set("uploadId", strings.TrimSpace(uploadID))
	if _, _, err := c.doSignedRequest(ctx, http.MethodDelete, key, query, nil, nil); err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}
	return nil
}

func (c *client) PresignGetObject(ctx context.Context, key, disposition string, expires time.Duration) (string, error) {
	_ = ctx
	query := url.Values{}
	if strings.TrimSpace(disposition) != "" {
		query.Set("response-content-disposition", strings.TrimSpace(disposition))
	}
	return c.presignURL(http.MethodGet, key, query, expires)
}

func (c *client) PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	_ = ctx
	return c.presignURL(http.MethodPut, key, url.Values{}, expires)
}

func (c *client) doSignedRequest(
	ctx context.Context,
	method string,
	key string,
	query url.Values,
	body []byte,
	extraHeaders map[string]string,
) ([]byte, http.Header, error) {
	requestURL, canonicalURI, host, err := c.buildObjectURL(key, query)
	if err != nil {
		return nil, nil, err
	}

	payloadHash := sha256Hex(body)
	timestamp := time.Now().UTC()
	amzDate := timestamp.Format("20060102T150405Z")
	dateStamp := timestamp.Format("20060102")
	credentialScope := dateStamp + "/" + c.region + "/s3/aws4_request"

	canonicalQuery := canonicalQueryString(query)
	headers := map[string]string{
		"host":                 host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	for key, value := range extraHeaders {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		headers[normalizedKey] = strings.TrimSpace(value)
	}

	signedHeaders, canonicalHeaders := canonicalHeadersString(headers)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(c.secretAccessKey, dateStamp, c.region, "s3"), stringToSign))
	authorization := strings.Join([]string{
		"AWS4-HMAC-SHA256 Credential=" + c.accessKeyID + "/" + credentialScope,
		"SignedHeaders=" + signedHeaders,
		"Signature=" + signature,
	}, ", ")

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build signed request: %w", err)
	}
	for key, value := range headers {
		if key == "host" {
			req.Host = value
			continue
		}
		req.Header.Set(key, value)
	}
	req.Header.Set("Authorization", authorization)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("send signed request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read signed request response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("object storage response status %d: %s", resp.StatusCode, truncateString(string(responseBody), 512))
	}

	return responseBody, resp.Header, nil
}

func (c *client) presignURL(method string, key string, query url.Values, expires time.Duration) (string, error) {
	requestURL, canonicalURI, host, err := c.buildObjectURL(key, query)
	if err != nil {
		return "", err
	}

	expiresSeconds := int(expires.Seconds())
	if expiresSeconds <= 0 {
		expiresSeconds = 900
	}
	if expiresSeconds > 604800 {
		expiresSeconds = 604800
	}

	timestamp := time.Now().UTC()
	amzDate := timestamp.Format("20060102T150405Z")
	dateStamp := timestamp.Format("20060102")
	credentialScope := dateStamp + "/" + c.region + "/s3/aws4_request"

	presignQuery := cloneURLValues(query)
	presignQuery.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	presignQuery.Set("X-Amz-Credential", c.accessKeyID+"/"+credentialScope)
	presignQuery.Set("X-Amz-Date", amzDate)
	presignQuery.Set("X-Amz-Expires", strconv.Itoa(expiresSeconds))
	presignQuery.Set("X-Amz-SignedHeaders", "host")

	canonicalQuery := canonicalQueryString(presignQuery)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		"host:" + strings.TrimSpace(strings.ToLower(host)) + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(c.secretAccessKey, dateStamp, c.region, "s3"), stringToSign))
	presignQuery.Set("X-Amz-Signature", signature)

	requestURL.RawQuery = canonicalQueryString(presignQuery)
	return requestURL.String(), nil
}

func (c *client) buildObjectURL(key string, query url.Values) (*url.URL, string, string, error) {
	objectKey := encodeObjectKey(strings.TrimSpace(key))
	if objectKey == "" {
		return nil, "", "", fmt.Errorf("object key is required")
	}

	basePath := strings.TrimSuffix(c.endpoint.EscapedPath(), "/")
	var host string
	var objectPath string
	if c.forcePathStyle {
		host = c.endpoint.Host
		parts := []string{basePath, "/" + awsEscapePath(c.bucket), "/" + objectKey}
		objectPath = joinPathParts(parts...)
	} else {
		host = c.bucket + "." + c.endpoint.Host
		parts := []string{basePath, "/" + objectKey}
		objectPath = joinPathParts(parts...)
	}

	fullURL := &url.URL{
		Scheme:   c.endpoint.Scheme,
		Host:     host,
		Path:     objectPath,
		RawPath:  objectPath,
		RawQuery: canonicalQueryString(query),
	}

	return fullURL, objectPath, host, nil
}

func canonicalHeadersString(headers map[string]string) (signedHeaders string, canonicalHeaders string) {
	keys := make([]string, 0, len(headers))
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		normalizedValue := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		normalized[normalizedKey] = normalizedValue
		keys = append(keys, normalizedKey)
	}
	sort.Strings(keys)

	uniqueKeys := keys[:0]
	var previous string
	for _, key := range keys {
		if key == previous {
			continue
		}
		uniqueKeys = append(uniqueKeys, key)
		previous = key
	}
	keys = uniqueKeys

	builder := strings.Builder{}
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(":")
		builder.WriteString(normalized[key])
		builder.WriteString("\n")
	}

	return strings.Join(keys, ";"), builder.String()
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	type kv struct {
		Key   string
		Value string
	}
	pairs := make([]kv, 0, len(values))
	for key, rawValues := range values {
		if len(rawValues) == 0 {
			pairs = append(pairs, kv{Key: key, Value: ""})
			continue
		}
		for _, value := range rawValues {
			pairs = append(pairs, kv{Key: key, Value: value})
		}
	}

	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].Key == pairs[j].Key {
			return pairs[i].Value < pairs[j].Value
		}
		return pairs[i].Key < pairs[j].Key
	})

	encoded := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		encoded = append(encoded, awsEscape(pair.Key)+"="+awsEscape(pair.Value))
	}
	return strings.Join(encoded, "&")
}

func cloneURLValues(source url.Values) url.Values {
	if len(source) == 0 {
		return url.Values{}
	}
	target := make(url.Values, len(source))
	for key, values := range source {
		clonedValues := make([]string, len(values))
		copy(clonedValues, values)
		target[key] = clonedValues
	}
	return target
}

func encodeObjectKey(key string) string {
	if key == "" {
		return ""
	}

	parts := strings.Split(key, "/")
	encodedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		encodedParts = append(encodedParts, awsEscapePath(part))
	}
	return strings.Join(encodedParts, "/")
}

func joinPathParts(parts ...string) string {
	if len(parts) == 0 {
		return "/"
	}
	joined := strings.Join(parts, "")
	if joined == "" || joined[0] != '/' {
		joined = "/" + joined
	}
	return strings.ReplaceAll(joined, "//", "/")
}

func awsEscape(raw string) string {
	escaped := url.QueryEscape(raw)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func awsEscapePath(raw string) string {
	escaped := url.PathEscape(raw)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func sha256Hex(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key []byte, payload string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func signingKey(secretAccessKey, date, region, service string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+secretAccessKey), date)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, service)
	return hmacSHA256(serviceKey, "aws4_request")
}

func truncateString(value string, maxLength int) string {
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
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
