package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type Config struct {
	S3       S3Config       `yaml:"s3"`
	Telegram TelegramConfig `yaml:"telegram"`
	Defaults Defaults       `yaml:"defaults"`
	Targets  []Target       `yaml:"targets"`
}

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	ForcePathStyle  bool   `yaml:"force_path_style"`
}

type TelegramConfig struct {
	Enabled        bool   `yaml:"enabled"`
	BotToken       string `yaml:"bot_token"`
	ChatID         string `yaml:"chat_id"`
	SuccessEnabled *bool  `yaml:"success_enabled"`
}

type Defaults struct {
	Retention        Retention `yaml:"retention"`
	BackupCron       string    `yaml:"backup_cron"`
	CleanupCron      string    `yaml:"cleanup_cron"`
	CompressionLevel int       `yaml:"compression_level"`
}

type Retention struct {
	Daily   int `yaml:"daily"`
	Weekly  int `yaml:"weekly"`
	Monthly int `yaml:"monthly"`
}

type Target struct {
	ID               string    `yaml:"id"`
	Engine           string    `yaml:"engine"`
	DatabaseURLEnv   string    `yaml:"database_url_env"`
	S3Prefix         string    `yaml:"s3_prefix"`
	BackupCron       string    `yaml:"backup_cron"`
	CleanupCron      string    `yaml:"cleanup_cron"`
	Retention        Retention `yaml:"retention"`
	CompressionLevel int       `yaml:"compression_level"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) TargetByID(id string) (Target, bool) {
	for _, target := range c.Targets {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

func (t Target) DatabaseURL() (string, error) {
	value := os.Getenv(t.DatabaseURLEnv)
	if value == "" {
		return "", fmt.Errorf("env %s for target %s is required", t.DatabaseURLEnv, t.ID)
	}
	return value, nil
}

func (t Target) CleanPrefix() string {
	return strings.Trim(strings.TrimSpace(t.S3Prefix), "/")
}

func (c *Config) TelegramSuccessEnabled() bool {
	if c.Telegram.SuccessEnabled == nil {
		return true
	}
	return *c.Telegram.SuccessEnabled
}

func (c *Config) applyDefaults() {
	if c.Defaults.BackupCron == "" {
		c.Defaults.BackupCron = "0 2 * * *"
	}
	if c.Defaults.CleanupCron == "" {
		c.Defaults.CleanupCron = "0 5 * * *"
	}
	if c.Defaults.CompressionLevel == 0 {
		c.Defaults.CompressionLevel = 9
	}
	for i := range c.Targets {
		target := &c.Targets[i]
		if target.Engine == "" {
			target.Engine = "postgres"
		}
		if target.BackupCron == "" {
			target.BackupCron = c.Defaults.BackupCron
		}
		if target.CleanupCron == "" {
			target.CleanupCron = c.Defaults.CleanupCron
		}
		if target.CompressionLevel == 0 {
			target.CompressionLevel = c.Defaults.CompressionLevel
		}
		if target.Retention.Daily == 0 {
			target.Retention.Daily = c.Defaults.Retention.Daily
		}
		if target.Retention.Weekly == 0 {
			target.Retention.Weekly = c.Defaults.Retention.Weekly
		}
		if target.Retention.Monthly == 0 {
			target.Retention.Monthly = c.Defaults.Retention.Monthly
		}
	}
}

func (c *Config) Validate() error {
	var joined []error
	require := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			joined = append(joined, fmt.Errorf("%s is required", name))
		}
	}

	require("s3.endpoint", c.S3.Endpoint)
	require("s3.region", c.S3.Region)
	require("s3.bucket", c.S3.Bucket)
	require("s3.access_key_id", c.S3.AccessKeyID)
	require("s3.secret_access_key", c.S3.SecretAccessKey)
	if c.Telegram.Enabled {
		require("telegram.bot_token", c.Telegram.BotToken)
		require("telegram.chat_id", c.Telegram.ChatID)
	}
	if len(c.Targets) == 0 {
		joined = append(joined, errors.New("at least one target is required"))
	}

	ids := map[string]struct{}{}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	for i, target := range c.Targets {
		label := fmt.Sprintf("targets[%d]", i)
		require(label+".id", target.ID)
		require(label+".database_url_env", target.DatabaseURLEnv)
		require(label+".s3_prefix", target.S3Prefix)
		if _, ok := ids[target.ID]; ok {
			joined = append(joined, fmt.Errorf("duplicate target id %q", target.ID))
		}
		ids[target.ID] = struct{}{}
		if target.Engine != "postgres" {
			joined = append(joined, fmt.Errorf("%s.engine must be postgres", label))
		}
		if target.CompressionLevel < 0 || target.CompressionLevel > 9 {
			joined = append(joined, fmt.Errorf("%s.compression_level must be between 0 and 9", label))
		}
		if _, err := parser.Parse(target.BackupCron); err != nil {
			joined = append(joined, fmt.Errorf("%s.backup_cron: %w", label, err))
		}
		if _, err := parser.Parse(target.CleanupCron); err != nil {
			joined = append(joined, fmt.Errorf("%s.cleanup_cron: %w", label, err))
		}
	}
	return errors.Join(joined...)
}

func (c *Config) ValidateDatabaseURLs() error {
	var joined []error
	for _, target := range c.Targets {
		if target.DatabaseURLEnv != "" && os.Getenv(target.DatabaseURLEnv) == "" {
			joined = append(joined, fmt.Errorf("env %s for target %s is required", target.DatabaseURLEnv, target.ID))
		}
	}
	return errors.Join(joined...)
}
