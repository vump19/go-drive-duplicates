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
	// 재귀적으로 모든 하위 폴더 포함하여 스캔
	return g.ListFilesRecursive(ctx, "root", true)
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

		call := g.service.Files.List().
			Q(query).
			PageSize(g.pageSize).
			Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, parents, webViewLink, md5Checksum)")

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		// 재시도 로직 적용 (500, 503 등 일시적 오류 대응)
		var fileList *drive.FileList
		var err error
		for attempt := 0; attempt <= g.maxRetries; attempt++ {
			fileList, err = call.Context(ctx).Do()
			if err == nil {
				break
			}

			// 재시도 가능한 오류인지 확인 (500, 503, rate limit 등)
			if isRetryableError(err) && attempt < g.maxRetries {
				waitTime := time.Duration(1<<uint(attempt)) * time.Second
				log.Printf("⚠️ Google Drive API 일시적 오류 (시도 %d/%d), %v 후 재시도: %v",
					attempt+1, g.maxRetries+1, waitTime, err)
				time.Sleep(waitTime)
				continue
			}
			break
		}

		if err != nil {
			log.Printf("❌ Google Drive API 호출 실패 [%s]: %v", folderID, err)
			return nil, fmt.Errorf("failed to list files: %v", err)
		}

		log.Printf("📊 Google Drive API 응답: %d개 항목 반환 (폴더: %s)", len(fileList.Files), folderID)

		for _, driveFile := range fileList.Files {
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
	}

	log.Printf("✅ listFilesInFolder 완료 - 폴더 [%s]: 총 %d개 항목 반환", folderID, len(allFiles))
	return allFiles, nil
}

// isRetryableError checks if the error is a transient error that can be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Google API 500, 503, 429 등 재시도 가능한 오류
	retryablePatterns := []string{
		"500",
		"503",
		"429",
		"Internal Error",
		"internalError",
		"Backend Error",
		"backendError",
		"Rate Limit",
		"rateLimitExceeded",
		"userRateLimitExceeded",
		"timeout",
		"Timeout",
		"temporarily unavailable",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
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

		// 재시도 로직 적용
		var fileList *drive.FileList
		var err error
		for attempt := 0; attempt <= g.maxRetries; attempt++ {
			fileList, err = call.Context(ctx).Do()
			if err == nil {
				break
			}

			if isRetryableError(err) && attempt < g.maxRetries {
				waitTime := time.Duration(1<<uint(attempt)) * time.Second
				log.Printf("⚠️ Google Drive API 일시적 오류 (폴더 목록, 시도 %d/%d), %v 후 재시도: %v",
					attempt+1, g.maxRetries+1, waitTime, err)
				time.Sleep(waitTime)
				continue
			}
			break
		}

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
	var driveFile *drive.File
	var err error

	// 재시도 로직 적용
	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		driveFile, err = g.service.Files.Get(fileID).
			Fields("id, name, size, mimeType, modifiedTime, parents, webViewLink").
			Context(ctx).Do()
		if err == nil {
			break
		}

		if isRetryableError(err) && attempt < g.maxRetries {
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("⚠️ Google Drive API 일시적 오류 (GetFile, 시도 %d/%d), %v 후 재시도: %v",
				attempt+1, g.maxRetries+1, waitTime, err)
			time.Sleep(waitTime)
			continue
		}
		break
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get file: %v", err)
	}

	return g.convertDriveFileToEntity(driveFile)
}

func (g *GoogleDriveAdapter) FileExists(ctx context.Context, fileID string) (bool, error) {
	file, err := g.service.Files.Get(fileID).
		Fields("id, trashed").
		Context(ctx).Do()

	if err != nil {
		errStr := err.Error()
		log.Printf("🔍 FileExists 오류 [%s]: %s", fileID, errStr)

		// Check if it's a 404 error (file not found) - various formats
		if strings.Contains(errStr, "404") ||
			strings.Contains(errStr, "not found") ||
			strings.Contains(errStr, "notFound") ||
			strings.Contains(errStr, "File not found") {
			log.Printf("🗑️ 파일 삭제됨 (404): %s", fileID)
			return false, nil
		}
		// Other errors should be returned
		return false, fmt.Errorf("failed to check file existence: %v", err)
	}

	// Check if file is in trash
	if file.Trashed {
		log.Printf("🗑️ 파일 휴지통에 있음: %s", fileID)
		return false, nil
	}

	return true, nil
}

func (g *GoogleDriveAdapter) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	resp, err := g.service.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}

	return resp.Body, nil
}

// UploadFile uploads a file to Google Drive
func (g *GoogleDriveAdapter) UploadFile(ctx context.Context, parentFolderID, fileName, mimeType string, content io.Reader) (string, error) {
	file := &drive.File{
		Name:     fileName,
		MimeType: mimeType,
		Parents:  []string{parentFolderID},
	}

	log.Printf("📤 파일 업로드 시작: %s → 폴더 %s", fileName, parentFolderID)

	createdFile, err := g.service.Files.Create(file).
		Context(ctx).
		Media(content).
		Fields("id, name, size, mimeType").
		Do()
	if err != nil {
		return "", fmt.Errorf("파일 업로드 실패: %v", err)
	}

	log.Printf("✅ 파일 업로드 완료: %s (ID: %s, 크기: %d)", createdFile.Name, createdFile.Id, createdFile.Size)
	return createdFile.Id, nil
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

// TrashFile moves a file to trash (can be recovered later)
// 파일을 휴지통으로 이동 (나중에 복구 가능)
func (g *GoogleDriveAdapter) TrashFile(ctx context.Context, fileID string) error {
	update := &drive.File{Trashed: true}
	_, err := g.service.Files.Update(fileID, update).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to trash file: %v", err)
	}
	log.Printf("🗑️ 파일 휴지통 이동 완료: %s", fileID)
	return nil
}

// TrashFiles moves multiple files to trash with worker pool
// 여러 파일을 휴지통으로 이동 (워커 풀 사용)
func (g *GoogleDriveAdapter) TrashFiles(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}

	// Use parallel trash with worker pool to respect rate limits
	const maxWorkers = 5
	jobs := make(chan string, len(fileIDs))
	results := make(chan error, len(fileIDs))

	// Start worker pool
	for w := 0; w < maxWorkers; w++ {
		go func() {
			for fileID := range jobs {
				err := g.TrashFile(ctx, fileID)
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
		return fmt.Errorf("some files failed to move to trash: %s", strings.Join(errors, "; "))
	}

	log.Printf("🗑️ %d개 파일 휴지통 이동 완료", len(fileIDs))
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

// TrashFolder moves a folder to trash (can be recovered later)
// 폴더를 휴지통으로 이동 (나중에 복구 가능)
func (g *GoogleDriveAdapter) TrashFolder(ctx context.Context, folderID string) error {
	return g.TrashFile(ctx, folderID)
}

func (g *GoogleDriveAdapter) ListFolders(ctx context.Context, parentFolderID string) ([]*entities.File, error) {
	return g.listFoldersInFolder(ctx, parentFolderID)
}

// GetFileMetadata gets file metadata and returns as FileListItem
func (g *GoogleDriveAdapter) GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error) {
	file, err := g.service.Files.Get(fileID).
		Fields("id, name, mimeType, size, modifiedTime, createdTime, parents, iconLink, thumbnailLink, webViewLink, webContentLink, shared, starred, trashed, owners, permissions, fileExtension, md5Checksum, version, description, viewedByMe, viewedByMeTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("파일 메타데이터 조회 실패: %w", err)
	}

	return g.convertToFileListItem(file), nil
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

// SearchFilesWithOptions searches files with pagination and sorting options
// 옵션을 사용한 파일 검색 (페이지네이션, 정렬 지원)
func (g *GoogleDriveAdapter) SearchFilesWithOptions(ctx context.Context, query string, options *entities.ListOptions) (*entities.FileListResponse, error) {
	if options == nil {
		options = entities.DefaultListOptions()
	}

	pageSize := int64(options.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	// Build search query for Google Drive
	driveQuery := fmt.Sprintf("name contains '%s' and trashed = false", query)

	call := g.service.Files.List().
		Q(driveQuery).
		PageSize(pageSize).
		Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, parents, webViewLink)")

	// Apply page token if provided
	if options.PageToken != "" {
		call = call.PageToken(options.PageToken)
	}

	// Apply sorting
	if options.SortBy != "" {
		orderBy := options.SortBy
		if options.SortOrder == "desc" {
			orderBy += " desc"
		}
		// Convert common field names to Google Drive API format
		switch options.SortBy {
		case "modifiedTime":
			orderBy = "modifiedTime"
		case "name":
			orderBy = "name"
		case "size":
			orderBy = "quotaBytesUsed"
		}
		if options.SortOrder == "desc" {
			orderBy += " desc"
		}
		call = call.OrderBy(orderBy)
	}

	fileList, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("파일 검색 실패: %w", err)
	}

	// Convert to FileListItem
	items := make([]*entities.FileListItem, 0, len(fileList.Files))
	for _, driveFile := range fileList.Files {
		item := g.convertToFileListItem(driveFile)
		items = append(items, item)
	}

	return &entities.FileListResponse{
		Items:         items,
		NextPageToken: fileList.NextPageToken,
		TotalCount:    len(items),
	}, nil
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

	// Use Google Drive's md5Checksum as hash (no download needed!)
	// Note: Google Apps files (Docs, Sheets, etc.) don't have md5Checksum
	if driveFile.Md5Checksum != "" {
		file.Hash = driveFile.Md5Checksum
		file.HashCalculated = true
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

// UploadCSVAsSpreadsheet uploads a CSV file to Google Drive and converts it to Google Sheets
// CSV 파일을 Google Drive에 업로드하고 Google Sheets로 자동 변환
func (g *GoogleDriveAdapter) UploadCSVAsSpreadsheet(ctx context.Context, parentFolderID, fileName string, content io.Reader) (string, string, error) {
	log.Printf("📊 CSV → 스프레드시트 업로드 시작: %s → 폴더 %s", fileName, parentFolderID)

	file := &drive.File{
		Name:     fileName,
		Parents:  []string{parentFolderID},
		MimeType: "application/vnd.google-apps.spreadsheet",
	}

	var createdFile *drive.File
	var err error
	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		createdFile, err = g.service.Files.Create(file).
			Context(ctx).
			Media(content).
			Fields("id, name, webViewLink").
			Do()
		if err == nil {
			break
		}
		if isRetryableError(err) && attempt < g.maxRetries {
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("⚠️ CSV 업로드 재시도 (%d/%d), %v 후 재시도: %v", attempt+1, g.maxRetries+1, waitTime, err)
			time.Sleep(waitTime)
			continue
		}
		break
	}
	if err != nil {
		return "", "", fmt.Errorf("CSV 스프레드시트 업로드 실패: %v", err)
	}

	log.Printf("✅ CSV → 스프레드시트 업로드 완료: %s (ID: %s)", createdFile.Name, createdFile.Id)
	return createdFile.Id, createdFile.WebViewLink, nil
}

// ===== File Explorer Methods =====

// ListFoldersRecursive retrieves folder tree structure recursively
// 폴더 트리 구조를 재귀적으로 조회
func (g *GoogleDriveAdapter) ListFoldersRecursive(ctx context.Context, folderID string, maxDepth int) ([]*entities.FolderNode, error) {
	log.Printf("🔍 폴더 트리 조회 시작: %s (최대 깊이: %d)", folderID, maxDepth)

	rootNode, err := g.buildFolderTree(ctx, folderID, "", 0, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("폴더 트리 구축 실패: %w", err)
	}

	// Convert tree to flat list
	result := flattenFolderTree(rootNode)
	log.Printf("✅ 폴더 트리 조회 완료: %d개 폴더", len(result))

	return result, nil
}

// buildFolderTree builds folder tree recursively
func (g *GoogleDriveAdapter) buildFolderTree(ctx context.Context, folderID, parentPath string, currentDepth, maxDepth int) (*entities.FolderNode, error) {
	// Get folder metadata
	folder, err := g.service.Files.Get(folderID).
		Fields("id, name, parents, modifiedTime, createdTime, webViewLink, thumbnailLink, shared, owners, permissions").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("폴더 메타데이터 조회 실패: %w", err)
	}

	// Create folder node
	node := entities.NewFolderNode(folder.Id, folder.Name, "")
	if len(folder.Parents) > 0 {
		node.ParentID = folder.Parents[0]
	}
	node.Path = parentPath + "/" + folder.Name
	node.Level = currentDepth
	node.WebViewLink = folder.WebViewLink
	node.ThumbnailLink = folder.ThumbnailLink
	node.Shared = folder.Shared

	if folder.ModifiedTime != "" {
		if modTime, err := time.Parse(time.RFC3339, folder.ModifiedTime); err == nil {
			node.ModifiedTime = modTime
		}
	}
	if folder.CreatedTime != "" {
		if createdTime, err := time.Parse(time.RFC3339, folder.CreatedTime); err == nil {
			node.CreatedTime = createdTime
		}
	}

	// Count files in this folder
	fileCount, err := g.countFilesInFolder(ctx, folderID)
	if err == nil {
		node.FileCount = fileCount
	}

	// Stop if reached max depth
	if maxDepth > 0 && currentDepth >= maxDepth {
		return node, nil
	}

	// List subfolders with pagination to get all folders
	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false", folderID)

	var allFolders []*drive.File
	pageToken := ""

	for {
		call := g.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name)").
			PageSize(1000). // Maximum allowed by Drive API
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		fileList, err := call.Do()
		if err != nil {
			log.Printf("⚠️  하위 폴더 조회 실패: %v", err)
			return node, nil
		}

		allFolders = append(allFolders, fileList.Files...)

		if fileList.NextPageToken == "" {
			break
		}
		pageToken = fileList.NextPageToken
	}

	// Recursively build child nodes
	for _, childFolder := range allFolders {
		childNode, err := g.buildFolderTree(ctx, childFolder.Id, node.Path, currentDepth+1, maxDepth)
		if err != nil {
			log.Printf("⚠️  하위 폴더 구축 실패 (%s): %v", childFolder.Name, err)
			continue
		}
		node.AddChild(childNode)
	}

	return node, nil
}

// countFilesInFolder counts number of files (not folders) in a folder
func (g *GoogleDriveAdapter) countFilesInFolder(ctx context.Context, folderID string) (int, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType != 'application/vnd.google-apps.folder' and trashed = false", folderID)

	count := 0
	pageToken := ""

	for {
		call := g.service.Files.List().
			Q(query).
			Fields("nextPageToken").
			PageSize(1000).
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		fileList, err := call.Do()
		if err != nil {
			return 0, err
		}

		count += len(fileList.Files)

		pageToken = fileList.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return count, nil
}

// flattenFolderTree converts tree structure to flat list
func flattenFolderTree(root *entities.FolderNode) []*entities.FolderNode {
	result := []*entities.FolderNode{root}
	for _, child := range root.Children {
		result = append(result, flattenFolderTree(child)...)
	}
	return result
}

// ListFolderContents lists files and folders in a folder with advanced options
// 폴더 내 파일 및 폴더 목록을 고급 옵션과 함께 조회
func (g *GoogleDriveAdapter) ListFolderContents(ctx context.Context, folderID string, options *entities.ListOptions) (*entities.FileListResponse, error) {
	log.Printf("📁 폴더 내용 조회: %s", folderID)

	// Validate options
	if err := options.Validate(); err != nil {
		return nil, err
	}

	// Build query
	query := options.BuildGoogleDriveQuery(folderID)

	// Make API call with fields
	call := g.service.Files.List().
		Q(query).
		Fields("nextPageToken, files(id, name, mimeType, size, modifiedTime, createdTime, parents, iconLink, thumbnailLink, webViewLink, webContentLink, shared, starred, trashed, owners, permissions, fileExtension, md5Checksum, version, description, viewedByMe, viewedByMeTime)").
		PageSize(int64(options.PageSize)).
		Context(ctx)

	if options.PageToken != "" {
		call = call.PageToken(options.PageToken)
	}

	// Add ordering
	if options.SortBy != "" {
		orderBy := options.SortBy
		if options.SortOrder == "desc" {
			orderBy += " desc"
		}
		call = call.OrderBy(orderBy)
	}

	fileList, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("폴더 내용 조회 실패: %w", err)
	}

	// Convert to FileListItem
	items := make([]*entities.FileListItem, 0, len(fileList.Files))
	for _, file := range fileList.Files {
		item := g.convertToFileListItem(file)
		items = append(items, item)
	}

	log.Printf("✅ 폴더 내용 조회 완료: %d개 항목", len(items))

	return &entities.FileListResponse{
		Items:         items,
		NextPageToken: fileList.NextPageToken,
		TotalCount:    len(items),
		HasMore:       fileList.NextPageToken != "",
	}, nil
}

// convertToFileListItem converts Google Drive File to FileListItem entity
func (g *GoogleDriveAdapter) convertToFileListItem(file *drive.File) *entities.FileListItem {
	item := entities.NewFileListItem(file.Id, file.Name, file.MimeType)
	item.Size = file.Size
	item.Parents = file.Parents
	item.IconLink = file.IconLink
	item.ThumbnailLink = file.ThumbnailLink
	item.WebViewLink = file.WebViewLink
	item.WebContentLink = file.WebContentLink
	item.Shared = file.Shared
	item.Starred = file.Starred
	item.Trashed = file.ExplicitlyTrashed
	item.FileExtension = file.FileExtension
	item.MD5Checksum = file.Md5Checksum
	item.Version = file.Version
	item.Description = file.Description
	item.ViewedByMe = file.ViewedByMe

	if file.ModifiedTime != "" {
		if modTime, err := time.Parse(time.RFC3339, file.ModifiedTime); err == nil {
			item.ModifiedTime = modTime
		}
	}
	if file.CreatedTime != "" {
		if createdTime, err := time.Parse(time.RFC3339, file.CreatedTime); err == nil {
			item.CreatedTime = createdTime
		}
	}
	if file.ViewedByMeTime != "" {
		if viewedTime, err := time.Parse(time.RFC3339, file.ViewedByMeTime); err == nil {
			item.ViewedByMeTime = &viewedTime
		}
	}

	// Convert owners
	for _, owner := range file.Owners {
		item.Owners = append(item.Owners, entities.Owner{
			DisplayName:  owner.DisplayName,
			EmailAddress: owner.EmailAddress,
			PhotoLink:    owner.PhotoLink,
		})
	}

	return item
}

// CopyFile copies a file to target folder
// 파일을 대상 폴더로 복사
func (g *GoogleDriveAdapter) CopyFile(ctx context.Context, fileID, targetFolderID string) (string, error) {
	log.Printf("📋 파일 복사: %s → %s", fileID, targetFolderID)

	fileCopy := &drive.File{
		Parents: []string{targetFolderID},
	}

	copiedFile, err := g.service.Files.Copy(fileID, fileCopy).
		Fields("id, name").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("파일 복사 실패: %w", err)
	}

	log.Printf("✅ 파일 복사 완료: %s → %s", fileID, copiedFile.Id)
	return copiedFile.Id, nil
}

// MoveFile moves a file to target folder
// 파일을 대상 폴더로 이동
func (g *GoogleDriveAdapter) MoveFile(ctx context.Context, fileID, targetFolderID string) error {
	log.Printf("📦 파일 이동: %s → %s", fileID, targetFolderID)

	// Get current parents
	file, err := g.service.Files.Get(fileID).Fields("parents").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("파일 정보 조회 실패: %w", err)
	}

	// Move file
	previousParents := strings.Join(file.Parents, ",")
	_, err = g.service.Files.Update(fileID, nil).
		AddParents(targetFolderID).
		RemoveParents(previousParents).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("파일 이동 실패: %w", err)
	}

	log.Printf("✅ 파일 이동 완료: %s", fileID)
	return nil
}

// RenameFile renames a file
// 파일 이름 변경
func (g *GoogleDriveAdapter) RenameFile(ctx context.Context, fileID, newName string) error {
	log.Printf("✏️  파일 이름 변경: %s → %s", fileID, newName)

	fileUpdate := &drive.File{
		Name: newName,
	}

	_, err := g.service.Files.Update(fileID, fileUpdate).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("파일 이름 변경 실패: %w", err)
	}

	log.Printf("✅ 파일 이름 변경 완료: %s", fileID)
	return nil
}

// CreateFolder creates a new folder in parent folder and returns the folder ID
// 부모 폴더에 새 폴더 생성하고 폴더 ID 반환
func (g *GoogleDriveAdapter) CreateFolder(ctx context.Context, parentID, folderName string) (string, error) {
	log.Printf("📁 새 폴더 생성: %s (부모: %s)", folderName, parentID)

	folderMetadata := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}

	folder, err := g.service.Files.Create(folderMetadata).
		Fields("id, name, webViewLink, createdTime, modifiedTime").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("폴더 생성 실패: %w", err)
	}

	log.Printf("✅ 폴더 생성 완료: %s (%s)", folder.Name, folder.Id)
	return folder.Id, nil
}

// ===== Permission Methods =====

// ListPermissions lists all permissions for a file/folder
// 파일/폴더의 모든 권한 목록 조회
func (g *GoogleDriveAdapter) ListPermissions(ctx context.Context, fileID string) ([]*services.Permission, error) {
	var allPerms []*services.Permission
	var err error

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		var permList *drive.PermissionList
		permList, err = g.service.Permissions.List(fileID).
			Fields("permissions(id, type, role, emailAddress, displayName, expirationTime)").
			Context(ctx).
			Do()
		if err == nil {
			for _, p := range permList.Permissions {
				perm := &services.Permission{
					ID:             p.Id,
					Type:           p.Type,
					Role:           p.Role,
					EmailAddress:   p.EmailAddress,
					DisplayName:    p.DisplayName,
					ExpirationTime: p.ExpirationTime,
				}
				allPerms = append(allPerms, perm)
			}
			return allPerms, nil
		}

		if isRetryableError(err) && attempt < g.maxRetries {
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("⚠️ Permission 목록 조회 재시도 (%d/%d): %v", attempt+1, g.maxRetries+1, err)
			time.Sleep(waitTime)
			continue
		}
		break
	}

	return nil, fmt.Errorf("권한 목록 조회 실패: %w", err)
}

// DeletePermission deletes a specific permission from a file/folder
// 파일/폴더에서 특정 권한 삭제
func (g *GoogleDriveAdapter) DeletePermission(ctx context.Context, fileID string, permissionID string) error {
	var err error

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		err = g.service.Permissions.Delete(fileID, permissionID).Context(ctx).Do()
		if err == nil {
			log.Printf("🔓 권한 삭제 완료: 파일=%s, 권한=%s", fileID, permissionID)
			return nil
		}

		if isRetryableError(err) && attempt < g.maxRetries {
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("⚠️ 권한 삭제 재시도 (%d/%d): %v", attempt+1, g.maxRetries+1, err)
			time.Sleep(waitTime)
			continue
		}
		break
	}

	return fmt.Errorf("권한 삭제 실패: %w", err)
}

// UpdatePermission updates the role of a specific permission
// 특정 권한의 역할 변경
func (g *GoogleDriveAdapter) UpdatePermission(ctx context.Context, fileID string, permissionID string, newRole string) error {
	var err error

	perm := &drive.Permission{
		Role: newRole,
	}

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		_, err = g.service.Permissions.Update(fileID, permissionID, perm).Context(ctx).Do()
		if err == nil {
			log.Printf("🔄 권한 변경 완료: 파일=%s, 권한=%s, 새역할=%s", fileID, permissionID, newRole)
			return nil
		}

		if isRetryableError(err) && attempt < g.maxRetries {
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("⚠️ 권한 변경 재시도 (%d/%d): %v", attempt+1, g.maxRetries+1, err)
			time.Sleep(waitTime)
			continue
		}
		break
	}

	return fmt.Errorf("권한 변경 실패: %w", err)
}
