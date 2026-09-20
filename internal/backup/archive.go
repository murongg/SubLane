package backup

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const maxManifestBytes = 16 << 10

type digest struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type manifest struct {
	Format        string `json:"format"`
	FormatVersion int    `json:"format_version"`
	Info
	Files map[string]digest `json:"files"`
}
type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
func regular(path string) (*os.File, error) {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(stat, opened) {
		file.Close()
		return nil, ErrInvalid
	}
	return file, nil
}
func copyKey(ctx context.Context, source, target string) error {
	file, err := regular(source)
	if err != nil {
		return errors.New("backup requires credentials.key beside sublane.db")
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() != 32 || stat.Mode().Perm()&0077 != 0 {
		return errors.New("backup credential key must be private and exactly 32 bytes")
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, file}, 33))
	if err != nil {
		return err
	}
	defer clear(data)
	if len(data) != 32 {
		return ErrInvalid
	}
	return os.WriteFile(target, data, 0600)
}
func hashFile(ctx context.Context, path string) (digest, error) {
	file, err := regular(path)
	if err != nil {
		return digest{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, contextReader{ctx, file})
	return digest{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, err
}
func writeArchive(ctx context.Context, directory, path string, info Info) (result error) {
	value := manifest{Format: "sublane-backup", FormatVersion: 1, Info: info, Files: map[string]digest{}}
	for _, name := range []string{databaseName, keyName} {
		sum, err := hashFile(ctx, filepath.Join(directory, name))
		if err != nil {
			return err
		}
		value.Files[name] = sum
	}
	if !value.valid() {
		return ErrInvalid
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); result == nil {
			result = err
		}
	}()
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	write := func(name string, size int64, reader io.Reader) error {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR, ModTime: info.CreatedAt}); err != nil {
			return err
		}
		_, err := io.Copy(archive, contextReader{ctx, reader})
		return err
	}
	err = write(manifestName, int64(len(raw)), strings.NewReader(string(raw)))
	for _, name := range []string{databaseName, keyName} {
		if err != nil {
			break
		}
		var input *os.File
		input, err = regular(filepath.Join(directory, name))
		if err != nil {
			break
		}
		err = write(name, value.Files[name].Size, input)
		closeErr := input.Close()
		if err == nil {
			err = closeErr
		}
	}
	tarErr := archive.Close()
	gzipErr := compressed.Close()
	if err != nil {
		return err
	}
	if tarErr != nil {
		return tarErr
	}
	if gzipErr != nil {
		return gzipErr
	}
	return file.Sync()
}
func (value manifest) valid() bool {
	if value.Format != "sublane-backup" || value.FormatVersion != 1 || value.CreatedAt.IsZero() || value.SchemaVersion < 1 || len(value.Version) == 0 || len(value.Version) > 128 || strings.IndexFunc(value.Version, unicode.IsControl) >= 0 || len(value.Files) != 2 {
		return false
	}
	database, ok := value.Files[databaseName]
	if !ok || database.Size <= 0 || database.Size > MaxDatabaseBytes || value.DatabaseBytes != database.Size {
		return false
	}
	key, ok := value.Files[keyName]
	if !ok || key.Size != 32 {
		return false
	}
	for _, file := range value.Files {
		if len(file.SHA256) != 64 {
			return false
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return false
		}
	}
	return true
}
func extractArchive(ctx context.Context, path, directory string) (info Info, result error) {
	defer func() {
		if err := ctx.Err(); err != nil {
			result = err
		}
	}()
	file, err := regular(path)
	if err != nil {
		return Info{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return Info{}, err
	}
	if stat.Size() <= 0 || stat.Size() > MaxArchiveBytes {
		return Info{}, ErrInvalid
	}
	buffered := bufio.NewReader(contextReader{ctx, file})
	compressed, err := gzip.NewReader(buffered)
	if err != nil {
		return Info{}, ErrInvalid
	}
	defer compressed.Close()
	compressed.Multistream(false)
	// Bound decompression even for extended tar headers handled internally by archive/tar.
	unpacked := &io.LimitedReader{R: compressed, N: MaxDatabaseBytes + (2 << 20)}
	archive := tar.NewReader(unpacked)
	seen := map[string]digest{}
	var value manifest
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Info{}, ErrInvalid
		}
		limit := int64(0)
		switch header.Name {
		case databaseName:
			limit = MaxDatabaseBytes
		case keyName:
			limit = 32
		case manifestName:
			limit = maxManifestBytes
		default:
			return Info{}, ErrInvalid
		}
		if _, duplicate := seen[header.Name]; duplicate || header.Typeflag != tar.TypeReg || header.Format != tar.FormatUSTAR || header.Size <= 0 || header.Size > limit {
			return Info{}, ErrInvalid
		}
		target, err := os.OpenFile(filepath.Join(directory, header.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return Info{}, err
		}
		hash := sha256.New()
		size, err := io.Copy(io.MultiWriter(target, hash), contextReader{ctx, archive})
		closeErr := target.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return Info{}, err
		}
		if size != header.Size {
			return Info{}, ErrInvalid
		}
		seen[header.Name] = digest{Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}
	}
	if unpacked.N <= 0 {
		return Info{}, ErrInvalid
	}
	// Consume the gzip footer to verify its checksum; reject appended archives or hidden trailing content.
	var trailing [1]byte
	if n, err := compressed.Read(trailing[:]); n != 0 || !errors.Is(err, io.EOF) {
		return Info{}, ErrInvalid
	}
	if _, err := buffered.Peek(1); !errors.Is(err, io.EOF) {
		return Info{}, ErrInvalid
	}
	if len(seen) != 3 {
		return Info{}, ErrInvalid
	}
	raw, err := os.ReadFile(filepath.Join(directory, manifestName))
	if err != nil {
		return Info{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || !value.valid() {
		return Info{}, ErrInvalid
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return Info{}, ErrInvalid
	}
	for name, expected := range value.Files {
		if seen[name] != expected {
			return Info{}, ErrInvalid
		}
	}
	return value.Info, nil
}
