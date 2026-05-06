package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"backup-service/internal/config"
	"backup-service/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
	if err := run(context.Background(), os.Args[1:], logger); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, logger *slog.Logger) error {
	if len(args) == 0 {
		return usage()
	}

	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var configPath string
	var targetID string
	fs.StringVar(&configPath, "config", "", "path to YAML config")
	fs.StringVar(&targetID, "target", "", "target id")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	svc, err := service.New(cfg, logger)
	if err != nil {
		return err
	}

	switch command {
	case "serve":
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return svc.Serve(ctx)
	case "backup":
		target, err := requireTarget(cfg, targetID)
		if err != nil {
			return err
		}
		_, err = svc.Backup(ctx, target)
		return err
	case "cleanup":
		target, err := requireTarget(cfg, targetID)
		if err != nil {
			return err
		}
		return svc.Cleanup(ctx, target)
	case "list":
		target, err := requireTarget(cfg, targetID)
		if err != nil {
			return err
		}
		objects, err := svc.List(ctx, target)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(objects)
	default:
		return usage()
	}
}

func requireTarget(cfg *config.Config, targetID string) (config.Target, error) {
	if targetID == "" {
		return config.Target{}, errors.New("--target is required")
	}
	target, ok := cfg.TargetByID(targetID)
	if !ok {
		return config.Target{}, fmt.Errorf("target %q not found", targetID)
	}
	return target, nil
}

func usage() error {
	return errors.New("usage: backup-service <serve|backup|cleanup|list> --config <path> [--target <id>]")
}
