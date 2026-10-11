package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/infrastructure/repositories/sqlite"

	"github.com/jmoiron/sqlx"
)

// fakeStorageProvider implements only the StorageProvider methods used by fileService tests.
type fakeStorageProvider struct {
	services.StorageProvider
	file      *entities.File
	exists    bool
	existsErr error
}

func (f *fakeStorageProvider) GetFile(ctx context.Context, fileID string) (*entities.File, error) {
	copied := *f.file
	return &copied, nil
}

func (f *fakeStorageProvider) FileExists(ctx context.Context, fileID string) (bool, error) {
	return f.exists, f.existsErr
}

var testModifiedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newRemoteFile() *entities.File {
	return &entities.File{
		ID: "file1", Name: "new.txt", Size: 10, MimeType: "text/plain",
		ModifiedTime: testModifiedTime, Parents: []string{"parent2"}, WebViewLink: "link",
	}
}

func TestEnrichFileMetadata_UnchangedContentKeepsHashAndPath(t *testing.T) {
	fs := &fileService{storageProvider: &fakeStorageProvider{file: newRemoteFile()}}
	file := &entities.File{ID: "file1", Name: "old.txt", Size: 10, ModifiedTime: testModifiedTime,
		Hash: "localhash", HashCalculated: true, Path: "/docs"}

	if err := fs.EnrichFileMetadata(context.Background(), file); err != nil {
		t.Fatalf("EnrichFileMetadata 실패: %v", err)
	}
	if file.Name != "new.txt" || file.Parents[0] != "parent2" || file.WebViewLink != "link" {
		t.Errorf("메타데이터가 갱신되지 않음: %+v", file)
	}
	if file.Hash != "localhash" || !file.HashCalculated {
		t.Errorf("내용이 같은데 해시가 바뀜: %q", file.Hash)
	}
	if file.Path != "/docs" {
		t.Errorf("로컬 경로가 바뀜: %q", file.Path)
	}
}

func TestEnrichFileMetadata_ChangedContentClearsHash(t *testing.T) {
	fs := &fileService{storageProvider: &fakeStorageProvider{file: newRemoteFile()}}
	file := &entities.File{ID: "file1", Size: 5, ModifiedTime: testModifiedTime, Hash: "localhash", HashCalculated: true}

	if err := fs.EnrichFileMetadata(context.Background(), file); err != nil {
		t.Fatalf("EnrichFileMetadata 실패: %v", err)
	}
	if file.Hash != "" || file.HashCalculated {
		t.Errorf("내용이 바뀌었는데 해시가 유지됨: %q", file.Hash)
	}
	if file.Size != 10 {
		t.Errorf("크기 = %d, 기대 = 10", file.Size)
	}
}

func TestEnrichFileMetadata_RequiresID(t *testing.T) {
	fs := &fileService{storageProvider: &fakeStorageProvider{file: newRemoteFile()}}
	if err := fs.EnrichFileMetadata(context.Background(), &entities.File{}); err == nil {
		t.Error("빈 ID에 오류가 반환되지 않음")
	}
}

func TestRefreshFileMetadata(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Connect("sqlite3", t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("DB 연결 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := sqlite.NewFileRepository(db)
	if err := repo.(*sqlite.FileRepository).CreateTables(ctx); err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}

	remote := newRemoteFile()
	remote.Hash, remote.HashCalculated = "drivemd5", true
	fs := &fileService{fileRepo: repo, storageProvider: &fakeStorageProvider{file: remote}}

	load := func() *entities.File {
		t.Helper()
		f, err := repo.GetByID(ctx, "file1")
		if err != nil || f == nil {
			t.Fatalf("파일 조회 실패: %v", err)
		}
		return f
	}

	t.Run("DB에 없는 파일은 추가", func(t *testing.T) {
		if _, err := fs.RefreshFileMetadata(ctx, "file1"); err != nil {
			t.Fatalf("RefreshFileMetadata 실패: %v", err)
		}
		if f := load(); f.Name != "new.txt" || f.Hash != "drivemd5" {
			t.Errorf("저장된 파일 = %+v", f)
		}
	})

	t.Run("내용이 같으면 로컬 해시와 경로 유지", func(t *testing.T) {
		f := load()
		f.Hash, f.Path, f.Name = "localsha256", "/docs", "old.txt"
		if err := repo.Save(ctx, f); err != nil {
			t.Fatalf("저장 실패: %v", err)
		}
		if _, err := fs.RefreshFileMetadata(ctx, "file1"); err != nil {
			t.Fatalf("RefreshFileMetadata 실패: %v", err)
		}
		if f := load(); f.Hash != "localsha256" || f.Path != "/docs" || f.Name != "new.txt" {
			t.Errorf("저장된 파일 = %+v", f)
		}
	})

	t.Run("내용이 바뀌면 Drive 해시 적용", func(t *testing.T) {
		remote.Size = 20
		if _, err := fs.RefreshFileMetadata(ctx, "file1"); err != nil {
			t.Fatalf("RefreshFileMetadata 실패: %v", err)
		}
		if f := load(); f.Hash != "drivemd5" || f.Size != 20 {
			t.Errorf("저장된 파일 = %+v", f)
		}
	})

	t.Run("내용이 바뀌고 Drive 해시가 없으면 초기화", func(t *testing.T) {
		remote.Size, remote.Hash, remote.HashCalculated = 30, "", false
		if _, err := fs.RefreshFileMetadata(ctx, "file1"); err != nil {
			t.Fatalf("RefreshFileMetadata 실패: %v", err)
		}
		if f := load(); f.Hash != "" || f.HashCalculated {
			t.Errorf("저장된 파일 = %+v", f)
		}
	})
}

func TestValidateFileAccess(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		fileID  string
		sp      *fakeStorageProvider
		wantErr bool
	}{
		{"DB에 없는 폴더도 Drive에 있으면 통과", "never-scanned", &fakeStorageProvider{exists: true}, false},
		{"Drive에 없거나 접근 불가", "missing", &fakeStorageProvider{exists: false}, true},
		{"Drive API 오류", "folder", &fakeStorageProvider{existsErr: errors.New("quota exceeded")}, true},
		{"빈 ID", "", &fakeStorageProvider{exists: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// fileRepo is nil: validation must not depend on the local database
			fs := &fileService{storageProvider: tt.sp}
			err := fs.ValidateFileAccess(ctx, tt.fileID)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
