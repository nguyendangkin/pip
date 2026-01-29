package archive

import (
	"os"
	"strings"
	"testing"
)

// TestZipSlip attempts to extract a file with ".." in the name
func TestZipSlip(t *testing.T) {
	// Setup malicious entry
	evilName := "../evil.txt"
	if os.PathSeparator == '\\' {
		evilName = "..\\evil.txt"
	}

	entry := FileEntry{
		Name:  evilName,
		Size:  10,
		Offset: 100,
		Mode:  0666,
	}

	// Create dummy reader with minimal fields
	reader := &Reader{}
	
	// Create temp output dir
	tmpDir, err := os.MkdirTemp("", "chin_test_zipslip")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Attempt extract
	err = reader.ExtractFile(entry, tmpDir, false)
	
	if err == nil {
		t.Fatal("Expected error for Zip Slip attempt, got nil")
	}

	if !strings.Contains(err.Error(), "security error") {
		t.Fatalf("Expected security error, got: %v", err)
	}
}
