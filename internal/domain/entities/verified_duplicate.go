package entities

import (
	"time"
)

// VerifiedDuplicate represents a duplicate group that has been verified by the user
// This prevents the same duplicate group from appearing in future searches
type VerifiedDuplicate struct {
	ID          int       `json:"id" db:"id"`
	Hash        string    `json:"hash" db:"hash"`               // File hash that identifies the duplicate group
	FileCount   int       `json:"file_count" db:"file_count"`   // Number of files in the group
	TotalSize   int64     `json:"total_size" db:"total_size"`   // Total size of all files
	Status      string    `json:"status" db:"status"`           // verified, ignored, resolved
	Description string    `json:"description" db:"description"` // User notes about this duplicate group
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// VerifiedDuplicateStatus constants
const (
	StatusVerified VerifiedDuplicateStatus = "verified" // User confirmed these are duplicates
	StatusIgnored  VerifiedDuplicateStatus = "ignored"  // User wants to ignore this group
	StatusResolved VerifiedDuplicateStatus = "resolved" // User has handled this group
)

type VerifiedDuplicateStatus string

// VerifiedDuplicateRequest represents a request to mark a duplicate group as verified
type VerifiedDuplicateRequest struct {
	Hash        string                  `json:"hash" validate:"required"`
	FileCount   int                     `json:"file_count" validate:"min=2"`
	TotalSize   int64                   `json:"total_size" validate:"min=1"`
	Status      VerifiedDuplicateStatus `json:"status" validate:"required"`
	Description string                  `json:"description"`
}

// VerifiedDuplicateFilter represents filters for querying verified duplicates
type VerifiedDuplicateFilter struct {
	Status    VerifiedDuplicateStatus `json:"status"`
	HashList  []string                `json:"hash_list"`  // Filter by specific hashes
	CreatedAt *time.Time              `json:"created_at"` // Filter by creation date
}
