package server

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/backup"
)

const maxWebBackupBytes int64 = 256 << 20

type backupHTTP struct {
	directory, version string
	auth               *auth.Service
	audit              *audit.Service
	slots              chan struct{}
}

func (h *backupHTTP) register(router chi.Router) {
	routeErrors(router)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.directory == "" || h.auth == nil || h.audit == nil {
				writeJSON(w, 503, map[string]string{"error": "backup_unavailable"})
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/", h.settings)
	router.Group(func(operations chi.Router) {
		operations.Use(h.operation)
		operations.Post("/export", h.export)
		operations.Post("/verify", func(w http.ResponseWriter, r *http.Request) { h.upload(w, r, false) })
		operations.Post("/restore", func(w http.ResponseWriter, r *http.Request) { h.upload(w, r, true) })
	})
}
func (h *backupHTTP) target() (string, error) {
	directory, err := filepath.Abs(h.directory)
	return filepath.Join(directory, "restore-ready"), err
}
func (h *backupHTTP) settings(w http.ResponseWriter, r *http.Request) {
	target, err := h.target()
	if err != nil {
		backupError(w, err)
		return
	}
	_, err = os.Lstat(target)
	if err != nil && !os.IsNotExist(err) {
		backupError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"max_upload_bytes": maxWebBackupBytes, "restore_directory": target, "destination_exists": err == nil})
}
func (h *backupHTTP) operation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case h.slots <- struct{}{}:
			defer func() { <-h.slots }()
		default:
			w.Header().Set("Retry-After", "5")
			writeJSON(w, 429, map[string]string{"error": "backup_busy"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), backup.Timeout)
		defer cancel()
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(backup.Timeout))
		_ = controller.SetWriteDeadline(time.Now().Add(backup.Timeout))
		// Deadlines belong to this operation, not later keep-alive requests.
		defer controller.SetReadDeadline(time.Time{})
		defer controller.SetWriteDeadline(time.Time{})
		interrupted := make(chan struct{})
		stopInterrupt := context.AfterFunc(ctx, func() {
			defer close(interrupted)
			// A canceled context alone does not interrupt a stalled request body or download.
			_ = controller.SetReadDeadline(time.Now())
			_ = controller.SetWriteDeadline(time.Now())
			_ = r.Body.Close()
		})
		defer func() {
			// Join before clearing deadlines so this callback cannot affect the next request.
			if !stopInterrupt() {
				<-interrupted
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (h *backupHTTP) stillAdmin(w http.ResponseWriter, r *http.Request) bool {
	state, err := h.auth.State(r.Context(), token(r))
	if err != nil {
		backupError(w, err)
		return false
	}
	if state.User == nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return false
	}
	if state.User.Role != "admin" {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return false
	}
	return true
}
func (h *backupHTTP) export(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	stage, err := os.MkdirTemp(h.directory, ".sublane-backup-")
	if err != nil {
		backupError(w, err)
		return
	}
	defer os.RemoveAll(stage)
	path := filepath.Join(stage, "export.tar.gz")
	info, err := backup.Create(r.Context(), h.directory, path, h.version)
	if err != nil {
		backupError(w, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		backupError(w, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		backupError(w, err)
		return
	}
	if stat.Size() > maxWebBackupBytes {
		writeJSON(w, 413, map[string]string{"error": "backup_too_large"})
		return
	}
	if !h.stillAdmin(w, r) {
		return
	}
	if err := h.audit.Observation(r.Context(), "backup.export", "backup", ""); err != nil {
		backupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="sublane-`+info.CreatedAt.Format("20060102-150405")+`.sublane-backup.tar.gz"`)
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}
func (h *backupHTTP) upload(w http.ResponseWriter, r *http.Request, restore bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (media != "application/gzip" && media != "application/octet-stream") {
		writeJSON(w, 415, map[string]string{"error": "invalid_backup"})
		return
	}
	if r.ContentLength > maxWebBackupBytes {
		writeJSON(w, 413, map[string]string{"error": "backup_too_large"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebBackupBytes)
	stage, err := os.MkdirTemp(h.directory, ".sublane-backup-")
	if err != nil {
		backupError(w, err)
		return
	}
	defer os.RemoveAll(stage)
	path := filepath.Join(stage, "upload.tar.gz")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		backupError(w, err)
		return
	}
	_, err = io.Copy(file, r.Body)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		backupError(w, err)
		return
	}
	if !h.stillAdmin(w, r) {
		return
	}
	if !restore {
		info, err := backup.Verify(r.Context(), path)
		if err != nil {
			backupError(w, err)
			return
		}
		if h.stillAdmin(w, r) {
			writeJSON(w, 200, info)
		}
		return
	}
	target, err := h.target()
	if err != nil {
		backupError(w, err)
		return
	}
	info, err := backup.Restore(r.Context(), path, target)
	if err != nil {
		backupError(w, err)
		return
	}
	// This prepares a separate filesystem artifact; the running instance is never replaced.
	if err := h.audit.Observation(r.Context(), "backup.prepare", "backup", ""); err != nil {
		// Keep valid, published recovery data even if the live audit database is unavailable.
		writeJSON(w, 503, map[string]string{"error": "backup_audit_failed"})
		return
	}
	writeJSON(w, 201, struct {
		backup.Info
		Directory string `json:"restore_directory"`
	}{info, target})
}
func backupError(w http.ResponseWriter, err error) {
	status, code := 503, "backup_failed"
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		status, code = 413, "backup_too_large"
	case errors.Is(err, backup.ErrInvalid):
		status, code = 400, "invalid_backup"
	case errors.Is(err, backup.ErrExists):
		status, code = 409, "backup_destination_exists"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
