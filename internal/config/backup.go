package config

import (
	"fmt"
	"strconv"
	"time"
)

func (c *Config) loadBackup(getenv func(string) string) error {
	c.Backup.Directory = getenv("SUBLANE_BACKUP_DIR")
	c.Backup.PasswordFile = getenv("SUBLANE_BACKUP_PASSWORD_FILE")
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
	if c.Backup.Directory == "" && c.Backup.PasswordFile == "" && getenv("SUBLANE_BACKUP_INTERVAL") == "" && getenv("SUBLANE_BACKUP_KEEP") == "" {
		return nil
	}
	return c.Backup.Validate()
}
