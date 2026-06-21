package main

import (
	"os"
	"path/filepath"
	"strings"
)

func shouldPrunePath(path string, seen map[string]struct{}, excluded map[string]struct{}) (bool, bool) {
	_, isExcluded := excluded[path]
	if isExcluded {
		return true, true
	}
	if seen == nil {
		return !regularFileExists(path), false
	}
	_, ok := seen[path]
	return !ok, false
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isUnderRoot(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
}

func contains(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}
