//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func canonicalTestTempDir(t *testing.T) string {
	t.Helper()
	// Windows CI can use an 8.3 TEMP path. Git and PowerShell return long names,
	// so use the same canonical spelling for fixtures and directory assertions.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
