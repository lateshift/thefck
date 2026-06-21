package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ScanOptions struct {
	DBPath           string
	ReportChanges    bool
	ReportDuplicates bool
}

// RunScan walks root recursively, stores current file metadata, and refreshes
// duplicate groups. The database file itself is ignored if it sits under root.
func RunScan(root string, opts ScanOptions) error {
	absRoot, err := cleanDirectory(root)
	if err != nil {
		return err
	}

	absDBPath, err := filepath.Abs(opts.DBPath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}

	store, err := OpenIndexStore(opts.DBPath, false)
	if err != nil {
		return err
	}
	defer store.Close()

	excludedPaths := map[string]struct{}{absDBPath: {}}
	seen := make(map[string]struct{})
	summary := scanSummary{}

	// Remove stale rows before scanning so duplicate reporting never points at
	// files that have already disappeared from disk.
	missing, err := store.PruneMissingFiles(absRoot, nil, excludedPaths)
	if err != nil {
		return err
	}
	reportMissing(missing, opts.ReportChanges, &summary)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "skip: %s: %v\n", path, walkErr)
			return nil
		}
		return scanEntry(store, path, d, absDBPath, seen, opts, &summary)
	})
	if err != nil {
		return err
	}

	// The second prune is a safety net for unreadable entries or files skipped
	// by WalkDir after the initial cleanup.
	missing, err = store.PruneMissingFiles(absRoot, seen, excludedPaths)
	if err != nil {
		return err
	}
	reportMissing(missing, opts.ReportChanges, &summary)

	duplicateGroups, err := store.CountDuplicateGroups()
	if err != nil {
		return err
	}
	summary.DuplicateGroups = duplicateGroups

	fmt.Printf("Indexed %d files under %s\n", summary.FilesScanned, absRoot)
	if opts.ReportChanges {
		fmt.Printf("Changes: %d new, %d changed, %d missing\n", summary.NewFiles, summary.ChangedFiles, summary.MissingFiles)
	}
	fmt.Printf("Duplicate groups: %d\n", summary.DuplicateGroups)

	return nil
}

func cleanDirectory(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve scan root: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("stat scan root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scan root is not a directory: %s", absRoot)
	}

	return absRoot, nil
}

func scanEntry(store *IndexStore, path string, d os.DirEntry, absDBPath string, seen map[string]struct{}, opts ScanOptions, summary *scanSummary) error {
	info, err := d.Info()
	if err != nil {
		fmt.Fprintf(os.Stderr, "skip: %s: %v\n", path, err)
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skip: %s: %v\n", path, err)
		return nil
	}
	if absPath == absDBPath {
		return nil
	}

	record, err := buildRecord(absPath, info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash error: %s: %v\n", absPath, err)
		return nil
	}

	result, err := store.Ingest(record)
	if err != nil {
		return err
	}

	seen[absPath] = struct{}{}
	summary.FilesScanned++
	reportFileChanges(absPath, result.OldRecord, record, opts.ReportChanges, summary)
	reportDuplicates(record, result.Duplicates, opts.ReportDuplicates)
	return nil
}

func reportMissing(missing []FileRecord, enabled bool, summary *scanSummary) {
	summary.MissingFiles += len(missing)
	if !enabled {
		return
	}
	for _, record := range missing {
		fmt.Printf("MISSING: %s\n", record.AbsolutePath)
	}
}

func reportFileChanges(path string, old *FileRecord, current FileRecord, enabled bool, summary *scanSummary) {
	if old == nil {
		summary.NewFiles++
		if enabled {
			fmt.Printf("NEW: %s\n", path)
		}
		return
	}
	if !recordChanged(*old, current) {
		return
	}

	summary.ChangedFiles++
	if enabled {
		fmt.Printf("CHANGED: %s\n", path)
		fmt.Printf("  size: %d -> %d\n", old.FileSize, current.FileSize)
		fmt.Printf("  date: %s -> %s\n", old.FileDate.Format(time.RFC3339Nano), current.FileDate.Format(time.RFC3339Nano))
		fmt.Printf("  checksum: %s -> %s\n", old.Checksum, current.Checksum)
	}
}

func reportDuplicates(record FileRecord, duplicates []string, enabled bool) {
	if !enabled || len(duplicates) == 0 {
		return
	}

	fmt.Printf("\nDUPLICATE: %s\n", record.AbsolutePath)
	for _, other := range duplicates {
		fmt.Printf("  matches: %s\n", other)
	}
}

func recordChanged(old FileRecord, current FileRecord) bool {
	return old.FileSize != current.FileSize ||
		!old.FileDate.Equal(current.FileDate) ||
		old.Checksum != current.Checksum ||
		old.Filename != current.Filename
}
