package usecases

import (
	"context"
	"testing"

	"go-drive-duplicates/internal/domain/entities"
)

func TestGroupFilesByHash(t *testing.T) {
	uc := &DuplicateFindingUseCase{progressService: noopProgressService{}}
	files := []*entities.File{
		hashedFile("a1", "a.txt", 10, "ha"),
		hashedFile("a2", "a2.txt", 10, "ha"),
		hashedFile("b1", "b.txt", 20, "hb"),
		hashedFile("n1", "no-hash.txt", 30, ""), // 해시 없는 파일은 그룹화하지 않음
	}

	groups := uc.groupFilesByHash(context.Background(), files, entities.NewProgress("test", 0), nil)

	byHash := make(map[string]*entities.DuplicateGroup)
	for _, g := range groups {
		byHash[g.Hash] = g
	}
	if len(byHash) != 2 || byHash[""] != nil {
		t.Fatalf("그룹 해시 = %v, 기대 = ha, hb", byHash)
	}
	if !byHash["ha"].IsValid() || byHash["ha"].Count != 2 {
		t.Errorf("ha 그룹: Count = %d, IsValid = %v", byHash["ha"].Count, byHash["ha"].IsValid())
	}
	// 파일이 하나뿐인 그룹은 IsValid()로 걸러짐 (performDuplicateSearch에서 사용)
	if byHash["hb"].IsValid() {
		t.Error("파일 1개 그룹이 유효한 중복으로 판정됨")
	}
}

func TestFilterFilesBySize(t *testing.T) {
	uc := &DuplicateFindingUseCase{}
	files := []*entities.File{
		hashedFile("small", "s", 1023, "h1"),
		hashedFile("edge", "e", 1024, "h2"),
		hashedFile("big", "b", 4096, "h3"),
	}

	assertIDs(t, "1KB 이상", uc.filterFilesBySize(files, 1024), "edge", "big")
	assertIDs(t, "제한 없음", uc.filterFilesBySize(files, 0), "small", "edge", "big")
}

func TestFormatFileSize(t *testing.T) {
	tests := map[int64]string{
		0:                      "0 B",
		1023:                   "1023 B",
		1024:                   "1.0 KB",
		1536:                   "1.5 KB",
		1024 * 1024:            "1.0 MB",
		5 * 1024 * 1024 * 1024: "5.0 GB",
	}
	for bytes, want := range tests {
		if got := formatFileSize(bytes); got != want {
			t.Errorf("formatFileSize(%d) = %q, 기대 = %q", bytes, got, want)
		}
	}
}
