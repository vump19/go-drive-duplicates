package entities

import "time"

// FolderNode represents a node in the folder tree structure
// 폴더 트리 구조의 노드를 나타냄
type FolderNode struct {
	ID             string        `json:"id"`             // Google Drive folder ID
	Name           string        `json:"name"`           // Folder name
	Path           string        `json:"path"`           // Full path from root
	ParentID       string        `json:"parentId"`       // Parent folder ID
	Children       []*FolderNode `json:"children"`       // Child folders
	FileCount      int           `json:"fileCount"`      // Number of files in this folder
	SubfolderCount int           `json:"subfolderCount"` // Number of subfolders
	HasChildren    bool          `json:"hasChildren"`    // True if has subfolders
	IsExpanded     bool          `json:"isExpanded"`     // UI state: expanded or collapsed
	Level          int           `json:"level"`          // Depth level in tree (0 = root)
	ModifiedTime   time.Time     `json:"modifiedTime"`   // Last modified time
	CreatedTime    time.Time     `json:"createdTime"`    // Creation time
	WebViewLink    string        `json:"webViewLink"`    // Google Drive web link
	ThumbnailLink  string        `json:"thumbnailLink"`  // Folder thumbnail
	Shared         bool          `json:"shared"`         // True if shared with others
	Owners         []string      `json:"owners"`         // Owner email addresses
	Permissions    []string      `json:"permissions"`    // User permissions (read, write, etc)
}

// FolderTreeOptions contains options for building folder tree
// 폴더 트리 구축 옵션
type FolderTreeOptions struct {
	MaxDepth       int    `json:"maxDepth"`       // Maximum depth to load (0 = unlimited)
	IncludeFiles   bool   `json:"includeFiles"`   // Include file count
	ExpandAll      bool   `json:"expandAll"`      // Expand all nodes by default
	SortBy         string `json:"sortBy"`         // Sort criteria: name, modified, created
	SortOrder      string `json:"sortOrder"`      // Sort order: asc, desc
	IncludeShared  bool   `json:"includeShared"`  // Include shared folders
	IncludeTrashed bool   `json:"includeTrashed"` // Include trashed folders
}

// NewFolderNode creates a new folder node
func NewFolderNode(id, name, parentID string) *FolderNode {
	return &FolderNode{
		ID:          id,
		Name:        name,
		ParentID:    parentID,
		Children:    make([]*FolderNode, 0),
		HasChildren: false,
		IsExpanded:  false,
		Level:       0,
	}
}

// AddChild adds a child folder node
func (f *FolderNode) AddChild(child *FolderNode) {
	f.Children = append(f.Children, child)
	f.SubfolderCount = len(f.Children)
	f.HasChildren = f.SubfolderCount > 0
	child.Level = f.Level + 1
}

// RemoveChild removes a child folder node
func (f *FolderNode) RemoveChild(childID string) bool {
	for i, child := range f.Children {
		if child.ID == childID {
			f.Children = append(f.Children[:i], f.Children[i+1:]...)
			f.SubfolderCount = len(f.Children)
			f.HasChildren = f.SubfolderCount > 0
			return true
		}
	}
	return false
}

// FindChild finds a child by ID (recursive search)
func (f *FolderNode) FindChild(childID string) *FolderNode {
	if f.ID == childID {
		return f
	}

	for _, child := range f.Children {
		if found := child.FindChild(childID); found != nil {
			return found
		}
	}

	return nil
}

// GetAllChildren returns all children recursively (flatten tree)
func (f *FolderNode) GetAllChildren() []*FolderNode {
	result := make([]*FolderNode, 0)

	for _, child := range f.Children {
		result = append(result, child)
		result = append(result, child.GetAllChildren()...)
	}

	return result
}

// ExpandAll expands all nodes in the tree
func (f *FolderNode) ExpandAll() {
	f.IsExpanded = true
	for _, child := range f.Children {
		child.ExpandAll()
	}
}

// CollapseAll collapses all nodes in the tree
func (f *FolderNode) CollapseAll() {
	f.IsExpanded = false
	for _, child := range f.Children {
		child.CollapseAll()
	}
}

// GetTotalFileCount returns total file count including all children
func (f *FolderNode) GetTotalFileCount() int {
	total := f.FileCount
	for _, child := range f.Children {
		total += child.GetTotalFileCount()
	}
	return total
}

// GetTotalSubfolderCount returns total subfolder count including all descendants
func (f *FolderNode) GetTotalSubfolderCount() int {
	total := f.SubfolderCount
	for _, child := range f.Children {
		total += child.GetTotalSubfolderCount()
	}
	return total
}

// DefaultFolderTreeOptions returns default options for folder tree
func DefaultFolderTreeOptions() *FolderTreeOptions {
	return &FolderTreeOptions{
		MaxDepth:       1,     // Load 1 level deep (lazy loading for performance)
		IncludeFiles:   true,  // Include file counts
		ExpandAll:      false, // Collapsed by default
		SortBy:         "name",
		SortOrder:      "asc",
		IncludeShared:  true,
		IncludeTrashed: false,
	}
}
