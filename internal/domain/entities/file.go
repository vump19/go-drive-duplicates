package entities

import (
	"time"
)

// File represents a file in the system with all its metadata and computed properties
type File struct {
	// Basic file metadata
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	MimeType     string    `json:"mimeType"`
	ModifiedTime time.Time `json:"modifiedTime"`

	// Computed properties
	Hash           string `json:"hash,omitempty"`
	HashCalculated bool   `json:"hashCalculated"`

	// Location and path information
	Parents []string `json:"parents,omitempty"`
	Path    string   `json:"path,omitempty"`

	// External links
	WebViewLink string `json:"webViewLink"`

	// Metadata
	LastUpdated time.Time `json:"lastUpdated"`
}

// NewFile creates a new File entity with required fields
func NewFile(id, name string, size int64, mimeType string, modifiedTime time.Time, webViewLink string) *File {
	return &File{
		ID:           id,
		Name:         name,
		Size:         size,
		MimeType:     mimeType,
		ModifiedTime: modifiedTime,
		WebViewLink:  webViewLink,
		LastUpdated:  time.Now(),
	}
}

// IsHashCalculated returns true if the file hash has been calculated
func (f *File) IsHashCalculated() bool {
	return f.HashCalculated && f.Hash != ""
}

// SetHash sets the file hash and marks it as calculated
func (f *File) SetHash(hash string) {
	f.Hash = hash
	f.HashCalculated = true
	f.LastUpdated = time.Now()
}

// GetFileExtension returns the file extension from the filename
func (f *File) GetFileExtension() string {
	for i := len(f.Name) - 1; i >= 0; i-- {
		if f.Name[i] == '.' {
			return f.Name[i:]
		}
	}
	return ""
}

// GetSizeCategory returns the size category of the file
func (f *File) GetSizeCategory() string {
	const (
		mb = 1024 * 1024
		gb = mb * 1024
	)

	switch {
	case f.Size < mb:
		return "small"
	case f.Size < 100*mb:
		return "medium"
	case f.Size < gb:
		return "large"
	default:
		return "very_large"
	}
}

// GetFileCategory returns the category based on MIME type
func (f *File) GetFileCategory() string {
	switch {
	case f.MimeType == "application/vnd.google-apps.folder":
		return "folder"
	case len(f.MimeType) >= 5 && f.MimeType[:5] == "image":
		return "image"
	case len(f.MimeType) >= 5 && f.MimeType[:5] == "video":
		return "video"
	case len(f.MimeType) >= 5 && f.MimeType[:5] == "audio":
		return "audio"
	case f.MimeType == "application/pdf" ||
		f.MimeType == "application/msword" ||
		f.MimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		f.MimeType == "application/vnd.ms-excel" ||
		f.MimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		f.MimeType == "application/vnd.ms-powerpoint" ||
		f.MimeType == "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "document"
	case len(f.MimeType) >= 4 && f.MimeType[:4] == "text":
		return "text"
	default:
		return "other"
	}
}

// IsLargeFile returns true if the file is considered large (>100MB)
func (f *File) IsLargeFile() bool {
	return f.Size > 100*1024*1024
}

// UpdatePath sets the path for the file
func (f *File) UpdatePath(path string) {
	f.Path = path
	f.LastUpdated = time.Now()
}

// AddParent adds a parent folder ID
func (f *File) AddParent(parentID string) {
	for _, existing := range f.Parents {
		if existing == parentID {
			return // Already exists
		}
	}
	f.Parents = append(f.Parents, parentID)
	f.LastUpdated = time.Now()
}
