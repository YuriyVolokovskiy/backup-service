package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"backup-service/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (s *Service) Cleanup(ctx context.Context, target config.Target) error {
	start := time.Now().UTC()
	objects, err := s.List(ctx, target)
	if err != nil {
		s.reportCleanupFailure(ctx, target.ID, time.Since(start), err)
		return err
	}

	deleteKeys := selectCleanupDeletes(objects, target.Retention, start, s.locks.ActiveKey(target.ID))
	s.logger.Info("cleanup delete candidates",
		"target_id", target.ID,
		"delete_count", len(deleteKeys),
		"delete_keys", deleteKeys,
	)

	var failures []error
	for _, key := range deleteKeys {
		_, err := s.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.cfg.S3.Bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", key, err))
			s.logger.Error("cleanup delete failed", "target_id", target.ID, "s3_key", key, "error", err)
			continue
		}
		s.logger.Info("cleanup deleted object", "target_id", target.ID, "s3_key", key)
	}

	duration := time.Since(start)
	if len(failures) > 0 {
		err := fmt.Errorf("cleanup failed to delete %d objects", len(failures))
		s.reportCleanupFailure(ctx, target.ID, duration, err)
		return err
	}

	s.logger.Info("cleanup finished", "target_id", target.ID, "duration", duration.String(), "deleted_count", len(deleteKeys), "status", "success")
	if err := s.telegram.Success(ctx, fmt.Sprintf("cleanup target_id=%s status=success duration=%s deleted_count=%d", target.ID, duration.Round(time.Second), len(deleteKeys))); err != nil {
		s.logger.Warn("telegram cleanup success notification failed", "target_id", target.ID, "error", err)
	}
	return nil
}

func selectCleanupDeletes(objects []BackupObject, retention config.Retention, now time.Time, activeKey string) []string {
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].CreatedAt.After(objects[j].CreatedAt)
	})

	keep := map[string]struct{}{}
	daily := map[string]struct{}{}
	weekly := map[string]struct{}{}
	monthly := map[string]struct{}{}

	for _, object := range objects {
		if object.Key == activeKey {
			keep[object.Key] = struct{}{}
			continue
		}
		if retention.Daily > 0 {
			if key, ok := dailyBucket(object.CreatedAt, now, retention.Daily); ok {
				if _, exists := daily[key]; !exists {
					daily[key] = struct{}{}
					keep[object.Key] = struct{}{}
				}
			}
		}
		if retention.Weekly > 0 {
			if key, ok := weeklyBucket(object.CreatedAt, now, retention.Weekly); ok {
				if _, exists := weekly[key]; !exists {
					weekly[key] = struct{}{}
					keep[object.Key] = struct{}{}
				}
			}
		}
		if retention.Monthly > 0 {
			if key, ok := monthlyBucket(object.CreatedAt, now, retention.Monthly); ok {
				if _, exists := monthly[key]; !exists {
					monthly[key] = struct{}{}
					keep[object.Key] = struct{}{}
				}
			}
		}
	}

	var deletes []string
	for _, object := range objects {
		if _, ok := keep[object.Key]; !ok {
			deletes = append(deletes, object.Key)
		}
	}
	sort.Strings(deletes)
	return deletes
}

func dailyBucket(ts, now time.Time, limit int) (string, bool) {
	tsDay := midnightUTC(ts)
	nowDay := midnightUTC(now)
	diff := int(nowDay.Sub(tsDay).Hours() / 24)
	if diff < 0 || diff >= limit {
		return "", false
	}
	return tsDay.Format("2006-01-02"), true
}

func weeklyBucket(ts, now time.Time, limit int) (string, bool) {
	tsWeek := mondayUTC(ts)
	nowWeek := mondayUTC(now)
	diff := int(nowWeek.Sub(tsWeek).Hours() / (24 * 7))
	if diff < 0 || diff >= limit {
		return "", false
	}
	year, week := ts.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week), true
}

func monthlyBucket(ts, now time.Time, limit int) (string, bool) {
	diff := (now.Year()-ts.Year())*12 + int(now.Month()-ts.Month())
	if diff < 0 || diff >= limit {
		return "", false
	}
	return ts.Format("2006-01"), true
}

func midnightUTC(ts time.Time) time.Time {
	ts = ts.UTC()
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC)
}

func mondayUTC(ts time.Time) time.Time {
	day := midnightUTC(ts)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return day.AddDate(0, 0, -(weekday - 1))
}

func (s *Service) reportCleanupFailure(ctx context.Context, targetID string, duration time.Duration, err error) {
	s.logger.Error("cleanup failed", "target_id", targetID, "duration", duration.String(), "status", "failed", "error", err)
	if notifyErr := s.telegram.Failure(ctx, fmt.Sprintf("cleanup target_id=%s status=failed duration=%s error=%s", targetID, duration.Round(time.Second), err)); notifyErr != nil {
		s.logger.Warn("telegram cleanup failure notification failed", "target_id", targetID, "error", notifyErr)
	}
}
