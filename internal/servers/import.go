package servers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/storage"
)

const exportManifestName = "mcm-export.json"
const maxExportManifestBytes = 64 << 10

// Export settings deliberately exclude container identities and host port bindings.
type exportSettings struct {
	Name          string       `json:"name"`
	ServerType    jars.JarType `json:"server_type"`
	Version       string       `json:"version"`
	Build         string       `json:"build,omitempty"`
	RAMMB         int          `json:"ram_mb"`
	CPULimit      float64      `json:"cpu_limit"`
	MemoryLimitMB int          `json:"memory_limit_mb"`
	JavaVersion   int          `json:"java_version"`
}

type exportManifest struct {
	FormatVersion int            `json:"format_version"`
	Server        exportSettings `json:"server"`
}

// ImportInput overrides exported settings, or supplies settings for legacy ZIPs.
type ImportInput struct {
	Name        string       `json:"name"`
	ServerType  jars.JarType `json:"server_type,omitempty"`
	Version     string       `json:"version,omitempty"`
	Build       string       `json:"build,omitempty"`
	RAMMB       int          `json:"ram_mb,omitempty"`
	JavaVersion int          `json:"java_version,omitempty"`
	NoHostPort  bool         `json:"no_host_port,omitempty"`
}

type ImportValidationError struct{ Message string }

func (e *ImportValidationError) Error() string { return e.Message }

func importValidation(message string) error { return &ImportValidationError{Message: message} }

// Import extracts into a new, unpublished directory before reserving a port and
// publishing a stopped server. It never provisions or downloads server software.
func (s *Store) Import(ctx context.Context, reader io.ReaderAt, size int64, in ImportInput) (Server, error) {
	if size <= 0 || size > storage.MaxUploadBytes {
		return Server{}, importValidation("Choose a server ZIP file of at most 2 GiB.")
	}
	zr, err := zip.NewReader(reader, size)
	if err != nil || len(zr.File) == 0 || len(zr.File) > storage.MaxArchiveEntries {
		return Server{}, ErrInvalidArchive
	}
	settings := exportSettings{RAMMB: 2048}
	seen := make(map[string]bool, len(zr.File))
	var expanded uint64
	for _, member := range zr.File {
		if err := ctx.Err(); err != nil {
			return Server{}, err
		}
		name, err := storage.Clean(member.Name)
		if err != nil || name == "." || seen[name] || (!member.Mode().IsRegular() && !member.FileInfo().IsDir()) {
			return Server{}, ErrInvalidArchive
		}
		seen[name] = true
		if member.UncompressedSize64 > uint64(storage.MaxArchiveBytes)-expanded {
			return Server{}, importValidation("The server ZIP exceeds the 8 GiB extracted size limit.")
		}
		expanded += member.UncompressedSize64
		if name != exportManifestName {
			continue
		}
		if member.FileInfo().IsDir() || member.UncompressedSize64 > maxExportManifestBytes {
			return Server{}, ErrInvalidArchive
		}
		rc, err := member.Open()
		if err != nil {
			return Server{}, ErrInvalidArchive
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxExportManifestBytes+1))
		rc.Close()
		var manifest exportManifest
		if readErr != nil || len(data) > maxExportManifestBytes || json.Unmarshal(data, &manifest) != nil {
			return Server{}, importValidation("The exported server settings are damaged. Export the server again.")
		}
		if manifest.FormatVersion != 1 {
			return Server{}, importValidation("This export format is not supported. Update MCM before importing it.")
		}
		settings = manifest.Server
	}
	if in.Name != "" {
		settings.Name = in.Name
	}
	if in.ServerType != "" {
		settings.ServerType = in.ServerType
	}
	if in.Version != "" {
		settings.Version = in.Version
	}
	if in.Build != "" {
		settings.Build = in.Build
	}
	if in.RAMMB != 0 {
		settings.RAMMB = in.RAMMB
	}
	if in.JavaVersion != 0 {
		settings.JavaVersion = in.JavaVersion
	}
	settings.Name = strings.TrimSpace(settings.Name)
	settings.Version = strings.TrimSpace(settings.Version)
	if !seen[exportManifestName] && (settings.Name == "" || settings.ServerType == "" || settings.Version == "") {
		return Server{}, importValidation("This older export has no server settings. Enter a server name, then open Server settings to choose its type and enter its version.")
	}
	if settings.Name == "" || len(settings.Name) > 255 {
		return Server{}, importValidation("Enter a server name of at most 255 characters.")
	}
	if settings.ServerType == "" || settings.Version == "" {
		return Server{}, importValidation("The export is missing server settings. Open Server settings to choose the server type and enter its version.")
	}
	settings.ServerType, err = jars.ParseJarType(string(settings.ServerType))
	if err != nil {
		return Server{}, importValidation("Choose a supported server type in Server settings.")
	}
	if settings.RAMMB <= 0 || len(settings.Version) > 128 || len(settings.Build) > 128 {
		return Server{}, importValidation("Enter valid server settings and a RAM allocation greater than zero.")
	}
	if err := validateLimits(settings.CPULimit, settings.MemoryLimitMB); err != nil {
		return Server{}, importValidation("The exported resource limits are invalid.")
	}
	if settings.JavaVersion == 0 {
		settings.JavaVersion = jars.RecommendJavaVersionForType(string(settings.ServerType), settings.Version)
	}
	if settings.JavaVersion < 8 || settings.JavaVersion > 99 {
		return Server{}, importValidation("Enter a Java version between 8 and 99 in Server settings.")
	}

	id := uuid.NewString()
	root, err := storage.OpenServer(s.dataDir, id)
	if err != nil {
		return Server{}, err
	}
	published := false
	defer func() {
		root.Close()
		if !published {
			_ = os.RemoveAll(s.dataPath(id))
		}
	}()
	var written int64
	for _, member := range zr.File {
		if err := ctx.Err(); err != nil {
			return Server{}, err
		}
		name, _ := storage.Clean(member.Name)
		if name == exportManifestName {
			continue
		}
		if member.FileInfo().IsDir() {
			err = root.MkdirAll(name, 0755)
		} else {
			var rc io.ReadCloser
			rc, err = member.Open()
			if err != nil {
				return Server{}, ErrInvalidArchive
			}
			var n int64
			n, err = storage.Write(ctx, root, name, rc, storage.MaxArchiveBytes-written, 0644|member.Mode().Perm()&0111)
			rc.Close()
			written += n
			if err == zip.ErrChecksum || err == zip.ErrFormat || err == io.ErrUnexpectedEOF {
				return Server{}, ErrInvalidArchive
			}
		}
		if err != nil {
			return Server{}, err
		}
	}

	s.portConfigMu.Lock()
	defer s.portConfigMu.Unlock()
	port := 0
	for attempt := 0; attempt < 20; attempt++ {
		if !in.NoHostPort {
			port, err = s.ports.Allocate(ctx)
			if err != nil {
				return Server{}, err
			}
		}
		if err := s.validateProposedPorts(ctx, "", port, nil); err != nil {
			return Server{}, err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, java_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', '', ?, ?, ?, ?)`,
			id, settings.Name, string(settings.ServerType), settings.Version, settings.Build, settings.RAMMB,
			settings.CPULimit, settings.MemoryLimitMB, port, StateStopped, now, now, settings.JavaVersion)
		if err == nil {
			break
		}
		if !isPortReservationError(err) {
			return Server{}, fmt.Errorf("insert imported server: %w", err)
		}
	}
	if err != nil {
		return Server{}, fmt.Errorf("%w: repeated reservation conflicts", ErrPortInUse)
	}
	defer func() {
		if !published {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = s.db.ExecContext(cleanupCtx, `DELETE FROM servers WHERE id=?`, id)
		}
	}()
	if err := s.createBackupSchedule(ctx, id); err != nil {
		return Server{}, err
	}
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	published = true
	return srv, nil
}
