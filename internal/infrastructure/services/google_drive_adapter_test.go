package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
