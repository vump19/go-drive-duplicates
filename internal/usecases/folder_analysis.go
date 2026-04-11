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
	"sync/atomic"
	"time"
)

// 병렬 처리를 위한 상수
const (
	maxConcurrentFolderAnalysis = 10 // 동시에 분석할 최대 폴더 수
)

// FolderAnalysisUseCase handles folder analysis operations
type FolderAnalysisUseCase struct {
	fileRepo        repositories.FileRepository
	progressRepo    repositories.ProgressRepository
	storageProvider services.StorageProvider
	progressService services.ProgressService
}

// NewFolderAnalysisUseCase creates a new folder analysis use case
func NewFolderAnalysisUseCase(
	fileRepo repositories.FileRepository,
	progressRepo repositories.ProgressRepository,
	storageProvider services.StorageProvider,
	progressService services.ProgressService,
) *FolderAnalysisUseCase {
	return &FolderAnalysisUseCase{
		fileRepo:        fileRepo,
		progressRepo:    progressRepo,
		storageProvider: storageProvider,
		progressService: progressService,
	}
}

// FolderInfo represents folder information with size and structure
type FolderInfo struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Path        string       `json:"path"`
	TotalSize   int64        `json:"totalSize"`
	FileCount   int          `json:"fileCount"`
	SubFolders  []FolderInfo `json:"subFolders"`
	IsEmpty     bool         `json:"isEmpty"`
	ParentID    string       `json:"parentId,omitempty"`
	WebViewLink string       `json:"webViewLink,omitempty"`
}

// FolderAnalysisResult represents the complete folder analysis result
type FolderAnalysisResult struct {
	RootFolder         FolderInfo   `json:"rootFolder"`
	EmptyFolders       []FolderInfo `json:"emptyFolders"`
	TotalFiles         int          `json:"totalFiles"`
	TotalSize          int64        `json:"totalSize"`
	AnalysisTime       string       `json:"analysisTime"`
	ExcludeFolderNames []string     `json:"excludeFolderNames,omitempty"` // 분석 시 사용된 제외 패턴
}

// AnalyzeFolder analyzes a folder structure and calculates sizes (without excludes)
func (uc *FolderAnalysisUseCase) AnalyzeFolder(ctx context.Context, folderID string) (*FolderAnalysisResult, error) {
	return uc.AnalyzeFolderWithExcludes(ctx, folderID, nil)
}

// AnalyzeFolderWithExcludes analyzes a folder structure with exclude patterns
func (uc *FolderAnalysisUseCase) AnalyzeFolderWithExcludes(ctx context.Context, folderID string, excludeFolderNames []string) (*FolderAnalysisResult, error) {
	if len(excludeFolderNames) > 0 {
		log.Printf("📊 폴더 분석 시작: 폴더 ID %s (제외 패턴: %v)", folderID, excludeFolderNames)
	} else {
		log.Printf("📊 폴더 분석 시작: 폴더 ID %s", folderID)
	}

	// Create progress tracking
	progress, err := uc.progressService.StartOperation(ctx, "folder_analysis", 0)
	if err != nil {
		log.Printf("⚠️ 진행 상황 생성 실패: %v", err)
	}

	defer func() {
		if progress != nil {
			uc.progressService.CompleteOperation(ctx, progress.ID)
		}
	}()

	// Analyze the folder structure with exclude patterns
	rootFolder, err := uc.analyzeFolderRecursiveWithExcludes(ctx, folderID, "", progress, excludeFolderNames)
	if err != nil {
		if progress != nil {
			uc.progressService.FailOperation(ctx, progress.ID, err.Error())
		}
		return nil, fmt.Errorf("폴더 분석 실패: %w", err)
	}

	// Find empty folders
	var emptyFolders []FolderInfo
	uc.findEmptyFolders(&rootFolder, &emptyFolders)

	// Calculate total statistics
	totalFiles, totalSize := uc.calculateTotals(&rootFolder)

	result := &FolderAnalysisResult{
		RootFolder:         rootFolder,
		EmptyFolders:       emptyFolders,
		TotalFiles:         totalFiles,
		TotalSize:          totalSize,
		AnalysisTime:       time.Now().Format("2006-01-02 15:04:05"),
		ExcludeFolderNames: excludeFolderNames,
	}

	log.Printf("✅ 폴더 분석 완료: 총 %d개 파일, %d MB, 빈 폴더 %d개",
		totalFiles, totalSize/(1024*1024), len(emptyFolders))

	return result, nil
}

// shouldExcludeFolder checks if a folder name matches any exclude pattern
func shouldExcludeFolder(folderName string, excludePatterns []string) bool {
	if len(excludePatterns) == 0 {
		return false
	}

	folderNameLower := strings.ToLower(folderName)
	for _, pattern := range excludePatterns {
		patternLower := strings.ToLower(strings.TrimSpace(pattern))
		if patternLower == "" {
			continue
		}
		// 대소문자 무시하고 부분 문자열 매칭
		if strings.Contains(folderNameLower, patternLower) {
			return true
		}
	}
	return false
}

// folderAnalysisResult holds the result of analyzing a single folder
type folderAnalysisResult struct {
	folderInfo FolderInfo
	err        error
	index      int // 원래 순서 유지용
}

// analyzeFolderRecursiveWithExcludes recursively analyzes folder structure with exclude patterns
func (uc *FolderAnalysisUseCase) analyzeFolderRecursiveWithExcludes(ctx context.Context, folderID, parentPath string, progress *entities.Progress, excludePatterns []string) (FolderInfo, error) {
	// Get folder metadata
	folderMeta, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		return FolderInfo{}, fmt.Errorf("폴더 메타데이터 조회 실패 (%s): %w", folderID, err)
	}

	currentPath := parentPath
	if currentPath == "" {
		currentPath = folderMeta.Name
	} else {
		currentPath = fmt.Sprintf("%s/%s", parentPath, folderMeta.Name)
	}

	folderInfo := FolderInfo{
		ID:          folderID,
		Name:        folderMeta.Name,
		Path:        currentPath,
		ParentID:    extractParentID(folderMeta.Parents),
		WebViewLink: folderMeta.WebViewLink,
	}

	// Update progress
	if progress != nil {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, fmt.Sprintf("분석 중: %s", currentPath))
	}

	// Get folder contents
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return FolderInfo{}, fmt.Errorf("폴더 내용 조회 실패 (%s): %w", folderID, err)
	}

	var totalSize int64
	var fileCount int
	var subFolderItems []*entities.File
	var excludedCount int

	// 파일과 폴더 분리 (제외 패턴 적용)
	for _, item := range contents {
		if item.MimeType == "application/vnd.google-apps.folder" {
			// 제외 패턴 체크
			if shouldExcludeFolder(item.Name, excludePatterns) {
				excludedCount++
				log.Printf("⏭️ 제외된 폴더: %s (패턴 매칭)", item.Name)
				continue
			}
			subFolderItems = append(subFolderItems, item)
		} else {
			// It's a file
			totalSize += item.Size
			fileCount++
		}
	}

	if excludedCount > 0 {
		log.Printf("📋 %s: %d개 하위 폴더 중 %d개 제외됨", folderMeta.Name, excludedCount+len(subFolderItems), excludedCount)
	}

	// 하위 폴더가 없으면 바로 반환
	if len(subFolderItems) == 0 {
		folderInfo.TotalSize = totalSize
		folderInfo.FileCount = fileCount
		folderInfo.IsEmpty = (fileCount == 0)
		return folderInfo, nil
	}

	// 하위 폴더를 병렬로 분석 (제외 패턴 전달)
	subFolders := uc.analyzeSubfoldersParallelWithExcludes(ctx, subFolderItems, currentPath, progress, excludePatterns)

	// 결과 집계
	for _, subFolder := range subFolders {
		totalSize += subFolder.TotalSize
		fileCount += subFolder.FileCount
	}

	// Sort subfolders by size (largest first)
	sort.Slice(subFolders, func(i, j int) bool {
		return subFolders[i].TotalSize > subFolders[j].TotalSize
	})

	folderInfo.TotalSize = totalSize
	folderInfo.FileCount = fileCount
	folderInfo.SubFolders = subFolders
	folderInfo.IsEmpty = (fileCount == 0 && len(subFolders) == 0)

	return folderInfo, nil
}

// analyzeSubfoldersParallelWithExcludes analyzes multiple subfolders in parallel with exclude patterns
func (uc *FolderAnalysisUseCase) analyzeSubfoldersParallelWithExcludes(ctx context.Context, subFolderItems []*entities.File, parentPath string, progress *entities.Progress, excludePatterns []string) []FolderInfo {
	if len(subFolderItems) == 0 {
		return nil
	}

	// 세마포어를 위한 채널 (동시 실행 수 제한)
	semaphore := make(chan struct{}, maxConcurrentFolderAnalysis)

	// 결과 수집을 위한 채널
	resultChan := make(chan folderAnalysisResult, len(subFolderItems))

	// WaitGroup으로 모든 고루틴 완료 대기
	var wg sync.WaitGroup

	log.Printf("🚀 병렬 폴더 분석 시작: %d개 하위 폴더 (동시 처리: %d개)",
		len(subFolderItems), maxConcurrentFolderAnalysis)

	for i, item := range subFolderItems {
		wg.Add(1)
		go func(idx int, folder *entities.File) {
			defer wg.Done()

			// 세마포어 획득 (동시 실행 수 제한)
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 컨텍스트 취소 확인
			if ctx.Err() != nil {
				resultChan <- folderAnalysisResult{
					err:   ctx.Err(),
					index: idx,
				}
				return
			}

			// 하위 폴더 재귀 분석 (제외 패턴 전달)
			subFolder, err := uc.analyzeFolderRecursiveWithExcludes(ctx, folder.ID, parentPath, progress, excludePatterns)
			if err != nil {
				log.Printf("⚠️ 하위 폴더 분석 실패 (%s): %v", folder.Name, err)
				resultChan <- folderAnalysisResult{
					err:   err,
					index: idx,
				}
				return
			}

			resultChan <- folderAnalysisResult{
				folderInfo: subFolder,
				index:      idx,
			}
		}(i, item)
	}

	// 결과 수집을 위한 고루틴
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 결과를 인덱스 순서대로 정렬하기 위한 맵
	results := make(map[int]FolderInfo)
	for result := range resultChan {
		if result.err == nil {
			results[result.index] = result.folderInfo
		}
	}

	// 원래 순서 유지
	subFolders := make([]FolderInfo, 0, len(results))
	for i := 0; i < len(subFolderItems); i++ {
		if folder, ok := results[i]; ok {
			subFolders = append(subFolders, folder)
		}
	}

	log.Printf("✅ 병렬 폴더 분석 완료: %d개 성공", len(subFolders))

	return subFolders
}

// analyzeFolderRecursive recursively analyzes folder structure with parallel processing
func (uc *FolderAnalysisUseCase) analyzeFolderRecursive(ctx context.Context, folderID, parentPath string, progress *entities.Progress) (FolderInfo, error) {
	// Get folder metadata
	folderMeta, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		return FolderInfo{}, fmt.Errorf("폴더 메타데이터 조회 실패 (%s): %w", folderID, err)
	}

	currentPath := parentPath
	if currentPath == "" {
		currentPath = folderMeta.Name
	} else {
		currentPath = fmt.Sprintf("%s/%s", parentPath, folderMeta.Name)
	}

	folderInfo := FolderInfo{
		ID:          folderID,
		Name:        folderMeta.Name,
		Path:        currentPath,
		ParentID:    extractParentID(folderMeta.Parents),
		WebViewLink: folderMeta.WebViewLink,
	}

	// Update progress
	if progress != nil {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, fmt.Sprintf("분석 중: %s", currentPath))
	}

	// Get folder contents
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return FolderInfo{}, fmt.Errorf("폴더 내용 조회 실패 (%s): %w", folderID, err)
	}

	var totalSize int64
	var fileCount int
	var subFolderItems []*entities.File

	// 파일과 폴더 분리
	for _, item := range contents {
		if item.MimeType == "application/vnd.google-apps.folder" {
			subFolderItems = append(subFolderItems, item)
		} else {
			// It's a file
			totalSize += item.Size
			fileCount++
		}
	}

	// 하위 폴더가 없으면 바로 반환
	if len(subFolderItems) == 0 {
		folderInfo.TotalSize = totalSize
		folderInfo.FileCount = fileCount
		folderInfo.IsEmpty = (fileCount == 0)
		return folderInfo, nil
	}

	// 하위 폴더를 병렬로 분석
	subFolders := uc.analyzeSubfoldersParallel(ctx, subFolderItems, currentPath, progress)

	// 결과 집계
	for _, subFolder := range subFolders {
		totalSize += subFolder.TotalSize
		fileCount += subFolder.FileCount
	}

	// Sort subfolders by size (largest first)
	sort.Slice(subFolders, func(i, j int) bool {
		return subFolders[i].TotalSize > subFolders[j].TotalSize
	})

	folderInfo.TotalSize = totalSize
	folderInfo.FileCount = fileCount
	folderInfo.SubFolders = subFolders
	folderInfo.IsEmpty = (fileCount == 0 && len(subFolders) == 0)

	return folderInfo, nil
}

// analyzeSubfoldersParallel analyzes multiple subfolders in parallel
func (uc *FolderAnalysisUseCase) analyzeSubfoldersParallel(ctx context.Context, subFolderItems []*entities.File, parentPath string, progress *entities.Progress) []FolderInfo {
	if len(subFolderItems) == 0 {
		return nil
	}

	// 세마포어를 위한 채널 (동시 실행 수 제한)
	semaphore := make(chan struct{}, maxConcurrentFolderAnalysis)

	// 결과 수집을 위한 채널
	resultChan := make(chan folderAnalysisResult, len(subFolderItems))

	// WaitGroup으로 모든 고루틴 완료 대기
	var wg sync.WaitGroup

	log.Printf("🚀 병렬 폴더 분석 시작: %d개 하위 폴더 (동시 처리: %d개)",
		len(subFolderItems), maxConcurrentFolderAnalysis)

	for i, item := range subFolderItems {
		wg.Add(1)
		go func(idx int, folder *entities.File) {
			defer wg.Done()

			// 세마포어 획득 (동시 실행 수 제한)
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 컨텍스트 취소 확인
			if ctx.Err() != nil {
				resultChan <- folderAnalysisResult{
					err:   ctx.Err(),
					index: idx,
				}
				return
			}

			// 하위 폴더 재귀 분석
			subFolder, err := uc.analyzeFolderRecursive(ctx, folder.ID, parentPath, progress)
			if err != nil {
				log.Printf("⚠️ 하위 폴더 분석 실패 (%s): %v", folder.Name, err)
				resultChan <- folderAnalysisResult{
					err:   err,
					index: idx,
				}
				return
			}

			resultChan <- folderAnalysisResult{
				folderInfo: subFolder,
				index:      idx,
			}
		}(i, item)
	}

	// 결과 수집을 위한 고루틴
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 결과를 인덱스 순서대로 정렬하기 위한 맵
	results := make(map[int]FolderInfo)
	for result := range resultChan {
		if result.err == nil {
			results[result.index] = result.folderInfo
		}
	}

	// 원래 순서 유지
	subFolders := make([]FolderInfo, 0, len(results))
	for i := 0; i < len(subFolderItems); i++ {
		if folder, ok := results[i]; ok {
			subFolders = append(subFolders, folder)
		}
	}

	log.Printf("✅ 병렬 폴더 분석 완료: %d개 성공", len(subFolders))

	return subFolders
}

// findEmptyFolders recursively finds all empty folders
func (uc *FolderAnalysisUseCase) findEmptyFolders(folder *FolderInfo, emptyFolders *[]FolderInfo) {
	if folder.IsEmpty {
		*emptyFolders = append(*emptyFolders, *folder)
	}

	// Recursively check subfolders
	for i := range folder.SubFolders {
		uc.findEmptyFolders(&folder.SubFolders[i], emptyFolders)
	}
}

// calculateTotals calculates total files and size recursively
func (uc *FolderAnalysisUseCase) calculateTotals(folder *FolderInfo) (int, int64) {
	totalFiles := folder.FileCount
	totalSize := folder.TotalSize

	return totalFiles, totalSize
}

// DeleteEmptyFolder deletes a single empty folder (휴지통으로 이동)
func (uc *FolderAnalysisUseCase) DeleteEmptyFolder(ctx context.Context, folderID string) error {
	return uc.DeleteEmptyFolderWithOptions(ctx, folderID, nil, false)
}

// DeleteEmptyFolderPermanent deletes a single empty folder (완전 삭제)
func (uc *FolderAnalysisUseCase) DeleteEmptyFolderPermanent(ctx context.Context, folderID string, permanentDelete bool) error {
	return uc.DeleteEmptyFolderWithOptions(ctx, folderID, nil, permanentDelete)
}

// DeleteEmptyFolderWithExcludes deletes a folder that may contain only excluded folders (휴지통으로 이동)
// It will first delete any excluded folders recursively, then delete the parent folder
func (uc *FolderAnalysisUseCase) DeleteEmptyFolderWithExcludes(ctx context.Context, folderID string, excludeFolderNames []string) error {
	return uc.DeleteEmptyFolderWithOptions(ctx, folderID, excludeFolderNames, false)
}

// DeleteEmptyFolderWithOptions deletes a folder with exclude patterns and delete option
// permanentDelete: true for permanent deletion, false for trash
func (uc *FolderAnalysisUseCase) DeleteEmptyFolderWithOptions(ctx context.Context, folderID string, excludeFolderNames []string, permanentDelete bool) error {
	deleteMethod := "휴지통 이동"
	if permanentDelete {
		deleteMethod = "완전 삭제"
	}
	log.Printf("🗑️ 빈 폴더 삭제 시작: 폴더 ID %s (제외 패턴: %v, 삭제 방식: %s)", folderID, excludeFolderNames, deleteMethod)

	// Get folder contents
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return fmt.Errorf("폴더 내용 확인 실패: %w", err)
	}

	// Check contents: files should not exist, only excluded folders are allowed
	var excludedFolders []*entities.File
	for _, item := range contents {
		if item.MimeType == "application/vnd.google-apps.folder" {
			// Check if this is an excluded folder
			if len(excludeFolderNames) > 0 && shouldExcludeFolder(item.Name, excludeFolderNames) {
				log.Printf("📁 제외 폴더 발견 (삭제 대상): %s", item.Name)
				excludedFolders = append(excludedFolders, item)
			} else {
				return fmt.Errorf("폴더가 비어있지 않습니다. 제외되지 않은 하위 폴더가 있습니다: %s", item.Name)
			}
		} else {
			return fmt.Errorf("폴더가 비어있지 않습니다. 파일이 있습니다: %s", item.Name)
		}
	}

	// Delete excluded folders first (recursively)
	for _, excludedFolder := range excludedFolders {
		log.Printf("🗑️ 제외 폴더 삭제 시작: %s (ID: %s)", excludedFolder.Name, excludedFolder.ID)
		err := uc.deleteExcludedFolderRecursiveWithOptions(ctx, excludedFolder.ID, permanentDelete)
		if err != nil {
			return fmt.Errorf("제외 폴더 삭제 실패 (%s): %w", excludedFolder.Name, err)
		}
		log.Printf("✅ 제외 폴더 삭제 완료: %s", excludedFolder.Name)
	}

	// Now delete the parent folder (should be truly empty now)
	if permanentDelete {
		err = uc.storageProvider.DeleteFile(ctx, folderID)
	} else {
		err = uc.storageProvider.TrashFile(ctx, folderID)
	}
	if err != nil {
		return fmt.Errorf("폴더 삭제 실패: %w", err)
	}

	log.Printf("✅ 빈 폴더 삭제 완료: 폴더 ID %s (%s)", folderID, deleteMethod)
	return nil
}

// deleteExcludedFolderRecursive recursively deletes a folder and all its contents (휴지통 이동)
func (uc *FolderAnalysisUseCase) deleteExcludedFolderRecursive(ctx context.Context, folderID string) error {
	return uc.deleteExcludedFolderRecursiveWithOptions(ctx, folderID, false)
}

// deleteExcludedFolderRecursiveWithOptions recursively deletes a folder with delete option
func (uc *FolderAnalysisUseCase) deleteExcludedFolderRecursiveWithOptions(ctx context.Context, folderID string, permanentDelete bool) error {
	// Google Drive API: 폴더를 삭제/휴지통 이동하면 하위 파일/폴더가 자동으로 함께 처리됨
	// 따라서 폴더만 직접 삭제하면 됨 (하위 항목 일일이 삭제할 필요 없음)
	var err error
	if permanentDelete {
		err = uc.storageProvider.DeleteFile(ctx, folderID)
	} else {
		err = uc.storageProvider.TrashFile(ctx, folderID)
	}
	if err != nil {
		return fmt.Errorf("폴더 삭제 실패: %w", err)
	}

	return nil
}

// DeleteAllEmptyFolders deletes all empty folders in a list (휴지통으로 이동)
func (uc *FolderAnalysisUseCase) DeleteAllEmptyFolders(ctx context.Context, folderIDs []string) (int, error) {
	return uc.DeleteAllEmptyFoldersWithOptions(ctx, folderIDs, nil, false)
}

// DeleteAllEmptyFoldersWithExcludes deletes all empty folders with exclude pattern support (휴지통으로 이동)
func (uc *FolderAnalysisUseCase) DeleteAllEmptyFoldersWithExcludes(ctx context.Context, folderIDs []string, excludeFolderNames []string) (int, error) {
	return uc.DeleteAllEmptyFoldersWithOptions(ctx, folderIDs, excludeFolderNames, false)
}

// DeleteAllEmptyFoldersWithOptions deletes all empty folders with options (parallel)
// After initial deletion, it cascades up to delete parent folders that became empty
func (uc *FolderAnalysisUseCase) DeleteAllEmptyFoldersWithOptions(ctx context.Context, folderIDs []string, excludeFolderNames []string, permanentDelete bool) (int, error) {
	deleteMethod := "휴지통 이동"
	if permanentDelete {
		deleteMethod = "완전 삭제"
	}

	if len(excludeFolderNames) > 0 {
		log.Printf("🗑️ 모든 빈 폴더 삭제 시작: %d개 폴더 (제외 패턴: %v, 삭제 방식: %s)", len(folderIDs), excludeFolderNames, deleteMethod)
	} else {
		log.Printf("🗑️ 모든 빈 폴더 삭제 시작: %d개 폴더 (삭제 방식: %s)", len(folderIDs), deleteMethod)
	}

	var successCount int32
	var errorsMu sync.Mutex
	var errors []string

	// Track parent folders that might become empty after deletion
	var parentIDsMu sync.Mutex
	parentIDsToCheck := make(map[string]bool)
	deletedIDs := make(map[string]bool)

	// Parallel deletion with max 5 concurrent (to avoid API rate limiting)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, folderID := range folderIDs {
		wg.Add(1)
		go func(fID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Get parent ID before deletion
			parentID, _ := uc.GetParentFolderID(ctx, fID)

			err := uc.DeleteEmptyFolderWithOptions(ctx, fID, excludeFolderNames, permanentDelete)
			if err != nil {
				log.Printf("❌ 폴더 삭제 실패 (%s): %v", fID, err)
				errorsMu.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", fID, err))
				errorsMu.Unlock()
			} else {
				atomic.AddInt32(&successCount, 1)

				// Track parent folder for cascade check
				if parentID != "" {
					parentIDsMu.Lock()
					parentIDsToCheck[parentID] = true
					deletedIDs[fID] = true
					parentIDsMu.Unlock()
				}
			}
		}(folderID)
	}

	wg.Wait()

	initialSuccess := int(successCount)
	log.Printf("✅ 초기 빈 폴더 삭제 완료: %d개 성공, %d개 실패 (%s)", initialSuccess, len(errors), deleteMethod)

	// Cascade deletion: check and delete parent folders that became empty
	cascadeDeletedCount := uc.CascadeDeleteEmptyParents(ctx, parentIDsToCheck, deletedIDs, excludeFolderNames, permanentDelete)
	if cascadeDeletedCount > 0 {
		log.Printf("🔄 연쇄 삭제 완료: 추가 %d개 부모 폴더 삭제됨", cascadeDeletedCount)
		successCount += int32(cascadeDeletedCount)
	}

	log.Printf("✅ 빈 폴더 삭제 총 완료: %d개 성공 (초기: %d, 연쇄: %d), %d개 실패 (%s)",
		successCount, initialSuccess, cascadeDeletedCount, len(errors), deleteMethod)

	if len(errors) > 0 {
		return int(successCount), fmt.Errorf("일부 폴더 삭제 실패: %v", errors)
	}

	return int(successCount), nil
}

// GetParentFolderID gets the parent folder ID for a given folder
func (uc *FolderAnalysisUseCase) GetParentFolderID(ctx context.Context, folderID string) (string, error) {
	folder, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		return "", err
	}
	if len(folder.Parents) > 0 {
		return folder.Parents[0], nil
	}
	return "", nil
}

// CascadeDeleteEmptyParents checks and deletes parent folders that became empty after their children were deleted
// Returns the number of additional folders deleted
func (uc *FolderAnalysisUseCase) CascadeDeleteEmptyParents(
	ctx context.Context,
	parentIDsToCheck map[string]bool,
	deletedIDs map[string]bool,
	excludeFolderNames []string,
	permanentDelete bool,
) int {
	if len(parentIDsToCheck) == 0 {
		return 0
	}

	totalDeleted := 0
	round := 1

	for len(parentIDsToCheck) > 0 {
		log.Printf("🔄 연쇄 삭제 라운드 %d: %d개 부모 폴더 확인 중...", round, len(parentIDsToCheck))

		// Get current batch of parent IDs to check
		currentBatch := make([]string, 0, len(parentIDsToCheck))
		for parentID := range parentIDsToCheck {
			// Skip if already deleted
			if deletedIDs[parentID] {
				continue
			}
			currentBatch = append(currentBatch, parentID)
		}

		// Clear for next round
		parentIDsToCheck = make(map[string]bool)

		if len(currentBatch) == 0 {
			break
		}

		// Check each parent folder
		var wg sync.WaitGroup
		var mu sync.Mutex
		nextParentsToCheck := make(map[string]bool)
		sem := make(chan struct{}, 5)

		for _, parentID := range currentBatch {
			wg.Add(1)
			go func(pID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// Check if parent folder is now empty
				isEmpty, err := uc.isFolderEmptyNow(ctx, pID)
				if err != nil {
					log.Printf("⚠️ 부모 폴더 확인 실패 (%s): %v", pID, err)
					return
				}

				if isEmpty {
					// Get grandparent ID before deletion
					grandparentID, _ := uc.GetParentFolderID(ctx, pID)

					// Delete the empty parent folder
					var deleteErr error
					if permanentDelete {
						deleteErr = uc.storageProvider.DeleteFile(ctx, pID)
					} else {
						deleteErr = uc.storageProvider.TrashFile(ctx, pID)
					}

					if deleteErr != nil {
						log.Printf("⚠️ 빈 부모 폴더 삭제 실패 (%s): %v", pID, deleteErr)
					} else {
						log.Printf("✅ 빈 부모 폴더 연쇄 삭제: %s", pID)
						mu.Lock()
						deletedIDs[pID] = true
						totalDeleted++
						// Queue grandparent for next round
						if grandparentID != "" && !deletedIDs[grandparentID] {
							nextParentsToCheck[grandparentID] = true
						}
						mu.Unlock()
					}
				}
			}(parentID)
		}

		wg.Wait()

		// Prepare for next round
		parentIDsToCheck = nextParentsToCheck
		round++

		// Safety limit to prevent infinite loops
		if round > 100 {
			log.Printf("⚠️ 연쇄 삭제 라운드 제한 도달 (100회)")
			break
		}
	}

	return totalDeleted
}

// isFolderEmptyNow checks if a folder is currently empty (no files or subfolders)
func (uc *FolderAnalysisUseCase) isFolderEmptyNow(ctx context.Context, folderID string) (bool, error) {
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}
	return len(contents) == 0, nil
}

// CleanupEmptySubfoldersRequest represents the request to cleanup empty subfolders
type CleanupEmptySubfoldersRequest struct {
	ParentFolderID string `json:"parentFolderId"`
	URL            string `json:"url,omitempty"`
}

// CleanupEmptySubfoldersResponse represents the response from cleanup operation
type CleanupEmptySubfoldersResponse struct {
	ParentFolderName string   `json:"parentFolderName"`
	DeletedCount     int      `json:"deletedCount"`
	DeletedFolders   []string `json:"deletedFolders"`
	SkippedCount     int      `json:"skippedCount"`
	SkippedFolders   []string `json:"skippedFolders"`
	TotalScanned     int      `json:"totalScanned"`
}

// CleanupEmptySubfolders finds and deletes all empty subfolders within a parent folder
// This function handles cascading deletion - when empty subfolders are deleted,
// their parent folders may become empty and should also be deleted.
func (uc *FolderAnalysisUseCase) CleanupEmptySubfolders(ctx context.Context, req *CleanupEmptySubfoldersRequest) (*CleanupEmptySubfoldersResponse, error) {
	log.Printf("🧹 특정 폴더의 빈 하위 폴더 정리 시작: %s", req.ParentFolderID)

	// Get parent folder metadata
	parentFolder, err := uc.storageProvider.GetFolder(ctx, req.ParentFolderID)
	if err != nil {
		return nil, fmt.Errorf("부모 폴더 조회 실패: %w", err)
	}

	// Create progress tracking
	progress, err := uc.progressService.StartOperation(ctx, "cleanup_empty_subfolders", 0)
	if err != nil {
		log.Printf("⚠️ 진행 상황 생성 실패: %v", err)
	}

	defer func() {
		if progress != nil {
			uc.progressService.CompleteOperation(ctx, progress.ID)
		}
	}()

	// Find all empty subfolders and collect ancestor folders
	emptySubfolders, ancestorFolders, totalScanned, err := uc.findEmptySubfoldersWithAncestors(ctx, req.ParentFolderID, progress)
	if err != nil {
		if progress != nil {
			uc.progressService.FailOperation(ctx, progress.ID, err.Error())
		}
		return nil, fmt.Errorf("빈 하위 폴더 검색 실패: %w", err)
	}

	response := &CleanupEmptySubfoldersResponse{
		ParentFolderName: parentFolder.Name,
		TotalScanned:     totalScanned,
		DeletedFolders:   make([]string, 0),
		SkippedFolders:   make([]string, 0),
	}

	// Delete empty subfolders
	if progress != nil {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "빈 폴더 삭제 중...")
	}

	// Sort empty folders by depth (deepest first) to ensure proper cascading deletion
	sort.Slice(emptySubfolders, func(i, j int) bool {
		return getFolderDepth(emptySubfolders[i].Path) > getFolderDepth(emptySubfolders[j].Path)
	})

	// Delete empty folders
	deletedIDs := make(map[string]bool)
	for _, emptyFolder := range emptySubfolders {
		err := uc.DeleteEmptyFolder(ctx, emptyFolder.ID)
		if err != nil {
			log.Printf("❌ 빈 하위 폴더 삭제 실패 (%s): %v", emptyFolder.Name, err)
			response.SkippedCount++
			response.SkippedFolders = append(response.SkippedFolders, emptyFolder.Name)
		} else {
			log.Printf("✅ 빈 하위 폴더 삭제 성공: %s", emptyFolder.Name)
			response.DeletedCount++
			response.DeletedFolders = append(response.DeletedFolders, emptyFolder.Name)
			deletedIDs[emptyFolder.ID] = true
		}
	}

	// Clean up ancestor folders that may have become empty
	cleanedUpAncestors := uc.cleanupEmptyAncestors(ctx, ancestorFolders, deletedIDs, req.ParentFolderID)
	for _, folderName := range cleanedUpAncestors {
		response.DeletedCount++
		response.DeletedFolders = append(response.DeletedFolders, folderName+" (상위 폴더)")
	}

	log.Printf("✅ 빈 하위 폴더 정리 완료: %d개 삭제, %d개 건너뜀 (총 %d개 스캔)",
		response.DeletedCount, response.SkippedCount, response.TotalScanned)

	return response, nil
}

// getFolderDepth returns the depth of a folder based on its path
func getFolderDepth(path string) int {
	if path == "" {
		return 0
	}
	depth := 1
	for _, c := range path {
		if c == '/' {
			depth++
		}
	}
	return depth
}

// cleanupEmptyAncestors checks and deletes ancestor folders that became empty
func (uc *FolderAnalysisUseCase) cleanupEmptyAncestors(ctx context.Context, ancestorFolders []FolderInfo, deletedIDs map[string]bool, rootFolderID string) []string {
	var cleanedUp []string

	// Sort ancestors by depth (deepest first)
	sort.Slice(ancestorFolders, func(i, j int) bool {
		return getFolderDepth(ancestorFolders[i].Path) > getFolderDepth(ancestorFolders[j].Path)
	})

	for _, ancestor := range ancestorFolders {
		// Skip if this is the root folder being cleaned
		if ancestor.ID == rootFolderID {
			continue
		}

		// Skip if already deleted
		if deletedIDs[ancestor.ID] {
			continue
		}

		// Check if folder is now empty (only files count, not subfolders that were deleted)
		isEmpty, err := uc.isFolderEmptyOfFiles(ctx, ancestor.ID)
		if err != nil {
			log.Printf("⚠️ 상위 폴더 확인 실패 (%s): %v", ancestor.Name, err)
			continue
		}

		if isEmpty {
			err := uc.storageProvider.TrashFile(ctx, ancestor.ID)
			if err != nil {
				log.Printf("⚠️ 빈 상위 폴더 휴지통 이동 실패 (%s): %v", ancestor.Name, err)
			} else {
				log.Printf("✅ 빈 상위 폴더 휴지통 이동 성공: %s", ancestor.Name)
				cleanedUp = append(cleanedUp, ancestor.Name)
				deletedIDs[ancestor.ID] = true
			}
		}
	}

	return cleanedUp
}

// isFolderEmptyOfFiles checks if a folder has no files (ignores empty subfolders)
func (uc *FolderAnalysisUseCase) isFolderEmptyOfFiles(ctx context.Context, folderID string) (bool, error) {
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}

	// Check if there are any files (not folders)
	for _, item := range contents {
		if item.MimeType != "application/vnd.google-apps.folder" {
			return false, nil // Has files
		}
	}

	// If there are folders, check if they still exist (not deleted)
	return len(contents) == 0, nil
}

// findEmptySubfoldersWithAncestors finds empty subfolders and collects their ancestor folders
func (uc *FolderAnalysisUseCase) findEmptySubfoldersWithAncestors(ctx context.Context, folderID string, progress *entities.Progress) ([]FolderInfo, []FolderInfo, int, error) {
	var emptyFolders []FolderInfo
	ancestorMap := make(map[string]FolderInfo) // Use map to avoid duplicates
	totalScanned := 0

	// Recursive helper function
	var findRecursive func(folderID, parentPath string, currentFolder *FolderInfo) error
	findRecursive = func(folderID, parentPath string, currentFolder *FolderInfo) error {
		// Update progress
		if progress != nil {
			uc.progressService.UpdateOperation(ctx, progress.ID, 0, fmt.Sprintf("폴더 스캔 중: %d개 확인됨", totalScanned))
		}

		// Get direct children
		contents, err := uc.storageProvider.ListFiles(ctx, folderID)
		if err != nil {
			return fmt.Errorf("폴더 내용 조회 실패 (%s): %w", folderID, err)
		}

		// Filter only folders
		var subfolders []*entities.File
		hasFiles := false
		for _, item := range contents {
			if item.MimeType == "application/vnd.google-apps.folder" {
				subfolders = append(subfolders, item)
				totalScanned++
			} else {
				hasFiles = true
			}
		}

		// Check each subfolder
		for _, subfolder := range subfolders {
			subfolderPath := subfolder.Name
			if parentPath != "" {
				subfolderPath = parentPath + "/" + subfolder.Name
			}

			// Check if this subfolder is empty
			isEmpty, err := uc.isFolderEmpty(ctx, subfolder.ID)
			if err != nil {
				log.Printf("⚠️ 폴더 비어있음 확인 실패 (%s): %v", subfolder.Name, err)
				continue
			}

			if isEmpty {
				// Add to empty folders list
				folderInfo := FolderInfo{
					ID:          subfolder.ID,
					Name:        subfolder.Name,
					Path:        subfolderPath,
					IsEmpty:     true,
					ParentID:    folderID,
					WebViewLink: subfolder.WebViewLink,
				}
				emptyFolders = append(emptyFolders, folderInfo)

				// Collect all ancestor folders (for potential cleanup after deletion)
				if currentFolder != nil {
					uc.collectAncestorFolders(ancestorMap, currentFolder, parentPath)
				}
			} else {
				// Create folder info for potential ancestor tracking
				thisFolder := &FolderInfo{
					ID:       subfolder.ID,
					Name:     subfolder.Name,
					Path:     subfolderPath,
					ParentID: folderID,
				}

				// Recursively check subfolders of this non-empty folder
				err := findRecursive(subfolder.ID, subfolderPath, thisFolder)
				if err != nil {
					log.Printf("⚠️ 하위 폴더 재귀 검색 실패 (%s): %v", subfolder.Name, err)
					continue
				}
			}
		}

		// If current folder has no files and only had subfolders (which may now be empty),
		// mark it as a potential ancestor for cleanup
		if currentFolder != nil && !hasFiles && len(subfolders) > 0 {
			uc.collectAncestorFolders(ancestorMap, currentFolder, parentPath)
		}

		return nil
	}

	// Start recursive search
	err := findRecursive(folderID, "", nil)
	if err != nil {
		return nil, nil, totalScanned, err
	}

	// Convert ancestor map to slice
	ancestors := make([]FolderInfo, 0, len(ancestorMap))
	for _, ancestor := range ancestorMap {
		ancestors = append(ancestors, ancestor)
	}

	return emptyFolders, ancestors, totalScanned, nil
}

// collectAncestorFolders collects all ancestor folders recursively
func (uc *FolderAnalysisUseCase) collectAncestorFolders(ancestorMap map[string]FolderInfo, folder *FolderInfo, path string) {
	if folder == nil || folder.ID == "" {
		return
	}

	// Add this folder to ancestor map if not already present
	if _, exists := ancestorMap[folder.ID]; !exists {
		ancestorMap[folder.ID] = *folder
	}
}

// findEmptySubfoldersRecursive recursively finds empty subfolders within a parent folder
// Kept for backward compatibility
func (uc *FolderAnalysisUseCase) findEmptySubfoldersRecursive(ctx context.Context, folderID string, progress *entities.Progress) ([]FolderInfo, int, error) {
	emptyFolders, _, totalScanned, err := uc.findEmptySubfoldersWithAncestors(ctx, folderID, progress)
	return emptyFolders, totalScanned, err
}

// isFolderEmpty checks if a folder is completely empty (no files, no subfolders)
func (uc *FolderAnalysisUseCase) isFolderEmpty(ctx context.Context, folderID string) (bool, error) {
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}
	return len(contents) == 0, nil
}

// extractParentID extracts parent ID from parents array
func extractParentID(parents []string) string {
	if len(parents) > 0 {
		return parents[0]
	}
	return ""
}

// MatchedFolder represents a folder that matches a search pattern
type MatchedFolder struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	FileCount   int    `json:"fileCount"`
	ParentID    string `json:"parentId,omitempty"`
	WebViewLink string `json:"webViewLink,omitempty"`
}

// FindFoldersByNameResult represents the result of folder search by name
type FindFoldersByNameResult struct {
	MatchedFolders []MatchedFolder `json:"matchedFolders"`
	TotalCount     int             `json:"totalCount"`
	TotalSize      int64           `json:"totalSize"`
	TotalFiles     int             `json:"totalFiles"`
	SearchPattern  string          `json:"searchPattern"`
	SearchTime     string          `json:"searchTime"`
}

// FindFoldersByName searches for folders matching the given name pattern recursively
func (uc *FolderAnalysisUseCase) FindFoldersByName(ctx context.Context, rootFolderID string, folderNamePattern string) (*FindFoldersByNameResult, error) {
	log.Printf("🔍 폴더명 검색 시작: '%s' (루트: %s)", folderNamePattern, rootFolderID)

	// Create progress tracking
	progress, err := uc.progressService.StartOperation(ctx, "find_folders_by_name", 0)
	if err != nil {
		log.Printf("⚠️ 진행 상황 생성 실패: %v", err)
	}

	defer func() {
		if progress != nil {
			uc.progressService.CompleteOperation(ctx, progress.ID)
		}
	}()

	var matchedFolders []MatchedFolder
	var mu sync.Mutex
	var scannedCount int32

	// Recursive search function
	// Returns true if this folder matched (so parent should not recurse into it)
	var searchRecursive func(folderID, parentPath string) error
	searchRecursive = func(folderID, parentPath string) error {
		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Update progress
		scanned := atomic.AddInt32(&scannedCount, 1)
		if progress != nil && scanned%100 == 0 {
			uc.progressService.UpdateOperation(ctx, progress.ID, 0, fmt.Sprintf("스캔 중: %d개 폴더 확인됨", scanned))
			log.Printf("🔍 스캔 진행 중: %d개 폴더 확인됨", scanned)
		}

		// Get folder contents
		contents, err := uc.storageProvider.ListFiles(ctx, folderID)
		if err != nil {
			return fmt.Errorf("폴더 내용 조회 실패 (%s): %w", folderID, err)
		}

		// Process each item
		for _, item := range contents {
			if item.MimeType == "application/vnd.google-apps.folder" {
				currentPath := item.Name
				if parentPath != "" {
					currentPath = parentPath + "/" + item.Name
				}

				// Check if folder name matches pattern (case-insensitive exact match)
				if matchesFolderName(item.Name, folderNamePattern) {
					// 매칭된 폴더 발견! 통계는 나중에 계산 (빠른 검색 우선)
					mu.Lock()
					matchedFolders = append(matchedFolders, MatchedFolder{
						ID:          item.ID,
						Name:        item.Name,
						Path:        currentPath,
						Size:        0, // 나중에 계산
						FileCount:   0, // 나중에 계산
						ParentID:    folderID,
						WebViewLink: item.WebViewLink,
					})
					matchedCount := len(matchedFolders)
					mu.Unlock()
					log.Printf("✅ 매칭된 폴더 발견 (#%d): %s", matchedCount, currentPath)

					// ⚠️ 매칭된 폴더 내부는 탐색하지 않음 (삭제 대상이므로)
					continue
				}

				// 매칭되지 않은 폴더만 하위 탐색
				err := searchRecursive(item.ID, currentPath)
				if err != nil {
					// Log but continue searching other folders
					log.Printf("⚠️ 하위 폴더 검색 실패 (%s): %v", item.Name, err)
				}
			}
		}

		return nil
	}

	// Start recursive search
	err = searchRecursive(rootFolderID, "")
	if err != nil {
		if progress != nil {
			uc.progressService.FailOperation(ctx, progress.ID, err.Error())
		}
		return nil, fmt.Errorf("폴더 검색 실패: %w", err)
	}

	log.Printf("📊 검색 완료, %d개 폴더 발견. 통계 계산 중...", len(matchedFolders))

	// 통계 계산 (병렬 처리)
	var totalSize int64
	var totalFiles int
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // 동시 5개 제한

	for i := range matchedFolders {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			size, fileCount, err := uc.calculateFolderStats(ctx, matchedFolders[idx].ID)
			if err != nil {
				log.Printf("⚠️ 폴더 통계 계산 실패 (%s): %v", matchedFolders[idx].Name, err)
				return
			}

			mu.Lock()
			matchedFolders[idx].Size = size
			matchedFolders[idx].FileCount = fileCount
			totalSize += size
			totalFiles += fileCount
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	result := &FindFoldersByNameResult{
		MatchedFolders: matchedFolders,
		TotalCount:     len(matchedFolders),
		TotalSize:      totalSize,
		TotalFiles:     totalFiles,
		SearchPattern:  folderNamePattern,
		SearchTime:     time.Now().Format("2006-01-02 15:04:05"),
	}

	log.Printf("✅ 폴더명 검색 완료: '%s' 패턴에 %d개 폴더 발견 (총 %d개 파일, %s)",
		folderNamePattern, len(matchedFolders), totalFiles, formatSize(totalSize))

	return result, nil
}

// matchesFolderName checks if folder name matches the pattern (case-insensitive, exact match)
func matchesFolderName(folderName, pattern string) bool {
	// Exact match (case-insensitive)
	return strings.EqualFold(folderName, pattern)
}

// calculateFolderStats calculates total size and file count for a folder recursively
func (uc *FolderAnalysisUseCase) calculateFolderStats(ctx context.Context, folderID string) (int64, int, error) {
	var totalSize int64
	var fileCount int

	// Get folder contents
	contents, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return 0, 0, err
	}

	for _, item := range contents {
		if item.MimeType == "application/vnd.google-apps.folder" {
			// Recursively calculate subfolder stats
			subSize, subCount, err := uc.calculateFolderStats(ctx, item.ID)
			if err != nil {
				log.Printf("⚠️ 하위 폴더 통계 계산 실패 (%s): %v", item.Name, err)
				continue
			}
			totalSize += subSize
			fileCount += subCount
		} else {
			totalSize += item.Size
			fileCount++
		}
	}

	return totalSize, fileCount, nil
}

// FolderSearchProgress represents progress update during folder search
type FolderSearchProgress struct {
	Type           string                   `json:"type"`           // "progress", "found", "stats", "complete", "error"
	ScannedFolders int                      `json:"scannedFolders"` // 검색된 폴더 수
	MatchedCount   int                      `json:"matchedCount"`   // 발견된 매칭 폴더 수
	CurrentPath    string                   `json:"currentPath"`    // 현재 검색 중인 경로
	MatchedFolder  *MatchedFolder           `json:"matchedFolder"`  // 발견된 폴더 (type="found"일 때)
	Result         *FindFoldersByNameResult `json:"result"`         // 최종 결과 (type="complete"일 때)
	Error          string                   `json:"error"`          // 에러 메시지 (type="error"일 때)
}

// FindFoldersByNameWithCallback searches for folders with progress callback for SSE
// skipStats: true면 통계 계산 생략 (빠른 검색)
func (uc *FolderAnalysisUseCase) FindFoldersByNameWithCallback(
	ctx context.Context,
	rootFolderID string,
	folderNamePattern string,
	skipStats bool,
	progressCallback func(progress FolderSearchProgress),
) (*FindFoldersByNameResult, error) {
	skipStatsText := ""
	if skipStats {
		skipStatsText = " (통계 생략)"
	}
	log.Printf("🔍 폴더명 검색 시작 (SSE): '%s' (루트: %s)%s", folderNamePattern, rootFolderID, skipStatsText)

	var matchedFolders []MatchedFolder
	var mu sync.Mutex
	var scannedCount int32
	var errorCount int32

	// Recursive search function - returns nil to continue search even on errors
	var searchRecursive func(folderID, parentPath string)
	searchRecursive = func(folderID, parentPath string) {
		// Panic recovery to prevent search from stopping unexpectedly
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔴 검색 중 패닉 복구: 폴더=%s, 패닉=%v", folderID, r)
				atomic.AddInt32(&errorCount, 1)
			}
		}()

		// Check context cancellation
		if ctx.Err() != nil {
			log.Printf("⚠️ 컨텍스트 취소됨, 검색 중단: %v", ctx.Err())
			return
		}

		// Update progress
		scanned := atomic.AddInt32(&scannedCount, 1)
		if scanned%50 == 0 {
			mu.Lock()
			matchedCount := len(matchedFolders)
			mu.Unlock()
			progressCallback(FolderSearchProgress{
				Type:           "progress",
				ScannedFolders: int(scanned),
				MatchedCount:   matchedCount,
				CurrentPath:    parentPath,
			})
		}

		// Get folder contents with retry
		var contents []*entities.File
		var err error
		maxRetries := 3
		for retry := 0; retry < maxRetries; retry++ {
			contents, err = uc.storageProvider.ListFiles(ctx, folderID)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				log.Printf("⚠️ 컨텍스트 취소됨: %v", ctx.Err())
				return
			}
			log.Printf("⚠️ 폴더 내용 조회 실패 (시도 %d/%d, 폴더=%s): %v", retry+1, maxRetries, folderID, err)
			if retry < maxRetries-1 {
				time.Sleep(time.Duration(retry+1) * time.Second) // 지수 백오프
			}
		}
		if err != nil {
			log.Printf("❌ 폴더 내용 조회 최종 실패 (%s): %v - 계속 진행", folderID, err)
			atomic.AddInt32(&errorCount, 1)
			return // 이 폴더는 건너뛰고 다른 폴더 계속 검색
		}

		// Process each item
		for _, item := range contents {
			// Check context cancellation periodically
			if ctx.Err() != nil {
				return
			}

			if item.MimeType == "application/vnd.google-apps.folder" {
				currentPath := item.Name
				if parentPath != "" {
					currentPath = parentPath + "/" + item.Name
				}

				// Check if folder name matches pattern
				if matchesFolderName(item.Name, folderNamePattern) {
					matched := MatchedFolder{
						ID:          item.ID,
						Name:        item.Name,
						Path:        currentPath,
						Size:        0,
						FileCount:   0,
						ParentID:    folderID,
						WebViewLink: item.WebViewLink,
					}
					mu.Lock()
					matchedFolders = append(matchedFolders, matched)
					matchedCount := len(matchedFolders)
					mu.Unlock()

					// 발견 알림
					progressCallback(FolderSearchProgress{
						Type:           "found",
						ScannedFolders: int(atomic.LoadInt32(&scannedCount)),
						MatchedCount:   matchedCount,
						MatchedFolder:  &matched,
					})

					continue
				}

				// 매칭되지 않은 폴더만 하위 탐색
				searchRecursive(item.ID, currentPath)
			}
		}
	}

	// Start recursive search
	log.Printf("🔍 루트 폴더에서 재귀 검색 시작: %s", rootFolderID)
	searchRecursive(rootFolderID, "")

	// Check if context was cancelled
	if ctx.Err() != nil {
		log.Printf("⚠️ 검색이 컨텍스트 취소로 중단됨: %v (스캔: %d, 에러: %d)", ctx.Err(), scannedCount, errorCount)
		progressCallback(FolderSearchProgress{
			Type:  "error",
			Error: fmt.Sprintf("검색 취소됨: %v", ctx.Err()),
		})
		return nil, ctx.Err()
	}

	log.Printf("📊 검색 완료: 스캔=%d, 매칭=%d, 에러=%d", scannedCount, len(matchedFolders), errorCount)

	var totalSize int64
	var totalFiles int

	if skipStats {
		log.Printf("⚡ 검색 완료, %d개 폴더 발견. (통계 계산 생략)", len(matchedFolders))
	} else {
		log.Printf("📊 검색 완료, %d개 폴더 발견. 통계 계산 중...", len(matchedFolders))

		// 통계 계산 진행 상황 알림
		progressCallback(FolderSearchProgress{
			Type:           "stats",
			ScannedFolders: int(scannedCount),
			MatchedCount:   len(matchedFolders),
			CurrentPath:    "통계 계산 중...",
		})

		// 통계 계산 (병렬 처리)
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for i := range matchedFolders {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				size, fileCount, err := uc.calculateFolderStats(ctx, matchedFolders[idx].ID)
				if err != nil {
					log.Printf("⚠️ 폴더 통계 계산 실패 (%s): %v", matchedFolders[idx].Name, err)
					return
				}

				mu.Lock()
				matchedFolders[idx].Size = size
				matchedFolders[idx].FileCount = fileCount
				totalSize += size
				totalFiles += fileCount
				mu.Unlock()
			}(i)
		}
		wg.Wait()
	}

	result := &FindFoldersByNameResult{
		MatchedFolders: matchedFolders,
		TotalCount:     len(matchedFolders),
		TotalSize:      totalSize,
		TotalFiles:     totalFiles,
		SearchPattern:  folderNamePattern,
		SearchTime:     time.Now().Format("2006-01-02 15:04:05"),
	}

	// 완료 알림
	progressCallback(FolderSearchProgress{
		Type:           "complete",
		ScannedFolders: int(scannedCount),
		MatchedCount:   len(matchedFolders),
		Result:         result,
	})

	log.Printf("✅ 폴더명 검색 완료 (SSE): '%s' 패턴에 %d개 폴더 발견", folderNamePattern, len(matchedFolders))

	return result, nil
}

// formatSize formats bytes to human-readable size
func formatSize(bytes int64) string {
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

// DeleteFoldersByNameRequest represents the request to delete folders by name
type DeleteFoldersByNameRequest struct {
	FolderIDs       []string `json:"folderIds"`
	PermanentDelete bool     `json:"permanentDelete,omitempty"`
}

// DeleteFoldersByNameResult represents the result of deleting folders by name
type DeleteFoldersByNameResult struct {
	SuccessCount int      `json:"successCount"`
	FailedCount  int      `json:"failedCount"`
	TotalCount   int      `json:"totalCount"`
	DeletedSize  int64    `json:"deletedSize"`
	FailedIDs    []string `json:"failedIds,omitempty"`
}

// DeleteFoldersByName deletes folders matching specific IDs (with all contents)
func (uc *FolderAnalysisUseCase) DeleteFoldersByName(ctx context.Context, folderIDs []string, permanentDelete bool) (*DeleteFoldersByNameResult, error) {
	deleteMethod := "휴지통 이동"
	if permanentDelete {
		deleteMethod = "완전 삭제"
	}
	log.Printf("🗑️ 특정 폴더명 삭제 시작: %d개 폴더 (삭제 방식: %s)", len(folderIDs), deleteMethod)

	result := &DeleteFoldersByNameResult{
		TotalCount: len(folderIDs),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Max 5 concurrent deletions

	for _, folderID := range folderIDs {
		wg.Add(1)
		go func(fID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Delete folder recursively (with all contents)
			err := uc.deleteExcludedFolderRecursiveWithOptions(ctx, fID, permanentDelete)
			if err != nil {
				log.Printf("❌ 폴더 삭제 실패 (%s): %v", fID, err)
				mu.Lock()
				result.FailedCount++
				result.FailedIDs = append(result.FailedIDs, fID)
				mu.Unlock()
			} else {
				log.Printf("✅ 폴더 삭제 성공: %s", fID)
				mu.Lock()
				result.SuccessCount++
				mu.Unlock()
			}
		}(folderID)
	}

	wg.Wait()

	log.Printf("✅ 특정 폴더명 삭제 완료: %d개 성공, %d개 실패 (%s)",
		result.SuccessCount, result.FailedCount, deleteMethod)

	return result, nil
}
