package entities

import (
	"strings"
	"testing"
)

func TestBuildGoogleDriveQuery_SizeFilter(t *testing.T) {
	opts := &ListOptions{IncludeFiles: true, IncludeFolders: true, MinSize: 1024, MaxSize: 1048576}

	query := opts.BuildGoogleDriveQuery("folder1")

	for _, want := range []string{"size >= 1024", "size <= 1048576"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q does not contain %q", query, want)
		}
	}
}

func TestBuildGoogleDriveQuery_NoSizeFilter(t *testing.T) {
	opts := &ListOptions{IncludeFiles: true, IncludeFolders: true}

	query := opts.BuildGoogleDriveQuery("folder1")

	if strings.Contains(query, "size") {
		t.Errorf("query %q should not contain a size filter", query)
	}
}
