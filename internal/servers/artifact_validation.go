package servers

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"github.com/mcm-panel/mcm/internal/storage"
	"hash"
	"io"
	"os"
	"strings"
)

// validateArtifact checks the ZIP directory, every member CRC and declared hashes
// before an artifact can replace an installed jar.
func validateArtifact(ctx context.Context, file string, hashes map[string]string) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return fmt.Errorf("%w: invalid JAR archive", ErrInvalidModName)
	}
	defer zr.Close()
	if len(zr.File) == 0 || len(zr.File) > storage.MaxArchiveEntries {
		return ErrInvalidModName
	}
	var total int64
	for _, entry := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return ErrInvalidModName
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		n, err := storage.Copy(ctx, io.Discard, rc, 1<<30-total)
		rc.Close()
		total += n
		if err != nil {
			return fmt.Errorf("%w: corrupt or oversized JAR", ErrInvalidModName)
		}
	}
	return verifyArtifactHashes(ctx, file, hashes)
}
func verifyArtifactHashes(ctx context.Context, file string, hashes map[string]string) error {
	for algorithm, expected := range hashes {
		if expected == "" {
			continue
		}
		var h hash.Hash
		switch algorithm {
		case "md5":
			h = md5.New()
		case "sha1":
			h = sha1.New()
		case "sha256":
			h = sha256.New()
		case "sha512":
			h = sha512.New()
		default:
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		_, err = storage.Copy(ctx, h, f, maxDownloadBytes)
		f.Close()
		if err != nil {
			return err
		}
		if !strings.EqualFold(expected, hex.EncodeToString(h.Sum(nil))) {
			return fmt.Errorf("%w: artifact %s checksum mismatch", ErrDownloadFailed, algorithm)
		}
	}
	return nil
}
