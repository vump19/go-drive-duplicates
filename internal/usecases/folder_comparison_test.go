package usecases_test

import (
	"context"
	"testing"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/infrastructure/config"
	"go-drive-duplicates/internal/infrastructure/repositories/sqlite"
	"go-drive-duplicates/internal/usecases"

	"github.com/jmoiron/sqlx"
)

type comparisonFixture struct {
	ctx  context.Context
	repo repositories.ProgressRepository
	ps   services.ProgressService
	uc   *usecases.FolderComparisonUseCase
}

func newComparisonFixture(t *testing.T) *comparisonFixture {
	t.Helper()
	ctx := context.Background()

	db, err := sqlx.Connect("sqlite3", t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("DB 연결 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := sqlite.NewProgressRepository(db)
	if err := repo.(*sqlite.ProgressRepository).CreateTables(ctx); err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}

	ps := config.NewProgressService(repo, nil)
	uc := usecases.NewFolderComparisonUseCase(nil, nil, repo, nil, nil, nil, ps)
	return &comparisonFixture{ctx: ctx, repo: repo, ps: ps, uc: uc}
}

func (f *comparisonFixture) createProgress(t *testing.T, opType, status string, start time.Time) *entities.Progress {
	t.Helper()
	p := entities.NewProgress(opType, 0)
	p.Status = status
	p.StartTime = start
	if err := f.repo.Save(f.ctx, p); err != nil {
		t.Fatalf("진행 상황 저장 실패: %v", err)
	}
	return p
}

func (f *comparisonFixture) status(t *testing.T, id int) string {
	t.Helper()
	p, err := f.repo.GetByID(f.ctx, id)
	if err != nil || p == nil {
		t.Fatalf("진행 상황 조회 실패: %v", err)
	}
	return p.Status
}

func TestGetPendingComparisons(t *testing.T) {
	f := newComparisonFixture(t)
	beforeStart := time.Now().Add(-time.Hour)

	orphan := f.createProgress(t, entities.OperationFolderComparison, entities.StatusRunning, beforeStart)
	resumed := f.createProgress(t, entities.OperationFolderComparison, entities.StatusRunning, beforeStart)
	f.createProgress(t, entities.OperationFileScan, entities.StatusRunning, beforeStart)
	f.createProgress(t, entities.OperationFolderComparison, entities.StatusCompleted, beforeStart)
	usecases.SetRunningOperation(f.uc, resumed.ID, func() {})
	live := f.createProgress(t, entities.OperationSingleFolderDuplicates, entities.StatusPending, time.Now())

	got, err := f.uc.GetPendingComparisons(f.ctx)
	if err != nil {
		t.Fatalf("GetPendingComparisons 실패: %v", err)
	}

	statuses := make(map[int]string)
	for _, p := range got {
		statuses[p.ID] = p.Status
	}
	want := map[int]string{
		orphan.ID:  entities.StatusFailed,  // 이전 프로세스에서 중단됨
		resumed.ID: entities.StatusRunning, // 현재 프로세스에서 재개됨
		live.ID:    entities.StatusPending, // 현재 프로세스에서 시작됨
	}
	if len(statuses) != len(want) {
		t.Fatalf("반환된 작업 = %v, 기대 = %v", statuses, want)
	}
	for id, status := range want {
		if statuses[id] != status {
			t.Errorf("작업 %d 상태 = %q, 기대 = %q", id, statuses[id], status)
		}
	}
	if f.status(t, orphan.ID) != entities.StatusFailed {
		t.Errorf("중단된 작업이 DB에서 실패 처리되지 않음")
	}

	// 실패 처리된 작업은 다음 조회부터 제외
	got, err = f.uc.GetPendingComparisons(f.ctx)
	if err != nil {
		t.Fatalf("GetPendingComparisons 재조회 실패: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("재조회 결과 = %d개, 기대 = 2개", len(got))
	}
}

func TestCancelComparison_RunningOperation(t *testing.T) {
	f := newComparisonFixture(t)
	p := f.createProgress(t, entities.OperationFolderComparison, entities.StatusRunning, time.Now())
	cancelled := false
	usecases.SetRunningOperation(f.uc, p.ID, func() { cancelled = true })

	if err := f.uc.CancelComparison(f.ctx, p.ID); err != nil {
		t.Fatalf("CancelComparison 실패: %v", err)
	}
	if !cancelled {
		t.Error("컨텍스트 취소 함수가 호출되지 않음")
	}
	if got := f.status(t, p.ID); got != entities.StatusCancelled {
		t.Errorf("취소 직후 상태 = %q, 기대 = %q", got, entities.StatusCancelled)
	}

	// 취소로 인해 뒤늦게 발생한 오류가 취소 상태를 덮어쓰지 않아야 함
	if err := f.ps.FailOperation(f.ctx, p.ID, "context canceled"); err != nil {
		t.Fatalf("FailOperation 실패: %v", err)
	}
	if got := f.status(t, p.ID); got != entities.StatusCancelled {
		t.Errorf("FailOperation 이후 상태 = %q, 기대 = %q", got, entities.StatusCancelled)
	}

	pending, err := f.uc.GetPendingComparisons(f.ctx)
	if err != nil {
		t.Fatalf("GetPendingComparisons 실패: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("취소된 작업이 진행 중 목록에 포함됨: %d개", len(pending))
	}
}

func TestCancelComparison_PreviousSessionOperation(t *testing.T) {
	f := newComparisonFixture(t)
	p := f.createProgress(t, entities.OperationFolderComparison, entities.StatusRunning, time.Now().Add(-time.Hour))

	if err := f.uc.CancelComparison(f.ctx, p.ID); err != nil {
		t.Fatalf("CancelComparison 실패: %v", err)
	}
	if got := f.status(t, p.ID); got != entities.StatusCancelled {
		t.Errorf("상태 = %q, 기대 = %q", got, entities.StatusCancelled)
	}
	if err := f.uc.CancelComparison(f.ctx, p.ID); err == nil {
		t.Error("이미 취소된 작업 재취소 시 오류가 반환되지 않음")
	}
}

func TestCancelOperation_KeepsCompletedStatus(t *testing.T) {
	f := newComparisonFixture(t)
	p := f.createProgress(t, entities.OperationFolderComparison, entities.StatusCompleted, time.Now())

	if err := f.ps.CancelOperation(f.ctx, p.ID); err != nil {
		t.Fatalf("CancelOperation 실패: %v", err)
	}
	if got := f.status(t, p.ID); got != entities.StatusCompleted {
		t.Errorf("상태 = %q, 기대 = %q", got, entities.StatusCompleted)
	}
}

func TestFailOperation_RunningOperation(t *testing.T) {
	f := newComparisonFixture(t)
	p := f.createProgress(t, entities.OperationFolderComparison, entities.StatusRunning, time.Now())

	if err := f.ps.FailOperation(f.ctx, p.ID, "오류"); err != nil {
		t.Fatalf("FailOperation 실패: %v", err)
	}
	if got := f.status(t, p.ID); got != entities.StatusFailed {
		t.Errorf("상태 = %q, 기대 = %q", got, entities.StatusFailed)
	}
}
