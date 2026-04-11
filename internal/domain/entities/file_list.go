package entities

import "time"

// FileListItem represents a file or folder in the file list
// 파일 목록의 파일 또는 폴더를 나타냄
type FileListItem struct {
	ID             string     `json:"id"`             // Google Drive file/folder ID
	Name           string     `json:"name"`           // File/folder name
	MimeType       string     `json:"mimeType"`       // MIME type
	Size           int64      `json:"size"`           // File size in bytes
	ModifiedTime   time.Time  `json:"modifiedTime"`   // Last modified time
	CreatedTime    time.Time  `json:"createdTime"`    // Creation time
	Parents        []string   `json:"parents"`        // Parent folder IDs
	Path           string     `json:"path,omitempty"` // Full path from root
	IconLink       string     `json:"iconLink"`       // Icon URL
	ThumbnailLink  string     `json:"thumbnailLink"`  // Thumbnail URL
	WebViewLink    string     `json:"webViewLink"`    // Google Drive web link
	WebContentLink string     `json:"webContentLink"` // Direct download link
	IsFolder       bool       `json:"isFolder"`       // True if this is a folder
	Shared         bool       `json:"shared"`         // True if shared
	Starred        bool       `json:"starred"`        // True if starred
	Trashed        bool       `json:"trashed"`        // True if in trash
	Owners         []Owner    `json:"owners"`         // File owners
	Permissions    []string   `json:"permissions"`    // User permissions
	FileExtension  string     `json:"fileExtension"`  // File extension (e.g., "pdf", "jpg")
	MD5Checksum    string     `json:"md5Checksum"`    // MD5 checksum for integrity
	SHA1Checksum   string     `json:"sha1Checksum"`   // SHA1 checksum
	SHA256Checksum string     `json:"sha256Checksum"` // SHA256 checksum (if calculated)
	Version        int64      `json:"version"`        // File version number
	Description    string     `json:"description"`    // File description
	ViewedByMe     bool       `json:"viewedByMe"`     // True if viewed by current user
	ViewedByMeTime *time.Time `json:"viewedByMeTime"` // Last viewed time
}

// Owner represents a file/folder owner
type Owner struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	PhotoLink    string `json:"photoLink"`
}

// ListOptions contains options for listing files
// 파일 목록 조회 옵션
type ListOptions struct {
	SortBy         string     `json:"sortBy"`         // Sort criteria: name, size, modified, created, type
	SortOrder      string     `json:"sortOrder"`      // Sort order: asc, desc
	SearchQuery    string     `json:"searchQuery"`    // Search query
	FileTypes      []string   `json:"fileTypes"`      // MIME type filters
	PageToken      string     `json:"pageToken"`      // Pagination token
	PageSize       int        `json:"pageSize"`       // Items per page
	IncludeFolders bool       `json:"includeFolders"` // Include folders in results
	IncludeFiles   bool       `json:"includeFiles"`   // Include files in results
	IncludeTrashed bool       `json:"includeTrashed"` // Include trashed items
	MinSize        int64      `json:"minSize"`        // Minimum file size filter
	MaxSize        int64      `json:"maxSize"`        // Maximum file size filter
	ModifiedAfter  *time.Time `json:"modifiedAfter"`  // Filter by modified date
	ModifiedBefore *time.Time `json:"modifiedBefore"` // Filter by modified date
}

// FileListResponse represents the response for file listing
type FileListResponse struct {
	Items         []*FileListItem `json:"items"`         // List of files/folders
	NextPageToken string          `json:"nextPageToken"` // Token for next page
	TotalCount    int             `json:"totalCount"`    // Total number of items
	HasMore       bool            `json:"hasMore"`       // True if more pages available
}

// NewFileListItem creates a new file list item
func NewFileListItem(id, name, mimeType string) *FileListItem {
	return &FileListItem{
		ID:          id,
		Name:        name,
		MimeType:    mimeType,
		IsFolder:    mimeType == "application/vnd.google-apps.folder",
		Parents:     make([]string, 0),
		Owners:      make([]Owner, 0),
		Permissions: make([]string, 0),
	}
}

// IsGoogleDoc returns true if this is a Google Docs file
func (f *FileListItem) IsGoogleDoc() bool {
	googleDocTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.drawing",
	}

	for _, docType := range googleDocTypes {
		if f.MimeType == docType {
			return true
		}
	}
	return false
}

// IsImage returns true if this is an image file
func (f *FileListItem) IsImage() bool {
	return len(f.MimeType) > 6 && f.MimeType[:6] == "image/"
}

// IsVideo returns true if this is a video file
func (f *FileListItem) IsVideo() bool {
	return len(f.MimeType) > 6 && f.MimeType[:6] == "video/"
}

// IsAudio returns true if this is an audio file
func (f *FileListItem) IsAudio() bool {
	return len(f.MimeType) > 6 && f.MimeType[:6] == "audio/"
}

// IsPDF returns true if this is a PDF file
func (f *FileListItem) IsPDF() bool {
	return f.MimeType == "application/pdf"
}

// IsText returns true if this is a text file
func (f *FileListItem) IsText() bool {
	return len(f.MimeType) > 5 && f.MimeType[:5] == "text/"
}

// IsArchive returns true if this is an archive file
func (f *FileListItem) IsArchive() bool {
	archiveTypes := []string{
		"application/zip",
		"application/x-zip-compressed",
		"application/x-rar-compressed",
		"application/x-7z-compressed",
		"application/x-tar",
		"application/gzip",
	}

	for _, archiveType := range archiveTypes {
		if f.MimeType == archiveType {
			return true
		}
	}
	return false
}

// GetFileTypeCategory returns a category for the file type
func (f *FileListItem) GetFileTypeCategory() string {
	if f.IsFolder {
		return "folder"
	}
	if f.IsImage() {
		return "image"
	}
	if f.IsVideo() {
		return "video"
	}
	if f.IsAudio() {
		return "audio"
	}
	if f.IsPDF() {
		return "pdf"
	}
	if f.IsText() {
		return "text"
	}
	if f.IsArchive() {
		return "archive"
	}
	if f.IsGoogleDoc() {
		return "google-doc"
	}
	return "other"
}

// DefaultListOptions returns default options for file listing
func DefaultListOptions() *ListOptions {
	return &ListOptions{
		SortBy:         "name",
		SortOrder:      "asc",
		PageSize:       100,
		IncludeFolders: true,
		IncludeFiles:   true,
		IncludeTrashed: false,
		FileTypes:      make([]string, 0),
	}
}

// Validate validates list options
func (opts *ListOptions) Validate() error {
	// Validate sort criteria
	validSortBy := map[string]bool{
		"name": true, "size": true, "modified": true,
		"created": true, "type": true,
	}
	if opts.SortBy != "" && !validSortBy[opts.SortBy] {
		return NewValidationError("sortBy", "invalid sortBy value")
	}

	// Validate sort order
	if opts.SortOrder != "" && opts.SortOrder != "asc" && opts.SortOrder != "desc" {
		return NewValidationError("sortOrder", "sortOrder must be 'asc' or 'desc'")
	}

	// Validate page size
	if opts.PageSize < 1 {
		opts.PageSize = 100
	}
	if opts.PageSize > 1000 {
		return NewValidationError("pageSize", "pageSize cannot exceed 1000")
	}

	// Validate size filters
	if opts.MinSize < 0 {
		return NewValidationError("minSize", "minSize cannot be negative")
	}
	if opts.MaxSize > 0 && opts.MaxSize < opts.MinSize {
		return NewValidationError("maxSize", "maxSize cannot be less than minSize")
	}

	// Validate date filters
	if opts.ModifiedAfter != nil && opts.ModifiedBefore != nil {
		if opts.ModifiedAfter.After(*opts.ModifiedBefore) {
			return NewValidationError("modifiedAfter", "modifiedAfter cannot be after modifiedBefore")
		}
	}

	return nil
}

// BuildGoogleDriveQuery builds a Google Drive API query string from options
func (opts *ListOptions) BuildGoogleDriveQuery(parentFolderID string) string {
	query := "'" + parentFolderID + "' in parents"

	if !opts.IncludeTrashed {
		query += " and trashed = false"
	}

	if !opts.IncludeFolders {
		query += " and mimeType != 'application/vnd.google-apps.folder'"
	}

	if !opts.IncludeFiles {
		query += " and mimeType = 'application/vnd.google-apps.folder'"
	}

	if opts.SearchQuery != "" {
		query += " and name contains '" + opts.SearchQuery + "'"
	}

	if len(opts.FileTypes) > 0 {
		mimeTypeQuery := " and ("
		for i, mimeType := range opts.FileTypes {
			if i > 0 {
				mimeTypeQuery += " or "
			}
			mimeTypeQuery += "mimeType = '" + mimeType + "'"
		}
		mimeTypeQuery += ")"
		query += mimeTypeQuery
	}

	if opts.MinSize > 0 {
		query += " and size >= " + string(opts.MinSize)
	}

	if opts.MaxSize > 0 {
		query += " and size <= " + string(opts.MaxSize)
	}

	return query
}
