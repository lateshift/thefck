package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cespare/xxhash/v2"
)

// buildRecord converts file system metadata into the record persisted in the
// index. The checksum is content-only; size is stored separately and used in
// the checksum bucket key to keep duplicate lookup cheap.
func buildRecord(path string, info os.FileInfo) (FileRecord, error) {
	checksum, err := checksum(path)
	if err != nil {
		return FileRecord{}, err
	}

	return FileRecord{
		FileDate:     info.ModTime().UTC(),
		FileSize:     info.Size(),
		AbsolutePath: path,
		Filename:     filepath.Base(path),
		Checksum:     checksum,
	}, nil
}

func checksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%016x", h.Sum64()), nil
}
