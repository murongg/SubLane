package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/murongg/SubLane/internal/backup"
)

const commandHelp = `Usage:
  sublane
  sublane --version
  sublane reset-admin-password --password-stdin
  sublane backup --output FILE --password-stdin [--data-dir DIR] [--key-file FILE]
  sublane backup verify --input FILE [--password-stdin]
  sublane restore --input FILE --data-dir NEW_DIRECTORY [--password-stdin]

New backups are password-encrypted and include the credential encryption key.
Read the password from a pipe or private file; never put it in arguments or environment.
Export passwords use 12–128 Unicode characters and cannot be reset if lost.
Legacy unencrypted gzip archives can still be verified and restored without a password.
Restore requires a new directory; stop the original instance before switching to it.
`

func runMaintenance(ctx context.Context, args []string, defaultDir, defaultKeyFile string, stdin io.Reader, output io.Writer) error {
	if len(args) == 0 {
		return errors.New(commandHelp)
	}
	mode := args[0]
	args = args[1:]
	if mode == "backup" && len(args) > 0 && args[0] == "verify" {
		mode = "verify"
		args = args[1:]
	}
	if mode != "backup" && mode != "verify" && mode != "restore" {
		return errors.New(commandHelp)
	}
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { fmt.Fprint(output, commandHelp) }
	var source, target, input, keyFile string
	var passwordStdin bool
	flags.BoolVar(&passwordStdin, "password-stdin", false, "read backup password from stdin")
	switch mode {
	case "backup":
		if defaultDir == "" {
			defaultDir = "./data"
		}
		flags.StringVar(&source, "data-dir", defaultDir, "existing instance data directory")
		flags.StringVar(&target, "output", "", "new encrypted backup archive")
		flags.StringVar(&keyFile, "key-file", defaultKeyFile, "credential key file outside the data directory")
	case "verify":
		flags.StringVar(&input, "input", "", "backup archive to validate")
	case "restore":
		flags.StringVar(&input, "input", "", "backup archive to restore")
		flags.StringVar(&target, "data-dir", "", "new directory for restored data (must not exist)")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (mode == "backup" && (source == "" || target == "")) || (mode == "verify" && input == "") || (mode == "restore" && (input == "" || target == "")) {
		return errors.New(commandHelp)
	}
	ctx, cancel := context.WithTimeout(ctx, backup.Timeout)
	defer cancel()
	password := ""
	if passwordStdin {
		var err error
		password, err = backupPassword(ctx, stdin)
		if err != nil {
			return err
		}
	}
	var info backup.Info
	var err error
	switch mode {
	case "backup":
		info, err = backup.Create(ctx, source, target, version, backup.Options{KeyFile: keyFile, Password: password})
	case "verify":
		info, err = backup.Verify(ctx, input, password)
	case "restore":
		info, err = backup.Restore(ctx, input, target, password)
	}
	if err != nil {
		return err
	}
	path := target
	if mode == "verify" {
		path = input
	}
	return json.NewEncoder(output).Encode(struct {
		Operation string `json:"operation"`
		Path      string `json:"path"`
		backup.Info
	}{mode, path, info})
}

func backupPassword(ctx context.Context, input io.Reader) (string, error) {
	if file, ok := input.(*os.File); ok {
		stat, err := file.Stat()
		if err != nil || stat.Mode()&os.ModeCharDevice != 0 {
			return "", errors.New("supply the backup password through a pipe or private redirected file")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if closer, ok := input.(io.Closer); ok {
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	raw, err := io.ReadAll(io.LimitReader(input, int64(backup.MaxPasswordBytes+3)))
	defer clear(raw)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err != nil {
		return "", errors.New("cannot read backup password")
	}
	if len(raw) > backup.MaxPasswordBytes+2 || !utf8.Valid(raw) {
		return "", backup.ErrPassword
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if password == "" {
		return "", backup.ErrPasswordRequired
	}
	if len(password) > backup.MaxPasswordBytes || strings.ContainsAny(password, "\r\n") {
		return "", backup.ErrPassword
	}
	return password, nil
}
