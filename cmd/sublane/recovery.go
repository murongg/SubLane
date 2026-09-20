package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/storage"
)

func recoverPassword(ctx context.Context, dataDir string, input io.Reader) error {
	path := filepath.Join(dataDir, "sublane.db")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("administrator recovery requires an existing database")
	}
	// At most twenty UTF-8 code points plus CRLF; never read an unbounded pipe or log its contents.
	raw, err := io.ReadAll(io.LimitReader(input, 83))
	if err != nil {
		return errors.New("cannot read the new password from stdin")
	}
	if len(raw) > 82 {
		return auth.ErrInput
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if strings.ContainsAny(password, "\r\n") {
		return auth.ErrInput
	}
	connection, err := storage.Open(ctx, path)
	if err != nil {
		return err
	}
	defer connection.Close()
	identity, err := auth.New(connection)
	if err != nil {
		return err
	}
	return identity.RecoverAdministrator(ctx, password)
}
