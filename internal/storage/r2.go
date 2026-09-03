package storage

import (
	"bytes"
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mopga/mythblog/internal/config"
)

type R2 struct {
	client             *s3.Client
	bucket, publicBase string
}

func NewR2(cfg config.Config) (*R2, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("auto"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, "")))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(cfg.R2Endpoint); o.UsePathStyle = true })
	return &R2{client: client, bucket: cfg.R2Bucket, publicBase: strings.TrimRight(cfg.R2PublicBaseURL, "/")}, nil
}
func (r *R2) Put(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String(contentType)})
	if err != nil {
		return "", err
	}
	return r.URL(key), nil
}
func (r *R2) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(key)})
	return err
}
func (r *R2) URL(key string) string { return r.publicBase + "/" + key }
