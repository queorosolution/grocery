package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"gp/internal/shared/secrets"
)

// keysSecret holds {"accessKeyId": "...", "secretAccessKey": "..."} (docs/13).
const keysSecret = "r2/keys"

// R2 signs links for a Cloudflare R2 bucket through its S3 interface. Signing
// is local maths: no request leaves the Lambda.
type R2 struct {
	accountID, bucket string
	secrets           secrets.Getter
}

func NewR2(accountID, bucket string, s secrets.Getter) *R2 {
	return &R2{accountID: accountID, bucket: bucket, secrets: s}
}

func (r *R2) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, error) {
	if r.accountID == "" || r.bucket == "" {
		return "", errors.New("upload: R2 is not configured")
	}
	raw, err := r.secrets.Get(ctx, keysSecret)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	var keys struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
	}
	if err := json.Unmarshal([]byte(raw), &keys); err != nil || keys.AccessKeyID == "" || keys.SecretAccessKey == "" {
		return "", errors.New("upload: r2/keys is not valid")
	}
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String("https://" + r.accountID + ".r2.cloudflarestorage.com"),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(keys.AccessKeyID, keys.SecretAccessKey, ""),
	})
	out, err := s3.NewPresignClient(client).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("upload: sign: %w", err)
	}
	return out.URL, nil
}
