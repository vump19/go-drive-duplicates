package services

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	"io"
	"time"
)

// MockStorageProvider provides a mock implementation for testing when no Google Drive credentials are available
type MockStorageProvider struct{}

// NewMockStorageProvider creates a new mock storage provider
func NewMockStorageProvider() services.StorageProvider {
	return &MockStorageProvider{}
}

// Authentication methods
func (m *MockStorageProvider) Authenticate(ctx context.Context) error {
	return nil
}

func (m *MockStorageProvider) IsAuthenticated() bool {
	return true
}

// File operations
func (m *MockStorageProvider) ListFiles(ctx context.Context, folderID string) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

func (m *MockStorageProvider) ListAllFiles(ctx context.Context) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

func (m *MockStorageProvider) GetFile(ctx context.Context, fileID string) (*entities.File, error) {
	if fileID == "" {
		return nil, fmt.Errorf("file ID cannot be empty")
	}

	return &entities.File{
		ID:           fileID,
		Name:         fmt.Sprintf("mock-file-%s.txt", fileID),
		Size:         1024,
		MimeType:     "text/plain",
		ModifiedTime: time.Now(),
		Parents:      []string{"mock-parent"},
		Path:         fmt.Sprintf("/mock-path/mock-file-%s.txt", fileID),
		WebViewLink:  fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID),
		LastUpdated:  time.Now(),
	}, nil
}

func (m *MockStorageProvider) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("mock storage provider: file download not available")
}

func (m *MockStorageProvider) DeleteFile(ctx context.Context, fileID string) error {
	return fmt.Errorf("mock storage provider: file deletion not available")
}

func (m *MockStorageProvider) DeleteFiles(ctx context.Context, fileIDs []string) error {
	return fmt.Errorf("mock storage provider: bulk file deletion not available")
}

// Folder operations
func (m *MockStorageProvider) GetFolder(ctx context.Context, folderID string) (*entities.File, error) {
	if folderID == "" {
		return nil, fmt.Errorf("folder ID cannot be empty")
	}

	return &entities.File{
		ID:           folderID,
		Name:         fmt.Sprintf("mock-folder-%s", folderID),
		Size:         0,
		MimeType:     "application/vnd.google-apps.folder",
		ModifiedTime: time.Now(),
		Parents:      []string{"root"},
		Path:         fmt.Sprintf("/mock-path/mock-folder-%s", folderID),
		LastUpdated:  time.Now(),
	}, nil
}

func (m *MockStorageProvider) ListFolders(ctx context.Context, parentID string) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

func (m *MockStorageProvider) DeleteFolder(ctx context.Context, folderID string) error {
	return fmt.Errorf("mock storage provider: folder deletion not available")
}

func (m *MockStorageProvider) GetFolderPath(ctx context.Context, folderID string) (string, error) {
	if folderID == "" {
		return "/", nil
	}
	return fmt.Sprintf("/mock-folder-%s", folderID), nil
}

// Search operations
func (m *MockStorageProvider) SearchFiles(ctx context.Context, query string) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

func (m *MockStorageProvider) SearchByMimeType(ctx context.Context, mimeType string) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

func (m *MockStorageProvider) SearchByName(ctx context.Context, name string) ([]*entities.File, error) {
	return []*entities.File{}, nil
}

// Metadata operations
func (m *MockStorageProvider) GetFileParents(ctx context.Context, fileID string) ([]string, error) {
	return []string{"mock-parent"}, nil
}

func (m *MockStorageProvider) UpdateFileMetadata(ctx context.Context, fileID string, metadata map[string]interface{}) error {
	return fmt.Errorf("mock storage provider: metadata update not available")
}

// Quota and limits
func (m *MockStorageProvider) GetQuota(ctx context.Context) (used, total int64, err error) {
	return 1024 * 1024 * 1024, 15 * 1024 * 1024 * 1024, nil // 1GB used, 15GB total
}

func (m *MockStorageProvider) GetRateLimit() (requestsPerSecond int) {
	return 100 // Mock rate limit
}

// Batch operations
func (m *MockStorageProvider) BatchDelete(ctx context.Context, fileIDs []string, batchSize int) error {
	return fmt.Errorf("mock storage provider: batch deletion not available")
}

func (m *MockStorageProvider) BatchGetFiles(ctx context.Context, fileIDs []string) ([]*entities.File, error) {
	files := make([]*entities.File, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if fileID != "" {
			file, _ := m.GetFile(ctx, fileID)
			if file != nil {
				files = append(files, file)
			}
		}
	}
	return files, nil
}

// Provider-specific operations
func (m *MockStorageProvider) GetProviderName() string {
	return "MockProvider"
}

func (m *MockStorageProvider) GetMaxBatchSize() int {
	return 100
}

func (m *MockStorageProvider) SupportsResumableDownload() bool {
	return false
}
