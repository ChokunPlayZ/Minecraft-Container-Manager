//go:build !linux && !darwin

package servers

import "errors"

func dataDiskSpace(path string) (DataDisk, error) {
	return DataDisk{}, errors.New("filesystem space reporting is unavailable on this platform")
}
