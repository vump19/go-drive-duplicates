package entities

import (
	"time"
)

// FileStatistics represents comprehensive statistics about files in the system
type FileStatistics struct {
	// Basic counts
	TotalFiles int   `json:"totalFiles"`
	TotalSize  int64 `json:"totalSize"`

	// File type statistics
	FilesByType map[string]int   `json:"filesByType"` // image, video, document, etc.
	SizesByType map[string]int64 `json:"sizesByType"`

	// Size distribution
	FilesBySize map[string]int   `json:"filesBySize"` // small, medium, large, very_large
	SizesBySize map[string]int64 `json:"sizesBySize"`

	// Time-based statistics
	FilesByMonth map[string]int   `json:"filesByMonth"` // YYYY-MM format
	SizesByMonth map[string]int64 `json:"sizesByMonth"`

	// Top folders by file count and size
	TopFolders []*FolderStats `json:"topFolders"`

	// Extension statistics
	TopExtensions []*ExtensionStats `json:"topExtensions"`

	// Timestamps
	GeneratedAt time.Time `json:"generatedAt"`
}

// FolderStats represents statistics for a specific folder
type FolderStats struct {
	FolderID   string `json:"folderId"`
	FolderName string `json:"folderName"`
	FileCount  int    `json:"fileCount"`
	TotalSize  int64  `json:"totalSize"`
	Path       string `json:"path,omitempty"`
}

// ExtensionStats represents statistics for a file extension
type ExtensionStats struct {
	Extension string `json:"extension"`
	Count     int    `json:"count"`
	TotalSize int64  `json:"totalSize"`
	AvgSize   int64  `json:"avgSize"`
}

// NewFileStatistics creates a new file statistics object
func NewFileStatistics() *FileStatistics {
	return &FileStatistics{
		FilesByType:   make(map[string]int),
		SizesByType:   make(map[string]int64),
		FilesBySize:   make(map[string]int),
		SizesBySize:   make(map[string]int64),
		FilesByMonth:  make(map[string]int),
		SizesByMonth:  make(map[string]int64),
		TopFolders:    make([]*FolderStats, 0),
		TopExtensions: make([]*ExtensionStats, 0),
		GeneratedAt:   time.Now(),
	}
}

// AddFile processes a file and updates statistics
func (fs *FileStatistics) AddFile(file *File) {
	fs.TotalFiles++
	fs.TotalSize += file.Size

	// Update file type statistics
	category := file.GetFileCategory()
	fs.FilesByType[category]++
	fs.SizesByType[category] += file.Size

	// Update size category statistics
	sizeCategory := file.GetSizeCategory()
	fs.FilesBySize[sizeCategory]++
	fs.SizesBySize[sizeCategory] += file.Size

	// Update time-based statistics
	month := file.ModifiedTime.Format("2006-01")
	fs.FilesByMonth[month]++
	fs.SizesByMonth[month] += file.Size
}

// AddFolderStats adds folder statistics
func (fs *FileStatistics) AddFolderStats(folderID, folderName string, fileCount int, totalSize int64, path string) {
	folderStat := &FolderStats{
		FolderID:   folderID,
		FolderName: folderName,
		FileCount:  fileCount,
		TotalSize:  totalSize,
		Path:       path,
	}
	fs.TopFolders = append(fs.TopFolders, folderStat)
}

// AddExtensionStats adds extension statistics
func (fs *FileStatistics) AddExtensionStats(extension string, count int, totalSize int64) {
	avgSize := int64(0)
	if count > 0 {
		avgSize = totalSize / int64(count)
	}

	extStat := &ExtensionStats{
		Extension: extension,
		Count:     count,
		TotalSize: totalSize,
		AvgSize:   avgSize,
	}
	fs.TopExtensions = append(fs.TopExtensions, extStat)
}

// GetAverageFileSize returns the average file size
func (fs *FileStatistics) GetAverageFileSize() int64 {
	if fs.TotalFiles == 0 {
		return 0
	}
	return fs.TotalSize / int64(fs.TotalFiles)
}

// GetLargestCategory returns the file category with the most files
func (fs *FileStatistics) GetLargestCategory() string {
	maxCount := 0
	largestCategory := ""

	for category, count := range fs.FilesByType {
		if count > maxCount {
			maxCount = count
			largestCategory = category
		}
	}

	return largestCategory
}

// GetTopFoldersByCount returns top folders sorted by file count
func (fs *FileStatistics) GetTopFoldersByCount(limit int) []*FolderStats {
	// Sort folders by file count (descending)
	folders := make([]*FolderStats, len(fs.TopFolders))
	copy(folders, fs.TopFolders)

	// Simple bubble sort for small datasets
	for i := 0; i < len(folders)-1; i++ {
		for j := 0; j < len(folders)-i-1; j++ {
			if folders[j].FileCount < folders[j+1].FileCount {
				folders[j], folders[j+1] = folders[j+1], folders[j]
			}
		}
	}

	if limit > 0 && limit < len(folders) {
		return folders[:limit]
	}
	return folders
}

// GetTopFoldersBySize returns top folders sorted by total size
func (fs *FileStatistics) GetTopFoldersBySize(limit int) []*FolderStats {
	// Sort folders by total size (descending)
	folders := make([]*FolderStats, len(fs.TopFolders))
	copy(folders, fs.TopFolders)

	// Simple bubble sort for small datasets
	for i := 0; i < len(folders)-1; i++ {
		for j := 0; j < len(folders)-i-1; j++ {
			if folders[j].TotalSize < folders[j+1].TotalSize {
				folders[j], folders[j+1] = folders[j+1], folders[j]
			}
		}
	}

	if limit > 0 && limit < len(folders) {
		return folders[:limit]
	}
	return folders
}

// GetTopExtensionsByCount returns top extensions sorted by file count
func (fs *FileStatistics) GetTopExtensionsByCount(limit int) []*ExtensionStats {
	// Sort extensions by count (descending)
	extensions := make([]*ExtensionStats, len(fs.TopExtensions))
	copy(extensions, fs.TopExtensions)

	// Simple bubble sort for small datasets
	for i := 0; i < len(extensions)-1; i++ {
		for j := 0; j < len(extensions)-i-1; j++ {
			if extensions[j].Count < extensions[j+1].Count {
				extensions[j], extensions[j+1] = extensions[j+1], extensions[j]
			}
		}
	}

	if limit > 0 && limit < len(extensions) {
		return extensions[:limit]
	}
	return extensions
}

// GetSpaceDistribution returns space distribution by category as percentages
func (fs *FileStatistics) GetSpaceDistribution() map[string]float64 {
	distribution := make(map[string]float64)

	if fs.TotalSize == 0 {
		return distribution
	}

	for category, size := range fs.SizesByType {
		percentage := float64(size) / float64(fs.TotalSize) * 100
		distribution[category] = percentage
	}

	return distribution
}
