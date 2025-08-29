package entities

import (
	"fmt"
	"time"
)

// DuplicateGroup represents a group of files that have the same hash (are duplicates)
type DuplicateGroup struct {
	ID        int       `json:"id"`
	Hash      string    `json:"hash"`
	Files     []*File   `json:"files"`
	TotalSize int64     `json:"totalSize"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewDuplicateGroup creates a new duplicate group with the given hash
func NewDuplicateGroup(hash string) *DuplicateGroup {
	now := time.Now()
	return &DuplicateGroup{
		Hash:      hash,
		Files:     make([]*File, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddFile adds a file to the duplicate group
func (dg *DuplicateGroup) AddFile(file *File) error {
	if file.Hash != dg.Hash {
		return fmt.Errorf("file hash %s does not match group hash %s", file.Hash, dg.Hash)
	}

	// Check if file already exists in the group
	for _, existingFile := range dg.Files {
		if existingFile.ID == file.ID {
			return fmt.Errorf("file %s already exists in the group", file.ID)
		}
	}

	dg.Files = append(dg.Files, file)
	dg.Count = len(dg.Files)
	dg.TotalSize = dg.calculateTotalSize()
	dg.UpdatedAt = time.Now()

	return nil
}

// RemoveFile removes a file from the duplicate group
func (dg *DuplicateGroup) RemoveFile(fileID string) error {
	for i, file := range dg.Files {
		if file.ID == fileID {
			// Remove file from slice
			dg.Files = append(dg.Files[:i], dg.Files[i+1:]...)
			dg.Count = len(dg.Files)
			dg.TotalSize = dg.calculateTotalSize()
			dg.UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("file %s not found in duplicate group", fileID)
}

// calculateTotalSize calculates the total size of all files in the group
func (dg *DuplicateGroup) calculateTotalSize() int64 {
	if len(dg.Files) == 0 {
		return 0
	}
	// All files in a duplicate group have the same size, so multiply by count
	return dg.Files[0].Size * int64(dg.Count)
}

// GetWastedSpace returns the amount of space wasted by duplicates (total size minus one copy)
func (dg *DuplicateGroup) GetWastedSpace() int64 {
	if dg.Count <= 1 {
		return 0
	}
	return dg.Files[0].Size * int64(dg.Count-1)
}

// IsValid returns true if the group has more than one file (actual duplicates)
func (dg *DuplicateGroup) IsValid() bool {
	return dg.Count > 1
}

// GetFileByID returns a file from the group by its ID
func (dg *DuplicateGroup) GetFileByID(fileID string) (*File, error) {
	for _, file := range dg.Files {
		if file.ID == fileID {
			return file, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in duplicate group", fileID)
}

// GetOldestFile returns the file with the earliest modification time
func (dg *DuplicateGroup) GetOldestFile() *File {
	if len(dg.Files) == 0 {
		return nil
	}

	oldest := dg.Files[0]
	for _, file := range dg.Files[1:] {
		if file.ModifiedTime.Before(oldest.ModifiedTime) {
			oldest = file
		}
	}
	return oldest
}

// GetNewestFile returns the file with the latest modification time
func (dg *DuplicateGroup) GetNewestFile() *File {
	if len(dg.Files) == 0 {
		return nil
	}

	newest := dg.Files[0]
	for _, file := range dg.Files[1:] {
		if file.ModifiedTime.After(newest.ModifiedTime) {
			newest = file
		}
	}
	return newest
}

// GetFilesExcept returns all files in the group except the one with the given ID
func (dg *DuplicateGroup) GetFilesExcept(excludeFileID string) []*File {
	var result []*File
	for _, file := range dg.Files {
		if file.ID != excludeFileID {
			result = append(result, file)
		}
	}
	return result
}

// HasFileInFolder returns true if any file in the group is in the specified folder
func (dg *DuplicateGroup) HasFileInFolder(folderID string) bool {
	for _, file := range dg.Files {
		for _, parent := range file.Parents {
			if parent == folderID {
				return true
			}
		}
	}
	return false
}

// GetFilesInFolder returns all files in the group that are in the specified folder
func (dg *DuplicateGroup) GetFilesInFolder(folderID string) []*File {
	var result []*File
	for _, file := range dg.Files {
		for _, parent := range file.Parents {
			if parent == folderID {
				result = append(result, file)
				break
			}
		}
	}
	return result
}
