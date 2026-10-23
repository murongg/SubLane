package server

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
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
	directory, keyFile, version string
	auth                        *auth.Service
	audit                       *audit.Service
	slots                       chan struct{}
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
	writeJSON(w, 200, map[string]any{"max_upload_bytes": maxWebBackupBytes, "restore_directory": target, "destination_exists": err == nil, "key_separated": h.keyFile != ""})
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
	var input struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	stage, err := os.MkdirTemp(h.directory, ".sublane-backup-")
	if err != nil {
		backupError(w, err)
		return
	}
	defer os.RemoveAll(stage)
	path := filepath.Join(stage, "export.tar.gz.age")
	info, err := backup.Create(r.Context(), h.directory, path, h.version, backup.Options{KeyFile: h.keyFile, Password: input.Password})
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
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="sublane-`+info.CreatedAt.Format("20060102-150405")+`.sublane-backup.tar.gz.age"`)
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}
func (h *backupHTTP) upload(w http.ResponseWriter, r *http.Request, restore bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (media != "application/gzip" && media != "application/octet-stream" && media != "multipart/form-data") {
		writeJSON(w, 415, map[string]string{"error": "invalid_backup"})
		return
	}
	limit := maxWebBackupBytes
	if media == "multipart/form-data" {
		limit += 4096
	}
	if r.ContentLength > limit {
		writeJSON(w, 413, map[string]string{"error": "backup_too_large"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	stage, err := os.MkdirTemp(h.directory, ".sublane-backup-")
	if err != nil {
		backupError(w, err)
		return
	}
	defer os.RemoveAll(stage)
	password := ""
	var source io.Reader = r.Body
	var parts *multipart.Reader
	if media == "multipart/form-data" {
		parts, err = r.MultipartReader()
		if err != nil {
			backupError(w, backup.ErrInvalid)
			return
		}
		part, err := parts.NextPart()
		if err != nil {
			backupError(w, backup.ErrInvalid)
			return
		}
		if part.FormName() == "password" && part.FileName() == "" {
			raw, err := io.ReadAll(io.LimitReader(part, backup.MaxPasswordBytes+1))
			if err != nil {
				backupError(w, err)
				return
			}
			if len(raw) > backup.MaxPasswordBytes {
				backupError(w, backup.ErrPassword)
				return
			}
			password = string(raw)
			clear(raw)
			if err := part.Close(); err != nil {
				backupError(w, err)
				return
			}
			part, err = parts.NextPart()
			if err != nil {
				backupError(w, backup.ErrInvalid)
				return
			}
		}
		if part.FormName() != "file" || part.FileName() == "" {
			backupError(w, backup.ErrInvalid)
			return
		}
		source = part
	}
	path := filepath.Join(stage, "upload")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		backupError(w, err)
		return
	}
	size, err := io.Copy(file, io.LimitReader(source, maxWebBackupBytes+1))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		backupError(w, err)
		return
	}
	if size > maxWebBackupBytes {
		writeJSON(w, 413, map[string]string{"error": "backup_too_large"})
		return
	}
	if parts != nil {
		// Reject duplicate fields or additional files before trusting the uploaded archive.
		if _, err := parts.NextPart(); !errors.Is(err, io.EOF) {
			backupError(w, backup.ErrInvalid)
			return
		}
	}
	if !h.stillAdmin(w, r) {
		return
	}
	if !restore {
		info, err := backup.Verify(r.Context(), path, password)
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
	info, err := backup.Restore(r.Context(), path, target, password)
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
	case errors.Is(err, backup.ErrPasswordRequired):
		status, code = 400, "backup_password_required"
	case errors.Is(err, backup.ErrPassword):
		status, code = 400, "backup_password_invalid"
	case errors.Is(err, backup.ErrDecrypt):
		status, code = 400, "backup_decryption_failed"
	case errors.Is(err, backup.ErrInvalid):
		status, code = 400, "invalid_backup"
	case errors.Is(err, backup.ErrExists):
		status, code = 409, "backup_destination_exists"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
