package repostructure

import (
	"fmt"
	"io"
	"os"
)

const maxMetadataFileSize int64 = 1 << 20

func readMetadataFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symbolic links are not allowed")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if info.Size() > maxMetadataFileSize {
		return nil, fmt.Errorf("file exceeds %d-byte metadata limit", maxMetadataFileSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("file changed while being inspected")
	}
	if openedInfo.Size() > maxMetadataFileSize {
		return nil, fmt.Errorf("file exceeds %d-byte metadata limit", maxMetadataFileSize)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxMetadataFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxMetadataFileSize {
		return nil, fmt.Errorf("file exceeds %d-byte metadata limit", maxMetadataFileSize)
	}
	return data, nil
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func directoryExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}
