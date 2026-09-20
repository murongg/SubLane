package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/murongg/SubLane/internal/backup"
)

const commandHelp = `Usage:
  sublane
  sublane --version
  sublane reset-admin-password --password-stdin
  sublane backup --output FILE [--data-dir DIR]
  sublane backup verify --input FILE
  sublane restore --input FILE --data-dir NEW_DIRECTORY

Backups include the credential encryption key and must be stored privately.
Restore requires a new directory; stop the original instance before switching to it.
`

func runMaintenance(ctx context.Context, args []string, defaultDir string, output io.Writer) error {
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
	var source, target, input string
	switch mode {
	case "backup":
		if defaultDir == "" {
			defaultDir = "./data"
		}
		flags.StringVar(&source, "data-dir", defaultDir, "existing instance data directory")
		flags.StringVar(&target, "output", "", "new private backup archive")
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
	var info backup.Info
	var err error
	switch mode {
	case "backup":
		info, err = backup.Create(ctx, source, target, version)
	case "verify":
		info, err = backup.Verify(ctx, input)
	case "restore":
		info, err = backup.Restore(ctx, input, target)
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
