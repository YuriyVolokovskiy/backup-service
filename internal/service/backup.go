package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"backup-service/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type BackupResult struct {
	TargetID  string        `json:"target_id"`
	Duration  time.Duration `json:"duration"`
	SizeBytes int64         `json:"size_bytes"`
	S3Key     string        `json:"s3_key"`
	Status    string        `json:"status"`
}

func (s *Service) Backup(ctx context.Context, target config.Target) (BackupResult, error) {
	start := time.Now().UTC()
	result := BackupResult{TargetID: target.ID, Status: "failed"}

	if !s.locks.TryLock(target.ID) {
		result.Status = "skipped"
		s.logger.Warn("backup skipped because target is already running", "target_id", target.ID, "status", result.Status)
		return result, nil
	}
	defer s.locks.Unlock(target.ID)

	key := backupKey(target, start)
	partialKey := key + ".partial"
	s.locks.SetActiveKey(target.ID, key)

	tmpPath, err := s.dumpPostgres(ctx, target)
	if err != nil {
		s.reportBackupFailure(ctx, target.ID, time.Since(start), err)
		return result, err
	}
	defer func() {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("temporary backup file removal failed", "target_id", target.ID, "path", tmpPath, "error", err)
		}
	}()

	info, err := os.Stat(tmpPath)
	if err != nil {
		s.reportBackupFailure(ctx, target.ID, time.Since(start), err)
		return result, err
	}

	if err := s.uploadFile(ctx, tmpPath, partialKey); err != nil {
		_, _ = s.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.S3.Bucket), Key: aws.String(partialKey)})
		s.reportBackupFailure(ctx, target.ID, time.Since(start), err)
		return result, err
	}

	if _, err := s.s3.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.cfg.S3.Bucket),
		Key:        aws.String(key),
		CopySource: aws.String(copySource(s.cfg.S3.Bucket, partialKey)),
	}); err != nil {
		_, _ = s.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.S3.Bucket), Key: aws.String(partialKey)})
		s.reportBackupFailure(ctx, target.ID, time.Since(start), err)
		return result, err
	}
	_, _ = s.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.S3.Bucket), Key: aws.String(partialKey)})

	result.Duration = time.Since(start)
	result.SizeBytes = info.Size()
	result.S3Key = key
	result.Status = "success"
	s.logger.Info("backup finished",
		"target_id", target.ID,
		"duration", result.Duration.String(),
		"size_bytes", result.SizeBytes,
		"s3_key", result.S3Key,
		"status", result.Status,
	)
	if err := s.telegram.Success(ctx, fmt.Sprintf("backup target_id=%s status=success duration=%s size_bytes=%d s3_key=%s", target.ID, result.Duration.Round(time.Second), result.SizeBytes, result.S3Key)); err != nil {
		s.logger.Warn("telegram success notification failed", "target_id", target.ID, "error", err)
	}
	return result, nil
}

func (s *Service) dumpPostgres(ctx context.Context, target config.Target) (string, error) {
	databaseURL, err := target.DatabaseURL()
	if err != nil {
		return "", err
	}

	file, err := os.CreateTemp("", "backup-service-*.dump")
	if err != nil {
		return "", err
	}
	tmpPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	args := []string{
		"--format=custom",
		fmt.Sprintf("--compress=%d", target.CompressionLevel),
		"--dbname=" + databaseURL,
		"--file=" + tmpPath,
	}
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("pg_dump failed for target %s: %w: %s", target.ID, err, sanitizeCommandOutput(string(output), databaseURL))
	}
	return tmpPath, nil
}

func (s *Service) uploadFile(ctx context.Context, path, key string) error {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			s.logger.Warn("backup file close failed", "path", path, "s3_key", key, "error", err)
		}
	}()

	_, err = s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.cfg.S3.Bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (s *Service) reportBackupFailure(ctx context.Context, targetID string, duration time.Duration, err error) {
	s.logger.Error("backup failed",
		"target_id", targetID,
		"duration", duration.String(),
		"status", "failed",
		"error", err,
	)
	if notifyErr := s.telegram.Failure(ctx, fmt.Sprintf("backup target_id=%s status=failed duration=%s error=%s", targetID, duration.Round(time.Second), err)); notifyErr != nil {
		s.logger.Warn("telegram failure notification failed", "target_id", targetID, "error", notifyErr)
	}
}

func backupKey(target config.Target, ts time.Time) string {
	return fmt.Sprintf("%s/%s/%s-%s.dump",
		target.CleanPrefix(),
		ts.Format("2006/01/02"),
		target.ID,
		ts.Format("20060102T150405Z"),
	)
}

func sanitizeCommandOutput(output, databaseURL string) string {
	output = strings.ReplaceAll(output, databaseURL, "[redacted_database_url]")
	if len(output) > 500 {
		return output[:500]
	}
	return output
}

func (r BackupResult) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("target_id", r.TargetID),
		slog.String("duration", r.Duration.String()),
		slog.Int64("size_bytes", r.SizeBytes),
		slog.String("s3_key", r.S3Key),
		slog.String("status", r.Status),
	}
}
