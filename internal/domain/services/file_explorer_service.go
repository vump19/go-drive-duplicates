package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// FileExplorerService defines interface for file explorer operations
// 파일 탐색기 작업을 위한 서비스 인터페이스
type FileExplorerService interface {
	// 폴더 트리 조회
	// GetFolderTree retrieves folder tree structure starting from root folder
	GetFolderTree(ctx context.Context, rootFolderID string, options *entities.FolderTreeOptions) (*entities.FolderNode, error)

	// 폴더 내 파일 목록 조회
	// ListFilesInFolder lists files and folders within specified folder
	ListFilesInFolder(ctx context.Context, folderID string, options *entities.ListOptions) (*entities.FileListResponse, error)

	// 파일 복사
	// CopyFiles copies files to target folder
	CopyFiles(ctx context.Context, fileIDs []string, targetFolderID string) ([]*CopyResult, error)

	// 파일 이동
	// MoveFiles moves files to target folder
	MoveFiles(ctx context.Context, fileIDs []string, targetFolderID string) ([]*MoveResult, error)

	// 파일 삭제
	// DeleteFiles deletes specified files
	DeleteFiles(ctx context.Context, fileIDs []string) ([]*DeleteResult, error)

	// 파일 이름 변경
	// RenameFile renames a file
	RenameFile(ctx context.Context, fileID string, newName string) error

	// 새 폴더 생성
	// CreateFolder creates a new folder in parent folder
	CreateFolder(ctx context.Context, parentFolderID string, folderName string) (*entities.FileListItem, error)

	// 폴더 정보 조회
	// GetFolderInfo retrieves folder metadata
	GetFolderInfo(ctx context.Context, folderID string) (*entities.FileListItem, error)

	// 파일 정보 조회
	// GetFileInfo retrieves file metadata
	GetFileInfo(ctx context.Context, fileID string) (*entities.FileListItem, error)

	// 파일 검색
	// SearchFiles searches files by query
	SearchFiles(ctx context.Context, query string, options *entities.ListOptions) (*entities.FileListResponse, error)
}

// CopyResult represents the result of a single file copy operation
type CopyResult struct {
	OriginalFileID string `json:"originalFileId"`
	NewFileID      string `json:"newFileId"`
	FileName       string `json:"fileName"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
}

// MoveResult represents the result of a single file move operation
type MoveResult struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// DeleteResult represents the result of a single file delete operation
type DeleteResult struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}
