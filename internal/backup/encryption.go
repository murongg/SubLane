package backup

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"filippo.io/age"
)

var ErrPasswordRequired = errors.New("backup_password_required")
var ErrPassword = errors.New("backup_password_invalid")
var ErrDecrypt = errors.New("backup_decryption_failed")

const MaxPasswordBytes = 512
const backupWorkFactor = 18

type Options struct {
	KeyFile  string
	Password string
}

func ValidPassword(password string) bool {
	length := utf8.RuneCountInString(password)
	return utf8.ValidString(password) && length >= 12 && length <= 128 && !strings.ContainsAny(password, "\r\n")
}

func encryptArchive(writer io.Writer, password string) (io.WriteCloser, error) {
	recipient, err := age.NewScryptRecipient(password)
	if err != nil {
		return nil, ErrPasswordRequired
	}
	recipient.SetWorkFactor(backupWorkFactor)
	return age.Encrypt(writer, recipient)
}

func decryptArchive(reader *bufio.Reader, password string) (io.Reader, bool, error) {
	prefix, _ := reader.Peek(len("age-encryption.org/"))
	if !bytes.HasPrefix(prefix, []byte("age-encryption.org/")) {
		return reader, false, nil
	}
	if password == "" {
		return nil, true, ErrPasswordRequired
	}
	if len(password) > MaxPasswordBytes || !utf8.ValidString(password) {
		return nil, true, ErrPassword
	}
	identity, err := age.NewScryptIdentity(password)
	if err != nil {
		return nil, true, ErrDecrypt
	}
	// Imported headers cannot choose a more expensive KDF than the one used for exports (~256 MiB).
	identity.SetMaxWorkFactor(backupWorkFactor)
	decrypted, err := age.Decrypt(reader, identity)
	if err != nil {
		return nil, true, ErrDecrypt
	}
	return authenticatedReader{decrypted}, true, nil
}

type authenticatedReader struct{ io.Reader }

func (reader authenticatedReader) Read(buffer []byte) (int, error) {
	n, err := reader.Reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, ErrDecrypt
	}
	return n, err
}
