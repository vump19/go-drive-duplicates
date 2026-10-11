package usecases

import (
	"context"
	"sort"
	"testing"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
)

// hashedFile creates a file whose content hash is known.
func hashedFile(id, name string, size int64, hash string) *entities.File {
	return &entities.File{ID: id, Name: name, Size: size, Hash: hash, HashCalculated: hash != ""}
}

func fileIDs(files []*entities.File) []string {
	ids := make([]string, 0, len(files))
	for _, f := range files {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	return ids
}

func assertIDs(t *testing.T, label string, files []*entities.File, want ...string) {
	t.Helper()
	got := fileIDs(files)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Errorf("%s = %v, 기대 = %v", label, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s = %v, 기대 = %v", label, got, want)
			return
		}
	}
}

func TestCompareFiles_HashMatch(t *testing.T) {
	uc := &FolderComparisonUseCase{}
	source := []*entities.File{
		hashedFile("s1", "a.txt", 10, "h1"),
		hashedFile("s2", "b.txt", 20, "h2"),
	}
	target := []*entities.File{
		hashedFile("t1", "a-copy.txt", 10, "h1"), // 이름이 달라도 내용이 같으면 중복
		hashedFile("t2", "c.txt", 30, "h3"),
	}

	result := uc.compareFilesWithUniqueTracking(source, target, true)

	assertIDs(t, "중복", result.Duplicates, "t1")
	assertIDs(t, "대상에만 있음", result.UniqueInTarget, "t2")
	assertIDs(t, "기준에만 있음", result.UniqueInSource, "s2")
}

func TestCompareFiles_SameNameSizeDifferentContentIsNotDuplicate(t *testing.T) {
	// 해시 비교 모드에서 이름·크기가 같아도 내용(해시)이 다르면 중복이 아님.
	// 중복으로 잘못 판정되면 사용자가 서로 다른 파일을 휴지통으로 옮길 수 있음.
	uc := &FolderComparisonUseCase{}
	source := []*entities.File{hashedFile("s1", "report.docx", 100, "h-old")}
	target := []*entities.File{hashedFile("t1", "report.docx", 100, "h-new")}

	result := uc.compareFilesWithUniqueTracking(source, target, true)

	assertIDs(t, "중복", result.Duplicates)
	assertIDs(t, "대상에만 있음", result.UniqueInTarget, "t1")
	assertIDs(t, "기준에만 있음", result.UniqueInSource, "s1")
}

func TestCompareFiles_NameSizeFallbackWithoutHash(t *testing.T) {
	// 해시가 없는 파일(예: Google 문서)은 이름+크기로 비교
	uc := &FolderComparisonUseCase{}
	source := []*entities.File{
		hashedFile("s1", "doc", 0, ""),
		hashedFile("s2", "photo.jpg", 50, "h1"),
	}
	target := []*entities.File{
		hashedFile("t1", "doc", 0, ""),
		hashedFile("t2", "photo.jpg", 50, ""), // 대상 쪽 해시 없음 → 이름+크기로 비교
	}

	result := uc.compareFilesWithUniqueTracking(source, target, true)

	assertIDs(t, "중복", result.Duplicates, "t1", "t2")
	assertIDs(t, "대상에만 있음", result.UniqueInTarget)
	assertIDs(t, "기준에만 있음", result.UniqueInSource)
}

func TestCompareFiles_ShallowComparisonIgnoresHash(t *testing.T) {
	uc := &FolderComparisonUseCase{}
	source := []*entities.File{hashedFile("s1", "a.txt", 10, "h1")}
	target := []*entities.File{
		hashedFile("t1", "a.txt", 10, "h-different"), // 빠른 비교: 이름+크기만 봄
		hashedFile("t2", "renamed.txt", 10, "h1"),    // 빠른 비교: 해시를 보지 않음
	}

	result := uc.compareFilesWithUniqueTracking(source, target, false)

	assertIDs(t, "중복", result.Duplicates, "t1")
	assertIDs(t, "대상에만 있음", result.UniqueInTarget, "t2")
	assertIDs(t, "기준에만 있음", result.UniqueInSource)
}

func TestShouldExcludeFolder(t *testing.T) {
	uc := &FolderComparisonUseCase{}
	exclude := []string{"node_modules", ".Git"}

	tests := map[string]bool{
		"node_modules":  true,
		"NODE_MODULES":  true, // 대소문자 무시
		".git":          true,
		"src":           false,
		"node_modules2": false, // 부분 일치는 제외하지 않음
	}
	for name, want := range tests {
		if got := uc.shouldExcludeFolder(name, exclude); got != want {
			t.Errorf("shouldExcludeFolder(%q) = %v, 기대 = %v", name, got, want)
		}
	}
	if uc.shouldExcludeFolder("anything", nil) {
		t.Error("제외 목록이 없으면 제외하지 않아야 함")
	}
}

func TestExtractFolderIdFromUrl(t *testing.T) {
	uc := &FolderComparisonUseCase{}
	const id = "1AbC_dEf-GhIjKlMnOpQrStUvWxYz0"

	valid := []string{
		"https://drive.google.com/drive/folders/" + id,
		"https://drive.google.com/drive/u/0/folders/" + id,
		"https://drive.google.com/drive/folders/" + id + "?usp=sharing",
		"https://drive.google.com/open?id=" + id,
		"https://drive.google.com/open?id=" + id + "&authuser=0",
		id,
	}
	for _, input := range valid {
		got, err := uc.ExtractFolderIdFromUrl(input)
		if err != nil || got != id {
			t.Errorf("ExtractFolderIdFromUrl(%q) = (%q, %v), 기대 = %q", input, got, err, id)
		}
	}

	for _, input := range []string{"", "short", "https://example.com/page"} {
		if got, err := uc.ExtractFolderIdFromUrl(input); err == nil {
			t.Errorf("ExtractFolderIdFromUrl(%q) = %q, 오류 기대", input, got)
		}
	}
}

func TestSplitPath(t *testing.T) {
	tests := map[string][]string{
		"a/b/c":  {"a", "b", "c"},
		"/a//b/": {"a", "b"},
		"":       nil,
		"한글/폴더":  {"한글", "폴더"},
	}
	for input, want := range tests {
		got := splitPath(input)
		if len(got) != len(want) {
			t.Errorf("splitPath(%q) = %v, 기대 = %v", input, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("splitPath(%q) = %v, 기대 = %v", input, got, want)
				break
			}
		}
	}
}

func TestSplitFileName(t *testing.T) {
	tests := []struct{ input, base, ext string }{
		{"photo.jpg", "photo", ".jpg"},
		{"archive.tar.gz", "archive.tar", ".gz"},
		{"README", "README", ""},
		{".bashrc", "", ".bashrc"},
	}
	for _, tt := range tests {
		base, ext := splitFileName(tt.input)
		if base != tt.base || ext != tt.ext {
			t.Errorf("splitFileName(%q) = (%q, %q), 기대 = (%q, %q)", tt.input, base, ext, tt.base, tt.ext)
		}
	}
}

// fakeFolderLister implements only ListFiles for name-conflict tests.
type fakeFolderLister struct {
	services.StorageProvider
	names []string
}

func (f *fakeFolderLister) ListFiles(ctx context.Context, folderID string) ([]*entities.File, error) {
	files := make([]*entities.File, 0, len(f.names))
	for _, name := range f.names {
		files = append(files, &entities.File{ID: name, Name: name})
	}
	return files, nil
}

func TestResolveFileNameConflict(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		existing []string
		want     string
	}{
		{nil, "report.pdf"},
		{[]string{"report.pdf"}, "report_1.pdf"},
		{[]string{"report.pdf", "report_1.pdf", "report_2.pdf"}, "report_3.pdf"},
		{[]string{"other.pdf"}, "report.pdf"},
	}
	for _, tt := range tests {
		uc := &FolderComparisonUseCase{storageProvider: &fakeFolderLister{names: tt.existing}}
		got, err := uc.resolveFileNameConflict(ctx, "folder", "report.pdf")
		if err != nil || got != tt.want {
			t.Errorf("기존 %v: resolveFileNameConflict = (%q, %v), 기대 = %q", tt.existing, got, err, tt.want)
		}
	}
}

// noopProgressService ignores progress updates.
type noopProgressService struct {
	services.ProgressService
}

func (noopProgressService) UpdateOperation(ctx context.Context, progressID int, processedItems int, currentStep string) error {
	return nil
}

func TestFindDuplicatesWithHashes_OnlyGroupsWithMultipleFiles(t *testing.T) {
	uc := &FolderComparisonUseCase{progressService: noopProgressService{}}
	files := []*entities.File{
		hashedFile("a1", "a.txt", 10, "ha"),
		hashedFile("a2", "a (1).txt", 10, "ha"),
		hashedFile("a3", "a-copy.txt", 10, "ha"),
		hashedFile("b1", "b.txt", 20, "hb"), // 단독 → 그룹 아님
	}

	groups, err := uc.findDuplicatesWithHashes(context.Background(), files, entities.NewProgress("test", 0))
	if err != nil {
		t.Fatalf("findDuplicatesWithHashes 실패: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("그룹 수 = %d, 기대 = 1", len(groups))
	}
	g := groups[0]
	assertIDs(t, "그룹 파일", g.Files, "a1", "a2", "a3")
	if g.Count != 3 || g.GetWastedSpace() != 20 {
		t.Errorf("Count = %d, 낭비 공간 = %d, 기대 = 3, 20", g.Count, g.GetWastedSpace())
	}
}
