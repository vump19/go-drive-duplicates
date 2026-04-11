package usecases

import (
	"archive/zip"
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FolderDownloadUseCase handles folder download operations
type FolderDownloadUseCase struct {
	fileRepo        repositories.FileRepository
	progressRepo    repositories.ProgressRepository
	storageProvider services.StorageProvider
	progressService services.ProgressService

	// 진행 중인 다운로드 추적
	activeDownloads   map[string]*DownloadProgress
	activeDownloadsMu sync.RWMutex
}

// NewFolderDownloadUseCase creates a new folder download use case
func NewFolderDownloadUseCase(
	fileRepo repositories.FileRepository,
	progressRepo repositories.ProgressRepository,
	storageProvider services.StorageProvider,
	progressService services.ProgressService,
) *FolderDownloadUseCase {
	return &FolderDownloadUseCase{
		fileRepo:        fileRepo,
		progressRepo:    progressRepo,
		storageProvider: storageProvider,
		progressService: progressService,
		activeDownloads: make(map[string]*DownloadProgress),
	}
}

// DownloadOptions represents options for folder download
type DownloadOptions struct {
	IncludeSubfolders   bool `json:"includeSubfolders"`   // 하위 폴더 포함
	SkipGoogleDocs      bool `json:"skipGoogleDocs"`      // Google Docs 건너뛰기
	ConcurrentDownloads int  `json:"concurrentDownloads"` // 동시 다운로드 수
}

// DefaultDownloadOptions returns default download options
func DefaultDownloadOptions() *DownloadOptions {
	return &DownloadOptions{
		IncludeSubfolders:   true,
		SkipGoogleDocs:      true,
		ConcurrentDownloads: 3,
	}
}

// GetFolderInfo retrieves basic folder information
func (uc *FolderDownloadUseCase) GetFolderInfo(ctx context.Context, folderID string) (*entities.File, error) {
	return uc.storageProvider.GetFolder(ctx, folderID)
}

// DownloadProgress tracks download progress
type DownloadProgress struct {
	ID             string    `json:"id"`
	FolderID       string    `json:"folderId"`
	FolderName     string    `json:"folderName"`
	Status         string    `json:"status"` // preparing, downloading, completed, failed, cancelled
	TotalFiles     int       `json:"totalFiles"`
	ProcessedFiles int32     `json:"processedFiles"`
	TotalSize      int64     `json:"totalSize"`
	ProcessedSize  int64     `json:"processedSize"`
	SkippedFiles   int32     `json:"skippedFiles"`
	SkippedReasons []string  `json:"skippedReasons,omitempty"`
	CurrentFile    string    `json:"currentFile,omitempty"`
	Error          string    `json:"error,omitempty"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime,omitempty"`

	// 내부 사용
	cancelFunc context.CancelFunc
	cancelled  int32
}

// PrepareDownloadResult represents the result of download preparation
type PrepareDownloadResult struct {
	FolderID    string              `json:"folderId"`
	FolderName  string              `json:"folderName"`
	FolderPath  string              `json:"folderPath"`
	TotalFiles  int                 `json:"totalFiles"`
	TotalSize   int64               `json:"totalSize"`
	SkippedInfo *SkippedFilesInfo   `json:"skippedInfo,omitempty"`
	Files       []*DownloadFileInfo `json:"files,omitempty"` // 상세 모드에서만 포함
}

// PrepareProgress tracks preparation progress
type PrepareProgress struct {
	Status         string `json:"status"` // scanning, processing, completed, failed
	Phase          string `json:"phase"`  // folder_info, listing_files, processing_files
	FolderID       string `json:"folderId"`
	FolderName     string `json:"folderName"`
	FolderPath     string `json:"folderPath"`
	CurrentFolder  string `json:"currentFolder,omitempty"`
	ScannedFolders int    `json:"scannedFolders"`
	ScannedFiles   int    `json:"scannedFiles"`
	ProcessedFiles int    `json:"processedFiles"`
	TotalSize      int64  `json:"totalSize"`
	SkippedFiles   int    `json:"skippedFiles"`
	Message        string `json:"message,omitempty"`
	Error          string `json:"error,omitempty"`
}

// SkippedFilesInfo contains information about skipped files
type SkippedFilesInfo struct {
	GoogleDocsCount int      `json:"googleDocsCount"`
	GoogleDocsSize  int64    `json:"googleDocsSize"`
	OtherCount      int      `json:"otherCount"`
	OtherReasons    []string `json:"otherReasons,omitempty"`
}

// DownloadFileInfo represents a file to be downloaded
type DownloadFileInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"` // 폴더 내 상대 경로
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
}

// isGoogleDocsFile checks if the file is a Google Docs file (cannot be downloaded directly)
func isGoogleDocsFile(mimeType string) bool {
	googleDocsMimeTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.script",
		"application/vnd.google-apps.site",
		"application/vnd.google-apps.map",
		"application/vnd.google-apps.fusiontable",
		"application/vnd.google-apps.jam",
		"application/vnd.google-apps.shortcut",
	}

	for _, docType := range googleDocsMimeTypes {
		if mimeType == docType {
			return true
		}
	}
	return false
}

// PrepareDownload scans the folder and prepares download information
func (uc *FolderDownloadUseCase) PrepareDownload(ctx context.Context, folderID string, options *DownloadOptions) (*PrepareDownloadResult, error) {
	if options == nil {
		options = DefaultDownloadOptions()
	}

	log.Printf("📦 다운로드 준비 시작: 폴더 ID %s", folderID)

	// Get folder info
	folderInfo, err := uc.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("폴더 정보 조회 실패: %w", err)
	}

	if folderInfo.MimeType != "application/vnd.google-apps.folder" {
		return nil, fmt.Errorf("지정된 ID는 폴더가 아닙니다: %s", folderID)
	}

	// Get folder path
	folderPath, err := uc.storageProvider.GetFolderPath(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 경로 조회 실패: %v", err)
		folderPath = "/" + folderInfo.Name
	}

	// List all files in the folder
	var files []*entities.File
	if options.IncludeSubfolders {
		files, err = uc.storageProvider.ListFilesRecursive(ctx, folderID, true)
	} else {
		files, err = uc.storageProvider.ListFiles(ctx, folderID)
	}

	if err != nil {
		return nil, fmt.Errorf("폴더 내용 조회 실패: %w", err)
	}

	// Filter files and calculate sizes
	var downloadableFiles []*DownloadFileInfo
	var totalSize int64
	skippedInfo := &SkippedFilesInfo{}

	for _, file := range files {
		// Skip folders
		if file.MimeType == "application/vnd.google-apps.folder" {
			continue
		}

		// Check for Google Docs
		if options.SkipGoogleDocs && isGoogleDocsFile(file.MimeType) {
			skippedInfo.GoogleDocsCount++
			skippedInfo.GoogleDocsSize += file.Size
			continue
		}

		downloadableFiles = append(downloadableFiles, &DownloadFileInfo{
			ID:       file.ID,
			Name:     file.Name,
			Path:     file.Path,
			Size:     file.Size,
			MimeType: file.MimeType,
		})
		totalSize += file.Size
	}

	result := &PrepareDownloadResult{
		FolderID:   folderID,
		FolderName: folderInfo.Name,
		FolderPath: folderPath,
		TotalFiles: len(downloadableFiles),
		TotalSize:  totalSize,
		Files:      downloadableFiles,
	}

	if skippedInfo.GoogleDocsCount > 0 || skippedInfo.OtherCount > 0 {
		result.SkippedInfo = skippedInfo
	}

	log.Printf("✅ 다운로드 준비 완료: %d개 파일, %d MB (Google Docs 제외: %d개)",
		len(downloadableFiles), totalSize/(1024*1024), skippedInfo.GoogleDocsCount)

	return result, nil
}

// PrepareDownloadWithProgress prepares download with progress callbacks
func (uc *FolderDownloadUseCase) PrepareDownloadWithProgress(
	ctx context.Context,
	folderID string,
	options *DownloadOptions,
	progressCallback func(*PrepareProgress),
) (*PrepareDownloadResult, error) {
	if options == nil {
		options = DefaultDownloadOptions()
	}

	progress := &PrepareProgress{
		Status:   "scanning",
		Phase:    "folder_info",
		FolderID: folderID,
		Message:  "폴더 정보 조회 중...",
	}

	if progressCallback != nil {
		progressCallback(progress)
	}

	log.Printf("📦 다운로드 준비 시작 (진행 상황 표시): 폴더 ID %s", folderID)

	// Get folder info
	folderInfo, err := uc.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		progress.Status = "failed"
		progress.Error = fmt.Sprintf("폴더 정보 조회 실패: %v", err)
		if progressCallback != nil {
			progressCallback(progress)
		}
		return nil, fmt.Errorf("폴더 정보 조회 실패: %w", err)
	}

	if folderInfo.MimeType != "application/vnd.google-apps.folder" {
		progress.Status = "failed"
		progress.Error = "지정된 ID는 폴더가 아닙니다"
		if progressCallback != nil {
			progressCallback(progress)
		}
		return nil, fmt.Errorf("지정된 ID는 폴더가 아닙니다: %s", folderID)
	}

	progress.FolderName = folderInfo.Name
	progress.Message = fmt.Sprintf("폴더 '%s' 경로 조회 중...", folderInfo.Name)
	if progressCallback != nil {
		progressCallback(progress)
	}

	// Get folder path
	folderPath, err := uc.storageProvider.GetFolderPath(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 경로 조회 실패: %v", err)
		folderPath = "/" + folderInfo.Name
	}
	progress.FolderPath = folderPath

	// Start listing files with progress
	progress.Phase = "listing_files"
	progress.Message = "파일 목록 스캔 중..."
	if progressCallback != nil {
		progressCallback(progress)
	}

	// List files recursively with progress tracking
	var allFiles []*entities.File
	if options.IncludeSubfolders {
		allFiles, err = uc.listFilesRecursiveWithProgress(ctx, folderID, "", progress, progressCallback)
	} else {
		allFiles, err = uc.storageProvider.ListFiles(ctx, folderID)
		progress.ScannedFolders = 1
		progress.ScannedFiles = len(allFiles)
		if progressCallback != nil {
			progressCallback(progress)
		}
	}

	if err != nil {
		progress.Status = "failed"
		progress.Error = fmt.Sprintf("파일 목록 조회 실패: %v", err)
		if progressCallback != nil {
			progressCallback(progress)
		}
		return nil, fmt.Errorf("폴더 내용 조회 실패: %w", err)
	}

	// Process files
	progress.Phase = "processing_files"
	progress.Message = "파일 처리 중..."
	if progressCallback != nil {
		progressCallback(progress)
	}

	var downloadableFiles []*DownloadFileInfo
	var totalSize int64
	skippedInfo := &SkippedFilesInfo{}

	for i, file := range allFiles {
		// Skip folders
		if file.MimeType == "application/vnd.google-apps.folder" {
			continue
		}

		// Check for Google Docs
		if options.SkipGoogleDocs && isGoogleDocsFile(file.MimeType) {
			skippedInfo.GoogleDocsCount++
			skippedInfo.GoogleDocsSize += file.Size
			progress.SkippedFiles++
			continue
		}

		downloadableFiles = append(downloadableFiles, &DownloadFileInfo{
			ID:       file.ID,
			Name:     file.Name,
			Path:     file.Path,
			Size:     file.Size,
			MimeType: file.MimeType,
		})
		totalSize += file.Size
		progress.ProcessedFiles++
		progress.TotalSize = totalSize

		// Report progress every 100 files
		if progressCallback != nil && (i+1)%100 == 0 {
			progress.Message = fmt.Sprintf("파일 처리 중... %d/%d", i+1, len(allFiles))
			progressCallback(progress)
		}
	}

	// Final progress update
	progress.Status = "completed"
	progress.Phase = "completed"
	progress.Message = fmt.Sprintf("준비 완료: %d개 파일, %s", len(downloadableFiles), formatBytes(totalSize))
	if progressCallback != nil {
		progressCallback(progress)
	}

	result := &PrepareDownloadResult{
		FolderID:   folderID,
		FolderName: folderInfo.Name,
		FolderPath: folderPath,
		TotalFiles: len(downloadableFiles),
		TotalSize:  totalSize,
		Files:      downloadableFiles,
	}

	if skippedInfo.GoogleDocsCount > 0 || skippedInfo.OtherCount > 0 {
		result.SkippedInfo = skippedInfo
	}

	log.Printf("✅ 다운로드 준비 완료 (진행 상황 표시): %d개 파일, %d MB",
		len(downloadableFiles), totalSize/(1024*1024))

	return result, nil
}

// listFilesRecursiveWithProgress lists files recursively with progress tracking
func (uc *FolderDownloadUseCase) listFilesRecursiveWithProgress(
	ctx context.Context,
	folderID string,
	currentPath string,
	progress *PrepareProgress,
	progressCallback func(*PrepareProgress),
) ([]*entities.File, error) {
	var allFiles []*entities.File

	// List files in current folder
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}

	progress.ScannedFolders++

	// Update file paths and collect files
	var subfolders []*entities.File
	for _, file := range files {
		if currentPath != "" {
			file.Path = filepath.Join(currentPath, file.Name)
		} else {
			file.Path = file.Name
		}

		if file.MimeType == "application/vnd.google-apps.folder" {
			subfolders = append(subfolders, file)
		} else {
			allFiles = append(allFiles, file)
			progress.ScannedFiles++
		}
	}

	// Report progress
	if progressCallback != nil {
		progress.CurrentFolder = currentPath
		if currentPath == "" {
			progress.CurrentFolder = "/"
		}
		progress.Message = fmt.Sprintf("스캔 중: %s (%d개 폴더, %d개 파일)",
			progress.CurrentFolder, progress.ScannedFolders, progress.ScannedFiles)
		progressCallback(progress)
	}

	// Recursively process subfolders
	for _, folder := range subfolders {
		folderPath := folder.Path
		subFiles, err := uc.listFilesRecursiveWithProgress(ctx, folder.ID, folderPath, progress, progressCallback)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 스캔 실패 (%s): %v", folder.Name, err)
			continue
		}
		allFiles = append(allFiles, subFiles...)
	}

	return allFiles, nil
}

// formatBytes formats bytes to human readable string
func formatBytes(bytes int64) string {
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

// StreamDownloadAsZip streams folder contents as a ZIP file
func (uc *FolderDownloadUseCase) StreamDownloadAsZip(
	ctx context.Context,
	folderID string,
	options *DownloadOptions,
	writer io.Writer,
	progressCallback func(*DownloadProgress),
) error {
	if options == nil {
		options = DefaultDownloadOptions()
	}

	// Create cancellable context
	downloadCtx, cancel := context.WithCancel(ctx)

	// Initialize progress
	progress := &DownloadProgress{
		ID:         fmt.Sprintf("download_%s_%d", folderID, time.Now().UnixNano()),
		FolderID:   folderID,
		Status:     "preparing",
		StartTime:  time.Now(),
		cancelFunc: cancel,
	}

	// Register active download
	uc.activeDownloadsMu.Lock()
	uc.activeDownloads[progress.ID] = progress
	uc.activeDownloadsMu.Unlock()

	defer func() {
		uc.activeDownloadsMu.Lock()
		delete(uc.activeDownloads, progress.ID)
		uc.activeDownloadsMu.Unlock()
		cancel()
	}()

	// Prepare download
	prepareResult, err := uc.PrepareDownload(downloadCtx, folderID, options)
	if err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		if progressCallback != nil {
			progressCallback(progress)
		}
		return err
	}

	progress.FolderName = prepareResult.FolderName
	progress.TotalFiles = prepareResult.TotalFiles
	progress.TotalSize = prepareResult.TotalSize
	progress.Status = "downloading"

	if progressCallback != nil {
		progressCallback(progress)
	}

	// Create ZIP writer
	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	// Download files and add to ZIP
	concurrentLimit := options.ConcurrentDownloads
	if concurrentLimit <= 0 {
		concurrentLimit = 3
	}

	sem := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var downloadErr error

	for _, file := range prepareResult.Files {
		// Check for cancellation
		if atomic.LoadInt32(&progress.cancelled) == 1 {
			progress.Status = "cancelled"
			if progressCallback != nil {
				progressCallback(progress)
			}
			return fmt.Errorf("다운로드가 취소되었습니다")
		}

		select {
		case <-downloadCtx.Done():
			progress.Status = "cancelled"
			if progressCallback != nil {
				progressCallback(progress)
			}
			return downloadCtx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(fileInfo *DownloadFileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			// Update current file
			mu.Lock()
			progress.CurrentFile = fileInfo.Path
			mu.Unlock()

			if progressCallback != nil {
				progressCallback(progress)
			}

			// Download file content
			reader, err := uc.storageProvider.DownloadFile(downloadCtx, fileInfo.ID)
			if err != nil {
				log.Printf("⚠️ 파일 다운로드 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: %v", fileInfo.Name, err))
				mu.Unlock()
				return
			}
			defer reader.Close()

			// Create file path in ZIP
			zipPath := fileInfo.Path
			if zipPath == "" {
				zipPath = fileInfo.Name
			}
			// Ensure path uses forward slashes for ZIP
			zipPath = filepath.ToSlash(zipPath)
			// Remove leading slash if present
			zipPath = strings.TrimPrefix(zipPath, "/")

			// Create file in ZIP
			mu.Lock()
			zipFile, err := zipWriter.Create(zipPath)
			mu.Unlock()

			if err != nil {
				log.Printf("⚠️ ZIP 파일 생성 실패: %s - %v", zipPath, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: ZIP 생성 실패", fileInfo.Name))
				mu.Unlock()
				return
			}

			// Copy content to ZIP
			written, err := io.Copy(zipFile, reader)
			if err != nil {
				log.Printf("⚠️ 파일 복사 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: 복사 실패", fileInfo.Name))
				mu.Unlock()
				return
			}

			// Update progress
			atomic.AddInt32(&progress.ProcessedFiles, 1)
			atomic.AddInt64(&progress.ProcessedSize, written)

			if progressCallback != nil {
				progressCallback(progress)
			}
		}(file)
	}

	wg.Wait()

	if downloadErr != nil {
		progress.Status = "failed"
		progress.Error = downloadErr.Error()
	} else {
		progress.Status = "completed"
	}
	progress.EndTime = time.Now()

	if progressCallback != nil {
		progressCallback(progress)
	}

	log.Printf("✅ ZIP 다운로드 완료: %s (%d개 파일, %d MB)",
		prepareResult.FolderName,
		atomic.LoadInt32(&progress.ProcessedFiles),
		atomic.LoadInt64(&progress.ProcessedSize)/(1024*1024))

	return downloadErr
}

// GetDownloadProgress returns the progress of an active download
func (uc *FolderDownloadUseCase) GetDownloadProgress(downloadID string) (*DownloadProgress, bool) {
	uc.activeDownloadsMu.RLock()
	defer uc.activeDownloadsMu.RUnlock()

	progress, exists := uc.activeDownloads[downloadID]
	return progress, exists
}

// GetAllActiveDownloads returns all active downloads
func (uc *FolderDownloadUseCase) GetAllActiveDownloads() []*DownloadProgress {
	uc.activeDownloadsMu.RLock()
	defer uc.activeDownloadsMu.RUnlock()

	var downloads []*DownloadProgress
	for _, progress := range uc.activeDownloads {
		downloads = append(downloads, progress)
	}
	return downloads
}

// CancelDownload cancels an active download
func (uc *FolderDownloadUseCase) CancelDownload(downloadID string) error {
	uc.activeDownloadsMu.Lock()
	defer uc.activeDownloadsMu.Unlock()

	progress, exists := uc.activeDownloads[downloadID]
	if !exists {
		return fmt.Errorf("다운로드를 찾을 수 없습니다: %s", downloadID)
	}

	atomic.StoreInt32(&progress.cancelled, 1)
	if progress.cancelFunc != nil {
		progress.cancelFunc()
	}

	log.Printf("🛑 다운로드 취소됨: %s", downloadID)
	return nil
}

// CancelDownloadByFolderID cancels download by folder ID
func (uc *FolderDownloadUseCase) CancelDownloadByFolderID(folderID string) error {
	uc.activeDownloadsMu.Lock()
	defer uc.activeDownloadsMu.Unlock()

	for _, progress := range uc.activeDownloads {
		if progress.FolderID == folderID {
			atomic.StoreInt32(&progress.cancelled, 1)
			if progress.cancelFunc != nil {
				progress.cancelFunc()
			}
			log.Printf("🛑 다운로드 취소됨 (폴더 ID): %s", folderID)
			return nil
		}
	}

	return fmt.Errorf("해당 폴더의 다운로드를 찾을 수 없습니다: %s", folderID)
}

// StreamDownloadWithCachedFiles streams folder contents as a ZIP file using pre-cached file list
// This avoids re-scanning the folder which can be very slow for large folders
func (uc *FolderDownloadUseCase) StreamDownloadWithCachedFiles(
	ctx context.Context,
	prepareResult *PrepareDownloadResult,
	options *DownloadOptions,
	writer io.Writer,
	progressCallback func(*DownloadProgress),
) error {
	if options == nil {
		options = DefaultDownloadOptions()
	}

	if prepareResult == nil || len(prepareResult.Files) == 0 {
		return fmt.Errorf("캐시된 파일 목록이 없습니다")
	}

	// Create cancellable context
	downloadCtx, cancel := context.WithCancel(ctx)

	// Initialize progress
	progress := &DownloadProgress{
		ID:         fmt.Sprintf("download_%s_%d", prepareResult.FolderID, time.Now().UnixNano()),
		FolderID:   prepareResult.FolderID,
		FolderName: prepareResult.FolderName,
		TotalFiles: prepareResult.TotalFiles,
		TotalSize:  prepareResult.TotalSize,
		Status:     "downloading",
		StartTime:  time.Now(),
		cancelFunc: cancel,
	}

	// Register active download
	uc.activeDownloadsMu.Lock()
	uc.activeDownloads[progress.ID] = progress
	uc.activeDownloadsMu.Unlock()

	defer func() {
		uc.activeDownloadsMu.Lock()
		delete(uc.activeDownloads, progress.ID)
		uc.activeDownloadsMu.Unlock()
		cancel()
	}()

	log.Printf("📦 캐시된 파일 목록으로 ZIP 다운로드 시작: %s (%d개 파일)", prepareResult.FolderName, len(prepareResult.Files))

	if progressCallback != nil {
		progressCallback(progress)
	}

	// Create ZIP writer
	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	// Download files and add to ZIP
	concurrentLimit := options.ConcurrentDownloads
	if concurrentLimit <= 0 {
		concurrentLimit = 3
	}

	sem := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var downloadErr error

	for _, file := range prepareResult.Files {
		// Check for cancellation
		if atomic.LoadInt32(&progress.cancelled) == 1 {
			progress.Status = "cancelled"
			if progressCallback != nil {
				progressCallback(progress)
			}
			return fmt.Errorf("다운로드가 취소되었습니다")
		}

		select {
		case <-downloadCtx.Done():
			progress.Status = "cancelled"
			if progressCallback != nil {
				progressCallback(progress)
			}
			return downloadCtx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(fileInfo *DownloadFileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			// Update current file
			mu.Lock()
			progress.CurrentFile = fileInfo.Path
			mu.Unlock()

			if progressCallback != nil {
				progressCallback(progress)
			}

			// Download file content
			reader, err := uc.storageProvider.DownloadFile(downloadCtx, fileInfo.ID)
			if err != nil {
				log.Printf("⚠️ 파일 다운로드 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: %v", fileInfo.Name, err))
				mu.Unlock()
				return
			}
			defer reader.Close()

			// Create file path in ZIP
			zipPath := fileInfo.Path
			if zipPath == "" {
				zipPath = fileInfo.Name
			}
			// Ensure path uses forward slashes for ZIP
			zipPath = filepath.ToSlash(zipPath)
			// Remove leading slash if present
			zipPath = strings.TrimPrefix(zipPath, "/")

			// Create file in ZIP
			mu.Lock()
			zipFile, err := zipWriter.Create(zipPath)
			mu.Unlock()

			if err != nil {
				log.Printf("⚠️ ZIP 파일 생성 실패: %s - %v", zipPath, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: ZIP 생성 실패", fileInfo.Name))
				mu.Unlock()
				return
			}

			// Copy content to ZIP
			written, err := io.Copy(zipFile, reader)
			if err != nil {
				log.Printf("⚠️ 파일 복사 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.SkippedFiles, 1)
				mu.Lock()
				progress.SkippedReasons = append(progress.SkippedReasons, fmt.Sprintf("%s: 복사 실패", fileInfo.Name))
				mu.Unlock()
				return
			}

			// Update progress
			atomic.AddInt32(&progress.ProcessedFiles, 1)
			atomic.AddInt64(&progress.ProcessedSize, written)

			if progressCallback != nil {
				progressCallback(progress)
			}
		}(file)
	}

	wg.Wait()

	if downloadErr != nil {
		progress.Status = "failed"
		progress.Error = downloadErr.Error()
	} else {
		progress.Status = "completed"
	}
	progress.EndTime = time.Now()

	if progressCallback != nil {
		progressCallback(progress)
	}

	log.Printf("✅ ZIP 다운로드 완료 (캐시 사용): %s (%d개 파일, %d MB)",
		prepareResult.FolderName,
		atomic.LoadInt32(&progress.ProcessedFiles),
		atomic.LoadInt64(&progress.ProcessedSize)/(1024*1024))

	return downloadErr
}

// ================================================================================
// 서버 사이드 다운로드 (파일을 서버에 먼저 다운로드 후 ZIP 생성)
// ================================================================================

// ServerDownloadProgress tracks server-side download progress
type ServerDownloadProgress struct {
	ID              string    `json:"id"`
	FolderID        string    `json:"folderId"`
	FolderName      string    `json:"folderName"`
	Status          string    `json:"status"` // preparing, downloading, creating_zip, completed, failed, cancelled
	Phase           string    `json:"phase"`  // scan, download, zip
	TotalFiles      int       `json:"totalFiles"`
	DownloadedFiles int32     `json:"downloadedFiles"`
	FailedFiles     int32     `json:"failedFiles"`
	TotalSize       int64     `json:"totalSize"`
	DownloadedSize  int64     `json:"downloadedSize"`
	CurrentFile     string    `json:"currentFile,omitempty"`
	ZipPath         string    `json:"zipPath,omitempty"`
	ZipSize         int64     `json:"zipSize,omitempty"`
	Error           string    `json:"error,omitempty"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime,omitempty"`
	Message         string    `json:"message,omitempty"`

	// 내부 사용
	cancelFunc context.CancelFunc
	cancelled  int32
	tempDir    string
}

// ServerDownloadResult represents the result of server-side download
type ServerDownloadResult struct {
	ID              string    `json:"id"`
	FolderID        string    `json:"folderId"`
	FolderName      string    `json:"folderName"`
	ZipPath         string    `json:"zipPath"`
	ZipSize         int64     `json:"zipSize"`
	TotalFiles      int       `json:"totalFiles"`
	DownloadedFiles int       `json:"downloadedFiles"`
	FailedFiles     int       `json:"failedFiles"`
	TotalSize       int64     `json:"totalSize"`
	DownloadedSize  int64     `json:"downloadedSize"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	Duration        string    `json:"duration"`
}

// StartServerDownload starts downloading files to server and creating ZIP
func (uc *FolderDownloadUseCase) StartServerDownload(
	ctx context.Context,
	prepareResult *PrepareDownloadResult,
	options *DownloadOptions,
	progressCallback func(*ServerDownloadProgress),
) (*ServerDownloadResult, error) {
	if options == nil {
		options = DefaultDownloadOptions()
	}

	if prepareResult == nil || len(prepareResult.Files) == 0 {
		return nil, fmt.Errorf("다운로드할 파일이 없습니다")
	}

	// Create cancellable context
	downloadCtx, cancel := context.WithCancel(ctx)

	// Create temp directory for downloads
	tempDir, err := os.MkdirTemp("", "gdrive_download_*")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("임시 디렉토리 생성 실패: %w", err)
	}

	// Initialize progress
	progress := &ServerDownloadProgress{
		ID:         fmt.Sprintf("server_dl_%s_%d", prepareResult.FolderID, time.Now().UnixNano()),
		FolderID:   prepareResult.FolderID,
		FolderName: prepareResult.FolderName,
		Status:     "downloading",
		Phase:      "download",
		TotalFiles: len(prepareResult.Files),
		TotalSize:  prepareResult.TotalSize,
		StartTime:  time.Now(),
		Message:    "파일 다운로드 시작...",
		cancelFunc: cancel,
		tempDir:    tempDir,
	}

	// Register active download
	uc.activeDownloadsMu.Lock()
	uc.activeDownloads[progress.ID] = &DownloadProgress{
		ID:         progress.ID,
		FolderID:   progress.FolderID,
		FolderName: progress.FolderName,
		Status:     progress.Status,
		TotalFiles: progress.TotalFiles,
		TotalSize:  progress.TotalSize,
		StartTime:  progress.StartTime,
		cancelFunc: cancel,
	}
	uc.activeDownloadsMu.Unlock()

	log.Printf("📥 서버 다운로드 시작: %s (%d개 파일, %s)", prepareResult.FolderName, len(prepareResult.Files), formatBytes(prepareResult.TotalSize))
	log.Printf("📂 임시 디렉토리: %s", tempDir)

	if progressCallback != nil {
		progressCallback(progress)
	}

	// Download files to temp directory
	concurrentLimit := options.ConcurrentDownloads
	if concurrentLimit <= 0 {
		concurrentLimit = 3
	}

	sem := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 진행 상황 주기적 업데이트를 위한 타이머
	progressTicker := time.NewTicker(500 * time.Millisecond)
	defer progressTicker.Stop()

	// 별도 고루틴에서 주기적으로 진행 상황 콜백 호출
	go func() {
		for {
			select {
			case <-downloadCtx.Done():
				return
			case <-progressTicker.C:
				if progressCallback != nil {
					mu.Lock()
					progress.Message = fmt.Sprintf("다운로드 중: %d/%d 파일", atomic.LoadInt32(&progress.DownloadedFiles), progress.TotalFiles)
					mu.Unlock()
					progressCallback(progress)
				}
			}
		}
	}()

downloadLoop:
	for _, file := range prepareResult.Files {
		// Check for cancellation
		if atomic.LoadInt32(&progress.cancelled) == 1 {
			break downloadLoop
		}

		select {
		case <-downloadCtx.Done():
			atomic.StoreInt32(&progress.cancelled, 1)
			break downloadLoop
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(fileInfo *DownloadFileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			// Check cancellation again
			if atomic.LoadInt32(&progress.cancelled) == 1 {
				return
			}

			// Check context cancellation
			select {
			case <-downloadCtx.Done():
				return
			default:
			}

			// Update current file
			mu.Lock()
			progress.CurrentFile = fileInfo.Path
			progress.Message = fmt.Sprintf("다운로드 중: %s", fileInfo.Name)
			mu.Unlock()

			if progressCallback != nil {
				progressCallback(progress)
			}

			// Create file path in temp directory
			filePath := filepath.Join(tempDir, fileInfo.Path)
			if fileInfo.Path == "" {
				filePath = filepath.Join(tempDir, fileInfo.Name)
			}

			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				log.Printf("⚠️ 디렉토리 생성 실패: %s - %v", filepath.Dir(filePath), err)
				atomic.AddInt32(&progress.FailedFiles, 1)
				return
			}

			// Download file content
			reader, err := uc.storageProvider.DownloadFile(downloadCtx, fileInfo.ID)
			if err != nil {
				log.Printf("⚠️ 파일 다운로드 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.FailedFiles, 1)
				return
			}
			defer reader.Close()

			// Create local file
			localFile, err := os.Create(filePath)
			if err != nil {
				log.Printf("⚠️ 로컬 파일 생성 실패: %s - %v", filePath, err)
				atomic.AddInt32(&progress.FailedFiles, 1)
				return
			}
			defer localFile.Close()

			// Copy content
			written, err := io.Copy(localFile, reader)
			if err != nil {
				log.Printf("⚠️ 파일 쓰기 실패: %s - %v", fileInfo.Name, err)
				atomic.AddInt32(&progress.FailedFiles, 1)
				os.Remove(filePath) // Clean up partial file
				return
			}

			// Update progress
			atomic.AddInt32(&progress.DownloadedFiles, 1)
			atomic.AddInt64(&progress.DownloadedSize, written)

			downloaded := atomic.LoadInt32(&progress.DownloadedFiles)
			if downloaded%100 == 0 || downloaded == int32(progress.TotalFiles) {
				log.Printf("📊 다운로드 진행: %d/%d 파일 완료", downloaded, progress.TotalFiles)
			}

			if progressCallback != nil {
				progressCallback(progress)
			}
		}(file)
	}

	wg.Wait()

	// Check if cancelled
	if atomic.LoadInt32(&progress.cancelled) == 1 {
		progress.Status = "cancelled"
		progress.Message = "다운로드가 취소되었습니다"
		os.RemoveAll(tempDir) // Clean up
		if progressCallback != nil {
			progressCallback(progress)
		}
		return nil, fmt.Errorf("다운로드가 취소되었습니다")
	}

	downloadedFiles := atomic.LoadInt32(&progress.DownloadedFiles)
	failedFiles := atomic.LoadInt32(&progress.FailedFiles)
	downloadedSize := atomic.LoadInt64(&progress.DownloadedSize)

	log.Printf("✅ 파일 다운로드 완료: %d개 성공, %d개 실패 (%s)",
		downloadedFiles, failedFiles, formatBytes(downloadedSize))

	// Phase 2: Create ZIP file
	progress.Status = "creating_zip"
	progress.Phase = "zip"
	progress.Message = "ZIP 파일 생성 중..."
	if progressCallback != nil {
		progressCallback(progress)
	}

	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d.zip", sanitizeFileName(prepareResult.FolderName), time.Now().Unix()))

	log.Printf("📦 ZIP 파일 생성 시작: %s", zipPath)

	zipSize, err := uc.createZipFromDirectory(tempDir, zipPath)
	if err != nil {
		progress.Status = "failed"
		progress.Error = fmt.Sprintf("ZIP 생성 실패: %v", err)
		os.RemoveAll(tempDir) // Clean up
		if progressCallback != nil {
			progressCallback(progress)
		}
		return nil, fmt.Errorf("ZIP 생성 실패: %w", err)
	}

	// Clean up temp directory
	os.RemoveAll(tempDir)

	// Update final progress
	progress.Status = "completed"
	progress.Phase = "completed"
	progress.ZipPath = zipPath
	progress.ZipSize = zipSize
	progress.EndTime = time.Now()
	progress.Message = fmt.Sprintf("완료: %s (%s)", filepath.Base(zipPath), formatBytes(zipSize))

	if progressCallback != nil {
		progressCallback(progress)
	}

	// Update active download status
	uc.activeDownloadsMu.Lock()
	if dl, ok := uc.activeDownloads[progress.ID]; ok {
		dl.Status = "completed"
		dl.ProcessedFiles = downloadedFiles
		dl.ProcessedSize = downloadedSize
		dl.EndTime = progress.EndTime
	}
	uc.activeDownloadsMu.Unlock()

	duration := progress.EndTime.Sub(progress.StartTime)
	log.Printf("✅ 서버 다운로드 완료: %s (%s, 소요시간: %s)",
		filepath.Base(zipPath), formatBytes(zipSize), duration.Round(time.Second))

	return &ServerDownloadResult{
		ID:              progress.ID,
		FolderID:        prepareResult.FolderID,
		FolderName:      prepareResult.FolderName,
		ZipPath:         zipPath,
		ZipSize:         zipSize,
		TotalFiles:      len(prepareResult.Files),
		DownloadedFiles: int(downloadedFiles),
		FailedFiles:     int(failedFiles),
		TotalSize:       prepareResult.TotalSize,
		DownloadedSize:  downloadedSize,
		StartTime:       progress.StartTime,
		EndTime:         progress.EndTime,
		Duration:        duration.Round(time.Second).String(),
	}, nil
}

// createZipFromDirectory creates a ZIP file from a directory
func (uc *FolderDownloadUseCase) createZipFromDirectory(sourceDir, zipPath string) (int64, error) {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return 0, fmt.Errorf("ZIP 파일 생성 실패: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through directory and add files
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes for ZIP
		relPath = filepath.ToSlash(relPath)

		// Create file in ZIP
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return fmt.Errorf("ZIP 엔트리 생성 실패 (%s): %w", relPath, err)
		}

		// Open source file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("파일 열기 실패 (%s): %w", path, err)
		}
		defer file.Close()

		// Copy content
		_, err = io.Copy(writer, file)
		if err != nil {
			return fmt.Errorf("파일 복사 실패 (%s): %w", relPath, err)
		}

		return nil
	})

	if err != nil {
		os.Remove(zipPath)
		return 0, err
	}

	// Close ZIP writer to flush
	zipWriter.Close()

	// Get ZIP file size
	stat, err := os.Stat(zipPath)
	if err != nil {
		return 0, err
	}

	return stat.Size(), nil
}

// sanitizeFileName removes invalid characters from filename
func sanitizeFileName(name string) string {
	// Replace invalid characters with underscore
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

// GetServerDownloadProgress returns progress by ID
func (uc *FolderDownloadUseCase) GetServerDownloadProgress(downloadID string) (*DownloadProgress, bool) {
	uc.activeDownloadsMu.RLock()
	defer uc.activeDownloadsMu.RUnlock()

	progress, exists := uc.activeDownloads[downloadID]
	return progress, exists
}

// CleanupZipFile removes the ZIP file after download
func (uc *FolderDownloadUseCase) CleanupZipFile(zipPath string) error {
	if zipPath == "" {
		return nil
	}

	// Verify it's in temp directory for safety
	if !strings.HasPrefix(zipPath, os.TempDir()) {
		return fmt.Errorf("보안상의 이유로 임시 디렉토리 외부 파일은 삭제할 수 없습니다")
	}

	err := os.Remove(zipPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ZIP 파일 삭제 실패: %w", err)
	}

	log.Printf("🗑️ ZIP 파일 정리 완료: %s", zipPath)
	return nil
}
