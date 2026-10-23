package config

import "testing"

func TestBackupWebhookConfiguration(t *testing.T) {
	for _, url := range []string{"https://hooks.example.test/backup", "https://127.0.0.1/backup", "http://hooks.example.test/backup"} {
		env := map[string]string{"SUBLANE_BACKUP_DIR": "/synthetic/backups", "SUBLANE_BACKUP_PASSWORD_FILE": "/synthetic/private-password", "SUBLANE_BACKUP_WEBHOOK_URL": url}
		cfg, err := Load(func(key string) string { return env[key] })
		valid := url == "https://hooks.example.test/backup"
		if (err == nil) != valid {
			t.Fatal("webhook configuration validation", err)
		}
		if valid && cfg.BackupWebhookURL != url {
			t.Fatal("webhook not configured")
		}
	}
}
