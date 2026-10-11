package usecases

import (
	"context"
	"errors"
	"testing"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
)

// trashFixture holds the scanned (DB) state and the current Drive state of files.
type trashFixture struct {
	scanned map[string]*entities.File // DB 기록 (중복 검사 시점)
	current map[string]*entities.File // Drive의 현재 상태 (nil이면 조회 실패)
	exists  map[string]bool           // 휴지통에 없고 존재하는지
	trashed []string
}

type trashFileRepo struct {
	repositories.FileRepository
	f *trashFixture
}

func (r *trashFileRepo) GetByID(ctx context.Context, id string) (*entities.File, error) {
	return r.f.scanned[id], nil
}

func (r *trashFileRepo) GetByHash(ctx context.Context, hash string) ([]*entities.File, error) {
	var files []*entities.File
	for _, f := range r.f.scanned {
		if f.Hash == hash {
			files = append(files, f)
		}
	}
	return files, nil
}

type trashFileService struct {
	services.FileService
	f *trashFixture
}

func (s *trashFileService) RefreshFileMetadata(ctx context.Context, id string) (*entities.File, error) {
	if s.f.current[id] == nil {
		return nil, errors.New("not found")
	}
	copied := *s.f.current[id]
	return &copied, nil
}

type trashStorage struct {
	services.StorageProvider
	f *trashFixture
}

func (s *trashStorage) FileExists(ctx context.Context, id string) (bool, error) {
	return s.f.exists[id], nil
}

func (s *trashStorage) TrashFile(ctx context.Context, id string) error {
	s.f.trashed = append(s.f.trashed, id)
	return nil
}

// newTrashFixture creates files a, b, c that all had hash "h" when scanned and still do.
func newTrashFixture() *trashFixture {
	f := &trashFixture{scanned: map[string]*entities.File{}, current: map[string]*entities.File{}, exists: map[string]bool{}}
	for _, id := range []string{"a", "b", "c"} {
		f.scanned[id] = hashedFile(id, id+".jpg", 10, "h")
		f.current[id] = hashedFile(id, id+".jpg", 10, "h")
		f.exists[id] = true
	}
	return f
}

func (f *trashFixture) useCase() *DuplicateFindingUseCase {
	return &DuplicateFindingUseCase{
		fileRepo:        &trashFileRepo{f: f},
		fileService:     &trashFileService{f: f},
		storageProvider: &trashStorage{f: f},
	}
}

func TestTrashFile_StillDuplicate(t *testing.T) {
	f := newTrashFixture()

	if err := f.useCase().TrashFile(context.Background(), "a"); err != nil {
		t.Fatalf("TrashFile 실패: %v", err)
	}
	if len(f.trashed) != 1 || f.trashed[0] != "a" {
		t.Errorf("휴지통 이동 = %v, 기대 = [a]", f.trashed)
	}
}

func TestTrashFile_RefusesWhenContentChanged(t *testing.T) {
	f := newTrashFixture()
	f.current["a"].Hash = "h-edited" // 검사 이후 a를 수정함

	err := f.useCase().TrashFile(context.Background(), "a")

	if !errors.Is(err, ErrDuplicateContentChanged) {
		t.Errorf("err = %v, 기대 = ErrDuplicateContentChanged", err)
	}
	if len(f.trashed) != 0 {
		t.Errorf("내용이 바뀐 파일이 휴지통으로 이동됨: %v", f.trashed)
	}
}

func TestTrashFile_ProtectsLastCopy(t *testing.T) {
	tests := map[string]func(f *trashFixture){
		"다른 사본이 모두 휴지통에 있음": func(f *trashFixture) { f.exists["b"], f.exists["c"] = false, false },
		"다른 사본의 내용이 모두 바뀜":  func(f *trashFixture) { f.current["b"].Hash, f.current["c"].Hash = "x", "y" },
		"다른 사본 조회 실패":       func(f *trashFixture) { f.current["b"], f.current["c"] = nil, nil },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			f := newTrashFixture()
			setup(f)

			err := f.useCase().TrashFile(context.Background(), "a")

			if !errors.Is(err, ErrNoRemainingDuplicate) {
				t.Errorf("err = %v, 기대 = ErrNoRemainingDuplicate", err)
			}
			if len(f.trashed) != 0 {
				t.Errorf("마지막 사본이 휴지통으로 이동됨: %v", f.trashed)
			}
		})
	}
}

func TestTrashFile_OneRemainingCopyIsEnough(t *testing.T) {
	f := newTrashFixture()
	f.exists["b"] = false      // b는 이미 휴지통
	f.current["c"].Name = "c2" // c는 이름만 바뀌고 내용은 같음

	if err := f.useCase().TrashFile(context.Background(), "a"); err != nil {
		t.Fatalf("TrashFile 실패: %v", err)
	}
	if len(f.trashed) != 1 {
		t.Errorf("휴지통 이동 = %v, 기대 = [a]", f.trashed)
	}
}

func TestTrashFile_RefusesWithoutScanRecord(t *testing.T) {
	f := newTrashFixture()
	delete(f.scanned, "a")

	if err := f.useCase().TrashFile(context.Background(), "a"); !errors.Is(err, ErrDuplicateNotVerifiable) {
		t.Errorf("err = %v, 기대 = ErrDuplicateNotVerifiable", err)
	}
	if len(f.trashed) != 0 {
		t.Errorf("검사 기록 없는 파일이 휴지통으로 이동됨: %v", f.trashed)
	}
}
