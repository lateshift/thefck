package main

import (
	"fmt"
	"time"
)

// FileRecord is the durable shape stored in bbolt and returned by the API.
// Keep the JSON field names stable so old indexes and the SPA agree.
type FileRecord struct {
	FileDate     time.Time `json:"filedate"`
	FileSize     int64     `json:"filesize"`
	AbsolutePath string    `json:"absolute_path"`
	Filename     string    `json:"filename"`
	Checksum     string    `json:"checksum"`
	Duplicate    bool      `json:"duplicate"`
}

// checksumKey groups records that have the same byte size and content hash.
func (r FileRecord) checksumKey() string {
	return fmt.Sprintf("%d:%s", r.FileSize, r.Checksum)
}

// scanSummary captures the counters printed at the end of a scan.
type scanSummary struct {
	FilesScanned    int
	NewFiles        int
	ChangedFiles    int
	MissingFiles    int
	DuplicateGroups int
}

// ingestResult carries the previous record and current duplicate matches.
type ingestResult struct {
	OldRecord  *FileRecord
	Duplicates []string
}
