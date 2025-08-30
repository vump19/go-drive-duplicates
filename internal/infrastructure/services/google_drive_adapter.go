package services

import (
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type GoogleDriveAdapter struct {
	service    *drive.Service
	apiKey     string
	pageSize   int64
	maxRetries int
}

func NewGoogleDriveAdapter(credentialsPath string, apiKey string) (services.StorageProvider, error) {
	ctx := context.Background()

	var service *drive.Service
	var err error

	if credentialsPath != "" {
		// Try OAuth2 client credentials first
		service, err = createOAuth2Service(ctx, credentialsPath)
		if err != nil {
			// Fallback to service account credentials
			service, err = drive.NewService(ctx, option.WithCredentialsFile(credentialsPath))
			if err != nil {
				return nil, fmt.Errorf("failed to create Drive service with credentials: %v", err)
			}
		}
	} else if apiKey != "" {
		// Use API key
		service, err = drive.NewService(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			return nil, fmt.Errorf("failed to create Drive service with API key: %v", err)
		}
	} else {
		return nil, fmt.Errorf("either credentials file or API key must be provided")
	}

	return &GoogleDriveAdapter{
		service:    service,
		apiKey:     apiKey,
		pageSize:   1000, // Maximum allowed by Drive API
		maxRetries: 3,
	}, nil
}

func (g *GoogleDriveAdapter) Authenticate(ctx context.Context) error {
	// For Google Drive API, authentication is handled during service creation
	// This method can be used to verify if the current credentials are valid
	_, err := g.service.About.Get().Fields("user").Context(ctx).Do()
	return err
}

func (g *GoogleDriveAdapter) IsAuthenticated() bool {
	return g.service != nil
}

func (g *GoogleDriveAdapter) ListFiles(ctx context.Context, folderID string) ([]*entities.File, error) {
	return g.listFilesInFolder(ctx, folderID)
}

func (g *GoogleDriveAdapter) ListAllFiles(ctx context.Context) ([]*entities.File, error) {
	return g.ListFiles(ctx, "root")
}

func (g *GoogleDriveAdapter) ListFilesRecursive(ctx context.Context, folderID string, recursive bool) ([]*entities.File, error) {
	if recursive {
		return g.listFilesRecursive(ctx, folderID, "")
	}

	return g.listFilesInFolder(ctx, folderID)
}

func (g *GoogleDriveAdapter) listFilesRecursive(ctx context.Context, folderID, currentPath string) ([]*entities.File, error) {
	var allFiles []*entities.File

	// List files in current folder
	files, err := g.listFilesInFolder(ctx, folderID)
	if err != nil {
		return nil, err
	}

	// Update file paths
	for _, file := range files {
		if currentPath != "" {
			file.Path = filepath.Join(currentPath, file.Name)
		} else {
			file.Path = file.Name
		}
	}
	allFiles = append(allFiles, files...)

	// List subfolders and recursively process them
	folders, err := g.listFoldersInFolder(ctx, folderID)
	if err != nil {
		return nil, err
	}

	for _, folder := range folders {
		folderPath := currentPath
		if folderPath != "" {
			folderPath = filepath.Join(folderPath, folder.Name)
		} else {
			folderPath = folder.Name
		}

		subFiles, err := g.listFilesRecursive(ctx, folder.ID, folderPath)
		if err != nil {
			return nil, fmt.Errorf("failed to list files in folder %s: %v", folder.Name, err)
		}
		allFiles = append(allFiles, subFiles...)
	}

	return allFiles, nil
}

func (g *GoogleDriveAdapter) listFilesInFolder(ctx context.Context, folderID string) ([]*entities.File, error) {
	var allFiles []*entities.File
	pageToken := ""
	
	log.Printf("🔍 Google Drive API 호출 - listFilesInFolder: %s", folderID)

	for {
		// Include both files and folders (remove folder exclusion)
		query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
		log.Printf("📋 Google Drive 쿼리: %s", query)

		call := g.service.Files.List().
			Q(query).
			PageSize(g.pageSize).
			Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, parents, webViewLink)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		fileList, err := call.Context(ctx).Do()
		if err != nil {
			log.Printf("❌ Google Drive API 호출 실패 [%s]: %v", folderID, err)
			return nil, fmt.Errorf("failed to list files: %v", err)
		}
		
		log.Printf("📊 Google Drive API 응답: %d개 항목 반환 (폴더: %s)", len(fileList.Files), folderID)

		for i, driveFile := range fileList.Files {
			log.Printf("  API 응답 항목 %d: %s (ID: %s, 타입: %s, 크기: %d)", 
				i+1, driveFile.Name, driveFile.Id, driveFile.MimeType, driveFile.Size)
			
			file, err := g.convertDriveFileToEntity(driveFile)
			if err != nil {
				log.Printf("  ⚠️ 항목 변환 실패: %s - %v", driveFile.Name, err)
				continue // Skip files that can't be converted
			}
			allFiles = append(allFiles, file)
		}

		pageToken = fileList.NextPageToken
		if pageToken == "" {
			break
		}
		log.Printf("📖 다음 페이지 토큰: %s", pageToken)
	}
	
	log.Printf("✅ listFilesInFolder 완료 - 폴더 [%s]: 총 %d개 항목 반환", folderID, len(allFiles))
	return allFiles, nil
}

func (g *GoogleDriveAdapter) listFoldersInFolder(ctx context.Context, folderID string) ([]*entities.File, error) {
	var allFolders []*entities.File
	pageToken := ""

	for {
		query := fmt.Sprintf("'%s' in parents and trashed = false and mimeType = 'application/vnd.google-apps.folder'", folderID)

		call := g.service.Files.List().
			Q(query).
			PageSize(g.pageSize).
			Fields("nextPageToken, files(id, name, mimeType, modifiedTime, parents, webViewLink)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		fileList, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list folders: %v", err)
		}

		for _, driveFile := range fileList.Files {
			folder := &entities.File{
				ID:          driveFile.Id,
				Name:        driveFile.Name,
				Size:        0, // Folders don't have size
				MimeType:    driveFile.MimeType,
				Parents:     driveFile.Parents,
				WebViewLink: driveFile.WebViewLink,
				LastUpdated: time.Now(),
			}

			if driveFile.ModifiedTime != "" {
				if modTime, err := time.Parse(time.RFC3339, driveFile.ModifiedTime); err == nil {
					folder.ModifiedTime = modTime
				}
			}

			allFolders = append(allFolders, folder)
		}

		pageToken = fileList.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return allFolders, nil
}

func (g *GoogleDriveAdapter) GetFile(ctx context.Context, fileID string) (*entities.File, error) {
	driveFile, err := g.service.Files.Get(fileID).
		Fields("id, name, size, mimeType, modifiedTime, parents, webViewLink").
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %v", err)
	}

	return g.convertDriveFileToEntity(driveFile)
}

func (g *GoogleDriveAdapter) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	resp, err := g.service.Files.Get(fileID).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}

	return resp.Body, nil
}

func (g *GoogleDriveAdapter) DeleteFile(ctx context.Context, fileID string) error {
	err := g.service.Files.Delete(fileID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to delete file: %v", err)
	}
	return nil
}

func (g *GoogleDriveAdapter) DeleteFiles(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	// Use parallel deletion with worker pool to respect rate limits
	const maxWorkers = 5 // Conservative limit to avoid hitting Google API rate limits
	jobs := make(chan string, len(fileIDs))
	results := make(chan error, len(fileIDs))

	// Start worker pool
	for w := 0; w < maxWorkers; w++ {
		go func() {
			for fileID := range jobs {
				err := g.DeleteFile(ctx, fileID)
				results <- err
			}
		}()
	}

	// Send jobs
	for _, fileID := range fileIDs {
		jobs <- fileID
	}
	close(jobs)

	// Collect results
	var errors []string
	for i := 0; i < len(fileIDs); i++ {
		if err := <-results; err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some files failed to delete: %s", strings.Join(errors, "; "))
	}

	return nil
}

func (g *GoogleDriveAdapter) GetFolder(ctx context.Context, folderID string) (*entities.File, error) {
	return g.GetFile(ctx, folderID)
}

func (g *GoogleDriveAdapter) GetFolderInfo(ctx context.Context, folderID string) (*entities.File, error) {
	return g.GetFile(ctx, folderID)
}

func (g *GoogleDriveAdapter) DeleteFolder(ctx context.Context, folderID string) error {
	return g.DeleteFile(ctx, folderID)
}

func (g *GoogleDriveAdapter) ListFolders(ctx context.Context, parentFolderID string) ([]*entities.File, error) {
	return g.listFoldersInFolder(ctx, parentFolderID)
}

func (g *GoogleDriveAdapter) GetFileMetadata(ctx context.Context, fileID string) (map[string]interface{}, error) {
	driveFile, err := g.service.Files.Get(fileID).
		Fields("*").
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %v", err)
	}

	metadata := map[string]interface{}{
		"id":                           driveFile.Id,
		"name":                         driveFile.Name,
		"mimeType":                     driveFile.MimeType,
		"size":                         driveFile.Size,
		"createdTime":                  driveFile.CreatedTime,
		"modifiedTime":                 driveFile.ModifiedTime,
		"version":                      driveFile.Version,
		"webViewLink":                  driveFile.WebViewLink,
		"webContentLink":               driveFile.WebContentLink,
		"iconLink":                     driveFile.IconLink,
		"thumbnailLink":                driveFile.ThumbnailLink,
		"viewedByMe":                   driveFile.ViewedByMe,
		"viewedByMeTime":               driveFile.ViewedByMeTime,
		"owners":                       driveFile.Owners,
		"parents":                      driveFile.Parents,
		"permissions":                  driveFile.Permissions,
		"shared":                       driveFile.Shared,
		"sharingUser":                  driveFile.SharingUser,
		"spaces":                       driveFile.Spaces,
		"starred":                      driveFile.Starred,
		"trashed":                      driveFile.Trashed,
		"explicitlyTrashed":            driveFile.ExplicitlyTrashed,
		"trashedTime":                  driveFile.TrashedTime,
		"originalFilename":             driveFile.OriginalFilename,
		"fullFileExtension":            driveFile.FullFileExtension,
		"fileExtension":                driveFile.FileExtension,
		"md5Checksum":                  driveFile.Md5Checksum,
		"sha1Checksum":                 driveFile.Sha1Checksum,
		"sha256Checksum":               driveFile.Sha256Checksum,
		"quotaBytesUsed":               driveFile.QuotaBytesUsed,
		"isAppAuthorized":              driveFile.IsAppAuthorized,
		"copyRequiresWriterPermission": driveFile.CopyRequiresWriterPermission,
		"writersCanShare":              driveFile.WritersCanShare,
		"viewersCanCopyContent":        driveFile.ViewersCanCopyContent,
		"description":                  driveFile.Description,
		"folderColorRgb":               driveFile.FolderColorRgb,
	}

	return metadata, nil
}

func (g *GoogleDriveAdapter) SearchFiles(ctx context.Context, query string) ([]*entities.File, error) {
	return g.SearchFilesWithLimit(ctx, query, 0)
}

func (g *GoogleDriveAdapter) SearchFilesWithLimit(ctx context.Context, query string, maxResults int) ([]*entities.File, error) {
	var allFiles []*entities.File
	pageToken := ""
	resultsCount := 0

	for {
		pageSize := g.pageSize
		if maxResults > 0 && int64(maxResults-resultsCount) < pageSize {
			pageSize = int64(maxResults - resultsCount)
		}

		call := g.service.Files.List().
			Q(query + " and trashed = false").
			PageSize(pageSize).
			Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, parents, webViewLink)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		fileList, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to search files: %v", err)
		}

		for _, driveFile := range fileList.Files {
			file, err := g.convertDriveFileToEntity(driveFile)
			if err != nil {
				continue // Skip files that can't be converted
			}
			allFiles = append(allFiles, file)
			resultsCount++

			if maxResults > 0 && resultsCount >= maxResults {
				return allFiles, nil
			}
		}

		pageToken = fileList.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return allFiles, nil
}

func (g *GoogleDriveAdapter) GetQuotaInfo(ctx context.Context) (map[string]interface{}, error) {
	about, err := g.service.About.Get().
		Fields("storageQuota, user").
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get quota info: %v", err)
	}

	quotaInfo := map[string]interface{}{
		"limit":             about.StorageQuota.Limit,
		"usage":             about.StorageQuota.Usage,
		"usageInDrive":      about.StorageQuota.UsageInDrive,
		"usageInDriveTrash": about.StorageQuota.UsageInDriveTrash,
		"user":              about.User,
	}

	return quotaInfo, nil
}

// Helper functions

func (g *GoogleDriveAdapter) convertDriveFileToEntity(driveFile *drive.File) (*entities.File, error) {
	file := &entities.File{
		ID:          driveFile.Id,
		Name:        driveFile.Name,
		MimeType:    driveFile.MimeType,
		Parents:     driveFile.Parents,
		WebViewLink: driveFile.WebViewLink,
		LastUpdated: time.Now(),
	}

	// Convert size (string to int64)
	if driveFile.Size != 0 {
		file.Size = driveFile.Size
	}

	// Parse modified time
	if driveFile.ModifiedTime != "" {
		if modTime, err := time.Parse(time.RFC3339, driveFile.ModifiedTime); err == nil {
			file.ModifiedTime = modTime
		}
	}

	return file, nil
}

func (g *GoogleDriveAdapter) IsFolder(file *entities.File) bool {
	return file.MimeType == "application/vnd.google-apps.folder"
}

func (g *GoogleDriveAdapter) GetFileExtension(file *entities.File) string {
	if g.IsFolder(file) {
		return ""
	}
	return strings.ToLower(filepath.Ext(file.Name))
}

func (g *GoogleDriveAdapter) GetFolderPath(ctx context.Context, folderID string) (string, error) {
	var pathParts []string
	currentID := folderID

	for currentID != "" && currentID != "root" {
		folder, err := g.GetFile(ctx, currentID)
		if err != nil {
			return "", err
		}

		pathParts = append([]string{folder.Name}, pathParts...)

		if len(folder.Parents) > 0 {
			currentID = folder.Parents[0]
		} else {
			break
		}
	}

	if len(pathParts) == 0 {
		return "/", nil
	}

	return "/" + strings.Join(pathParts, "/"), nil
}

// Batch operations for better performance

func (g *GoogleDriveAdapter) BatchGetFiles(ctx context.Context, fileIDs []string) ([]*entities.File, error) {
	var files []*entities.File
	var errors []string

	// Google Drive API doesn't have native batch get, so we make individual calls
	// In a production system, you might want to implement proper batching
	for _, fileID := range fileIDs {
		file, err := g.GetFile(ctx, fileID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to get file %s: %v", fileID, err))
			continue
		}
		files = append(files, file)
	}

	if len(errors) > 0 && len(files) == 0 {
		return nil, fmt.Errorf("all file requests failed: %s", strings.Join(errors, "; "))
	}

	return files, nil
}

// Retry mechanism for API calls

func (g *GoogleDriveAdapter) withRetry(operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		if err := operation(); err != nil {
			lastErr = err
			if attempt < g.maxRetries {
				// Exponential backoff
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
				continue
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("operation failed after %d retries: %v", g.maxRetries, lastErr)
}

// Additional interface methods

func (g *GoogleDriveAdapter) SearchByMimeType(ctx context.Context, mimeType string) ([]*entities.File, error) {
	query := fmt.Sprintf("mimeType='%s'", mimeType)
	return g.SearchFiles(ctx, query)
}

func (g *GoogleDriveAdapter) SearchByName(ctx context.Context, name string) ([]*entities.File, error) {
	query := fmt.Sprintf("name contains '%s'", name)
	return g.SearchFiles(ctx, query)
}

func (g *GoogleDriveAdapter) GetFileParents(ctx context.Context, fileID string) ([]string, error) {
	file, err := g.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return file.Parents, nil
}

func (g *GoogleDriveAdapter) UpdateFileMetadata(ctx context.Context, fileID string, metadata map[string]interface{}) error {
	// This would require implementing Google Drive file update API
	// For now, return not implemented
	return fmt.Errorf("metadata update not implemented")
}

func (g *GoogleDriveAdapter) GetQuota(ctx context.Context) (used, total int64, err error) {
	quotaInfo, err := g.GetQuotaInfo(ctx)
	if err != nil {
		return 0, 0, err
	}

	if usage, ok := quotaInfo["usage"].(int64); ok {
		used = usage
	}
	if limit, ok := quotaInfo["limit"].(int64); ok {
		total = limit
	}

	return used, total, nil
}

func (g *GoogleDriveAdapter) GetRateLimit() int {
	return 100 // Default rate limit requests per second
}

func (g *GoogleDriveAdapter) BatchDelete(ctx context.Context, fileIDs []string, batchSize int) error {
	return g.DeleteFiles(ctx, fileIDs)
}

func (g *GoogleDriveAdapter) GetProviderName() string {
	return "Google Drive"
}

func (g *GoogleDriveAdapter) GetMaxBatchSize() int {
	return 100 // Google Drive API batch limit
}

func (g *GoogleDriveAdapter) SupportsResumableDownload() bool {
	return true
}

// OAuth2 client credential support functions

func createOAuth2Service(ctx context.Context, credentialsPath string) (*drive.Service, error) {
	// Read OAuth2 client credentials
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %v", err)
	}

	// Parse the credentials file
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file to config: %v", err)
	}

	// Get the token
	token, err := getTokenFromFile()
	if err != nil {
		return nil, fmt.Errorf("unable to get valid token: %v", err)
	}

	// Create HTTP client with token
	client := config.Client(ctx, token)

	// Create Drive service
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Drive service: %v", err)
	}

	return service, nil
}

func getTokenFromFile() (*oauth2.Token, error) {
	tokenFile := "token.json"
	f, err := os.Open(tokenFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}
