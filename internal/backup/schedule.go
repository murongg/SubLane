package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const scheduledLayout = "20060102T150405.000000000Z"
const scheduledSuffix = ".sublane-backup.tar.gz.age"

type Schedule struct {
	Directory    string
	PasswordFile string
	Interval     time.Duration
	Keep         int
}

func (s Schedule) Validate() error {
	if s.Directory == "" || s.PasswordFile == "" || s.Interval < time.Hour || s.Interval > 30*24*time.Hour || s.Keep < 1 || s.Keep > 365 {
		return errors.New("scheduled backups require a directory, private password file, interval of 1h–720h and retention of 1–365 archives")
	}
	return nil
}

// Maintain runs serially and is joined by the process before storage closes.
func (s Schedule) Maintain(ctx context.Context, dataDir, version, keyFile string, logger *slog.Logger, onFailure func(context.Context)) {
	for ctx.Err() == nil {
		started := time.Now()
		path, err := s.Run(ctx, dataDir, version, keyFile)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// Filesystem errors can contain private paths; keep failure logs bounded to the operation.
			logger.Error("Scheduled backup failed; check directory access, password file, space and archive integrity")
			if onFailure != nil {
				onFailure(ctx)
			}
		} else {
			logger.Info("Scheduled backup verified", "archive", filepath.Base(path), "completed_at", time.Now().UTC(), "duration", time.Since(started))
		}
		timer := time.NewTimer(s.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s Schedule) Run(ctx context.Context, dataDir, version, keyFile string) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	password, err := s.password()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(s.Directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", ErrInvalid
	}
	path := filepath.Join(s.Directory, "scheduled-"+time.Now().UTC().Format(scheduledLayout)+scheduledSuffix)
	if _, err := Create(ctx, dataDir, path, version, Options{KeyFile: keyFile, Password: password}); err != nil {
		return "", err
	}
	// Never rotate the last known good archives until the replacement passes full verification.
	if _, err := Verify(ctx, path, password); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		return path, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if name == filepath.Base(path) {
			continue
		}
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, "scheduled-") || !strings.HasSuffix(name, scheduledSuffix) {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, "scheduled-"), scheduledSuffix)
		if _, err := time.Parse(scheduledLayout, stamp); err == nil {
			names = append(names, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[min(s.Keep-1, len(names)):] {
		if err := ctx.Err(); err != nil {
			return path, err
		}
		if err := os.Remove(filepath.Join(s.Directory, name)); err != nil {
			return path, err
		}
	}
	return path, nil
}

func (s Schedule) password() (string, error) {
	info, err := os.Lstat(s.PasswordFile)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", ErrPassword
	}
	file, err := os.Open(s.PasswordFile)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", ErrPassword
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxPasswordBytes+3))
	defer clear(raw)
	if err != nil {
		return "", err
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if len(raw) > MaxPasswordBytes+2 || !ValidPassword(password) {
		return "", ErrPassword
	}
	return password, nil
}
