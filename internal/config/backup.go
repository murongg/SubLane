package config

import (
	"fmt"
	"github.com/murongg/SubLane/internal/alerts"
	"strconv"
	"time"
)

func (c *Config) loadBackup(getenv func(string) string) error {
	c.Backup.Directory = getenv("SUBLANE_BACKUP_DIR")
	c.Backup.PasswordFile = getenv("SUBLANE_BACKUP_PASSWORD_FILE")
	c.BackupWebhookURL = getenv("SUBLANE_BACKUP_WEBHOOK_URL")
	if c.BackupWebhookURL != "" {
		if _, err := alerts.ValidateDestination(c.BackupWebhookURL); err != nil {
			return fmt.Errorf("SUBLANE_BACKUP_WEBHOOK_URL must be a public HTTPS destination")
		}
	}
	c.Backup.Interval = 24 * time.Hour
	c.Backup.Keep = 7
	if value := getenv("SUBLANE_BACKUP_INTERVAL"); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid SUBLANE_BACKUP_INTERVAL")
		}
		c.Backup.Interval = interval
	}
	if value := getenv("SUBLANE_BACKUP_KEEP"); value != "" {
		keep, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid SUBLANE_BACKUP_KEEP")
		}
		c.Backup.Keep = keep
	}
	if c.Backup.Directory == "" && c.Backup.PasswordFile == "" && c.BackupWebhookURL == "" && getenv("SUBLANE_BACKUP_INTERVAL") == "" && getenv("SUBLANE_BACKUP_KEEP") == "" {
		return nil
	}
	return c.Backup.Validate()
}
