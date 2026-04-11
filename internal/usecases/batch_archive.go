package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// BatchArchiveUseCase handles batch folder archiving operations
type BatchArchiveUseCase struct {
	storageProvider  services.StorageProvider
	folderDownloadUC *FolderDownloadUseCase

	// 활성 작업 관리
	activeJobs   map[string]*BatchArchiveJob
	activeJobsMu sync.RWMutex
}

// NewBatchArchiveUseCase creates a new batch archive use case
func NewBatchArchiveUseCase(
	storageProvider services.StorageProvider,
	folderDownloadUC *FolderDownloadUseCase,
) *BatchArchiveUseCase {
	return &BatchArchiveUseCase{
		storageProvider:  storageProvider,
		folderDownloadUC: folderDownloadUC,
		activeJobs:       make(map[string]*BatchArchiveJob),
	}
}

// FolderInput represents a folder to archive
type FolderInput struct {
	URL      string `json:"url,omitempty"`
	FolderID string `json:"folderId,omitempty"`
}

// BatchArchiveOptions represents options for batch archive
type BatchArchiveOptions struct {
	IncludeSubfolders bool   `json:"includeSubfolders"`
	SkipGoogleDocs    bool   `json:"skipGoogleDocs"`
	ZipNameFormat     string `json:"zipNameFormat"`
	DeleteMethod      string `json:"deleteMethod"`    // "trash" or "permanent"
	UploadFolderID    string `json:"uploadFolderId"`  // 지정된 폴더에 업로드 (비어있으면 상위 폴더)
	UploadFolderURL   string `json:"uploadFolderUrl"` // URL로 지정 가능
}

// DefaultBatchArchiveOptions returns default options
func DefaultBatchArchiveOptions() *BatchArchiveOptions {
	return &BatchArchiveOptions{
		IncludeSubfolders: true,
		SkipGoogleDocs:    true,
		ZipNameFormat:     "{folderName}_{date}",
		DeleteMethod:      "trash",
	}
}

// BatchArchiveJob represents a batch archive job
type BatchArchiveJob struct {
	ID               string                 `json:"jobId"`
	Status           string                 `json:"status"` // pending, running, completed, failed, cancelled
	TotalFolders     int                    `json:"totalFolders"`
	CompletedFolders int                    `json:"completedFolders"`
	CurrentIndex     int                    `json:"currentFolderIndex"`
	CurrentFolder    *FolderArchiveProgress `json:"currentFolder,omitempty"`
	Results          []FolderArchiveResult  `json:"results"`
	Errors           []FolderArchiveError   `json:"errors"`
	StartTime        time.Time              `json:"startTime"`
	EndTime          time.Time              `json:"endTime,omitempty"`

	// 내부 사용
	folders    []string // 처리할 폴더 ID 목록
	options    *BatchArchiveOptions
	cancelFunc context.CancelFunc
	cancelled  int32
	mu         sync.Mutex
}

// FolderArchiveProgress represents progress for a single folder
type FolderArchiveProgress struct {
	FolderID        string `json:"folderId"`
	FolderName      string `json:"folderName"`
	ParentFolderID  string `json:"parentFolderId"`
	Phase           string `json:"phase"` // downloading, zipping, uploading, trashing, completed, failed
	DownloadedFiles int32  `json:"downloadedFiles"`
	TotalFiles      int    `json:"totalFiles"`
	UploadProgress  int    `json:"uploadProgress"`
	Message         string `json:"message"`
}

// FolderArchiveResult represents the result for a single folder
type FolderArchiveResult struct {
	FolderID     string `json:"folderId"`
	FolderName   string `json:"folderName"`
	Status       string `json:"status"` // completed, failed
	ZipFileID    string `json:"zipFileId,omitempty"`
	ZipFileName  string `json:"zipFileName,omitempty"`
	ZipSize      int64  `json:"zipSize,omitempty"`
	OriginalSize int64  `json:"originalSize,omitempty"`
	SavedSpace   int64  `json:"savedSpace,omitempty"`
	Error        string `json:"error,omitempty"`
}

// FolderArchiveError represents an error for a single folder
type FolderArchiveError struct {
	FolderID   string `json:"folderId"`
	FolderName string `json:"folderName"`
	Phase      string `json:"phase"`
	Error      string `json:"error"`
}

// extractFolderIDFromURL extracts folder ID from Google Drive URL
func extractFolderIDFromURL(url string) (string, error) {
	// Pattern 1: https://drive.google.com/drive/folders/{folderId}
	pattern1 := regexp.MustCompile(`/folders/([a-zA-Z0-9_-]+)`)
	if matches := pattern1.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	// Pattern 2: https://drive.google.com/drive/u/0/folders/{folderId}
	pattern2 := regexp.MustCompile(`/drive/u/\d+/folders/([a-zA-Z0-9_-]+)`)
	if matches := pattern2.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("유효하지 않은 Google Drive URL입니다: %s", url)
}

// StartBatchArchive starts a batch archive job
func (uc *BatchArchiveUseCase) StartBatchArchive(
	ctx context.Context,
	folders []FolderInput,
	options *BatchArchiveOptions,
) (*BatchArchiveJob, error) {
	if len(folders) == 0 {
		return nil, fmt.Errorf("아카이브할 폴더가 없습니다")
	}

	if options == nil {
		options = DefaultBatchArchiveOptions()
	}

	// Extract folder IDs from inputs
	folderIDs := make([]string, 0, len(folders))
	for _, f := range folders {
		var folderID string
		if f.FolderID != "" {
			folderID = f.FolderID
		} else if f.URL != "" {
			id, err := extractFolderIDFromURL(f.URL)
			if err != nil {
				return nil, err
			}
			folderID = id
		} else {
			return nil, fmt.Errorf("폴더 ID 또는 URL이 필요합니다")
		}
		folderIDs = append(folderIDs, folderID)
	}

	// Create job
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &BatchArchiveJob{
		ID:           fmt.Sprintf("batch_archive_%d", time.Now().UnixNano()),
		Status:       "running",
		TotalFolders: len(folderIDs),
		StartTime:    time.Now(),
		folders:      folderIDs,
		options:      options,
		cancelFunc:   cancel,
		Results:      make([]FolderArchiveResult, 0),
		Errors:       make([]FolderArchiveError, 0),
	}

	// Register job
	uc.activeJobsMu.Lock()
	uc.activeJobs[job.ID] = job
	uc.activeJobsMu.Unlock()

	log.Printf("📦 일괄 아카이브 작업 시작: %s (%d개 폴더)", job.ID, len(folderIDs))

	// Start processing in background
	go uc.processBatchArchive(jobCtx, job)

	return job, nil
}

// processBatchArchive processes folders one by one
func (uc *BatchArchiveUseCase) processBatchArchive(ctx context.Context, job *BatchArchiveJob) {
	defer func() {
		job.EndTime = time.Now()
		if atomic.LoadInt32(&job.cancelled) == 1 {
			job.Status = "cancelled"
		} else if len(job.Errors) > 0 && job.CompletedFolders == 0 {
			job.Status = "failed"
		} else {
			job.Status = "completed"
		}
		log.Printf("📦 일괄 아카이브 작업 완료: %s (성공: %d, 실패: %d)",
			job.ID, job.CompletedFolders, len(job.Errors))
	}()

	for i, folderID := range job.folders {
		// Check for cancellation
		if atomic.LoadInt32(&job.cancelled) == 1 {
			log.Printf("🛑 일괄 아카이브 작업 취소됨: %s", job.ID)
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		job.mu.Lock()
		job.CurrentIndex = i
		job.mu.Unlock()

		log.Printf("📂 폴더 %d/%d 처리 시작: %s", i+1, job.TotalFolders, folderID)

		result, err := uc.archiveSingleFolder(ctx, job, folderID)

		job.mu.Lock()
		if err != nil {
			job.Errors = append(job.Errors, FolderArchiveError{
				FolderID:   folderID,
				FolderName: result.FolderName,
				Phase:      job.CurrentFolder.Phase,
				Error:      err.Error(),
			})
			result.Status = "failed"
			result.Error = err.Error()
			log.Printf("❌ 폴더 아카이브 실패: %s - %v", folderID, err)
		} else if result.Status == "skipped" {
			job.CompletedFolders++
			log.Printf("⏭️ 폴더 건너뜀: %s (다운로드할 파일 없음)", result.FolderName)
		} else {
			job.CompletedFolders++
			result.Status = "completed"
			log.Printf("✅ 폴더 아카이브 완료: %s → %s", result.FolderName, result.ZipFileName)
		}
		job.Results = append(job.Results, *result)
		job.mu.Unlock()
	}
}

// archiveSingleFolder archives a single folder
func (uc *BatchArchiveUseCase) archiveSingleFolder(
	ctx context.Context,
	job *BatchArchiveJob,
	folderID string,
) (*FolderArchiveResult, error) {
	result := &FolderArchiveResult{
		FolderID: folderID,
	}

	// Initialize progress
	progress := &FolderArchiveProgress{
		FolderID: folderID,
		Phase:    "downloading",
		Message:  "폴더 정보 조회 중...",
	}
	job.mu.Lock()
	job.CurrentFolder = progress
	job.mu.Unlock()

	// 1. Get folder info
	folderInfo, err := uc.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		return result, fmt.Errorf("폴더 정보 조회 실패: %w", err)
	}
	result.FolderName = folderInfo.Name
	progress.FolderName = folderInfo.Name

	// Determine upload folder ID
	var uploadFolderID string
	if job.options.UploadFolderID != "" {
		uploadFolderID = job.options.UploadFolderID
		log.Printf("📁 폴더: %s, 지정된 업로드 폴더: %s", folderInfo.Name, uploadFolderID)
	} else if job.options.UploadFolderURL != "" {
		id, err := extractFolderIDFromURL(job.options.UploadFolderURL)
		if err != nil {
			return result, fmt.Errorf("업로드 폴더 URL 파싱 실패: %w", err)
		}
		uploadFolderID = id
		log.Printf("📁 폴더: %s, URL에서 추출한 업로드 폴더: %s", folderInfo.Name, uploadFolderID)
	} else {
		// 기본값: 상위 폴더
		parents, err := uc.storageProvider.GetFileParents(ctx, folderID)
		if err != nil || len(parents) == 0 {
			return result, fmt.Errorf("상위 폴더 조회 실패: %w", err)
		}
		uploadFolderID = parents[0]
		log.Printf("📁 폴더: %s, 상위 폴더: %s", folderInfo.Name, uploadFolderID)
	}
	progress.ParentFolderID = uploadFolderID

	// 2. Prepare download
	progress.Message = "다운로드 준비 중..."
	downloadOptions := &DownloadOptions{
		IncludeSubfolders:   job.options.IncludeSubfolders,
		SkipGoogleDocs:      job.options.SkipGoogleDocs,
		ConcurrentDownloads: 3,
	}

	prepareResult, err := uc.folderDownloadUC.PrepareDownload(ctx, folderID, downloadOptions)
	if err != nil {
		return result, fmt.Errorf("다운로드 준비 실패: %w", err)
	}

	// 다운로드할 파일이 없으면 건너뛰기 (빈 폴더 또는 Google Docs만 있는 경우)
	if prepareResult.TotalFiles == 0 || len(prepareResult.Files) == 0 {
		progress.Phase = "completed"
		progress.Message = "건너뜀 (다운로드할 파일 없음)"
		result.Status = "skipped"
		log.Printf("⏭️ 폴더 '%s' 건너뜀: 다운로드할 파일이 없습니다", folderInfo.Name)
		return result, nil
	}

	result.OriginalSize = prepareResult.TotalSize
	progress.TotalFiles = prepareResult.TotalFiles

	// 3. Start server download (download files to temp dir and create ZIP)
	progress.Phase = "downloading"
	progress.Message = fmt.Sprintf("파일 다운로드 중... (0/%d)", prepareResult.TotalFiles)

	downloadResult, err := uc.folderDownloadUC.StartServerDownload(ctx, prepareResult, downloadOptions, func(p *ServerDownloadProgress) {
		job.mu.Lock()
		progress.DownloadedFiles = p.DownloadedFiles
		if p.Phase == "zip" {
			progress.Phase = "zipping"
			progress.Message = "ZIP 파일 생성 중..."
		} else {
			progress.Message = fmt.Sprintf("파일 다운로드 중... (%d/%d)", p.DownloadedFiles, p.TotalFiles)
		}
		job.mu.Unlock()
	})
	if err != nil {
		return result, fmt.Errorf("서버 다운로드 실패: %w", err)
	}

	// 4. Upload ZIP to parent folder
	progress.Phase = "uploading"
	progress.Message = "ZIP 파일 업로드 중..."

	zipFileName := uc.formatZipFileName(folderInfo.Name, job.options.ZipNameFormat)
	result.ZipFileName = zipFileName

	zipFile, err := os.Open(downloadResult.ZipPath)
	if err != nil {
		uc.folderDownloadUC.CleanupZipFile(downloadResult.ZipPath)
		return result, fmt.Errorf("ZIP 파일 열기 실패: %w", err)
	}

	uploadedFileID, err := uc.storageProvider.UploadFile(
		ctx,
		progress.ParentFolderID,
		zipFileName,
		"application/zip",
		zipFile,
	)
	zipFile.Close()

	// Clean up local ZIP file
	uc.folderDownloadUC.CleanupZipFile(downloadResult.ZipPath)

	if err != nil {
		return result, fmt.Errorf("ZIP 업로드 실패: %w", err)
	}

	result.ZipFileID = uploadedFileID
	result.ZipSize = downloadResult.ZipSize
	result.SavedSpace = result.OriginalSize - result.ZipSize

	log.Printf("📤 ZIP 업로드 완료: %s (ID: %s)", zipFileName, uploadedFileID)

	// 5. Trash or delete original folder
	progress.Phase = "trashing"
	if job.options.DeleteMethod == "permanent" {
		progress.Message = "원본 폴더 삭제 중..."
		err = uc.storageProvider.DeleteFolder(ctx, folderID)
	} else {
		progress.Message = "원본 폴더 휴지통 이동 중..."
		err = uc.storageProvider.TrashFolder(ctx, folderID)
	}

	if err != nil {
		return result, fmt.Errorf("원본 폴더 처리 실패: %w", err)
	}

	progress.Phase = "completed"
	progress.Message = "완료"

	return result, nil
}

// formatZipFileName formats the ZIP file name based on the format string
func (uc *BatchArchiveUseCase) formatZipFileName(folderName, format string) string {
	now := time.Now()

	result := format
	result = strings.ReplaceAll(result, "{folderName}", folderName)
	result = strings.ReplaceAll(result, "{date}", now.Format("20060102"))
	result = strings.ReplaceAll(result, "{timestamp}", fmt.Sprintf("%d", now.Unix()))

	// Sanitize filename
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}

	if !strings.HasSuffix(result, ".zip") {
		result += ".zip"
	}

	return result
}

// GetJobProgress returns the progress of a batch archive job
func (uc *BatchArchiveUseCase) GetJobProgress(jobID string) (*BatchArchiveJob, bool) {
	uc.activeJobsMu.RLock()
	defer uc.activeJobsMu.RUnlock()

	job, exists := uc.activeJobs[jobID]
	return job, exists
}

// CancelJob cancels a batch archive job
func (uc *BatchArchiveUseCase) CancelJob(jobID string) error {
	uc.activeJobsMu.Lock()
	defer uc.activeJobsMu.Unlock()

	job, exists := uc.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("작업을 찾을 수 없습니다: %s", jobID)
	}

	atomic.StoreInt32(&job.cancelled, 1)
	if job.cancelFunc != nil {
		job.cancelFunc()
	}

	log.Printf("🛑 일괄 아카이브 작업 취소 요청: %s", jobID)
	return nil
}

// CleanupJob removes a completed job from active jobs
func (uc *BatchArchiveUseCase) CleanupJob(jobID string) {
	uc.activeJobsMu.Lock()
	defer uc.activeJobsMu.Unlock()
	delete(uc.activeJobs, jobID)
}

// SubfolderInfo represents a subfolder's basic information
type SubfolderInfo struct {
	FolderID string `json:"folderId"`
	Name     string `json:"name"`
}

// ListSubfolders returns direct subfolders of the given parent folder
func (uc *BatchArchiveUseCase) ListSubfolders(ctx context.Context, parentFolderURL string, parentFolderID string) (string, string, []SubfolderInfo, error) {
	// Determine folder ID
	folderID := parentFolderID
	if folderID == "" && parentFolderURL != "" {
		id, err := extractFolderIDFromURL(parentFolderURL)
		if err != nil {
			return "", "", nil, err
		}
		folderID = id
	}
	if folderID == "" {
		return "", "", nil, fmt.Errorf("부모 폴더 ID 또는 URL이 필요합니다")
	}

	// Get parent folder info
	parentInfo, err := uc.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		return folderID, "", nil, fmt.Errorf("부모 폴더 정보 조회 실패: %w", err)
	}

	// List direct subfolders
	folders, err := uc.storageProvider.ListFolders(ctx, folderID)
	if err != nil {
		return folderID, parentInfo.Name, nil, fmt.Errorf("하위 폴더 목록 조회 실패: %w", err)
	}

	subfolders := make([]SubfolderInfo, 0, len(folders))
	for _, f := range folders {
		subfolders = append(subfolders, SubfolderInfo{
			FolderID: f.ID,
			Name:     f.Name,
		})
	}

	log.Printf("📂 부모 폴더 '%s'의 하위 폴더 %d개 조회 완료", parentInfo.Name, len(subfolders))
	return folderID, parentInfo.Name, subfolders, nil
}

// GetAllJobs returns all active jobs
func (uc *BatchArchiveUseCase) GetAllJobs() []*BatchArchiveJob {
	uc.activeJobsMu.RLock()
	defer uc.activeJobsMu.RUnlock()

	jobs := make([]*BatchArchiveJob, 0, len(uc.activeJobs))
	for _, job := range uc.activeJobs {
		jobs = append(jobs, job)
	}
	return jobs
}
