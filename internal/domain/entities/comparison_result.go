package entities

import (
	"fmt"
	"time"
)

// ComparisonResult represents the result of comparing two folders for duplicate files
type ComparisonResult struct {
	ID               int    `json:"id"`
	SourceFolderID   string `json:"sourceFolderId"`
	TargetFolderID   string `json:"targetFolderId"`
	SourceFolderName string `json:"sourceFolderName"`
	TargetFolderName string `json:"targetFolderName"`

	// Statistics
	SourceFileCount int `json:"sourceFileCount"`
	TargetFileCount int `json:"targetFileCount"`
	DuplicateCount  int `json:"duplicateCount"`

	// Size information
	SourceTotalSize int64 `json:"sourceTotalSize"`
	TargetTotalSize int64 `json:"targetTotalSize"`
	DuplicateSize   int64 `json:"duplicateSize"`

	// Duplicate files (files in target that also exist in source)
	DuplicateFiles []*File `json:"duplicateFiles"`

	// Deletion recommendation
	CanDeleteTargetFolder bool    `json:"canDeleteTargetFolder"`
	DuplicationPercentage float64 `json:"duplicationPercentage"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewComparisonResult creates a new comparison result
func NewComparisonResult(sourceFolderID, targetFolderID, sourceFolderName, targetFolderName string) *ComparisonResult {
	now := time.Now()
	return &ComparisonResult{
		SourceFolderID:   sourceFolderID,
		TargetFolderID:   targetFolderID,
		SourceFolderName: sourceFolderName,
		TargetFolderName: targetFolderName,
		DuplicateFiles:   make([]*File, 0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// AddDuplicateFile adds a duplicate file to the result
func (cr *ComparisonResult) AddDuplicateFile(file *File) {
	cr.DuplicateFiles = append(cr.DuplicateFiles, file)
	cr.DuplicateCount = len(cr.DuplicateFiles)
	cr.DuplicateSize += file.Size
	cr.UpdatedAt = time.Now()

	// Recalculate duplication percentage and deletion recommendation
	cr.calculateRecommendation()
}

// SetSourceStats sets the source folder statistics
func (cr *ComparisonResult) SetSourceStats(fileCount int, totalSize int64) {
	cr.SourceFileCount = fileCount
	cr.SourceTotalSize = totalSize
	cr.UpdatedAt = time.Now()
	cr.calculateRecommendation()
}

// SetTargetStats sets the target folder statistics
func (cr *ComparisonResult) SetTargetStats(fileCount int, totalSize int64) {
	cr.TargetFileCount = fileCount
	cr.TargetTotalSize = totalSize
	cr.UpdatedAt = time.Now()
	cr.calculateRecommendation()
}

// calculateRecommendation calculates whether the target folder can be deleted
func (cr *ComparisonResult) calculateRecommendation() {
	if cr.TargetFileCount == 0 {
		cr.DuplicationPercentage = 0
		cr.CanDeleteTargetFolder = false
		return
	}

	// Calculate percentage of files in target that are duplicates
	cr.DuplicationPercentage = float64(cr.DuplicateCount) / float64(cr.TargetFileCount) * 100

	// Recommend deletion only if 100% of files are duplicates (conservative approach)
	cr.CanDeleteTargetFolder = cr.DuplicationPercentage >= 100.0
}

// GetWastedSpace returns the amount of space that could be saved by deleting duplicates
func (cr *ComparisonResult) GetWastedSpace() int64 {
	return cr.DuplicateSize
}

// GetUniqueFilesInTarget returns the number of unique files in target folder
func (cr *ComparisonResult) GetUniqueFilesInTarget() int {
	return cr.TargetFileCount - cr.DuplicateCount
}

// GetUniqueFilesSize returns the size of unique files in target folder
func (cr *ComparisonResult) GetUniqueFilesSize() int64 {
	return cr.TargetTotalSize - cr.DuplicateSize
}

// IsSignificantSavings returns true if deleting duplicates would save significant space
func (cr *ComparisonResult) IsSignificantSavings() bool {
	const significantThreshold = 100 * 1024 * 1024 // 100MB
	return cr.GetWastedSpace() > significantThreshold
}

// GetDuplicateFileIDs returns a slice of duplicate file IDs
func (cr *ComparisonResult) GetDuplicateFileIDs() []string {
	ids := make([]string, len(cr.DuplicateFiles))
	for i, file := range cr.DuplicateFiles {
		ids[i] = file.ID
	}
	return ids
}

// GetDuplicatesByHash groups duplicate files by their hash
func (cr *ComparisonResult) GetDuplicatesByHash() map[string][]*File {
	result := make(map[string][]*File)
	for _, file := range cr.DuplicateFiles {
		result[file.Hash] = append(result[file.Hash], file)
	}
	return result
}

// HasDuplicates returns true if there are any duplicate files
func (cr *ComparisonResult) HasDuplicates() bool {
	return cr.DuplicateCount > 0
}

// Summary returns a human-readable summary of the comparison
func (cr *ComparisonResult) Summary() string {
	if !cr.HasDuplicates() {
		return "중복 파일이 발견되지 않았습니다."
	}

	return fmt.Sprintf(
		"총 %d개 파일 중 %d개 중복 파일 발견 (%.1f%%), %s 절약 가능",
		cr.TargetFileCount,
		cr.DuplicateCount,
		cr.DuplicationPercentage,
		formatFileSize(cr.DuplicateSize),
	)
}

// formatFileSize formats file size in human readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
