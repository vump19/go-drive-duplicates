package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// SharingManagerUseCase handles sharing scan and permission change operations
type SharingManagerUseCase struct {
	storageProvider services.StorageProvider

	// 활성 스캔 작업 관리
	scanJobs   map[string]*SharingScanJob
	scanJobsMu sync.RWMutex

	// 활성 변경 작업 관리
	changeJobs   map[string]*PermissionChangeJob
	changeJobsMu sync.RWMutex
}

// NewSharingManagerUseCase creates a new sharing manager use case
func NewSharingManagerUseCase(storageProvider services.StorageProvider) *SharingManagerUseCase {
	return &SharingManagerUseCase{
		storageProvider: storageProvider,
		scanJobs:        make(map[string]*SharingScanJob),
		changeJobs:      make(map[string]*PermissionChangeJob),
	}
}

// FilePermissionInfo represents a single permission entry for display
type FilePermissionInfo struct {
	PermissionID string `json:"permissionId"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	IsOwner      bool   `json:"isOwner"`
}

// FileShareInfo represents a file/folder with its sharing information
type FileShareInfo struct {
	FileID      string               `json:"fileId"`
	FileName    string               `json:"fileName"`
	MimeType    string               `json:"mimeType"`
	IsFolder    bool                 `json:"isFolder"`
	Path        string               `json:"path"`
	WebViewLink string               `json:"webViewLink"`
	Permissions []FilePermissionInfo `json:"permissions"`
	IsShared    bool                 `json:"isShared"`
}

// SharingScanJob represents a sharing scan job
type SharingScanJob struct {
	ID           string          `json:"jobId"`
	Status       string          `json:"status"` // running, completed, failed, cancelled
	FolderID     string          `json:"folderId"`
	FolderName   string          `json:"folderName"`
	FolderPath   string          `json:"folderPath"`
	TotalItems   int32           `json:"totalItems"`
	ScannedItems int32           `json:"scannedItems"`
	SharedItems  int32           `json:"sharedItems"`
	Results      []FileShareInfo `json:"results,omitempty"`
	Errors       []string        `json:"errors,omitempty"`
	StartTime    time.Time       `json:"startTime"`
	EndTime      time.Time       `json:"endTime,omitempty"`

	// internal
	cancelFunc context.CancelFunc
	cancelled  int32
	mu         sync.Mutex
}

// PermissionChangeResult represents the result of a single permission change
type PermissionChangeResult struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// PermissionChangeJob represents a permission change job
type PermissionChangeJob struct {
	ID             string                   `json:"jobId"`
	Status         string                   `json:"status"` // running, completed, failed, cancelled
	TotalItems     int                      `json:"totalItems"`
	ProcessedItems int32                    `json:"processedItems"`
	SuccessCount   int32                    `json:"successCount"`
	FailureCount   int32                    `json:"failureCount"`
	Results        []PermissionChangeResult `json:"results"`

	// internal
	cancelFunc context.CancelFunc
	cancelled  int32
	mu         sync.Mutex
}

// PermissionChangeRequest represents a request to change permissions
type PermissionChangeRequest struct {
	FileIDs     []string `json:"fileIds"`
	Action      string   `json:"action"`           // "remove" or "changeRole"
	TargetEmail string   `json:"targetEmail"`       // email to match
	NewRole     string   `json:"newRole,omitempty"` // for changeRole action
}

// StartSharingScan starts a background sharing scan job
func (uc *SharingManagerUseCase) StartSharingScan(ctx context.Context, folderID string, includeSubfolders bool) (*SharingScanJob, error) {
	if folderID == "" {
		return nil, fmt.Errorf("폴더 ID가 필요합니다")
	}

	// Get folder info
	folderInfo, err := uc.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("폴더 정보 조회 실패: %w", err)
	}

	folderPath, _ := uc.storageProvider.GetFolderPath(ctx, folderID)

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &SharingScanJob{
		ID:         fmt.Sprintf("sharing_scan_%d", time.Now().UnixNano()),
		Status:     "running",
		FolderID:   folderID,
		FolderName: folderInfo.Name,
		FolderPath: folderPath,
		StartTime:  time.Now(),
		Results:    make([]FileShareInfo, 0),
		Errors:     make([]string, 0),
		cancelFunc: cancel,
	}

	uc.scanJobsMu.Lock()
	uc.scanJobs[job.ID] = job
	uc.scanJobsMu.Unlock()

	log.Printf("🔍 공유 스캔 시작: %s (%s)", folderInfo.Name, job.ID)

	go uc.processSharingScan(jobCtx, job, includeSubfolders)

	return job, nil
}

// processSharingScan performs the actual sharing scan
func (uc *SharingManagerUseCase) processSharingScan(ctx context.Context, job *SharingScanJob, includeSubfolders bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 공유 스캔 중 패닉 복구: %v", r)
			job.mu.Lock()
			job.Status = "failed"
			job.Errors = append(job.Errors, fmt.Sprintf("패닉: %v", r))
			job.EndTime = time.Now()
			job.mu.Unlock()
			return
		}
	}()

	defer func() {
		job.mu.Lock()
		job.EndTime = time.Now()
		if atomic.LoadInt32(&job.cancelled) == 1 {
			job.Status = "cancelled"
		} else if job.Status == "running" {
			job.Status = "completed"
		}
		job.mu.Unlock()
		log.Printf("✅ 공유 스캔 완료: %s (스캔: %d, 공유: %d)", job.ID, job.ScannedItems, job.SharedItems)
	}()

	uc.scanFolderPermissions(ctx, job, job.FolderID, job.FolderName, includeSubfolders)
}

// scanFolderPermissions scans a folder and its contents for sharing info
func (uc *SharingManagerUseCase) scanFolderPermissions(ctx context.Context, job *SharingScanJob, folderID string, currentPath string, includeSubfolders bool) {
	if atomic.LoadInt32(&job.cancelled) == 1 {
		return
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// List files in this folder with retry
	var files []*entities.File
	var err error
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		files, err = uc.storageProvider.ListFiles(ctx, folderID)
		if err == nil {
			break
		}
		log.Printf("⚠️ 폴더 내용 조회 실패 (시도 %d/%d): %v", retry+1, maxRetries, err)
		time.Sleep(time.Duration(retry+1) * time.Second)
	}

	if err != nil {
		job.mu.Lock()
		job.Errors = append(job.Errors, fmt.Sprintf("폴더 조회 실패 (%s): %v", folderID, err))
		job.mu.Unlock()
		return
	}

	atomic.AddInt32(&job.TotalItems, int32(len(files)))

	// Check permissions for each file with semaphore
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, file := range files {
		if atomic.LoadInt32(&job.cancelled) == 1 {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(f *entities.File) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("🔴 파일 권한 조회 중 패닉: %v", r)
					atomic.AddInt32(&job.ScannedItems, 1)
				}
			}()

			uc.checkFilePermissions(ctx, job, f, currentPath)
		}(file)
	}
	wg.Wait()

	if !includeSubfolders {
		return
	}

	// Recurse into subfolders
	for _, file := range files {
		if atomic.LoadInt32(&job.cancelled) == 1 {
			break
		}

		if file.MimeType == "application/vnd.google-apps.folder" {
			subPath := currentPath + "/" + file.Name
			uc.scanFolderPermissions(ctx, job, file.ID, subPath, true)
		}
	}
}

// checkFilePermissions checks and records permissions for a single file
func (uc *SharingManagerUseCase) checkFilePermissions(ctx context.Context, job *SharingScanJob, file *entities.File, currentPath string) {
	atomic.AddInt32(&job.ScannedItems, 1)

	perms, err := uc.storageProvider.ListPermissions(ctx, file.ID)
	if err != nil {
		job.mu.Lock()
		job.Errors = append(job.Errors, fmt.Sprintf("권한 조회 실패 (%s): %v", file.Name, err))
		job.mu.Unlock()
		return
	}

	permInfos := make([]FilePermissionInfo, 0, len(perms))
	isShared := false

	for _, p := range perms {
		isOwner := p.Role == "owner"
		if !isOwner {
			isShared = true
		}
		permInfos = append(permInfos, FilePermissionInfo{
			PermissionID: p.ID,
			Type:         p.Type,
			Role:         p.Role,
			EmailAddress: p.EmailAddress,
			DisplayName:  p.DisplayName,
			IsOwner:      isOwner,
		})
	}

	if isShared {
		shareInfo := FileShareInfo{
			FileID:      file.ID,
			FileName:    file.Name,
			MimeType:    file.MimeType,
			IsFolder:    file.MimeType == "application/vnd.google-apps.folder",
			Path:        currentPath + "/" + file.Name,
			WebViewLink: file.WebViewLink,
			Permissions: permInfos,
			IsShared:    true,
		}

		job.mu.Lock()
		job.Results = append(job.Results, shareInfo)
		job.mu.Unlock()

		atomic.AddInt32(&job.SharedItems, 1)
	}
}

// GetScanProgress returns scan job progress (without results for polling)
func (uc *SharingManagerUseCase) GetScanProgress(jobID string) (*SharingScanJob, bool) {
	uc.scanJobsMu.RLock()
	defer uc.scanJobsMu.RUnlock()

	job, exists := uc.scanJobs[jobID]
	if !exists {
		return nil, false
	}

	return &SharingScanJob{
		ID:           job.ID,
		Status:       job.Status,
		FolderID:     job.FolderID,
		FolderName:   job.FolderName,
		FolderPath:   job.FolderPath,
		TotalItems:   atomic.LoadInt32(&job.TotalItems),
		ScannedItems: atomic.LoadInt32(&job.ScannedItems),
		SharedItems:  atomic.LoadInt32(&job.SharedItems),
		StartTime:    job.StartTime,
		EndTime:      job.EndTime,
	}, true
}

// GetScanResults returns the full scan results
func (uc *SharingManagerUseCase) GetScanResults(jobID string) (*SharingScanJob, bool) {
	uc.scanJobsMu.RLock()
	defer uc.scanJobsMu.RUnlock()

	job, exists := uc.scanJobs[jobID]
	return job, exists
}

// CancelScan cancels a running scan job
func (uc *SharingManagerUseCase) CancelScan(jobID string) error {
	uc.scanJobsMu.Lock()
	defer uc.scanJobsMu.Unlock()

	job, exists := uc.scanJobs[jobID]
	if !exists {
		return fmt.Errorf("스캔 작업을 찾을 수 없습니다: %s", jobID)
	}

	atomic.StoreInt32(&job.cancelled, 1)
	if job.cancelFunc != nil {
		job.cancelFunc()
	}

	log.Printf("🛑 공유 스캔 취소 요청: %s", jobID)
	return nil
}

// StartPermissionChange starts a background permission change job
func (uc *SharingManagerUseCase) StartPermissionChange(ctx context.Context, req *PermissionChangeRequest) (*PermissionChangeJob, error) {
	if len(req.FileIDs) == 0 {
		return nil, fmt.Errorf("변경할 파일이 없습니다")
	}
	if req.Action != "remove" && req.Action != "changeRole" {
		return nil, fmt.Errorf("유효하지 않은 액션: %s (remove 또는 changeRole 필요)", req.Action)
	}
	if req.Action == "changeRole" && req.NewRole == "" {
		return nil, fmt.Errorf("역할 변경 시 newRole이 필요합니다")
	}
	if req.TargetEmail == "" {
		return nil, fmt.Errorf("대상 이메일이 필요합니다")
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &PermissionChangeJob{
		ID:         fmt.Sprintf("perm_change_%d", time.Now().UnixNano()),
		Status:     "running",
		TotalItems: len(req.FileIDs),
		Results:    make([]PermissionChangeResult, 0),
		cancelFunc: cancel,
	}

	uc.changeJobsMu.Lock()
	uc.changeJobs[job.ID] = job
	uc.changeJobsMu.Unlock()

	log.Printf("🔄 권한 변경 시작: %s (파일 %d개, 액션: %s, 대상: %s)", job.ID, len(req.FileIDs), req.Action, req.TargetEmail)

	go uc.processPermissionChange(jobCtx, job, req)

	return job, nil
}

// processPermissionChange processes permission changes for each file
func (uc *SharingManagerUseCase) processPermissionChange(ctx context.Context, job *PermissionChangeJob, req *PermissionChangeRequest) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 권한 변경 중 패닉 복구: %v", r)
			job.mu.Lock()
			job.Status = "failed"
			job.mu.Unlock()
			return
		}
	}()

	defer func() {
		job.mu.Lock()
		if atomic.LoadInt32(&job.cancelled) == 1 {
			job.Status = "cancelled"
		} else {
			job.Status = "completed"
		}
		job.mu.Unlock()
		log.Printf("✅ 권한 변경 완료: %s (성공: %d, 실패: %d)", job.ID, job.SuccessCount, job.FailureCount)
	}()

	for _, fileID := range req.FileIDs {
		if atomic.LoadInt32(&job.cancelled) == 1 {
			break
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		result := uc.changeFilePermission(ctx, fileID, req)

		job.mu.Lock()
		job.Results = append(job.Results, result)
		job.mu.Unlock()

		atomic.AddInt32(&job.ProcessedItems, 1)
		if result.Success {
			atomic.AddInt32(&job.SuccessCount, 1)
		} else {
			atomic.AddInt32(&job.FailureCount, 1)
		}

		// Rate limit: 200ms delay between API calls
		time.Sleep(200 * time.Millisecond)
	}
}

// changeFilePermission changes permission for a single file
func (uc *SharingManagerUseCase) changeFilePermission(ctx context.Context, fileID string, req *PermissionChangeRequest) PermissionChangeResult {
	result := PermissionChangeResult{
		FileID: fileID,
	}

	fileInfo, err := uc.storageProvider.GetFile(ctx, fileID)
	if err != nil {
		result.Error = fmt.Sprintf("파일 정보 조회 실패: %v", err)
		return result
	}
	result.FileName = fileInfo.Name

	perms, err := uc.storageProvider.ListPermissions(ctx, fileID)
	if err != nil {
		result.Error = fmt.Sprintf("권한 조회 실패: %v", err)
		return result
	}

	for _, p := range perms {
		if p.Role == "owner" {
			continue
		}
		if p.EmailAddress != req.TargetEmail {
			continue
		}

		switch req.Action {
		case "remove":
			if err := uc.storageProvider.DeletePermission(ctx, fileID, p.ID); err != nil {
				result.Error = fmt.Sprintf("권한 삭제 실패: %v", err)
				return result
			}
			result.Success = true
			return result

		case "changeRole":
			if err := uc.storageProvider.UpdatePermission(ctx, fileID, p.ID, req.NewRole); err != nil {
				result.Error = fmt.Sprintf("권한 변경 실패: %v", err)
				return result
			}
			result.Success = true
			return result
		}
	}

	result.Error = fmt.Sprintf("대상 이메일(%s)에 해당하는 권한을 찾을 수 없습니다", req.TargetEmail)
	return result
}

// GetChangeProgress returns permission change job progress
func (uc *SharingManagerUseCase) GetChangeProgress(jobID string) (*PermissionChangeJob, bool) {
	uc.changeJobsMu.RLock()
	defer uc.changeJobsMu.RUnlock()

	job, exists := uc.changeJobs[jobID]
	return job, exists
}
