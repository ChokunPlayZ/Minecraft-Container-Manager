package storage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExtractTarGz validates every member, enforces budgets, and verifies gzip CRC.
// A nil root validates without publishing any files.
func ExtractTarGz(ctx context.Context, reader io.Reader, root *os.Root) error {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("backup must be a valid tar.gz archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var expanded int64
	count := 0
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		if count > MaxArchiveEntries {
			return fmt.Errorf("archive contains too many entries")
		}
		name, err := Clean(hdr.Name)
		if err != nil || name == "." && hdr.Typeflag != tar.TypeDir {
			return ErrUnsafe
		}
		if hdr.Size < 0 || hdr.Size > MaxArchiveBytes-expanded {
			return fmt.Errorf("archive exceeds expanded byte limit")
		}
		if seen[name] && hdr.Typeflag != tar.TypeDir {
			return fmt.Errorf("duplicate archive member %q", name)
		}
		seen[name] = true
		switch hdr.Typeflag {
		case tar.TypeDir:
			if root != nil {
				if err = Check(root, name); err == nil {
					err = root.MkdirAll(name, 0755)
				}
				if err != nil {
					return err
				}
			}
		case tar.TypeReg, tar.TypeRegA:
			if strings.HasSuffix(name, "/") {
				return ErrUnsafe
			}
			var n int64
			if root == nil {
				n, err = Copy(ctx, io.Discard, tr, MaxArchiveBytes-expanded)
			} else {
				n, err = Write(ctx, root, name, tr, MaxArchiveBytes-expanded, 0644)
			}
			expanded += n
			if err != nil {
				return err
			}
			if n != hdr.Size {
				return io.ErrUnexpectedEOF
			}
		default:
			return fmt.Errorf("archive contains unsupported file type")
		}
	}
	if count == 0 {
		return fmt.Errorf("archive contains no files")
	}
	// tar EOF alone does not consume gzip's trailer; drain it to verify checksum.
	_, err = Copy(ctx, io.Discard, gz, 1<<20)
	return err
}
