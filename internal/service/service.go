package service

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"backup-service/internal/config"
	"backup-service/internal/notify"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Service struct {
	cfg      *config.Config
	s3       *s3.Client
	logger   *slog.Logger
	telegram *notify.Telegram
	locks    *targetLocks
}

func New(cfg *config.Config, logger *slog.Logger) (*Service, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.S3.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3.AccessKeyID,
			cfg.S3.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.S3.ForcePathStyle
		options.BaseEndpoint = aws.String(cfg.S3.Endpoint)
	})

	return &Service{
		cfg:      cfg,
		s3:       client,
		logger:   logger,
		telegram: notify.NewTelegram(cfg),
		locks:    newTargetLocks(),
	}, nil
}

func copySource(bucket, key string) string {
	escaped := url.PathEscape(key)
	escaped = strings.ReplaceAll(escaped, "%2F", "/")
	return bucket + "/" + escaped
}
