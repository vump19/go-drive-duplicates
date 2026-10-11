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

func TestEscapeDriveQueryValue(t *testing.T) {
	tests := map[string]string{
		"report":       "report",
		"John's file":  `John\'s file`,
		`back\slash`:   `back\\slash`,
		`it's a \test`: `it\'s a \\test`,
	}
	for input, want := range tests {
		if got := EscapeDriveQueryValue(input); got != want {
			t.Errorf("EscapeDriveQueryValue(%q) = %q, 기대 = %q", input, got, want)
		}
	}
}

func TestBuildGoogleDriveQuery_EscapesSearchQuery(t *testing.T) {
	opts := &ListOptions{IncludeFiles: true, IncludeFolders: true, SearchQuery: "John's"}

	query := opts.BuildGoogleDriveQuery("folder1")

	if !strings.Contains(query, `name contains 'John\'s'`) {
		t.Errorf("검색어가 이스케이프되지 않음: %q", query)
	}
}
