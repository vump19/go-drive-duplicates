package usecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
)

// fakeFolderCreator implements only the StorageProvider method used by CreateFolder.
type fakeFolderCreator struct {
	services.StorageProvider
	createdParent, createdName string
	err                        error
}

func (f *fakeFolderCreator) CreateFolder(ctx context.Context, parentID, folderName string) (string, error) {
	f.createdParent, f.createdName = parentID, folderName
	return "new-folder-id", f.err
}

// fakeFolderCreatorWithMetadata also provides full metadata of the created folder.
type fakeFolderCreatorWithMetadata struct {
	fakeFolderCreator
}

func (f *fakeFolderCreatorWithMetadata) GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error) {
	item := entities.NewFileListItem(fileID, "from-metadata", "application/vnd.google-apps.folder")
	item.WebViewLink = "link"
	return item, nil
}

func TestCreateFolder_UsesStorageProvider(t *testing.T) {
	provider := &fakeFolderCreator{}
	uc := NewFileExplorerUseCase(provider, nil)

	item, err := uc.CreateFolder(context.Background(), "parent-id", "새 폴더")
	if err != nil {
		t.Fatalf("CreateFolder 실패: %v", err)
	}
	if provider.createdParent != "parent-id" || provider.createdName != "새 폴더" {
		t.Errorf("CreateFolder 호출 인자 = (%q, %q)", provider.createdParent, provider.createdName)
	}
	if item.ID != "new-folder-id" || item.Name != "새 폴더" || !item.IsFolder {
		t.Errorf("반환 항목 = %+v", item)
	}
}

func TestCreateFolder_ReturnsFullMetadataWhenAvailable(t *testing.T) {
	uc := NewFileExplorerUseCase(&fakeFolderCreatorWithMetadata{}, nil)

	item, err := uc.CreateFolder(context.Background(), "parent-id", "새 폴더")
	if err != nil {
		t.Fatalf("CreateFolder 실패: %v", err)
	}
	if item.Name != "from-metadata" || item.WebViewLink != "link" {
		t.Errorf("메타데이터가 반영되지 않음: %+v", item)
	}
}

func TestCreateFolder_Errors(t *testing.T) {
	ctx := context.Background()

	uc := NewFileExplorerUseCase(&fakeFolderCreator{}, nil)
	if _, err := uc.CreateFolder(ctx, "", "name"); err == nil {
		t.Error("빈 부모 ID에 오류가 반환되지 않음")
	}
	if _, err := uc.CreateFolder(ctx, "parent", ""); err == nil {
		t.Error("빈 폴더명에 오류가 반환되지 않음")
	}

	failing := NewFileExplorerUseCase(&fakeFolderCreator{err: errors.New("quota")}, nil)
	if _, err := failing.CreateFolder(ctx, "parent", "name"); err == nil {
		t.Error("Drive 오류가 전달되지 않음")
	}
}

// memoryOperationRepo is an in-memory FileOperationRepository for tests.
type memoryOperationRepo struct {
	repositories.FileOperationRepository
	mu     sync.Mutex
	nextID int
	ops    map[int]entities.FileOperation
}

func newMemoryOperationRepo() *memoryOperationRepo {
	return &memoryOperationRepo{ops: make(map[int]entities.FileOperation)}
}

func (r *memoryOperationRepo) Create(ctx context.Context, op *entities.FileOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	op.ID = r.nextID
	r.ops[op.ID] = *op
	return nil
}

func (r *memoryOperationRepo) GetByID(ctx context.Context, id int) (*entities.FileOperation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.ops[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &op, nil
}

func (r *memoryOperationRepo) Update(ctx context.Context, op *entities.FileOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops[op.ID] = *op
	return nil
}

func (r *memoryOperationRepo) UpdateProgress(ctx context.Context, id int, files int, bytes int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	op := r.ops[id]
	op.ProcessedFiles, op.ProcessedBytes = files, bytes
	r.ops[id] = op
	return nil
}

// fakeFileMover implements the methods used by copy/move/delete.
// Files listed in block wait until the context is cancelled.
type fakeFileMover struct {
	services.StorageProvider
	mu      sync.Mutex
	calls   []string
	trashed []string
	deleted []string
	fail    map[string]bool
	block   map[string]bool
	started chan string
}

func (f *fakeFileMover) process(ctx context.Context, fileID string) error {
	f.mu.Lock()
	f.calls = append(f.calls, fileID)
	f.mu.Unlock()
	if f.started != nil {
		f.started <- fileID
	}
	if f.block[fileID] {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.fail[fileID] {
		return errors.New("drive error")
	}
	return nil
}

func (f *fakeFileMover) CopyFile(ctx context.Context, fileID, target string) (string, error) {
	return "copy-of-" + fileID, f.process(ctx, fileID)
}

func (f *fakeFileMover) MoveFile(ctx context.Context, fileID, target string) error {
	return f.process(ctx, fileID)
}

func (f *fakeFileMover) TrashFile(ctx context.Context, fileID string) error {
	f.mu.Lock()
	f.trashed = append(f.trashed, fileID)
	f.mu.Unlock()
	return f.process(ctx, fileID)
}

func (f *fakeFileMover) DeleteFile(ctx context.Context, fileID string) error {
	f.mu.Lock()
	f.deleted = append(f.deleted, fileID)
	f.mu.Unlock()
	return f.process(ctx, fileID)
}

func (f *fakeFileMover) GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error) {
	item := entities.NewFileListItem(fileID, "name-"+fileID, "text/plain")
	item.Size = 10
	return item, nil
}

func (f *fakeFileMover) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// waitForFinish polls the operation until it reaches a final status.
func waitForFinish(t *testing.T, uc *FileExplorerUseCase, id int) *entities.FileOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, err := uc.GetOperationProgress(context.Background(), id)
		if err != nil {
			t.Fatalf("GetOperationProgress 실패: %v", err)
		}
		if op.IsCompleted() { // completed, failed or cancelled
			return op
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("작업 %d이 제한 시간 내에 끝나지 않음", id)
	return nil
}

func copyRequest(fileIDs ...string) *entities.FileOperationRequest {
	return &entities.FileOperationRequest{
		OperationType:  entities.OperationTypeCopy,
		SourceFolderID: "src",
		TargetFolderID: "dst",
		FileIDs:        fileIDs,
	}
}

func TestCopyFiles_RunsInBackground(t *testing.T) {
	mover := &fakeFileMover{}
	uc := NewFileExplorerUseCase(mover, newMemoryOperationRepo())

	resp, err := uc.CopyFiles(context.Background(), copyRequest("a", "b", "c"))
	if err != nil {
		t.Fatalf("CopyFiles 실패: %v", err)
	}
	if resp.OperationID == 0 || resp.TotalFiles != 3 {
		t.Errorf("응답 = %+v", resp)
	}

	op := waitForFinish(t, uc, resp.OperationID)
	if op.Status != entities.OperationStatusCompleted || op.ProcessedFiles != 3 || op.ProcessedBytes != 30 {
		t.Errorf("작업 = status %s, files %d, bytes %d", op.Status, op.ProcessedFiles, op.ProcessedBytes)
	}
}

func TestCopyFiles_PartialFailure(t *testing.T) {
	mover := &fakeFileMover{fail: map[string]bool{"b": true}}
	uc := NewFileExplorerUseCase(mover, newMemoryOperationRepo())

	resp, err := uc.CopyFiles(context.Background(), copyRequest("a", "b", "c"))
	if err != nil {
		t.Fatalf("CopyFiles 실패: %v", err)
	}

	op := waitForFinish(t, uc, resp.OperationID)
	if op.Status != entities.OperationStatusFailed || op.ProcessedFiles != 3 {
		t.Errorf("작업 = status %s, files %d", op.Status, op.ProcessedFiles)
	}
	if len(op.ErrorDetails) != 1 || op.ErrorDetails[0].FileID != "b" {
		t.Errorf("오류 상세 = %+v", op.ErrorDetails)
	}
}

func TestCancelOperation_StopsRunningOperation(t *testing.T) {
	mover := &fakeFileMover{block: map[string]bool{"b": true}, started: make(chan string, 3)}
	uc := NewFileExplorerUseCase(mover, newMemoryOperationRepo())

	resp, err := uc.MoveFiles(context.Background(), &entities.FileOperationRequest{
		OperationType: entities.OperationTypeMove, TargetFolderID: "dst", FileIDs: []string{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("MoveFiles 실패: %v", err)
	}

	// b 처리 중(블록)에 취소
	<-mover.started
	if got := <-mover.started; got != "b" {
		t.Fatalf("두 번째 처리 파일 = %q", got)
	}
	if err := uc.CancelOperation(context.Background(), resp.OperationID); err != nil {
		t.Fatalf("CancelOperation 실패: %v", err)
	}

	op := waitForFinish(t, uc, resp.OperationID)
	if op.Status != entities.OperationStatusCancelled {
		t.Errorf("상태 = %s, 기대 = cancelled", op.Status)
	}
	if op.ProcessedFiles != 1 {
		t.Errorf("처리된 파일 = %d, 기대 = 1", op.ProcessedFiles)
	}
	if len(op.ErrorDetails) != 0 {
		t.Errorf("취소로 중단된 파일이 오류로 기록됨: %+v", op.ErrorDetails)
	}
	if n := mover.callCount(); n != 2 {
		t.Errorf("처리 시도 파일 수 = %d, 기대 = 2 (c는 처리되지 않아야 함)", n)
	}

	if err := uc.CancelOperation(context.Background(), resp.OperationID); !errors.Is(err, entities.ErrOperationNotRunning) {
		t.Errorf("끝난 작업 재취소 오류 = %v, 기대 = ErrOperationNotRunning", err)
	}
}

func TestCancelOperation_OrphanedOperation(t *testing.T) {
	repo := newMemoryOperationRepo()
	uc := NewFileExplorerUseCase(&fakeFileMover{}, repo)

	// 이전 서버 프로세스에서 시작되어 DB에만 진행 중으로 남은 작업
	op := entities.NewFileOperation(entities.OperationTypeCopy, "src", "dst", []string{"a"})
	op.Start()
	repo.Create(context.Background(), op)

	if err := uc.CancelOperation(context.Background(), op.ID); err != nil {
		t.Fatalf("CancelOperation 실패: %v", err)
	}
	got, _ := repo.GetByID(context.Background(), op.ID)
	if got.Status != entities.OperationStatusCancelled {
		t.Errorf("상태 = %s, 기대 = cancelled", got.Status)
	}
}

func TestDeleteFiles_TrashByDefault(t *testing.T) {
	mover := &fakeFileMover{}
	uc := NewFileExplorerUseCase(mover, newMemoryOperationRepo())

	resp, err := uc.DeleteFiles(context.Background(), []string{"a", "b"}, false)
	if err != nil {
		t.Fatalf("DeleteFiles 실패: %v", err)
	}
	op := waitForFinish(t, uc, resp.OperationID)

	if op.Status != entities.OperationStatusCompleted {
		t.Errorf("상태 = %s", op.Status)
	}
	if len(mover.trashed) != 2 || len(mover.deleted) != 0 {
		t.Errorf("휴지통 이동 %v, 완전 삭제 %v — 기본은 휴지통 이동이어야 함", mover.trashed, mover.deleted)
	}
}
