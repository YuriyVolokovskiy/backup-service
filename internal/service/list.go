package service

import (
	"context"
	"path"
	"strings"
	"time"

	"backup-service/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type BackupObject struct {
	Key          string    `json:"key"`
	SizeBytes    int64     `json:"size_bytes"`
	LastModified time.Time `json:"last_modified"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Service) List(ctx context.Context, target config.Target) ([]BackupObject, error) {
	prefix := target.CleanPrefix() + "/"
	paginator := s3.NewListObjectsV2Paginator(s.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.cfg.S3.Bucket),
		Prefix: aws.String(prefix),
	})

	var objects []BackupObject
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Contents {
			if item.Key == nil || strings.HasSuffix(*item.Key, ".partial") || !strings.HasSuffix(*item.Key, ".dump") {
				continue
			}
			createdAt, ok := parseBackupTimestamp(target, *item.Key)
			if !ok {
				continue
			}
			lastModified := time.Time{}
			if item.LastModified != nil {
				lastModified = *item.LastModified
			}
			objects = append(objects, BackupObject{
				Key:          *item.Key,
				SizeBytes:    *item.Size,
				LastModified: lastModified,
				CreatedAt:    createdAt,
			})
		}
	}
	return objects, nil
}

func parseBackupTimestamp(target config.Target, key string) (time.Time, bool) {
	base := path.Base(key)
	prefix := target.ID + "-"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, ".dump") {
		return time.Time{}, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(base, prefix), ".dump")
	ts, err := time.Parse("20060102T150405Z", value)
	return ts, err == nil
}
