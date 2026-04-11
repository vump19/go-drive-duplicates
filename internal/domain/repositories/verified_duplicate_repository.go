package repositories

import (
	"context"

	"go-drive-duplicates/internal/domain/entities"
)

// VerifiedDuplicateRepository defines the interface for verified duplicate persistence
type VerifiedDuplicateRepository interface {
	// Create saves a new verified duplicate record
	Create(ctx context.Context, verified *entities.VerifiedDuplicate) error

	// GetByHash retrieves a verified duplicate by its hash
	GetByHash(ctx context.Context, hash string) (*entities.VerifiedDuplicate, error)

	// GetByStatus retrieves all verified duplicates with a specific status
	GetByStatus(ctx context.Context, status entities.VerifiedDuplicateStatus) ([]*entities.VerifiedDuplicate, error)

	// Update modifies an existing verified duplicate record
	Update(ctx context.Context, verified *entities.VerifiedDuplicate) error

	// Delete removes a verified duplicate record
	Delete(ctx context.Context, id int) error

	// List retrieves verified duplicates with optional filtering
	List(ctx context.Context, filter *entities.VerifiedDuplicateFilter) ([]*entities.VerifiedDuplicate, error)

	// ExistsByHash checks if a hash is already verified
	ExistsByHash(ctx context.Context, hash string) (bool, error)

	// GetVerifiedHashes returns all verified hashes for exclusion from duplicate search
	GetVerifiedHashes(ctx context.Context) ([]string, error)

	// BatchCheckVerified checks multiple hashes at once for verification status
	BatchCheckVerified(ctx context.Context, hashes []string) (map[string]*entities.VerifiedDuplicate, error)
}
