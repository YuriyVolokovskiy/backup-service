package service

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
)

func (s *Service) Serve(ctx context.Context) error {
	if err := s.cfg.ValidateDatabaseURLs(); err != nil {
		return err
	}

	scheduler := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	for _, target := range s.cfg.Targets {
		target := target
		if _, err := scheduler.AddFunc(target.BackupCron, func() {
			if _, err := s.Backup(ctx, target); err != nil {
				s.logger.Error("scheduled backup failed", "target_id", target.ID, "error", err)
			}
		}); err != nil {
			return err
		}
		if _, err := scheduler.AddFunc(target.CleanupCron, func() {
			if err := s.Cleanup(ctx, target); err != nil {
				s.logger.Error("scheduled cleanup failed", "target_id", target.ID, "error", err)
			}
		}); err != nil {
			return err
		}
		s.logger.Info("registered target jobs",
			"target_id", target.ID,
			"backup_cron", target.BackupCron,
			"cleanup_cron", target.CleanupCron,
		)
	}

	scheduler.Start()
	s.logger.Info("backup service started")
	<-ctx.Done()

	stopCtx := scheduler.Stop()
	select {
	case <-stopCtx.Done():
	case <-time.After(30 * time.Second):
		s.logger.Warn("scheduler shutdown timed out")
	}
	s.logger.Info("backup service stopped")
	return nil
}
