package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"sort"
	"strings"
	"sync"
)

// DuplicateFindingUseCase handles duplicate detection operations
type DuplicateFindingUseCase struct {
	fileRepo         repositories.FileRepository
	duplicateRepo    repositories.DuplicateRepository
	progressRepo     repositories.ProgressRepository
	hashService      services.HashService
	duplicateService services.DuplicateService
	progressService  services.ProgressService
	storageProvider  services.StorageProvider

	// Configuration
	batchSize   int
	workerCount int
	minFileSize int64
	maxResults  int
}

// NewDuplicateFindingUseCase creates a new duplicate finding use case
func NewDuplicateFindingUseCase(
	fileRepo repositories.FileRepository,
	duplicateRepo repositories.DuplicateRepository,
	progressRepo repositories.ProgressRepository,
	hashService services.HashService,
	duplicateService services.DuplicateService,
	progressService services.ProgressService,
	storageProvider services.StorageProvider,
) *DuplicateFindingUseCase {
	return &DuplicateFindingUseCase{
		fileRepo:         fileRepo,
		duplicateRepo:    duplicateRepo,
		progressRepo:     progressRepo,
		hashService:      hashService,
		duplicateService: duplicateService,
		progressService:  progressService,
		storageProvider:  storageProvider,
		batchSize:        100,
		workerCount:      5,
		minFileSize:      1024, // 1KB minimum
		maxResults:       1000,
	}
}

// FindDuplicatesRequest represents the request for finding duplicates
type FindDuplicatesRequest struct {
	CalculateHashes  bool                     `json:"calculateHashes"`
	ForceRecalculate bool                     `json:"forceRecalculate"`
	MinFileSize      int64                    `json:"minFileSize,omitempty"`
	MaxResults       int                      `json:"maxResults,omitempty"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// FindDuplicatesResponse represents the response for finding duplicates
type FindDuplicatesResponse struct {
	Progress         *entities.Progress         `json:"progress"`
	DuplicateGroups  []*entities.DuplicateGroup `json:"duplicateGroups"`
	TotalGroups      int                        `json:"totalGroups"`
	TotalFiles       int                        `json:"totalFiles"`
	TotalWastedSpace int64                      `json:"totalWastedSpace"`
	HashesCalculated int                        `json:"hashesCalculated"`
	Errors           []string                   `json:"errors,omitempty"`
}

// FindDuplicatesInFolderRequest represents the request for finding duplicates in a folder
type FindDuplicatesInFolderRequest struct {
	FolderID         string                   `json:"folderId"`
	Recursive        bool                     `json:"recursive"`
	CalculateHashes  bool                     `json:"calculateHashes"`
	MinFileSize      int64                    `json:"minFileSize,omitempty"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// CalculateHashesRequest represents the request for calculating file hashes
type CalculateHashesRequest struct {
	FileIDs          []string                 `json:"fileIds,omitempty"`
	ForceRecalculate bool                     `json:"forceRecalculate"`
	WorkerCount      int                      `json:"workerCount,omitempty"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// CalculateHashesResponse represents the response for calculating hashes
type CalculateHashesResponse struct {
	Progress         *entities.Progress `json:"progress"`
	TotalFiles       int                `json:"totalFiles"`
	ProcessedFiles   int                `json:"processedFiles"`
	SuccessfulHashes int                `json:"successfulHashes"`
	FailedHashes     int                `json:"failedHashes"`
	Errors           []string           `json:"errors,omitempty"`
}

// GetDuplicateGroupsResponse represents the paginated response for duplicate groups
type GetDuplicateGroupsResponse struct {
	Groups      []*entities.DuplicateGroup `json:"groups"`
	TotalGroups int                        `json:"totalGroups"`
	TotalPages  int                        `json:"totalPages"`
	CurrentPage int                        `json:"currentPage"`
	PageSize    int                        `json:"pageSize"`
	HasNext     bool                       `json:"hasNext"`
	HasPrev     bool                       `json:"hasPrev"`
}

// FindDuplicates finds duplicate files across the entire system
func (uc *DuplicateFindingUseCase) FindDuplicates(ctx context.Context, req *FindDuplicatesRequest) (*FindDuplicatesResponse, error) {
	log.Println("🔍 중복 파일 검색 시작")

	// Apply configuration
	if req.MinFileSize > 0 {
		uc.minFileSize = req.MinFileSize
	}
	if req.MaxResults > 0 {
		uc.maxResults = req.MaxResults
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationDuplicateSearch, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &FindDuplicatesResponse{
		Progress: progress,
		Errors:   make([]string, 0),
	}

	// Start duplicate finding in background with a new context (not tied to HTTP request)
	go uc.performDuplicateSearch(context.Background(), req, progress, response)

	return response, nil
}

// FindDuplicatesInFolder finds duplicate files within a specific folder
func (uc *DuplicateFindingUseCase) FindDuplicatesInFolder(ctx context.Context, req *FindDuplicatesInFolderRequest) (*FindDuplicatesResponse, error) {
	log.Printf("📁 폴더 내 중복 파일 검색 시작: %s", req.FolderID)

	// Apply configuration
	if req.MinFileSize > 0 {
		uc.minFileSize = req.MinFileSize
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationDuplicateSearch, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &FindDuplicatesResponse{
		Progress: progress,
		Errors:   make([]string, 0),
	}

	// Start folder duplicate finding in background with a new context (not tied to HTTP request)
	go uc.performFolderDuplicateSearch(context.Background(), req, progress, response)

	return response, nil
}

// CalculateHashes calculates hashes for files that don't have them
func (uc *DuplicateFindingUseCase) CalculateHashes(ctx context.Context, req *CalculateHashesRequest) (*CalculateHashesResponse, error) {
	log.Println("🔐 파일 해시 계산 시작")

	// Apply configuration
	if req.WorkerCount > 0 {
		uc.workerCount = req.WorkerCount
	}

	// Get files that need hash calculation
	var files []*entities.File
	var err error

	if len(req.FileIDs) > 0 {
		// Calculate hashes for specific files
		files = make([]*entities.File, 0, len(req.FileIDs))
		for _, fileID := range req.FileIDs {
			file, err := uc.fileRepo.GetByID(ctx, fileID)
			if err != nil {
				log.Printf("⚠️ 파일 조회 실패 [%s]: %v", fileID, err)
				continue
			}
			if req.ForceRecalculate || !file.IsHashCalculated() {
				files = append(files, file)
			}
		}
	} else {
		// Calculate hashes for all files without hashes
		if req.ForceRecalculate {
			files, err = uc.fileRepo.GetAll(ctx)
		} else {
			files, err = uc.fileRepo.GetWithoutHash(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("해시 계산 대상 파일 조회 실패: %w", err)
		}
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationHashCalculation, len(files))
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &CalculateHashesResponse{
		Progress:   progress,
		TotalFiles: len(files),
		Errors:     make([]string, 0),
	}

	// Start hash calculation in background with a new context (not tied to HTTP request)
	go uc.performHashCalculation(context.Background(), files, req.ProgressCallback, progress, response)

	return response, nil
}

// performDuplicateSearch performs the actual duplicate search
func (uc *DuplicateFindingUseCase) performDuplicateSearch(ctx context.Context, req *FindDuplicatesRequest, progress *entities.Progress, response *FindDuplicatesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 중복 검색 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "중복 검색 준비 중...")

	// Calculate hashes if requested
	if req.CalculateHashes {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 해시 계산 중...")
		
		// Get files that need hash calculation
		var filesNeedingHash []*entities.File
		var err error
		
		if req.ForceRecalculate {
			filesNeedingHash, err = uc.fileRepo.GetAll(ctx)
		} else {
			filesNeedingHash, err = uc.fileRepo.GetWithoutHash(ctx)
		}
		
		if err != nil {
			log.Printf("❌ 해시 계산 대상 파일 조회 실패: %v", err)
			response.Errors = append(response.Errors, fmt.Sprintf("해시 계산 대상 파일 조회 실패: %v", err))
		} else if len(filesNeedingHash) > 0 {
			log.Printf("🔐 해시 계산 시작: %d개 파일", len(filesNeedingHash))
			
			// Calculate hashes synchronously 
			hashResponse := &CalculateHashesResponse{
				TotalFiles: len(filesNeedingHash),
				Errors:     make([]string, 0),
			}
			uc.performHashCalculation(context.Background(), filesNeedingHash, req.ProgressCallback, progress, hashResponse)
			response.HashesCalculated = hashResponse.SuccessfulHashes
			response.Errors = append(response.Errors, hashResponse.Errors...)
			
			log.Printf("✅ 해시 계산 완료: %d개 성공, %d개 실패", hashResponse.SuccessfulHashes, hashResponse.FailedHashes)
		} else {
			log.Printf("ℹ️ 해시 계산이 필요한 파일이 없습니다")
		}
	}

	// Get all files with hashes
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "해시가 있는 파일 조회 중...")
	files, err := uc.fileRepo.GetByHashCalculated(ctx, true)
	if err != nil {
		log.Printf("❌ 파일 조회 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("파일 조회 실패: %v", err))
		return
	}

	// Filter by minimum file size
	filteredFiles := uc.filterFilesBySize(files, uc.minFileSize)

	log.Printf("📊 해시 계산된 파일: %d개 (최소 크기 필터 적용 후: %d개)", len(files), len(filteredFiles))

	// Update progress total
	progress.SetTotal(len(filteredFiles))

	// Group files by hash
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "해시별로 파일 그룹화 중...")
	duplicateGroups := uc.groupFilesByHash(ctx, filteredFiles, progress, req.ProgressCallback)

	// Filter groups with more than one file (actual duplicates)
	validGroups := make([]*entities.DuplicateGroup, 0)
	for _, group := range duplicateGroups {
		if group.IsValid() {
			validGroups = append(validGroups, group)
		}
	}

	// Sort groups by wasted space (largest first)
	sort.Slice(validGroups, func(i, j int) bool {
		return validGroups[i].GetWastedSpace() > validGroups[j].GetWastedSpace()
	})

	// Limit results
	if len(validGroups) > uc.maxResults {
		validGroups = validGroups[:uc.maxResults]
	}

	// Calculate statistics
	response.DuplicateGroups = validGroups
	response.TotalGroups = len(validGroups)

	totalFiles := 0
	totalWastedSpace := int64(0)
	for _, group := range validGroups {
		totalFiles += group.Count
		totalWastedSpace += group.GetWastedSpace()
	}
	response.TotalFiles = totalFiles
	response.TotalWastedSpace = totalWastedSpace

	// Save duplicate groups to database
	uc.progressService.UpdateOperation(ctx, progress.ID, len(filteredFiles), "중복 그룹 저장 중...")
	uc.saveDuplicateGroups(ctx, validGroups, response)

	// Complete the operation
	log.Printf("🏁 중복 검색 완료 처리 시작: 진행 상황 ID %d", progress.ID)
	
	// Update progress status to completed directly via repository
	progress.SetTotal(len(filteredFiles))
	progress.Complete()
	err = uc.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("⚠️ 진행 상황 완료 처리 실패: %v", err)
	} else {
		log.Printf("✅ 진행 상황 완료 처리 성공 - DB 직접 업데이트")
	}

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 중복 검색 완료: %d개 그룹, %d개 파일, %s 절약 가능",
		response.TotalGroups, response.TotalFiles, formatFileSize(response.TotalWastedSpace))
}

// performFolderDuplicateSearch performs duplicate search within a folder
func (uc *DuplicateFindingUseCase) performFolderDuplicateSearch(ctx context.Context, req *FindDuplicatesInFolderRequest, progress *entities.Progress, response *FindDuplicatesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 폴더 중복 검색 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "폴더 파일 조회 중...")

	// Get files in folder
	files, err := uc.fileRepo.GetByParent(ctx, req.FolderID)
	if err != nil {
		log.Printf("❌ 폴더 파일 조회 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("폴더 파일 조회 실패: %v", err))
		return
	}

	// Calculate hashes if requested
	if req.CalculateHashes {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 해시 계산 중...")
		filesNeedingHash := make([]*entities.File, 0)
		for _, file := range files {
			if !file.IsHashCalculated() {
				filesNeedingHash = append(filesNeedingHash, file)
			}
		}

		if len(filesNeedingHash) > 0 {
			hashResponse := &CalculateHashesResponse{Errors: make([]string, 0)}
			uc.performHashCalculation(ctx, filesNeedingHash, req.ProgressCallback, progress, hashResponse)
			response.HashesCalculated = hashResponse.SuccessfulHashes
			response.Errors = append(response.Errors, hashResponse.Errors...)
		}
	}

	// Filter files with hashes and minimum size
	hashedFiles := make([]*entities.File, 0)
	for _, file := range files {
		if file.IsHashCalculated() && file.Size >= uc.minFileSize {
			hashedFiles = append(hashedFiles, file)
		}
	}

	log.Printf("📊 폴더 내 해시 계산된 파일: %d개", len(hashedFiles))

	// Update progress total
	progress.SetTotal(len(hashedFiles))

	// Group files by hash
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "해시별로 파일 그룹화 중...")
	duplicateGroups := uc.groupFilesByHash(ctx, hashedFiles, progress, req.ProgressCallback)

	// Filter valid groups
	validGroups := make([]*entities.DuplicateGroup, 0)
	for _, group := range duplicateGroups {
		if group.IsValid() {
			validGroups = append(validGroups, group)
		}
	}

	// Sort and limit results
	sort.Slice(validGroups, func(i, j int) bool {
		return validGroups[i].GetWastedSpace() > validGroups[j].GetWastedSpace()
	})

	// Calculate statistics
	response.DuplicateGroups = validGroups
	response.TotalGroups = len(validGroups)

	totalFiles := 0
	totalWastedSpace := int64(0)
	for _, group := range validGroups {
		totalFiles += group.Count
		totalWastedSpace += group.GetWastedSpace()
	}
	response.TotalFiles = totalFiles
	response.TotalWastedSpace = totalWastedSpace

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 폴더 중복 검색 완료: %d개 그룹, %d개 파일", response.TotalGroups, response.TotalFiles)
}

// performHashCalculation performs the actual hash calculation
func (uc *DuplicateFindingUseCase) performHashCalculation(ctx context.Context, files []*entities.File, callback func(*entities.Progress), progress *entities.Progress, response *CalculateHashesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 해시 계산 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()

	// Use worker pool for parallel hash calculation
	jobs := make(chan *entities.File, len(files))
	results := make(chan struct {
		success bool
		err     error
	}, len(files))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < uc.workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				err := uc.calculateFileHash(ctx, file)
				results <- struct {
					success bool
					err     error
				}{success: err == nil, err: err}
			}
		}()
	}

	// Send jobs
	for _, file := range files {
		jobs <- file
	}
	close(jobs)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		response.ProcessedFiles++

		if result.success {
			response.SuccessfulHashes++
		} else {
			response.FailedHashes++
			if result.err != nil {
				response.Errors = append(response.Errors, result.err.Error())
			}
		}

		// Update progress
		progress.UpdateProgress(response.ProcessedFiles, fmt.Sprintf("해시 계산 중... (%d/%d)", response.ProcessedFiles, response.TotalFiles))
		uc.progressService.UpdateOperation(ctx, progress.ID, response.ProcessedFiles, progress.CurrentStep)

		// Call progress callback
		if callback != nil {
			callback(progress)
		}

		// Log progress
		if response.ProcessedFiles%100 == 0 || response.ProcessedFiles == response.TotalFiles {
			log.Printf("📈 해시 계산 진행: %d/%d (%.1f%%)", response.ProcessedFiles, response.TotalFiles, float64(response.ProcessedFiles)/float64(response.TotalFiles)*100)
		}
	}

	// Complete the operation
	log.Printf("🏁 해시 계산 완료 처리 시작: 진행 상황 ID %d", progress.ID)
	
	// Update progress status to completed directly via repository
	progress.SetTotal(len(files))
	progress.Complete()
	err := uc.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("⚠️ 진행 상황 완료 처리 실패: %v", err)
	} else {
		log.Printf("✅ 진행 상황 완료 처리 성공 - DB 직접 업데이트")
	}

	log.Printf("✅ 해시 계산 완료: %d개 성공, %d개 실패", response.SuccessfulHashes, response.FailedHashes)
}

// calculateFileHash calculates hash for a single file
func (uc *DuplicateFindingUseCase) calculateFileHash(ctx context.Context, file *entities.File) error {
	hash, err := uc.hashService.CalculateFileHash(ctx, file)
	if err != nil {
		return fmt.Errorf("파일 해시 계산 실패 [%s]: %w", file.ID, err)
	}

	// Update file with hash
	file.SetHash(hash)

	// Save to database
	return uc.fileRepo.UpdateHash(ctx, file.ID, hash)
}

// groupFilesByHash groups files by their hash values
func (uc *DuplicateFindingUseCase) groupFilesByHash(ctx context.Context, files []*entities.File, progress *entities.Progress, callback func(*entities.Progress)) []*entities.DuplicateGroup {
	hashGroups := make(map[string][]*entities.File)

	// Group files by hash
	for i, file := range files {
		if file.Hash != "" {
			hashGroups[file.Hash] = append(hashGroups[file.Hash], file)
		}

		// Update progress
		if i%100 == 0 || i == len(files)-1 {
			progress.UpdateProgress(i+1, fmt.Sprintf("파일 그룹화 중... (%d/%d)", i+1, len(files)))
			uc.progressService.UpdateOperation(ctx, progress.ID, i+1, progress.CurrentStep)

			if callback != nil {
				callback(progress)
			}
		}
	}

	// Convert to duplicate groups
	duplicateGroups := make([]*entities.DuplicateGroup, 0, len(hashGroups))
	for hash, groupFiles := range hashGroups {
		group := entities.NewDuplicateGroup(hash)
		for _, file := range groupFiles {
			group.AddFile(file)
		}
		duplicateGroups = append(duplicateGroups, group)
	}

	return duplicateGroups
}

// filterFilesBySize filters files by minimum size
func (uc *DuplicateFindingUseCase) filterFilesBySize(files []*entities.File, minSize int64) []*entities.File {
	filtered := make([]*entities.File, 0, len(files))
	for _, file := range files {
		if file.Size >= minSize {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// saveDuplicateGroups saves duplicate groups to the database
func (uc *DuplicateFindingUseCase) saveDuplicateGroups(ctx context.Context, groups []*entities.DuplicateGroup, response *FindDuplicatesResponse) {
	// Clean up existing groups
	count, err := uc.duplicateRepo.CleanupEmptyGroups(ctx)
	if err != nil {
		log.Printf("⚠️ 기존 중복 그룹 정리 실패: %v", err)
	} else if count > 0 {
		log.Printf("🧹 %d개 빈 중복 그룹 정리 완료", count)
	}

	// Save new groups
	for _, group := range groups {
		err := uc.duplicateRepo.Save(ctx, group)
		if err != nil {
			log.Printf("⚠️ 중복 그룹 저장 실패 [%s]: %v", group.Hash, err)
			response.Errors = append(response.Errors, fmt.Sprintf("중복 그룹 저장 실패: %v", err))
		}
	}
}

// GetDuplicateProgress returns the current duplicate finding progress
func (uc *DuplicateFindingUseCase) GetDuplicateProgress(ctx context.Context) (*entities.Progress, error) {
	activeProgress, err := uc.progressService.GetActiveOperations(ctx)
	if err != nil {
		return nil, err
	}

	for _, progress := range activeProgress {
		if progress.OperationType == entities.OperationDuplicateSearch {
			return progress, nil
		}
	}

	return nil, fmt.Errorf("활성 중복 검색 작업을 찾을 수 없습니다")
}

// SetConfiguration sets the use case configuration
func (uc *DuplicateFindingUseCase) SetConfiguration(batchSize, workerCount int, minFileSize int64, maxResults int) {
	if batchSize > 0 {
		uc.batchSize = batchSize
	}
	if workerCount > 0 {
		uc.workerCount = workerCount
	}
	if minFileSize > 0 {
		uc.minFileSize = minFileSize
	}
	if maxResults > 0 {
		uc.maxResults = maxResults
	}
}

// formatFileSize formats file size in human readable format
func formatFileSize(bytes int64) string {
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

// GetDuplicateGroups returns all duplicate groups
func (uc *DuplicateFindingUseCase) GetDuplicateGroups(ctx context.Context) ([]*entities.DuplicateGroup, error) {
	groups, err := uc.duplicateRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	return groups, nil
}

// GetDuplicateGroupsPaginated returns paginated duplicate groups
func (uc *DuplicateFindingUseCase) GetDuplicateGroupsPaginated(ctx context.Context, page, pageSize int) (*GetDuplicateGroupsResponse, error) {
	// Calculate offset
	offset := (page - 1) * pageSize

	// Get total count
	totalCount, err := uc.duplicateRepo.CountValid(ctx)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 수 조회 실패: %w", err)
	}

	// Get paginated groups
	groups, err := uc.duplicateRepo.GetValidGroupsPaginated(ctx, offset, pageSize)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	// Calculate pagination info
	totalPages := (totalCount + pageSize - 1) / pageSize // Ceiling division
	if totalPages == 0 {
		totalPages = 1
	}

	response := &GetDuplicateGroupsResponse{
		Groups:      groups,
		TotalGroups: totalCount,
		TotalPages:  totalPages,
		CurrentPage: page,
		PageSize:    pageSize,
		HasNext:     page < totalPages,
		HasPrev:     page > 1,
	}

	return response, nil
}

// GetDuplicateGroup returns a specific duplicate group by ID
func (uc *DuplicateFindingUseCase) GetDuplicateGroup(ctx context.Context, groupID int) (*entities.DuplicateGroup, error) {
	group, err := uc.duplicateRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	if group == nil {
		return nil, fmt.Errorf("중복 그룹을 찾을 수 없습니다: %d", groupID)
	}

	return group, nil
}

// DeleteDuplicateGroup deletes a duplicate group and its associated files
func (uc *DuplicateFindingUseCase) DeleteDuplicateGroup(ctx context.Context, groupID int) error {
	log.Printf("🗑️ 중복 그룹 삭제 시작: 그룹 ID %d", groupID)

	// First, check if the group exists
	group, err := uc.duplicateRepo.GetByID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	if group == nil {
		return fmt.Errorf("중복 그룹을 찾을 수 없습니다: %d", groupID)
	}

	log.Printf("📋 삭제할 그룹 정보: 해시=%s, 파일 수=%d", group.Hash, group.Count)

	// Delete the duplicate group (this should cascade delete the file associations)
	err = uc.duplicateRepo.Delete(ctx, groupID)
	if err != nil {
		return fmt.Errorf("중복 그룹 삭제 실패: %w", err)
	}

	log.Printf("✅ 중복 그룹 삭제 완료: 그룹 ID %d", groupID)
	return nil
}

// GetFilePath returns the folder path for a specific file
func (uc *DuplicateFindingUseCase) GetFilePath(ctx context.Context, fileID string) (*FilePathResponse, error) {
	// Get file details from database first
	dbFile, err := uc.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("파일 조회 실패: %w", err)
	}

	if dbFile == nil {
		return nil, fmt.Errorf("파일을 찾을 수 없습니다: %s", fileID)
	}

	log.Printf("🔍 파일 경로 조회 시작: 파일 ID=%s, 파일명=%s", dbFile.ID, dbFile.Name)
	log.Printf("📁 데이터베이스 부모 폴더 정보: %v", dbFile.Parents)

	// Get fresh file information from Google Drive API to ensure we have current parent info
	log.Printf("🔄 Google Drive에서 실시간 파일 정보 조회 중...")
	freshFile, err := uc.storageProvider.GetFile(ctx, fileID)
	if err != nil {
		log.Printf("⚠️ Google Drive 파일 정보 조회 실패: %v", err)
		log.Printf("🔄 데이터베이스 정보 사용")
		freshFile = dbFile
	} else {
		log.Printf("✅ Google Drive 파일 정보 조회 성공")
		log.Printf("📁 Google Drive 부모 폴더 정보: %v", freshFile.Parents)
	}

	// Get the actual folder path from Google Drive
	var actualPath string
	var parentID string
	
	if len(freshFile.Parents) > 0 {
		parentID = freshFile.Parents[0] // 첫 번째 부모 폴더 ID 사용
		log.Printf("🔍 부모 폴더 경로 조회 중: 폴더 ID=%s", parentID)
		
		// Get the actual folder path using Google Drive API
		folderPath, err := uc.storageProvider.GetFolderPath(ctx, parentID)
		if err != nil {
			log.Printf("⚠️ 폴더 경로 조회 실패: %v", err)
			log.Printf("🔄 저장된 경로로 대체: %s", freshFile.Path)
			if freshFile.Path != "" {
				actualPath = freshFile.Path
				if !strings.HasSuffix(actualPath, freshFile.Name) {
					actualPath = actualPath + "/" + freshFile.Name
				}
			} else {
				actualPath = "/" + freshFile.Name
			}
		} else {
			log.Printf("✅ 폴더 경로 조회 성공: %s", folderPath)
			actualPath = folderPath + "/" + freshFile.Name
			log.Printf("🎯 최종 경로: %s", actualPath)
		}
	} else {
		log.Printf("📍 루트 디렉토리의 파일")
		actualPath = "/" + freshFile.Name // Root directory
	}
	
	response := &FilePathResponse{
		FileID:     freshFile.ID,
		Name:       freshFile.Name,
		Path:       actualPath,
		ParentID:   parentID,
		WebViewURL: freshFile.WebViewLink,
	}

	return response, nil
}

// FilePathResponse represents the response for file path information
type FilePathResponse struct {
	FileID     string `json:"fileId"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentID   string `json:"parentId,omitempty"`
	WebViewURL string `json:"webViewUrl,omitempty"`
}
