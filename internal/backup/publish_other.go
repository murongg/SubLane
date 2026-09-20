//go:build !linux && !darwin && !windows

package backup

import "errors"

func renameExclusive(string, string) error {
	return errors.New("atomic backup publication is not supported on this platform")
}
func syncDirectory(string) error { return nil }
