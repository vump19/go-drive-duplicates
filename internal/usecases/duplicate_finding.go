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
	"time"
)

// DuplicateFindingUseCase handles duplicate detection operations
type DuplicateFindingUseCase struct {
	fileRepo         repositories.FileRepository
	duplicateRepo    repositories.DuplicateRepository
	progressRepo     repositories.ProgressRepository
	verifiedRepo     repositories.VerifiedDuplicateRepository
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
	verifiedRepo repositories.VerifiedDuplicateRepository,
	hashService services.HashService,
	duplicateService services.DuplicateService,
	progressService services.ProgressService,
	storageProvider services.StorageProvider,
) *DuplicateFindingUseCase {
	return &DuplicateFindingUseCase{
		fileRepo:         fileRepo,
		duplicateRepo:    duplicateRepo,
		progressRepo:     progressRepo,
		verifiedRepo:     verifiedRepo,
		hashService:      hashService,
		duplicateService: duplicateService,
		progressService:  progressService,
		storageProvider:  storageProvider,
		batchSize:        100,
		workerCount:      5,
		minFileSize:      1024, // 1KB minimum
		maxResults:       0,    // 0 = 제한 없음 (모든 중복 그룹 저장)
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

	// Apply configuration - always use requested minFileSize (including 0)
	uc.minFileSize = req.MinFileSize
	if req.MaxResults > 0 {
		uc.maxResults = req.MaxResults
	}

	// 🔄 체크포인트: 이전 미완료 해시 계산 작업 확인 (calculateHashes가 true일 때만)
	var progress *entities.Progress
	var err error

	if req.CalculateHashes {
		// 재개 가능한 중복 검색 작업 찾기 (paused 또는 completed이지만 resumable인 작업)
		// 가장 최근의 작업 (currentBatch가 가장 큰 것) 선택
		// Get all duplicate_search operations
		allOperations, err := uc.progressRepo.GetByOperationType(ctx, entities.OperationDuplicateSearch)
		if err == nil && len(allOperations) > 0 {
			var latestResumable *entities.Progress
			latestBatch := 0

			for _, op := range allOperations {
				// resumable 메타데이터 확인
				if resumable, ok := op.GetMetadata("resumable"); ok {
					if isResumable, ok := resumable.(bool); ok && isResumable {
						// Check if status is paused OR (completed but resumable)
						if op.Status == entities.StatusPaused ||
							(op.Status == entities.StatusCompleted && isResumable) {

							// 현재 배치 번호 확인
							currentBatch := 0
							if batch, ok := op.GetMetadata("currentBatch"); ok {
								if batchNum, ok := batch.(float64); ok {
									currentBatch = int(batchNum)
								}
							}

							// 가장 최근의 작업 (배치 번호가 가장 큰 것) 선택
							if latestResumable == nil || currentBatch > latestBatch {
								latestResumable = op
								latestBatch = currentBatch
							}
						}
					}
				}
			}

			if latestResumable != nil {
				progress = latestResumable
				log.Printf("🔄 재개 가능한 해시 계산 작업 발견 (ID: %d, 배치: %d, 상태: %s), 이어서 진행합니다",
					progress.ID, latestBatch, progress.Status)
			}
		}
	}

	// 미완료 작업이 없으면 새로 생성
	if progress == nil {
		progress, err = uc.progressService.StartOperation(ctx, entities.OperationDuplicateSearch, 0)
		if err != nil {
			return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
		}
	} else {
		// 재개: 상태를 강제로 running으로 변경 (completed일 수도 있음)
		progress.Status = entities.StatusRunning
		progress.LastUpdated = time.Now()
		err := uc.progressRepo.Update(ctx, progress)
		if err != nil {
			log.Printf("⚠️ 진행 상황 재개 실패: %v", err)
		} else {
			log.Printf("✅ 진행 상황 재개 성공: 상태를 running으로 변경")
		}
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

	// Apply configuration - always use requested minFileSize (including 0)
	uc.minFileSize = req.MinFileSize

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
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 해시 계산 준비 중...")

		// 🔄 체크포인트: 기존 미완료 작업이 있는지 확인
		totalSuccessful := 0
		totalFailed := 0
		totalDeletedFiles := 0 // 삭제된 파일 카운터
		startBatch := 1

		// 이전 미완료 작업 확인
		if resumableBatch, ok := progress.GetMetadata("currentBatch"); ok {
			if batchNum, ok := resumableBatch.(float64); ok {
				startBatch = int(batchNum) + 1 // 다음 배치부터 시작

				if successCount, ok := progress.GetMetadata("totalSuccessful"); ok {
					if count, ok := successCount.(float64); ok {
						totalSuccessful = int(count)
					}
				}

				if failCount, ok := progress.GetMetadata("totalFailed"); ok {
					if count, ok := failCount.(float64); ok {
						totalFailed = int(count)
					}
				}

				if deletedCount, ok := progress.GetMetadata("totalDeletedFiles"); ok {
					if count, ok := deletedCount.(float64); ok {
						totalDeletedFiles = int(count)
					}
				}

				log.Printf("🔄 이전 작업 재개: 배치 %d부터 계속 (기존 성공: %d, 실패: %d, 삭제됨: %d)",
					startBatch, totalSuccessful, totalFailed, totalDeletedFiles)
			}
		}

		// 배치 처리: 해시가 없는 파일이 남아있는 동안 계속 처리
		batchNumber := startBatch - 1
		const maxBatches = 1000 // 안전장치: 최대 1000번 반복 (100만 파일)

		for batchNumber < maxBatches {
			batchNumber++

			// Get next batch of files that need hash calculation
			var filesNeedingHash []*entities.File
			var err error

			if req.ForceRecalculate && batchNumber == 1 {
				// ForceRecalculate는 첫 배치에서만 모든 파일 조회
				filesNeedingHash, err = uc.fileRepo.GetAll(ctx)
			} else {
				// 해시가 없는 파일 1000개씩 가져오기
				filesNeedingHash, err = uc.fileRepo.GetWithoutHash(ctx)
			}

			if err != nil {
				log.Printf("❌ 해시 계산 대상 파일 조회 실패 (배치 %d): %v", batchNumber, err)
				response.Errors = append(response.Errors, fmt.Sprintf("배치 %d 조회 실패: %v", batchNumber, err))

				// 💾 체크포인트 저장 (에러 발생 시에도 진행 상황 저장)
				progress.SetMetadata("currentBatch", batchNumber)
				progress.SetMetadata("totalSuccessful", totalSuccessful)
				progress.SetMetadata("totalFailed", totalFailed)
				progress.SetMetadata("resumable", true)
				progress.Pause() // 일시 정지 상태로 변경
				uc.progressRepo.Update(ctx, progress)

				break
			}

			// 🧹 삭제된 것으로 보이는 파일 미리 제거 (경로 비어있는 파일)
			validFiles := make([]*entities.File, 0, len(filesNeedingHash))
			deletedCount := 0
			for _, file := range filesNeedingHash {
				// 경로가 비어있고 부모 정보가 없으면 삭제된 파일로 간주
				if file.Path == "" && (file.Parents == nil || len(file.Parents) == 0) {
					uc.fileRepo.Delete(ctx, file.ID)
					deletedCount++
				} else {
					validFiles = append(validFiles, file)
				}
			}

			if deletedCount > 0 {
				log.Printf("🗑️ 배치 %d: 사전 정리로 %d개 삭제된 파일 제거됨", batchNumber, deletedCount)
				totalDeletedFiles += deletedCount
			}

			filesNeedingHash = validFiles

			// 더 이상 처리할 파일이 없으면 종료
			if len(filesNeedingHash) == 0 {
				log.Printf("✅ 모든 해시 계산 완료: 총 %d개 배치 처리 (삭제됨: %d개)", batchNumber-1, totalDeletedFiles)
				break
			}

			log.Printf("🔐 해시 계산 배치 %d 시작: %d개 파일", batchNumber, len(filesNeedingHash))

			// 🔄 배치 시작 시 즉시 메타데이터 업데이트 (프론트엔드에 현재 배치 표시)
			progress.SetMetadata("currentBatch", batchNumber)
			progress.SetMetadata("totalSuccessful", totalSuccessful)
			progress.SetMetadata("totalFailed", totalFailed)
			progress.SetMetadata("totalDeletedFiles", totalDeletedFiles)
			uc.progressRepo.Update(ctx, progress)

			// Calculate hashes for this batch
			hashResponse := &CalculateHashesResponse{
				TotalFiles: len(filesNeedingHash),
				Errors:     make([]string, 0),
			}
			uc.performHashCalculation(context.Background(), filesNeedingHash, req.ProgressCallback, progress, hashResponse)

			totalSuccessful += hashResponse.SuccessfulHashes
			totalFailed += hashResponse.FailedHashes
			response.Errors = append(response.Errors, hashResponse.Errors...)

			log.Printf("✅ 배치 %d 완료: %d개 성공, %d개 실패 (누적: 성공 %d, 실패 %d, 삭제됨 %d)",
				batchNumber, hashResponse.SuccessfulHashes, hashResponse.FailedHashes,
				totalSuccessful, totalFailed, totalDeletedFiles)

			// 💾 체크포인트 저장 (매 배치마다)
			progress.SetMetadata("currentBatch", batchNumber)
			progress.SetMetadata("totalSuccessful", totalSuccessful)
			progress.SetMetadata("totalFailed", totalFailed)
			progress.SetMetadata("totalDeletedFiles", totalDeletedFiles)
			progress.SetMetadata("resumable", true)
			progress.SetMetadata("lastCheckpoint", time.Now())
			uc.progressRepo.Update(ctx, progress)

			// 진행 상황 업데이트
			uc.progressService.UpdateOperation(ctx, progress.ID, 0,
				fmt.Sprintf("해시 계산 중... 배치 %d (누적: %d개 성공)", batchNumber, totalSuccessful))

			// 다음 배치를 위해 계속 루프 (파일이 0개가 될 때까지)
		}

		response.HashesCalculated = totalSuccessful

		// 완료 시 체크포인트 정보 제거
		progress.SetMetadata("resumable", false)
		progress.SetMetadata("completedBatches", batchNumber)
		progress.SetMetadata("totalDeletedFiles", totalDeletedFiles)
		uc.progressRepo.Update(ctx, progress)

		if totalSuccessful > 0 || totalFailed > 0 || totalDeletedFiles > 0 {
			log.Printf("🎯 전체 해시 계산 완료: %d개 성공, %d개 실패, %d개 삭제됨 (%d개 배치)",
				totalSuccessful, totalFailed, totalDeletedFiles, batchNumber)
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

	// Limit results (0 = 제한 없음)
	if uc.maxResults > 0 && len(validGroups) > uc.maxResults {
		validGroups = validGroups[:uc.maxResults]
	}

	// 💾 중복 그룹을 데이터베이스에 저장
	log.Printf("💾 %d개 중복 그룹을 데이터베이스에 저장 중...", len(validGroups))
	savedGroups := 0
	for _, group := range validGroups {
		// Save duplicate group to DB (creates new or updates existing)
		err := uc.duplicateRepo.Save(ctx, group)
		if err != nil {
			log.Printf("⚠️ 중복 그룹 저장 실패 (hash: %s): %v", group.Hash, err)
		} else {
			savedGroups++
		}
	}
	log.Printf("✅ %d개 중복 그룹 저장 완료", savedGroups)

	// 🔔 진행 상황 메타데이터에 발견된 중복 그룹 수 업데이트 (실시간 표시용)
	progress.SetMetadata("foundDuplicateGroups", savedGroups)
	progress.SetMetadata("lastGroupUpdate", time.Now())
	uc.progressRepo.Update(ctx, progress)
	log.Printf("📊 진행 상황 업데이트: 발견된 중복 그룹 %d개", savedGroups)

	// Calculate statistics
	// Note: Invalid group filtering is now done only when listing groups (GetDuplicateGroupsPaginated)
	// to avoid performance issues during hash calculation
	unverifiedGroups := validGroups

	log.Printf("🔍 %d개 중복 그룹 생성 완료 (필터링은 목록 조회 시 수행)", len(unverifiedGroups))

	// Update response with unverified groups
	response.DuplicateGroups = unverifiedGroups
	response.TotalGroups = len(unverifiedGroups)

	// Recalculate totals for unverified groups only
	totalFiles := 0
	totalWastedSpace := int64(0)
	for _, group := range unverifiedGroups {
		totalFiles += group.Count
		totalWastedSpace += group.GetWastedSpace()
	}
	response.TotalFiles = totalFiles
	response.TotalWastedSpace = totalWastedSpace

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
// Note: Does NOT complete the progress - caller is responsible for completing
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
	errorSampleCount := 0
	const maxErrorSamples = 10 // Log first 10 errors for debugging

	for result := range results {
		response.ProcessedFiles++

		if result.success {
			response.SuccessfulHashes++
		} else {
			response.FailedHashes++
			if result.err != nil {
				errorMsg := result.err.Error()
				response.Errors = append(response.Errors, errorMsg)

				// Log first few errors for debugging
				if errorSampleCount < maxErrorSamples {
					log.Printf("❌ 해시 계산 실패 [%d/%d]: %s", response.FailedHashes, response.TotalFiles, errorMsg)
					errorSampleCount++
				}
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
			log.Printf("📈 해시 계산 진행: %d/%d (%.1f%%) - 성공: %d, 실패: %d",
				response.ProcessedFiles, response.TotalFiles,
				float64(response.ProcessedFiles)/float64(response.TotalFiles)*100,
				response.SuccessfulHashes, response.FailedHashes)
		}
	}

	// Note: Do NOT complete the progress here - caller is responsible
	log.Printf("✅ 해시 계산 배치 완료: %d개 성공, %d개 실패", response.SuccessfulHashes, response.FailedHashes)
}

// calculateFileHash calculates hash for a single file
func (uc *DuplicateFindingUseCase) calculateFileHash(ctx context.Context, file *entities.File) error {
	hash, err := uc.hashService.CalculateFileHash(ctx, file)
	if err != nil {
		// 404 에러 체크: 파일이 Google Drive에서 삭제됨
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "notFound") {
			log.Printf("🗑️ 파일이 Google Drive에서 삭제됨, DB에서 제거: %s (%s)", file.Name, file.ID)
			// 데이터베이스에서 파일 삭제
			uc.fileRepo.Delete(ctx, file.ID)
			return fmt.Errorf("파일이 존재하지 않음 (삭제됨): %s", file.ID)
		}
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

// GetDuplicateProgress returns the current duplicate finding or validation progress
func (uc *DuplicateFindingUseCase) GetDuplicateProgress(ctx context.Context) (*entities.Progress, error) {
	activeProgress, err := uc.progressService.GetActiveOperations(ctx)
	if err != nil {
		return nil, err
	}

	// 중복 검색 또는 중복 그룹 검증 작업 찾기
	for _, progress := range activeProgress {
		if progress.OperationType == entities.OperationDuplicateSearch ||
			progress.OperationType == entities.OperationValidateDuplicates {
			return progress, nil
		}
	}

	return nil, nil
}

// GetResumableHashCalculation returns the resumable hash calculation task if exists
func (uc *DuplicateFindingUseCase) GetResumableHashCalculation(ctx context.Context) (*entities.Progress, error) {
	// 일시 정지 상태의 중복 검색 작업 찾기
	pausedOperations, err := uc.progressRepo.GetByStatus(ctx, entities.StatusPaused)
	if err != nil {
		return nil, err
	}

	for _, op := range pausedOperations {
		if op.OperationType == entities.OperationDuplicateSearch {
			// resumable 메타데이터 확인
			if resumable, ok := op.GetMetadata("resumable"); ok {
				if isResumable, ok := resumable.(bool); ok && isResumable {
					return op, nil
				}
			}
		}
	}

	return nil, nil // 재개 가능한 작업 없음 (에러 아님)
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

// filterInvalidGroups filters out groups with deleted or non-existent files
func (uc *DuplicateFindingUseCase) filterInvalidGroups(ctx context.Context, groups []*entities.DuplicateGroup) ([]*entities.DuplicateGroup, error) {
	if len(groups) == 0 {
		return groups, nil
	}

	log.Printf("🧹 무효한 그룹 필터링 시작: %d개 그룹", len(groups))

	var validGroups []*entities.DuplicateGroup
	deletedGroups := 0

	for _, group := range groups {
		// Get all files in this group
		files, err := uc.fileRepo.GetByHash(ctx, group.Hash)
		if err != nil {
			log.Printf("⚠️ 그룹 %d 파일 조회 실패: %v", group.ID, err)
			continue
		}

		// 빠른 검증: 경로가 비어있는 파일 개수 확인
		validFileCount := 0
		deletedFileIDs := []string{}

		for _, file := range files {
			// 경로가 비어있거나 부모 정보가 없으면 삭제된 파일로 간주
			if file.Path == "" && (file.Parents == nil || len(file.Parents) == 0) {
				deletedFileIDs = append(deletedFileIDs, file.ID)
			} else {
				validFileCount++
			}
		}

		// 삭제된 파일을 데이터베이스에서 제거
		if len(deletedFileIDs) > 0 {
			log.Printf("🗑️ 그룹 %d: %d개 삭제된 파일 정리 중...", group.ID, len(deletedFileIDs))
			for _, fileID := range deletedFileIDs {
				uc.fileRepo.Delete(ctx, fileID)
			}
		}

		// 유효한 파일이 2개 미만이면 그룹 삭제
		if validFileCount < 2 {
			log.Printf("🗑️ 그룹 %d: 유효한 파일 %d개 (삭제됨 %d개) - 그룹 삭제",
				group.ID, validFileCount, len(deletedFileIDs))
			uc.duplicateRepo.Delete(ctx, group.ID)
			deletedGroups++
		} else {
			// 그룹 정보 업데이트 (파일 개수 조정)
			if len(deletedFileIDs) > 0 {
				group.Count = validFileCount
				if validFileCount > 0 && len(files) > 0 {
					group.TotalSize = int64(validFileCount) * files[0].Size
				}
				uc.duplicateRepo.Update(ctx, group)
				log.Printf("✅ 그룹 %d: 업데이트됨 (유효: %d, 삭제됨: %d)",
					group.ID, validFileCount, len(deletedFileIDs))
			}
			validGroups = append(validGroups, group)
		}
	}

	log.Printf("✅ 무효한 그룹 필터링 완료: %d개 그룹 삭제, %d개 유지", deletedGroups, len(validGroups))
	return validGroups, nil
}

// validateGroupsWithGoogleDrive validates groups by checking if their files actually exist in Google Drive
func (uc *DuplicateFindingUseCase) validateGroupsWithGoogleDrive(ctx context.Context, groups []*entities.DuplicateGroup) ([]*entities.DuplicateGroup, error) {
	if len(groups) == 0 {
		return groups, nil
	}

	log.Printf("🔍 Google Drive로 %d개 그룹 실제 검증 시작", len(groups))

	var validGroups []*entities.DuplicateGroup
	deletedGroups := 0
	deletedFiles := 0

	for _, group := range groups {
		// Get all files in this group
		files, err := uc.fileRepo.GetByHash(ctx, group.Hash)
		if err != nil {
			log.Printf("⚠️ 그룹 %d 파일 조회 실패: %v", group.ID, err)
			continue
		}

		validFileCount := 0
		deletedFileIDs := []string{}

		// Check each file with Google Drive API
		for _, file := range files {
			// 빠른 체크: path와 parents가 모두 없으면 확실히 삭제된 파일
			if file.Path == "" && (file.Parents == nil || len(file.Parents) == 0) {
				deletedFileIDs = append(deletedFileIDs, file.ID)
				continue
			}

			// Google Drive API로 실제 존재 여부 확인
			_, err := uc.storageProvider.GetFile(ctx, file.ID)
			if err != nil {
				// 404 오류 = 파일이 Google Drive에서 삭제됨
				if strings.Contains(err.Error(), "404") ||
					strings.Contains(err.Error(), "not found") ||
					strings.Contains(err.Error(), "notFound") {
					log.Printf("🗑️ Google Drive에서 삭제된 파일 발견: %s (%s)", file.Name, file.ID)
					deletedFileIDs = append(deletedFileIDs, file.ID)
				} else {
					// 다른 오류 (네트워크 등)는 무시하고 유효한 파일로 간주
					log.Printf("⚠️ 파일 검증 실패 (유효한 파일로 간주): %s - %v", file.ID, err)
					validFileCount++
				}
			} else {
				// 파일이 실제로 존재함
				validFileCount++
			}
		}

		// Delete invalid files from database
		for _, fileID := range deletedFileIDs {
			if err := uc.fileRepo.Delete(ctx, fileID); err != nil {
				log.Printf("⚠️ 파일 삭제 실패 (그룹 ID: %d): %s - %v", group.ID, fileID, err)
			} else {
				deletedFiles++
			}
		}

		// If less than 2 valid files remain, delete the group
		if validFileCount < 2 {
			log.Printf("🗑️ 그룹 %d: 유효한 파일 %d개 (삭제됨 %d개) - 그룹 삭제",
				group.ID, validFileCount, len(deletedFileIDs))
			if err := uc.duplicateRepo.Delete(ctx, group.ID); err != nil {
				log.Printf("⚠️ 중복 그룹 삭제 실패: %d - %v", group.ID, err)
			} else {
				deletedGroups++
			}
		} else {
			// Group is still valid, update counts if needed
			if len(deletedFileIDs) > 0 {
				group.Count = validFileCount
				if validFileCount > 0 && len(files) > 0 {
					group.TotalSize = int64(validFileCount) * files[0].Size
				}
				uc.duplicateRepo.Update(ctx, group)
			}
			validGroups = append(validGroups, group)
		}
	}

	log.Printf("✅ Google Drive 검증 완료: %d개 그룹 삭제, %d개 파일 삭제, %d개 그룹 유지",
		deletedGroups, deletedFiles, len(validGroups))

	return validGroups, nil
}

// filterUnverifiedGroups filters out groups that have already been verified or ignored
func (uc *DuplicateFindingUseCase) filterUnverifiedGroups(ctx context.Context, groups []*entities.DuplicateGroup) ([]*entities.DuplicateGroup, error) {
	if len(groups) == 0 {
		return groups, nil
	}

	log.Printf("🔍 검증된 중복 필터링 시작: %d개 그룹", len(groups))

	// Extract hashes from groups
	hashes := make([]string, len(groups))
	for i, group := range groups {
		hashes[i] = group.Hash
	}

	// Batch check which hashes are verified
	verifiedMap, err := uc.verifiedRepo.BatchCheckVerified(ctx, hashes)
	if err != nil {
		return nil, fmt.Errorf("검증된 중복 조회 실패: %w", err)
	}

	// Filter out verified groups
	var unverifiedGroups []*entities.DuplicateGroup
	excludedCount := 0

	for _, group := range groups {
		if verified, exists := verifiedMap[group.Hash]; exists {
			// Skip groups that are verified or ignored
			if verified.Status == "verified" || verified.Status == "ignored" {
				excludedCount++
				continue
			}
		}
		unverifiedGroups = append(unverifiedGroups, group)
	}

	log.Printf("✅ 검증된 중복 필터링 완료: %d개 제외, %d개 유지", excludedCount, len(unverifiedGroups))
	return unverifiedGroups, nil
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

// GetDuplicateGroupsPaginated returns paginated duplicate groups (excluding verified ones)
func (uc *DuplicateFindingUseCase) GetDuplicateGroupsPaginated(ctx context.Context, page, pageSize int) (*GetDuplicateGroupsResponse, error) {
	// Calculate offset
	offset := (page - 1) * pageSize

	// 🎯 최적화: DB 레벨에서 페이지네이션 적용 (전체 로드 안함)
	// 1. 전체 그룹 개수만 조회 (COUNT 쿼리 - 매우 빠름)
	totalCount, err := uc.duplicateRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 개수 조회 실패: %w", err)
	}

	// 2. 현재 페이지의 그룹만 DB에서 조회 (LIMIT/OFFSET - 빠름)
	groups, err := uc.duplicateRepo.GetPaginated(ctx, offset, pageSize)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 페이지 조회 실패: %w", err)
	}

	log.Printf("📊 페이지네이션 결과: 전체 %d개 그룹 → 페이지 %d (%d-%d) → %d개 조회",
		totalCount, page, offset+1, offset+len(groups), len(groups))

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
// Automatically cleans up deleted files before returning the group
func (uc *DuplicateFindingUseCase) GetDuplicateGroup(ctx context.Context, groupID int) (*entities.DuplicateGroup, error) {
	group, err := uc.duplicateRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	if group == nil {
		return nil, fmt.Errorf("중복 그룹을 찾을 수 없습니다: %d", groupID)
	}

	// Get files in this group
	files, err := uc.fileRepo.GetByHash(ctx, group.Hash)
	if err != nil {
		return nil, fmt.Errorf("그룹 파일 조회 실패: %w", err)
	}

	// Clean up deleted files (files with empty path and no parents)
	validFileCount := 0
	deletedFileIDs := []string{}

	for _, file := range files {
		if file.Path == "" && (file.Parents == nil || len(file.Parents) == 0) {
			deletedFileIDs = append(deletedFileIDs, file.ID)
			log.Printf("🗑️ 삭제된 파일 발견 (그룹 ID: %d): %s (%s)", groupID, file.Name, file.ID)
		} else {
			validFileCount++
		}
	}

	// Delete invalid files from database
	for _, fileID := range deletedFileIDs {
		if err := uc.fileRepo.Delete(ctx, fileID); err != nil {
			log.Printf("⚠️ 파일 삭제 실패 (그룹 ID: %d): %s - %v", groupID, fileID, err)
		}
	}

	// If less than 2 valid files remain, delete the group
	if validFileCount < 2 {
		log.Printf("🗑️ 유효한 파일이 %d개만 남아 그룹 삭제: ID %d", validFileCount, groupID)
		if err := uc.duplicateRepo.Delete(ctx, groupID); err != nil {
			log.Printf("⚠️ 중복 그룹 삭제 실패: %d - %v", groupID, err)
		}
		return nil, fmt.Errorf("이 중복 그룹의 파일들이 삭제되어 그룹이 제거되었습니다")
	}

	// Log cleanup summary
	if len(deletedFileIDs) > 0 {
		log.Printf("✅ 그룹 ID %d 정리 완료: %d개 파일 삭제, %d개 유효 파일 남음", groupID, len(deletedFileIDs), validFileCount)
	}

	return group, nil
}

// GetDuplicateGroupByHash returns a specific duplicate group by hash (includes verified/ignored groups)
func (uc *DuplicateFindingUseCase) GetDuplicateGroupByHash(ctx context.Context, hash string) (*entities.DuplicateGroup, error) {
	log.Printf("🔍 해시로 중복 그룹 조회: %s", hash)

	group, err := uc.duplicateRepo.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	if group == nil {
		return nil, fmt.Errorf("중복 그룹을 찾을 수 없습니다: %s", hash)
	}

	log.Printf("✅ 중복 그룹 조회 성공: %d개 파일", len(group.Files))
	return group, nil
}

// UpdateGroupMemo updates the memo for a duplicate group
func (uc *DuplicateFindingUseCase) UpdateGroupMemo(ctx context.Context, groupID int, memo string) error {
	return uc.duplicateRepo.UpdateMemo(ctx, groupID, memo)
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
		// 404 에러 체크: 파일이 Google Drive에서 삭제됨
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "notFound") {
			log.Printf("🗑️ 파일이 Google Drive에서 삭제됨, DB에서 제거: %s (%s)", dbFile.Name, fileID)
			// 데이터베이스에서 파일 삭제
			uc.fileRepo.Delete(ctx, fileID)
			return nil, fmt.Errorf("파일이 Google Drive에서 삭제되어 데이터베이스에서 제거했습니다: %s", fileID)
		}

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

// TrashFile moves a file to the Google Drive trash
func (uc *DuplicateFindingUseCase) TrashFile(ctx context.Context, fileID string) error {
	log.Printf("🗑️ 파일 휴지통 이동 시작: %s", fileID)

	err := uc.storageProvider.TrashFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("파일 휴지통 이동 실패: %w", err)
	}

	log.Printf("✅ 파일 휴지통 이동 완료: %s", fileID)
	return nil
}

// FilePathResponse represents the response for file path information
type FilePathResponse struct {
	FileID     string `json:"fileId"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentID   string `json:"parentId,omitempty"`
	WebViewURL string `json:"webViewUrl,omitempty"`
}

// ValidateAndCleanupDuplicateGroupsRequest represents the request for validating duplicate groups
type ValidateAndCleanupDuplicateGroupsRequest struct {
	WorkerCount int `json:"workerCount,omitempty"`
}

// ValidateAndCleanupDuplicateGroupsResponse represents the response for validating duplicate groups
type ValidateAndCleanupDuplicateGroupsResponse struct {
	Progress             *entities.Progress `json:"progress"`
	TotalGroupsChecked   int                `json:"totalGroupsChecked"`
	ValidGroups          int                `json:"validGroups"`
	InvalidGroups        int                `json:"invalidGroups"`
	InvalidGroupsDeleted int                `json:"invalidGroupsDeleted"`
	Errors               []string           `json:"errors"`
}

// ValidateAndCleanupDuplicateGroups validates existing duplicate groups and removes invalid ones
func (uc *DuplicateFindingUseCase) ValidateAndCleanupDuplicateGroups(ctx context.Context, req *ValidateAndCleanupDuplicateGroupsRequest) (*ValidateAndCleanupDuplicateGroupsResponse, error) {
	log.Println("🔍 중복 그룹 유효성 검증 및 정리 시작")

	// Apply configuration
	if req.WorkerCount > 0 {
		uc.workerCount = req.WorkerCount
	}

	// Create progress entry
	progress := &entities.Progress{
		OperationType:  entities.OperationValidateDuplicates,
		Status:         entities.StatusRunning,
		CurrentStep:    "중복 그룹 유효성 검증 중...",
		ProcessedItems: 0,
		TotalItems:     0, // Will be updated after getting groups
	}
	err := uc.progressRepo.Save(ctx, progress)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &ValidateAndCleanupDuplicateGroupsResponse{
		Progress: progress,
		Errors:   make([]string, 0),
	}

	// Start validation in background with a new context (not tied to HTTP request)
	go uc.performDuplicateGroupValidation(context.Background(), req, progress, response)

	return response, nil
}

// performDuplicateGroupValidation performs the actual validation in background
func (uc *DuplicateFindingUseCase) performDuplicateGroupValidation(ctx context.Context, req *ValidateAndCleanupDuplicateGroupsRequest, progress *entities.Progress, response *ValidateAndCleanupDuplicateGroupsResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 중복 그룹 검증 패닉: %v", r)
			progress.Status = entities.StatusFailed
			progress.ErrorMessage = fmt.Sprintf("중복 그룹 검증 중 오류 발생: %v", r)
			uc.progressRepo.Update(ctx, progress)
		}
	}()

	// 🎯 최적화: 전체 개수만 먼저 조회 (메모리 절약)
	totalCount, err := uc.duplicateRepo.Count(ctx)
	if err != nil {
		log.Printf("❌ 중복 그룹 개수 조회 실패: %v", err)
		progress.Status = entities.StatusFailed
		progress.ErrorMessage = "중복 그룹 개수 조회 실패"
		uc.progressRepo.Update(ctx, progress)
		return
	}

	// Update total steps
	progress.TotalItems = totalCount
	progress.CurrentStep = fmt.Sprintf("%d개 중복 그룹 검증 중...", totalCount)
	uc.progressRepo.Update(ctx, progress)

	log.Printf("📊 총 %d개 중복 그룹 검증 시작 (배치 처리)", totalCount)

	validGroups := 0
	invalidGroups := 0
	invalidGroupsDeleted := 0
	var validationErrors []string
	processedTotal := 0

	// 🎯 최적화: 배치 단위로 처리 (한 번에 100개씩)
	const batchSize = 100
	offset := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("⏹️ 중복 그룹 검증 작업이 취소되었습니다")
			progress.Status = entities.StatusFailed
			progress.ErrorMessage = "작업이 취소되었습니다"
			uc.progressRepo.Update(ctx, progress)
			return
		default:
		}

		// 배치 단위로 그룹 조회
		groups, err := uc.duplicateRepo.GetPaginated(ctx, offset, batchSize)
		if err != nil {
			log.Printf("❌ 중복 그룹 배치 조회 실패 (offset=%d): %v", offset, err)
			progress.Status = entities.StatusFailed
			progress.ErrorMessage = fmt.Sprintf("중복 그룹 배치 조회 실패: %v", err)
			uc.progressRepo.Update(ctx, progress)
			return
		}

		// 더 이상 그룹이 없으면 종료
		if len(groups) == 0 {
			break
		}

		log.Printf("📦 배치 처리 중: offset=%d, 이번 배치=%d개", offset, len(groups))

		// Validate each group in this batch
		for _, group := range groups {
			select {
			case <-ctx.Done():
				log.Println("⏹️ 중복 그룹 검증 작업이 취소되었습니다")
				progress.Status = entities.StatusFailed
				progress.ErrorMessage = "작업이 취소되었습니다"
				uc.progressRepo.Update(ctx, progress)
				return
			default:
			}

			processedTotal++

			// Update progress (매 10개마다)
			if processedTotal%10 == 0 || processedTotal == totalCount {
				progress.ProcessedItems = processedTotal
				progress.CurrentStep = fmt.Sprintf("중복 그룹 검증 중... (%d/%d) - 그룹 ID: %d", processedTotal, totalCount, group.ID)
				uc.progressRepo.Update(ctx, progress)
			}

			// 개별 그룹 검증을 패닉 복구 래퍼로 감싸기
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("🔴 그룹 %d 검증 중 패닉 복구: %v", group.ID, r)
						invalidGroups++
						validationErrors = append(validationErrors, fmt.Sprintf("그룹 %d 검증 중 패닉: %v", group.ID, r))
					}
				}()

				// Check if group is still valid (has actual duplicates)
				isValid, err := uc.validateDuplicateGroup(ctx, group)
				if err != nil {
					log.Printf("❌ 그룹 %d 검증 중 오류 발생, 삭제 예정: %v", group.ID, err)
					invalidGroups++

					// Delete group with validation errors
					delErr := uc.DeleteDuplicateGroup(ctx, group.ID)
					if delErr != nil {
						log.Printf("⚠️ 오류 그룹 %d 삭제 실패: %v", group.ID, delErr)
						validationErrors = append(validationErrors, fmt.Sprintf("오류 그룹 %d 삭제 실패: %v", group.ID, delErr))
					} else {
						invalidGroupsDeleted++
						log.Printf("🗑️ 오류 그룹 %d 삭제 완료", group.ID)
					}
					return
				}

				if isValid {
					validGroups++
					// 유효한 그룹은 로그를 매 100개마다만 출력 (로그 폭주 방지)
					if validGroups%100 == 0 {
						log.Printf("✅ %d개 유효한 중복 그룹 확인됨", validGroups)
					}
				} else {
					invalidGroups++
					log.Printf("❌ 그룹 %d: 더 이상 중복이 아님, 삭제 예정", group.ID)

					// Delete invalid group
					err := uc.DeleteDuplicateGroup(ctx, group.ID)
					if err != nil {
						log.Printf("⚠️ 무효한 그룹 %d 삭제 실패: %v", group.ID, err)
						validationErrors = append(validationErrors, fmt.Sprintf("그룹 %d 삭제 실패: %v", group.ID, err))
					} else {
						invalidGroupsDeleted++
						log.Printf("🗑️ 무효한 그룹 %d 삭제 완료", group.ID)
					}
				}
			}()
		}

		// 다음 배치로 이동
		// 주의: 삭제로 인해 offset이 변경될 수 있으므로, 삭제된 그룹 수만큼 보정
		// 하지만 GetPaginated는 ID 순서대로 가져오므로 삭제된 항목은 자동으로 건너뜀
		offset += batchSize
	}

	// Update final results
	response.TotalGroupsChecked = processedTotal
	response.ValidGroups = validGroups
	response.InvalidGroups = invalidGroups
	response.InvalidGroupsDeleted = invalidGroupsDeleted
	response.Errors = validationErrors

	// Complete progress
	progress.Status = entities.StatusCompleted
	progress.CurrentStep = fmt.Sprintf("검증 완료: %d개 그룹 중 %d개 유효, %d개 무효 그룹 삭제", processedTotal, validGroups, invalidGroupsDeleted)
	uc.progressRepo.Update(ctx, progress)

	log.Printf("✅ 중복 그룹 검증 완료: %d개 검증, %d개 유효, %d개 무효 삭제", processedTotal, validGroups, invalidGroupsDeleted)
}

// validateDuplicateGroup checks if a duplicate group is still valid (has actual duplicates)
func (uc *DuplicateFindingUseCase) validateDuplicateGroup(ctx context.Context, group *entities.DuplicateGroup) (bool, error) {
	// 패닉 복구 로직
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 validateDuplicateGroup 패닉 복구: 그룹 ID=%d, 패닉=%v", group.ID, r)
		}
	}()

	// Get all files in this group
	files, err := uc.fileRepo.GetByHash(ctx, group.Hash)
	if err != nil {
		return false, fmt.Errorf("해시 %s의 파일 조회 실패: %w", group.Hash, err)
	}

	// Check if files still exist and are accessible
	var existingFiles []*entities.File
	for i, file := range files {
		// 컨텍스트 취소 확인
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		// 5초 타임아웃으로 파일 존재 확인
		fileCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		exists, err := uc.storageProvider.FileExists(fileCtx, file.ID)
		cancel()

		if err != nil {
			log.Printf("⚠️ 파일 %s 존재 확인 중 오류: %v", file.ID, err)
			// 타임아웃이나 에러 시 파일이 존재한다고 가정 (보수적 접근)
			existingFiles = append(existingFiles, file)
			continue
		}

		if exists {
			existingFiles = append(existingFiles, file)
		} else {
			log.Printf("🗑️ 파일 %s은 더 이상 존재하지 않음", file.ID)
			// Remove non-existent file from database
			uc.fileRepo.Delete(ctx, file.ID)
		}

		// API Rate Limiting 방지: 10개 파일마다 50ms 대기
		if i > 0 && i%10 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// A group is valid if it has 2 or more existing files
	isValid := len(existingFiles) >= 2

	// Update group information if needed
	if len(existingFiles) != len(files) {
		// Some files were removed, update the group
		group.Count = len(existingFiles)
		if len(existingFiles) > 0 {
			group.TotalSize = int64(len(existingFiles)) * existingFiles[0].Size
		} else {
			group.TotalSize = 0
		}

		if isValid {
			// Update the group with new count and size
			err = uc.duplicateRepo.Update(ctx, group)
			if err != nil {
				log.Printf("⚠️ 그룹 %d 업데이트 실패: %v", group.ID, err)
			}
		}
	}

	return isValid, nil
}

// ResumableTaskInfo contains information about a resumable task
type ResumableTaskInfo struct {
	ProgressID      int       `json:"progressId"`
	OperationType   string    `json:"operationType"`
	Status          string    `json:"status"`
	CurrentBatch    int       `json:"currentBatch"`
	TotalSuccessful int       `json:"totalSuccessful"`
	TotalFailed     int       `json:"totalFailed"`
	StartTime       time.Time `json:"startTime"`
	LastUpdated     time.Time `json:"lastUpdated"`
	CanResume       bool      `json:"canResume"`
}

// CheckResumableTasks checks if there are any resumable duplicate search tasks
func (uc *DuplicateFindingUseCase) CheckResumableTasks(ctx context.Context) (*ResumableTaskInfo, error) {
	// Get all duplicate_search operations
	allOperations, err := uc.progressRepo.GetByOperationType(ctx, entities.OperationDuplicateSearch)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 조회 실패: %w", err)
	}

	if len(allOperations) == 0 {
		return nil, nil
	}

	// Find the latest resumable task
	var latestResumable *entities.Progress
	latestBatch := 0

	for _, op := range allOperations {
		// Check if resumable
		if resumable, ok := op.GetMetadata("resumable"); ok {
			if isResumable, ok := resumable.(bool); ok && isResumable {
				// Check if status is paused OR (completed but resumable)
				if op.Status == entities.StatusPaused ||
					(op.Status == entities.StatusCompleted && isResumable) {

					// Get current batch number
					currentBatch := 0
					if batch, ok := op.GetMetadata("currentBatch"); ok {
						if batchNum, ok := batch.(float64); ok {
							currentBatch = int(batchNum)
						}
					}

					// Select the latest task (highest batch number)
					if latestResumable == nil || currentBatch > latestBatch {
						latestResumable = op
						latestBatch = currentBatch
					}
				}
			}
		}
	}

	// No resumable task found
	if latestResumable == nil {
		return nil, nil
	}

	// Extract metadata
	totalSuccessful := 0
	totalFailed := 0

	if success, ok := latestResumable.GetMetadata("totalSuccessful"); ok {
		if count, ok := success.(float64); ok {
			totalSuccessful = int(count)
		}
	}

	if fail, ok := latestResumable.GetMetadata("totalFailed"); ok {
		if count, ok := fail.(float64); ok {
			totalFailed = int(count)
		}
	}

	// Build response
	info := &ResumableTaskInfo{
		ProgressID:      latestResumable.ID,
		OperationType:   string(latestResumable.OperationType),
		Status:          string(latestResumable.Status),
		CurrentBatch:    latestBatch,
		TotalSuccessful: totalSuccessful,
		TotalFailed:     totalFailed,
		StartTime:       latestResumable.StartTime,
		LastUpdated:     latestResumable.LastUpdated,
		CanResume:       true,
	}

	log.Printf("✅ 재개 가능한 작업 발견: ID %d, 배치 %d, 성공 %d개, 실패 %d개",
		info.ProgressID, info.CurrentBatch, info.TotalSuccessful, info.TotalFailed)

	return info, nil
}

// ResetAllProgress deletes all duplicate search progress to start fresh
func (uc *DuplicateFindingUseCase) ResetAllProgress(ctx context.Context) error {
	log.Printf("🗑️ 모든 중복 검색 진행 상황 삭제 시작")

	// Get all duplicate_search operations
	allOperations, err := uc.progressRepo.GetByOperationType(ctx, entities.OperationDuplicateSearch)
	if err != nil {
		return fmt.Errorf("진행 상황 조회 실패: %w", err)
	}

	deletedCount := 0
	for _, op := range allOperations {
		// Delete the progress entry
		err := uc.progressRepo.Delete(ctx, op.ID)
		if err != nil {
			log.Printf("⚠️ 진행 상황 삭제 실패 (ID: %d): %v", op.ID, err)
		} else {
			deletedCount++
			batchInfo := "N/A"
			if batch, ok := op.GetMetadata("currentBatch"); ok {
				batchInfo = fmt.Sprintf("%v", batch)
			}
			log.Printf("🗑️ 진행 상황 삭제 완료: ID %d (배치: %s, 상태: %s)",
				op.ID, batchInfo, op.Status)
		}
	}

	log.Printf("✅ 총 %d개의 진행 상황 삭제 완료", deletedCount)
	return nil
}
