package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// DuplicateRepository defines the interface for duplicate group data access operations
type DuplicateRepository interface {
	// Basic CRUD operations
	Save(ctx context.Context, group *entities.DuplicateGroup) error
	GetByID(ctx context.Context, id int) (*entities.DuplicateGroup, error)
	GetByHash(ctx context.Context, hash string) (*entities.DuplicateGroup, error)
	GetAll(ctx context.Context) ([]*entities.DuplicateGroup, error)
	Update(ctx context.Context, group *entities.DuplicateGroup) error
	Delete(ctx context.Context, id int) error
	DeleteByHash(ctx context.Context, hash string) error

	// Group operations
	AddFileToGroup(ctx context.Context, groupID int, file *entities.File) error
	RemoveFileFromGroup(ctx context.Context, groupID int, fileID string) error
	GetGroupFiles(ctx context.Context, groupID int) ([]*entities.File, error)

	// Query operations
	GetValidGroups(ctx context.Context) ([]*entities.DuplicateGroup, error) // Groups with more than one file
	GetGroupsWithMinFiles(ctx context.Context, minFiles int) ([]*entities.DuplicateGroup, error)
	GetGroupsWithMinSize(ctx context.Context, minSize int64) ([]*entities.DuplicateGroup, error)
	GetGroupsByFileCount(ctx context.Context, ascending bool) ([]*entities.DuplicateGroup, error)
	GetGroupsByTotalSize(ctx context.Context, ascending bool) ([]*entities.DuplicateGroup, error)

	// Statistics operations
	Count(ctx context.Context) (int, error)
	CountValid(ctx context.Context) (int, error)
	GetTotalWastedSpace(ctx context.Context) (int64, error)
	GetTotalDuplicateFiles(ctx context.Context) (int, error)

	// Folder-specific operations
	GetGroupsInFolder(ctx context.Context, folderID string) ([]*entities.DuplicateGroup, error)
	GetGroupsWithFilesInBothFolders(ctx context.Context, folderID1, folderID2 string) ([]*entities.DuplicateGroup, error)

	// Cleanup operations
	CleanupEmptyGroups(ctx context.Context) (int, error)
	DeleteGroupsOlderThan(ctx context.Context, days int) (int, error)

	// Pagination
	GetPaginated(ctx context.Context, offset, limit int) ([]*entities.DuplicateGroup, error)
	GetValidGroupsPaginated(ctx context.Context, offset, limit int) ([]*entities.DuplicateGroup, error)

	// Search operations
	SearchByFileName(ctx context.Context, filename string) ([]*entities.DuplicateGroup, error)
	SearchByMimeType(ctx context.Context, mimeType string) ([]*entities.DuplicateGroup, error)

	// Validation
	Exists(ctx context.Context, id int) (bool, error)
	ExistsByHash(ctx context.Context, hash string) (bool, error)

	// Batch operations
	SaveBatch(ctx context.Context, groups []*entities.DuplicateGroup) error
	DeleteBatch(ctx context.Context, ids []int) error
}
