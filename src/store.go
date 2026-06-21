package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	filesBucket      = []byte("files")      // absolute path -> FileRecord JSON
	sumsBucket       = []byte("sums")       // checksum key -> []absolute paths
	duplicatesBucket = []byte("duplicates") // checksum key -> []absolute paths
)

type IndexStore struct {
	db *bolt.DB
}

// OpenIndexStore opens the bbolt database and creates buckets for writers.
func OpenIndexStore(path string, readOnly bool) (*IndexStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		ReadOnly: readOnly,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &IndexStore{db: db}
	if !readOnly {
		if err := store.ensureBuckets(); err != nil {
			db.Close()
			return nil, err
		}
	}

	return store, nil
}

func (s *IndexStore) Close() error {
	return s.db.Close()
}

func (s *IndexStore) ensureBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{filesBucket, sumsBucket, duplicatesBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
}

// Ingest stores one record, refreshes its checksum group, and returns current
// duplicate matches for quick reporting.
func (s *IndexStore) Ingest(record FileRecord) (ingestResult, error) {
	var result ingestResult

	err := s.db.Update(func(tx *bolt.Tx) error {
		files := tx.Bucket(filesBucket)
		sums := tx.Bucket(sumsBucket)
		duplicates := tx.Bucket(duplicatesBucket)

		oldRecord, oldChecksumKey, err := loadRecord(files.Get([]byte(record.AbsolutePath)), record.AbsolutePath)
		if err != nil {
			return fmt.Errorf("load existing record for %s: %w", record.AbsolutePath, err)
		}
		result.OldRecord = oldRecord

		newChecksumKey := record.checksumKey()
		if oldChecksumKey != "" && oldChecksumKey != newChecksumKey {
			if err := removePathFromChecksumGroup(sums, oldChecksumKey, record.AbsolutePath); err != nil {
				return err
			}
			if _, err := refreshDuplicateGroup(files, sums, duplicates, oldChecksumKey); err != nil {
				return err
			}
		}

		paths, err := loadPaths(sums.Get([]byte(newChecksumKey)))
		if err != nil {
			return err
		}
		if !contains(paths, record.AbsolutePath) {
			paths = append(paths, record.AbsolutePath)
			sort.Strings(paths)
		}
		if err := storePaths(sums, newChecksumKey, paths); err != nil {
			return err
		}
		if err := storeRecord(files, record); err != nil {
			return err
		}

		group, err := refreshDuplicateGroup(files, sums, duplicates, newChecksumKey)
		if err != nil {
			return err
		}
		for _, path := range group {
			if path != record.AbsolutePath {
				result.Duplicates = append(result.Duplicates, path)
			}
		}

		return nil
	})

	return result, err
}

// ListFiles returns every indexed file record, sorted for stable API output.
func (s *IndexStore) ListFiles() ([]FileRecord, error) {
	var records []FileRecord

	err := s.db.View(func(tx *bolt.Tx) error {
		files := tx.Bucket(filesBucket)
		if files == nil {
			return nil
		}
		return files.ForEach(func(key, value []byte) error {
			record, _, err := loadRecord(value, absoluteIndexPath(string(key)))
			if err != nil {
				return err
			}
			if record != nil {
				records = append(records, *record)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].AbsolutePath < records[j].AbsolutePath
	})
	return records, nil
}

func (s *IndexStore) CountDuplicateGroups() (int, error) {
	count := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		duplicates := tx.Bucket(duplicatesBucket)
		if duplicates == nil {
			return nil
		}
		return duplicates.ForEach(func(_, _ []byte) error {
			count++
			return nil
		})
	})
	return count, err
}

// PruneMissingFiles removes stale records. If seen is nil it checks the file
// system directly; otherwise it prunes anything under root that was not scanned.
func (s *IndexStore) PruneMissingFiles(root string, seen map[string]struct{}, excluded map[string]struct{}) ([]FileRecord, error) {
	var missing []FileRecord

	err := s.db.Update(func(tx *bolt.Tx) error {
		files := tx.Bucket(filesBucket)
		sums := tx.Bucket(sumsBucket)
		duplicates := tx.Bucket(duplicatesBucket)
		checksumKeysToRefresh := make(map[string]struct{})

		cursor := files.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			indexedPath := string(key)
			absolutePath := absoluteIndexPath(indexedPath)
			if !isUnderRoot(absolutePath, root) {
				continue
			}
			shouldPrune, isExcluded := shouldPrunePath(absolutePath, seen, excluded)
			if !shouldPrune {
				continue
			}

			record, checksumKey, err := loadRecord(value, absolutePath)
			if err != nil {
				return err
			}
			if record != nil && !isExcluded {
				missing = append(missing, *record)
			}
			if checksumKey != "" {
				checksumKeysToRefresh[checksumKey] = struct{}{}
				if err := removePathFromChecksumGroup(sums, checksumKey, indexedPath); err != nil {
					return err
				}
				if indexedPath != absolutePath {
					if err := removePathFromChecksumGroup(sums, checksumKey, absolutePath); err != nil {
						return err
					}
				}
			}
			if err := cursor.Delete(); err != nil {
				return err
			}
		}

		sort.Slice(missing, func(i, j int) bool {
			return missing[i].AbsolutePath < missing[j].AbsolutePath
		})
		for checksumKey := range checksumKeysToRefresh {
			if _, err := refreshDuplicateGroup(files, sums, duplicates, checksumKey); err != nil {
				return err
			}
		}
		return nil
	})

	return missing, err
}

func refreshDuplicateGroup(files, sums, duplicates *bolt.Bucket, checksumKey string) ([]string, error) {
	paths, err := loadPaths(sums.Get([]byte(checksumKey)))
	if err != nil {
		return nil, err
	}

	paths = compactExistingPaths(files, paths)
	if len(paths) == 0 {
		if err := sums.Delete([]byte(checksumKey)); err != nil {
			return nil, err
		}
		return nil, duplicates.Delete([]byte(checksumKey))
	}
	if err := storePaths(sums, checksumKey, paths); err != nil {
		return nil, err
	}

	duplicate := len(paths) > 1
	if duplicate {
		if err := storePaths(duplicates, checksumKey, paths); err != nil {
			return nil, err
		}
	} else if err := duplicates.Delete([]byte(checksumKey)); err != nil {
		return nil, err
	}

	for _, path := range paths {
		record, _, err := loadRecord(files.Get([]byte(path)), path)
		if err != nil {
			return nil, err
		}
		if record == nil || record.Duplicate == duplicate {
			continue
		}
		record.Duplicate = duplicate
		if err := storeRecord(files, *record); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func compactExistingPaths(files *bolt.Bucket, paths []string) []string {
	filtered := paths[:0]
	for _, path := range paths {
		if files.Get([]byte(path)) != nil {
			filtered = append(filtered, path)
		}
	}
	sort.Strings(filtered)
	return filtered
}

func removePathFromChecksumGroup(bucket *bolt.Bucket, checksumKey string, path string) error {
	paths, err := loadPaths(bucket.Get([]byte(checksumKey)))
	if err != nil {
		return err
	}

	filtered := paths[:0]
	for _, p := range paths {
		if p != path {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return bucket.Delete([]byte(checksumKey))
	}
	return storePaths(bucket, checksumKey, filtered)
}

func loadRecord(data []byte, path string) (*FileRecord, string, error) {
	if data == nil {
		return nil, "", nil
	}

	var record FileRecord
	if err := json.Unmarshal(data, &record); err == nil && record.AbsolutePath != "" {
		record.AbsolutePath = absoluteIndexPath(record.AbsolutePath)
		record.Filename = filepath.Base(record.AbsolutePath)
		return &record, record.checksumKey(), nil
	}

	legacyChecksumKey := string(data)
	if legacyChecksumKey == "" {
		return nil, "", errors.New("empty file record")
	}
	return legacyRecord(path, legacyChecksumKey), legacyChecksumKey, nil
}

func legacyRecord(path string, checksumKey string) *FileRecord {
	parts := strings.SplitN(checksumKey, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil
	}
	return &FileRecord{
		FileSize:     size,
		AbsolutePath: absoluteIndexPath(path),
		Filename:     filepath.Base(path),
		Checksum:     parts[1],
	}
}

func absoluteIndexPath(indexedPath string) string {
	if filepath.IsAbs(indexedPath) {
		return indexedPath
	}

	absolutePath, err := filepath.Abs(indexedPath)
	if err != nil {
		return indexedPath
	}
	return absolutePath
}

func storeRecord(bucket *bolt.Bucket, record FileRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return bucket.Put([]byte(record.AbsolutePath), encoded)
}

func loadPaths(data []byte) ([]string, error) {
	if data == nil {
		return []string{}, nil
	}

	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func storePaths(bucket *bolt.Bucket, checksumKey string, paths []string) error {
	encoded, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	return bucket.Put([]byte(checksumKey), encoded)
}
