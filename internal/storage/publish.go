package storage

import (
	"errors"
	"github.com/google/uuid"
	"io/fs"
	"os"
	"path"
	"strings"
)

// PublishTree installs a staged overlay and removes obsolete tracked paths.
// Previous bytes remain in a recovery directory until the caller commits its
// database changes. A failed publication restores all touched paths.
func PublishTree(root *os.Root, stage string, obsolete []string) (rollback func() error, cleanup func() error, err error) {
	recovery := ".mcm-publish-recovery-" + uuid.NewString()
	if err = root.Mkdir(recovery, 0700); err != nil {
		return nil, nil, err
	}
	type change struct {
		name           string
		old, published bool
	}
	changes := []change{}
	tracked := map[string]bool{}
	rollback = func() error {
		var errs []error
		for i := len(changes) - 1; i >= 0; i-- {
			c := changes[i]
			if c.published {
				errs = append(errs, root.RemoveAll(c.name))
			}
			if c.old {
				errs = append(errs, root.RemoveAll(c.name))
				errs = append(errs, root.MkdirAll(path.Dir(c.name), 0755), root.Rename(path.Join(recovery, c.name), c.name))
			}
		}
		return errors.Join(errs...)
	}
	cleanup = func() error { return root.RemoveAll(recovery) }
	save := func(name string) (int, error) {
		clean, e := Clean(name)
		if e != nil || clean == "." || strings.HasPrefix(clean, ".mcm-publish-recovery-") || clean == stage || strings.HasPrefix(clean, stage+"/") {
			return 0, ErrUnsafe
		}
		if tracked[clean] {
			for i, c := range changes {
				if c.name == clean {
					return i, nil
				}
			}
		}
		if e = Check(root, path.Dir(clean)); e != nil {
			return 0, e
		}
		c := change{name: clean}
		if _, e = root.Lstat(clean); e == nil {
			if e = root.MkdirAll(path.Join(recovery, path.Dir(clean)), 0700); e != nil {
				return 0, e
			}
			if e = root.Rename(clean, path.Join(recovery, clean)); e != nil {
				return 0, e
			}
			c.old = true
		} else if !os.IsNotExist(e) {
			return 0, e
		}
		tracked[clean] = true
		changes = append(changes, c)
		return len(changes) - 1, nil
	}
	fail := func(cause error) (func() error, func() error, error) {
		undoErr := rollback()
		if undoErr == nil {
			cleanup()
		}
		return nil, nil, errors.Join(cause, undoErr)
	}
	for _, name := range obsolete {
		if _, e := save(name); e != nil {
			return fail(e)
		}
	}
	err = fs.WalkDir(root.FS(), stage, func(name string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if name == stage {
			return nil
		}
		rel := strings.TrimPrefix(name, stage+"/")
		if e = Check(root, name); e != nil {
			return e
		}
		if d.IsDir() {
			return root.MkdirAll(rel, 0755)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return ErrUnsafe
		}
		index, e := save(rel)
		if e != nil {
			return e
		}
		if e = root.MkdirAll(path.Dir(rel), 0755); e != nil {
			return e
		}
		if e = root.Rename(name, rel); e != nil {
			return e
		}
		changes[index].published = true
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return rollback, cleanup, nil
}
