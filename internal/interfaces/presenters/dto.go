package presenters

import (
	"time"
)

// Common DTOs for API requests and responses

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string            `json:"error"`
	Code    string            `json:"code,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// SuccessResponse represents a standard success response
type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ProgressDTO represents progress information
type ProgressDTO struct {
	ID             int                    `json:"id"`
	OperationType  string                 `json:"operationType"`
	ProcessedItems int                    `json:"processedItems"`
	TotalItems     int                    `json:"totalItems"`
	Status         string                 `json:"status"`
	CurrentStep    string                 `json:"currentStep"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
	Percentage     float64                `json:"percentage"`
	StartTime      time.Time              `json:"startTime"`
	EndTime        *time.Time             `json:"endTime,omitempty"`
	LastUpdated    time.Time              `json:"lastUpdated"`
	ETA            *time.Time             `json:"eta,omitempty"`
	Duration       string                 `json:"duration"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// FileDTO represents file information
type FileDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Size           int64     `json:"size"`
	SizeFormatted  string    `json:"sizeFormatted"`
	MimeType       string    `json:"mimeType"`
	ModifiedTime   time.Time `json:"modifiedTime"`
	Hash           string    `json:"hash,omitempty"`
	HashCalculated bool      `json:"hashCalculated"`
	Parents        []string  `json:"parents,omitempty"`
	Path           string    `json:"path,omitempty"`
	WebViewLink    string    `json:"webViewLink"`
	Category       string    `json:"category"`
	SizeCategory   string    `json:"sizeCategory"`
	IsLargeFile    bool      `json:"isLargeFile"`
}

// DuplicateGroupDTO represents a group of duplicate files
type DuplicateGroupDTO struct {
	ID                   int        `json:"id"`
	Hash                 string     `json:"hash"`
	Files                []*FileDTO `json:"files"`
	Count                int        `json:"count"`
	TotalSize            int64      `json:"totalSize"`
	TotalSizeFormatted   string     `json:"totalSizeFormatted"`
	WastedSpace          int64      `json:"wastedSpace"`
	WastedSpaceFormatted string     `json:"wastedSpaceFormatted"`
	OldestFile           *FileDTO   `json:"oldestFile,omitempty"`
	NewestFile           *FileDTO   `json:"newestFile,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// ComparisonResultDTO represents folder comparison result
type ComparisonResultDTO struct {
	ID                       int        `json:"id"`
	SourceFolderID           string     `json:"sourceFolderId"`
	TargetFolderID           string     `json:"targetFolderId"`
	SourceFolderName         string     `json:"sourceFolderName"`
	TargetFolderName         string     `json:"targetFolderName"`
	SourceFileCount          int        `json:"sourceFileCount"`
	TargetFileCount          int        `json:"targetFileCount"`
	DuplicateCount           int        `json:"duplicateCount"`
	SourceTotalSize          int64      `json:"sourceTotalSize"`
	SourceTotalSizeFormatted string     `json:"sourceTotalSizeFormatted"`
	TargetTotalSize          int64      `json:"targetTotalSize"`
	TargetTotalSizeFormatted string     `json:"targetTotalSizeFormatted"`
	DuplicateSize            int64      `json:"duplicateSize"`
	DuplicateSizeFormatted   string     `json:"duplicateSizeFormatted"`
	DuplicateFiles           []*FileDTO `json:"duplicateFiles"`
	CanDeleteTargetFolder    bool       `json:"canDeleteTargetFolder"`
	DuplicationPercentage    float64    `json:"duplicationPercentage"`
	UniqueFilesInTarget      int        `json:"uniqueFilesInTarget"`
	UniqueFilesSize          int64      `json:"uniqueFilesSize"`
	UniqueFilesSizeFormatted string     `json:"uniqueFilesSizeFormatted"`
	IsSignificantSavings     bool       `json:"isSignificantSavings"`
	Summary                  string     `json:"summary"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

// FileStatisticsDTO represents file statistics
type FileStatisticsDTO struct {
	TotalFiles               int                  `json:"totalFiles"`
	TotalSize                int64                `json:"totalSize"`
	TotalSizeFormatted       string               `json:"totalSizeFormatted"`
	AverageFileSize          int64                `json:"averageFileSize"`
	AverageFileSizeFormatted string               `json:"averageFileSizeFormatted"`
	FilesByType              map[string]int       `json:"filesByType"`
	SizesByType              map[string]int64     `json:"sizesByType"`
	FilesBySize              map[string]int       `json:"filesBySize"`
	SizesBySize              map[string]int64     `json:"sizesBySize"`
	FilesByMonth             map[string]int       `json:"filesByMonth"`
	SizesByMonth             map[string]int64     `json:"sizesByMonth"`
	TopFolders               []*FolderStatsDTO    `json:"topFolders"`
	TopExtensions            []*ExtensionStatsDTO `json:"topExtensions"`
	SpaceDistribution        map[string]float64   `json:"spaceDistribution"`
	LargestCategory          string               `json:"largestCategory"`
	GeneratedAt              time.Time            `json:"generatedAt"`
}

// FolderStatsDTO represents statistics for a folder
type FolderStatsDTO struct {
	FolderID           string `json:"folderId"`
	FolderName         string `json:"folderName"`
	FileCount          int    `json:"fileCount"`
	TotalSize          int64  `json:"totalSize"`
	TotalSizeFormatted string `json:"totalSizeFormatted"`
	Path               string `json:"path,omitempty"`
}

// ExtensionStatsDTO represents statistics for a file extension
type ExtensionStatsDTO struct {
	Extension          string `json:"extension"`
	Count              int    `json:"count"`
	TotalSize          int64  `json:"totalSize"`
	TotalSizeFormatted string `json:"totalSizeFormatted"`
	AvgSize            int64  `json:"avgSize"`
	AvgSizeFormatted   string `json:"avgSizeFormatted"`
}

// Request DTOs

// ScanFilesRequestDTO represents a file scanning request
type ScanFilesRequestDTO struct {
	FolderID           string `json:"folderId,omitempty"`
	Recursive          bool   `json:"recursive,omitempty"`
	UpdatePaths        bool   `json:"updatePaths,omitempty"`
	ResumeFromProgress bool   `json:"resumeFromProgress,omitempty"`
	BatchSize          int    `json:"batchSize,omitempty"`
	WorkerCount        int    `json:"workerCount,omitempty"`
}

// FindDuplicatesRequestDTO represents a duplicate finding request
type FindDuplicatesRequestDTO struct {
	FolderID         string `json:"folderId,omitempty"`
	Recursive        bool   `json:"recursive,omitempty"`
	CalculateHashes  bool   `json:"calculateHashes,omitempty"`
	ForceRecalculate bool   `json:"forceRecalculate,omitempty"`
	MinFileSize      int64  `json:"minFileSize,omitempty"`
	MaxResults       int    `json:"maxResults,omitempty"`
}

// CompareFoldersRequestDTO represents a folder comparison request
type CompareFoldersRequestDTO struct {
	SourceFolderID    string `json:"sourceFolderId"`
	TargetFolderID    string `json:"targetFolderId"`
	IncludeSubfolders bool   `json:"includeSubfolders,omitempty"`
	DeepComparison    bool   `json:"deepComparison,omitempty"`
	MinFileSize       int64  `json:"minFileSize,omitempty"`
	WorkerCount       int    `json:"workerCount,omitempty"`
}

// DeleteFilesRequestDTO represents a file deletion request
type DeleteFilesRequestDTO struct {
	FileIDs        []string `json:"fileIds"`
	CleanupFolders bool     `json:"cleanupFolders,omitempty"`
	SafetyChecks   bool     `json:"safetyChecks,omitempty"`
	BatchSize      int      `json:"batchSize,omitempty"`
}

// DeleteDuplicatesRequestDTO represents a duplicate deletion request
type DeleteDuplicatesRequestDTO struct {
	GroupID        int    `json:"groupId"`
	KeepFileID     string `json:"keepFileId"`
	CleanupFolders bool   `json:"cleanupFolders,omitempty"`
}

// BulkDeleteRequestDTO represents a bulk deletion request
type BulkDeleteRequestDTO struct {
	FolderID       string `json:"folderId"`
	Pattern        string `json:"pattern"`
	Recursive      bool   `json:"recursive,omitempty"`
	DryRun         bool   `json:"dryRun,omitempty"`
	CleanupFolders bool   `json:"cleanupFolders,omitempty"`
}

// CleanupFoldersRequestDTO represents a folder cleanup request
type CleanupFoldersRequestDTO struct {
	RootFolderID string `json:"rootFolderId,omitempty"`
	Recursive    bool   `json:"recursive,omitempty"`
}

// Response DTOs

// ScanResponseDTO represents a file scanning response
type ScanResponseDTO struct {
	Progress       *ProgressDTO `json:"progress"`
	TotalFiles     int          `json:"totalFiles"`
	ProcessedFiles int          `json:"processedFiles"`
	NewFiles       int          `json:"newFiles"`
	UpdatedFiles   int          `json:"updatedFiles"`
	FolderPath     string       `json:"folderPath,omitempty"`
	Errors         []string     `json:"errors,omitempty"`
}

// DuplicatesResponseDTO represents a duplicate finding response
type DuplicatesResponseDTO struct {
	Progress                  *ProgressDTO         `json:"progress"`
	DuplicateGroups           []*DuplicateGroupDTO `json:"duplicateGroups"`
	TotalGroups               int                  `json:"totalGroups"`
	TotalFiles                int                  `json:"totalFiles"`
	TotalWastedSpace          int64                `json:"totalWastedSpace"`
	TotalWastedSpaceFormatted string               `json:"totalWastedSpaceFormatted"`
	HashesCalculated          int                  `json:"hashesCalculated"`
	Errors                    []string             `json:"errors,omitempty"`
}

// ComparisonResponseDTO represents a folder comparison response
type ComparisonResponseDTO struct {
	Progress         *ProgressDTO         `json:"progress"`
	ComparisonResult *ComparisonResultDTO `json:"comparisonResult"`
	Errors           []string             `json:"errors,omitempty"`
}

// DeleteResponseDTO represents a file deletion response
type DeleteResponseDTO struct {
	Progress            *ProgressDTO `json:"progress"`
	TotalFiles          int          `json:"totalFiles"`
	DeletedFiles        int          `json:"deletedFiles"`
	FailedFiles         int          `json:"failedFiles"`
	DeletedFolders      int          `json:"deletedFolders,omitempty"`
	SpaceSaved          int64        `json:"spaceSaved"`
	SpaceSavedFormatted string       `json:"spaceSavedFormatted"`
	Errors              []string     `json:"errors,omitempty"`
}

// CleanupResponseDTO represents a cleanup response
type CleanupResponseDTO struct {
	Progress       *ProgressDTO `json:"progress"`
	DeletedFolders int          `json:"deletedFolders"`
	Errors         []string     `json:"errors,omitempty"`
}

// HashCalculationResponseDTO represents a hash calculation response
type HashCalculationResponseDTO struct {
	Progress         *ProgressDTO `json:"progress"`
	TotalFiles       int          `json:"totalFiles"`
	ProcessedFiles   int          `json:"processedFiles"`
	SuccessfulHashes int          `json:"successfulHashes"`
	FailedHashes     int          `json:"failedHashes"`
	Errors           []string     `json:"errors,omitempty"`
}
