package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
	"io"
)

// HashCalculator defines the interface for different hash algorithms
type HashCalculator interface {
	// Basic hash calculation
	Calculate(data io.Reader) (string, error)
	CalculateFromBytes(data []byte) (string, error)
	CalculateFromString(data string) (string, error)

	// Algorithm information
	GetAlgorithmName() string
	GetHashLength() int

	// Stream processing for large files
	NewHasher() HashWriter
}

// HashWriter defines an interface for streaming hash calculation
type HashWriter interface {
	io.Writer
	Sum() (string, error)
	Reset()
}

// HashService defines the domain service for hash operations
type HashService interface {
	// Hash calculation
	CalculateFileHash(ctx context.Context, file *entities.File) (string, error)
	CalculateFileHashes(ctx context.Context, files []*entities.File) error
	CalculateHashFromReader(ctx context.Context, reader io.Reader) (string, error)

	// Batch operations
	CalculateHashesBatch(ctx context.Context, files []*entities.File, workerCount int) error
	CalculateHashesParallel(ctx context.Context, files []*entities.File, workerCount int) error

	// Progress tracking
	CalculateHashesWithProgress(ctx context.Context, files []*entities.File, workerCount int, progressCallback func(processed, total int)) error

	// Algorithm management
	SetHashAlgorithm(algorithm string) error
	GetSupportedAlgorithms() []string
	GetCurrentAlgorithm() string

	// Validation
	ValidateHash(hash string) error
	CompareHashes(hash1, hash2 string) bool

	// Performance
	GetOptimalWorkerCount() int
	EstimateCalculationTime(files []*entities.File) (estimatedSeconds int)

	// Retry logic
	CalculateWithRetry(ctx context.Context, file *entities.File, maxRetries int) (string, error)
}
