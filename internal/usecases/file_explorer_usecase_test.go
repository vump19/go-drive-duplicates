package usecases

import (
	"context"
	"errors"
	"testing"

	"go-drive-duplicates/internal/domain/entities"
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
