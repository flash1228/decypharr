package storage

import (
	"path/filepath"
	"strings"
)

// SelectMainM2tsFile filters a file list for Blu-ray/DVD rips that contain
// multiple .m2ts files. In a BDMV structure the main feature is always the
// largest file; everything else (menu animations, trailers, bonus content) is
// significantly smaller. Probing all of them causes a flood of debrid CDN
// requests that triggers 429 rate-limiting.
//
// When 2 or more .m2ts files are present, only the largest is kept; all others
// are removed from the returned slice. Non-.m2ts files are always preserved.
//
// Returns the filtered slice and true when selection was applied, or the
// original slice and false when there was nothing to do.
func SelectMainM2tsFile(files []*File) ([]*File, bool) {
	var m2tsFiles []*File
	var otherFiles []*File

	for _, f := range files {
		if strings.ToLower(filepath.Ext(f.Name)) == ".m2ts" {
			m2tsFiles = append(m2tsFiles, f)
		} else {
			otherFiles = append(otherFiles, f)
		}
	}

	if len(m2tsFiles) < 2 {
		return files, false
	}

	// Largest .m2ts is the main feature.
	main := m2tsFiles[0]
	for _, f := range m2tsFiles[1:] {
		if f.Size > main.Size {
			main = f
		}
	}

	return append(otherFiles, main), true
}
