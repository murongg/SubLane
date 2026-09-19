// Package vault encrypts upstream credentials with an instance-local key.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrDecrypt = errors.New("credential_decryption_failed")

type Vault struct{ aead cipher.AEAD }

func Open(path string, allowCreate bool) (*Vault, error) {
	key, err := loadKey(path)
	if os.IsNotExist(err) && allowCreate {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		key = make([]byte, 32)
		_, _ = rand.Read(key)
		file, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(createErr, os.ErrExist) {
			key, err = loadKey(path)
		} else if createErr != nil {
			return nil, createErr
		} else {
			_, err = file.Write(key)
			if err == nil {
				err = file.Sync()
			}
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(path)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("load credential encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("credential encryption key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func loadKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("credential encryption key must be a private regular file (0600)")
	}
	return os.ReadFile(path)
}

func (v *Vault) Seal(accountID string, plaintext []byte) ([]byte, error) {
	if accountID == "" {
		return nil, errors.New("account ID is required for encryption")
	}
	nonce := make([]byte, v.aead.NonceSize())
	_, _ = rand.Read(nonce)
	envelope := append([]byte{1}, nonce...)
	// Binding the ciphertext to its row prevents copying credentials between account records.
	return v.aead.Seal(envelope, nonce, plaintext, []byte("sublane:account:"+accountID)), nil
}

func (v *Vault) Open(accountID string, encrypted []byte) ([]byte, error) {
	nonceSize := v.aead.NonceSize()
	if accountID == "" || len(encrypted) < 1+nonceSize+v.aead.Overhead() || encrypted[0] != 1 {
		return nil, ErrDecrypt
	}
	plain, err := v.aead.Open(nil, encrypted[1:1+nonceSize], encrypted[1+nonceSize:], []byte("sublane:account:"+accountID))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}
