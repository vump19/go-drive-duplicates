package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-drive-duplicates/internal/domain/entities"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestBuildMetadataUpdate_RejectsInvalidInput(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"빈 메타데이터":         nil,
		"빈 이름":            {"name": ""},
		"문자열이 아닌 이름":      {"name": 1},
		"불리언이 아닌 starred": {"starred": "yes"},
		"문자열이 아닌 설명":      {"description": 3},
		"문자열이 아닌 속성 값":    {"properties": map[string]interface{}{"key": 1}},
		"휴지통 이동 키":        {"trashed": true},
		"파일 이동 키":         {"parents": []string{"folder"}},
	}
	for name, metadata := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := buildMetadataUpdate(metadata); err == nil {
				t.Errorf("%v에 오류가 반환되지 않음", metadata)
			}
		})
	}
}

func TestUpdateFileMetadata_SendsPatchRequest(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file1"}`))
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("Drive 서비스 생성 실패: %v", err)
	}
	adapter := &GoogleDriveAdapter{service: service}

	err = adapter.UpdateFileMetadata(context.Background(), "file1", map[string]interface{}{
		"name":        "renamed.txt",
		"description": "",
		"starred":     false,
		"properties":  map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Fatalf("UpdateFileMetadata 실패: %v", err)
	}

	if gotMethod != http.MethodPatch || gotPath != "/files/file1" {
		t.Errorf("요청 = %s %s, 기대 = PATCH /files/file1", gotMethod, gotPath)
	}
	if body["name"] != "renamed.txt" {
		t.Errorf("name = %v", body["name"])
	}
	// zero 값도 전송되어야 별 해제·설명 삭제가 가능
	if v, ok := body["starred"]; !ok || v != false {
		t.Errorf("starred=false가 전송되지 않음: %v", body)
	}
	if v, ok := body["description"]; !ok || v != "" {
		t.Errorf("빈 description이 전송되지 않음: %v", body)
	}
	if props, _ := body["properties"].(map[string]interface{}); props["key"] != "value" {
		t.Errorf("properties = %v", body["properties"])
	}
	if _, ok := body["trashed"]; ok {
		t.Error("trashed가 전송됨")
	}
}

func TestUpdateFileMetadata_RequiresID(t *testing.T) {
	adapter := &GoogleDriveAdapter{}
	if err := adapter.UpdateFileMetadata(context.Background(), "", map[string]interface{}{"name": "a"}); err == nil {
		t.Error("빈 ID에 오류가 반환되지 않음")
	}
}

func TestDriveOrderBy(t *testing.T) {
	tests := []struct {
		sortBy, sortOrder, want string
	}{
		{"name", "asc", "name"},
		{"name", "desc", "name desc"},
		{"size", "asc", "quotaBytesUsed"},
		{"modified", "desc", "modifiedTime desc"},
		{"created", "asc", "createdTime"},
		{"type", "desc", "folder,name"},
		{"", "asc", ""},
	}
	for _, tt := range tests {
		got := driveOrderBy(&entities.ListOptions{SortBy: tt.sortBy, SortOrder: tt.sortOrder})
		if got != tt.want {
			t.Errorf("driveOrderBy(%q, %q) = %q, 기대 = %q", tt.sortBy, tt.sortOrder, got, tt.want)
		}
	}
}

func TestListFolderContents_SendsDriveOrderBy(t *testing.T) {
	var gotOrderBy string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrderBy = r.URL.Query().Get("orderBy")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("Drive 서비스 생성 실패: %v", err)
	}
	adapter := &GoogleDriveAdapter{service: service}

	// 탐색기 화면의 정렬 옵션 값
	tests := map[string]string{
		"name":     "name",
		"modified": "modifiedTime desc",
		"size":     "quotaBytesUsed desc",
		"type":     "folder,name",
	}
	for sortBy, want := range tests {
		options := entities.DefaultListOptions()
		options.SortBy = sortBy
		if sortBy != "name" {
			options.SortOrder = "desc"
		}
		if _, err := adapter.ListFolderContents(context.Background(), "folder1", options); err != nil {
			t.Errorf("sortBy=%q: ListFolderContents 실패: %v", sortBy, err)
			continue
		}
		if gotOrderBy != want {
			t.Errorf("sortBy=%q: orderBy = %q, 기대 = %q", sortBy, gotOrderBy, want)
		}
	}
}

func TestListFoldersRecursive_ReportsSubfoldersAtMaxDepth(t *testing.T) {
	// root ─┬─ A ── A1
	//       └─ B
	subfolders := map[string][]string{"root": {"A", "B"}, "A": {"A1"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/files" {
			// Files.Get: folder metadata
			id := strings.TrimPrefix(r.URL.Path, "/files/")
			json.NewEncoder(w).Encode(map[string]string{"id": id, "name": "folder-" + id})
			return
		}
		// Files.List: only subfolder queries return results
		q := r.URL.Query().Get("q")
		var files []map[string]string
		if strings.Contains(q, "mimeType = 'application/vnd.google-apps.folder'") {
			for parent, children := range subfolders {
				if strings.HasPrefix(q, "'"+parent+"' in parents") {
					for _, id := range children {
						files = append(files, map[string]string{"id": id, "name": "folder-" + id})
					}
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"files": files})
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("Drive 서비스 생성 실패: %v", err)
	}
	adapter := &GoogleDriveAdapter{service: service}

	nodes, err := adapter.ListFoldersRecursive(context.Background(), "root", 1)
	if err != nil {
		t.Fatalf("ListFoldersRecursive 실패: %v", err)
	}

	hasChildren := make(map[string]bool)
	for _, n := range nodes {
		hasChildren[n.ID] = n.HasChildren
	}
	if len(nodes) != 3 {
		t.Fatalf("노드 수 = %d, 기대 = 3 (root, A, B)", len(nodes))
	}
	// 최대 깊이에 있는 A는 하위 폴더가 있으므로 펼치기 토글이 표시되어야 함
	want := map[string]bool{"root": true, "A": true, "B": false}
	for id, w := range want {
		if hasChildren[id] != w {
			t.Errorf("%s.HasChildren = %v, 기대 = %v", id, hasChildren[id], w)
		}
	}
}

func TestSearchFilesWithOptions_EscapesQuery(t *testing.T) {
	var gotQ string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(),
		option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("Drive 서비스 생성 실패: %v", err)
	}
	adapter := &GoogleDriveAdapter{service: service}

	if _, err := adapter.SearchFilesWithOptions(context.Background(), "John's", nil); err != nil {
		t.Fatalf("SearchFilesWithOptions 실패: %v", err)
	}
	if !strings.HasPrefix(gotQ, `name contains 'John\'s'`) {
		t.Errorf("q = %q, 작은따옴표가 이스케이프되지 않음", gotQ)
	}
}
