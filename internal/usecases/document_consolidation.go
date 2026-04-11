package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go-drive-duplicates/internal/domain/services"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DocumentConsolidationUseCase handles document consolidation operations
type DocumentConsolidationUseCase struct {
	storageProvider services.StorageProvider

	// 활성 작업 관리
	activeJobs   map[string]*ConsolidationJob
	activeJobsMu sync.RWMutex
}

// NewDocumentConsolidationUseCase creates a new document consolidation use case
func NewDocumentConsolidationUseCase(
	storageProvider services.StorageProvider,
) *DocumentConsolidationUseCase {
	return &DocumentConsolidationUseCase{
		storageProvider: storageProvider,
		activeJobs:      make(map[string]*ConsolidationJob),
	}
}

// ConsolidationRequest represents the request to start consolidation
type ConsolidationRequest struct {
	FolderID          string   `json:"folderId"`
	IncludeSubfolders bool     `json:"includeSubfolders"`
	MaxFileSizeMB     int      `json:"maxFileSizeMB"`
	FileExtensions    []string `json:"fileExtensions"`
}

// ConsolidationResult represents the result of a consolidation
type ConsolidationResult struct {
	TotalFilesScanned int              `json:"totalFilesScanned"`
	TextFilesFound    int              `json:"textFilesFound"`
	DuplicatesRemoved int              `json:"duplicatesRemoved"`
	UniqueDocuments   int              `json:"uniqueDocuments"`
	OutputFiles       []OutputFileInfo `json:"outputFiles"`
	TotalInputSize    int64            `json:"totalInputSize"`
	TotalOutputSize   int64            `json:"totalOutputSize"`
}

// OutputFileInfo represents info about an output file
type OutputFileInfo struct {
	FileID        string `json:"fileId"`
	FileName      string `json:"fileName"`
	FileSize      int64  `json:"fileSize"`
	WebViewLink   string `json:"webViewLink"`
	DocumentCount int    `json:"documentCount"`
}

// ConsolidationJob represents an active consolidation job
type ConsolidationJob struct {
	ID        string               `json:"jobId"`
	Status    string               `json:"status"` // running, completed, failed, cancelled
	Phase     string               `json:"phase"`  // scanning, downloading, deduplicating, merging, uploading, completed
	Message   string               `json:"message"`
	Progress  *ConsolidationProgress `json:"progress"`
	Result    *ConsolidationResult   `json:"result,omitempty"`
	Error     string               `json:"error,omitempty"`
	StartTime time.Time            `json:"startTime"`
	EndTime   time.Time            `json:"endTime,omitempty"`

	// 내부 사용
	cancelFunc context.CancelFunc
	cancelled  int32
	mu         sync.Mutex
}

// ConsolidationProgress represents progress of the consolidation
type ConsolidationProgress struct {
	TotalFiles      int   `json:"totalFiles"`
	ProcessedFiles  int32 `json:"processedFiles"`
	TextFilesFound  int32 `json:"textFilesFound"`
	DownloadedFiles int32 `json:"downloadedFiles"`
	DuplicatesFound int32 `json:"duplicatesFound"`
	TotalInputSize  int64 `json:"totalInputSize"`
}

// TextFileEntry represents a downloaded text file with its content
type TextFileEntry struct {
	FileID   string
	FileName string
	FilePath string
	Content  string
	Size     int64
	Hash     string
}

// 텍스트 MIME 타입 목록
var textMimeTypes = map[string]bool{
	"text/plain":                        true, // .txt, .log
	"text/csv":                          true, // .csv
	"text/markdown":                     true, // .md
	"text/xml":                          true, // .xml
	"application/json":                  true, // .json
	"application/xml":                   true, // .xml
	"text/html":                         true, // .html
	"text/x-python":                     true, // .py
	"text/x-java-source":               true, // .java
	"application/javascript":            true, // .js
	"application/x-yaml":               true, // .yaml
	"text/yaml":                         true, // .yaml
	"text/x-shellscript":               true, // .sh
	"application/x-sh":                 true, // .sh
}

// 텍스트 파일 확장자 목록 (MIME 타입이 없거나 부정확한 경우 사용)
var textExtensions = map[string]bool{
	".txt": true, ".csv": true, ".log": true, ".md": true,
	".json": true, ".xml": true, ".html": true, ".htm": true,
	".py": true, ".java": true, ".js": true, ".ts": true,
	".yaml": true, ".yml": true, ".sh": true, ".bat": true,
	".cfg": true, ".ini": true, ".conf": true, ".properties": true,
}

// StartConsolidation starts a consolidation job in the background
func (uc *DocumentConsolidationUseCase) StartConsolidation(
	ctx context.Context,
	req *ConsolidationRequest,
) (*ConsolidationJob, error) {
	if req.FolderID == "" {
		return nil, fmt.Errorf("폴더 ID가 필요합니다")
	}

	if req.MaxFileSizeMB <= 0 {
		req.MaxFileSizeMB = 10 // 기본값 10MB
	}

	// Create job
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &ConsolidationJob{
		ID:        fmt.Sprintf("consolidate_%d", time.Now().UnixNano()),
		Status:    "running",
		Phase:     "scanning",
		Message:   "텍스트 파일 탐색 중...",
		StartTime: time.Now(),
		Progress: &ConsolidationProgress{},
		cancelFunc: cancel,
	}

	// Register job
	uc.activeJobsMu.Lock()
	uc.activeJobs[job.ID] = job
	uc.activeJobsMu.Unlock()

	log.Printf("📄 문서 통합 작업 시작: %s (폴더: %s)", job.ID, req.FolderID)

	// Start processing in background
	go uc.processConsolidation(jobCtx, job, req)

	return job, nil
}

// processConsolidation processes the consolidation job
func (uc *DocumentConsolidationUseCase) processConsolidation(
	ctx context.Context,
	job *ConsolidationJob,
	req *ConsolidationRequest,
) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 문서 통합 중 패닉 복구: %v", r)
			job.mu.Lock()
			job.Status = "failed"
			job.Error = fmt.Sprintf("예기치 않은 오류: %v", r)
			job.EndTime = time.Now()
			job.mu.Unlock()
		}
	}()

	defer func() {
		job.mu.Lock()
		job.EndTime = time.Now()
		if atomic.LoadInt32(&job.cancelled) == 1 {
			job.Status = "cancelled"
			job.Message = "작업이 취소되었습니다"
		}
		job.mu.Unlock()
	}()

	// 1. 텍스트 파일 목록 수집
	textFiles, err := uc.scanTextFiles(ctx, job, req)
	if err != nil {
		uc.failJob(job, "텍스트 파일 탐색 실패: "+err.Error())
		return
	}

	if uc.isCancelled(job) {
		return
	}

	if len(textFiles) == 0 {
		uc.failJob(job, "텍스트 파일을 찾을 수 없습니다")
		return
	}

	log.Printf("📄 텍스트 파일 %d개 발견", len(textFiles))

	// 2. 파일 다운로드 및 텍스트 추출
	entries, err := uc.downloadAndExtractTexts(ctx, job, textFiles)
	if err != nil {
		uc.failJob(job, "파일 다운로드 실패: "+err.Error())
		return
	}

	if uc.isCancelled(job) {
		return
	}

	// 3. 중복 제거
	job.mu.Lock()
	job.Phase = "deduplicating"
	job.Message = "중복 내용 제거 중..."
	job.mu.Unlock()

	uniqueEntries := uc.deduplicateByContent(entries)
	duplicatesRemoved := len(entries) - len(uniqueEntries)
	atomic.StoreInt32(&job.Progress.DuplicatesFound, int32(duplicatesRemoved))

	log.Printf("📄 중복 제거: %d개 중 %d개 고유 문서 (%d개 중복 제거)",
		len(entries), len(uniqueEntries), duplicatesRemoved)

	if uc.isCancelled(job) {
		return
	}

	// 4. Markdown으로 병합
	job.mu.Lock()
	job.Phase = "merging"
	job.Message = "Markdown 파일 생성 중..."
	job.mu.Unlock()

	folderInfo, err := uc.storageProvider.GetFile(ctx, req.FolderID)
	folderName := req.FolderID
	if err == nil && folderInfo != nil {
		folderName = folderInfo.Name
	}

	mdContents := uc.mergeToMarkdown(uniqueEntries, folderName, len(entries), duplicatesRemoved)

	// 5. 용량 초과 시 분할
	maxSizeBytes := int64(req.MaxFileSizeMB) * 1024 * 1024
	splitFiles := uc.splitBySize(mdContents, uniqueEntries, maxSizeBytes, folderName, len(entries), duplicatesRemoved)

	if uc.isCancelled(job) {
		return
	}

	// 6. Google Drive에 업로드
	job.mu.Lock()
	job.Phase = "uploading"
	job.Message = fmt.Sprintf("Google Drive에 업로드 중... (0/%d)", len(splitFiles))
	job.mu.Unlock()

	outputFiles, err := uc.uploadResults(ctx, job, req.FolderID, splitFiles)
	if err != nil {
		uc.failJob(job, "업로드 실패: "+err.Error())
		return
	}

	// 7. 완료
	var totalOutputSize int64
	for _, f := range outputFiles {
		totalOutputSize += f.FileSize
	}

	result := &ConsolidationResult{
		TotalFilesScanned: int(job.Progress.TotalFiles),
		TextFilesFound:    int(atomic.LoadInt32(&job.Progress.TextFilesFound)),
		DuplicatesRemoved: duplicatesRemoved,
		UniqueDocuments:   len(uniqueEntries),
		OutputFiles:       outputFiles,
		TotalInputSize:    atomic.LoadInt64(&job.Progress.TotalInputSize),
		TotalOutputSize:   totalOutputSize,
	}

	job.mu.Lock()
	job.Phase = "completed"
	job.Status = "completed"
	job.Message = fmt.Sprintf("통합 완료: %d개 고유 문서 → %d개 파일", len(uniqueEntries), len(outputFiles))
	job.Result = result
	job.mu.Unlock()

	log.Printf("✅ 문서 통합 완료: %s (입력: %d개, 고유: %d개, 출력: %d개 파일)",
		job.ID, len(entries), len(uniqueEntries), len(outputFiles))
}

// scanTextFiles scans the folder for text files
func (uc *DocumentConsolidationUseCase) scanTextFiles(
	ctx context.Context,
	job *ConsolidationJob,
	req *ConsolidationRequest,
) ([]TextFileEntry, error) {
	var files []*struct {
		ID       string
		Name     string
		MimeType string
		Size     int64
	}

	// 폴더 내 파일 목록 수집
	allFiles, err := uc.storageProvider.ListFilesRecursive(ctx, req.FolderID, req.IncludeSubfolders)
	if err != nil {
		return nil, fmt.Errorf("폴더 파일 목록 조회 실패: %w", err)
	}

	job.mu.Lock()
	job.Progress.TotalFiles = len(allFiles)
	job.mu.Unlock()

	// 텍스트 파일 필터링
	extensionFilter := make(map[string]bool)
	for _, ext := range req.FileExtensions {
		ext = strings.ToLower(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extensionFilter[ext] = true
	}

	for _, f := range allFiles {
		// 폴더는 건너뛰기
		if f.MimeType == "application/vnd.google-apps.folder" {
			continue
		}

		// Google Docs/Sheets/Slides 등은 건너뛰기
		if strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
			continue
		}

		isText := false

		// 확장자 필터가 있으면 확장자 기준으로 필터링
		if len(extensionFilter) > 0 {
			ext := getFileExtension(f.Name)
			if extensionFilter[ext] {
				isText = true
			}
		} else {
			// 확장자 필터가 없으면 MIME 타입 + 확장자로 판단
			if textMimeTypes[f.MimeType] {
				isText = true
			} else {
				ext := getFileExtension(f.Name)
				if textExtensions[ext] {
					isText = true
				}
			}
		}

		if isText {
			files = append(files, &struct {
				ID       string
				Name     string
				MimeType string
				Size     int64
			}{
				ID:       f.ID,
				Name:     f.Name,
				MimeType: f.MimeType,
				Size:     f.Size,
			})
		}
	}

	atomic.StoreInt32(&job.Progress.TextFilesFound, int32(len(files)))

	job.mu.Lock()
	job.Message = fmt.Sprintf("텍스트 파일 %d개 발견 (전체 %d개 중)", len(files), len(allFiles))
	job.mu.Unlock()

	// TextFileEntry 목록으로 변환
	var textFiles []TextFileEntry
	for _, f := range files {
		textFiles = append(textFiles, TextFileEntry{
			FileID:   f.ID,
			FileName: f.Name,
			Size:     f.Size,
		})
	}

	return textFiles, nil
}

// downloadAndExtractTexts downloads files and extracts text content
func (uc *DocumentConsolidationUseCase) downloadAndExtractTexts(
	ctx context.Context,
	job *ConsolidationJob,
	textFiles []TextFileEntry,
) ([]TextFileEntry, error) {
	job.mu.Lock()
	job.Phase = "downloading"
	job.Message = fmt.Sprintf("파일 다운로드 중... (0/%d)", len(textFiles))
	job.mu.Unlock()

	var entries []TextFileEntry
	var entriesMu sync.Mutex

	// 동시 다운로드 제한 (5개씩)
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for _, tf := range textFiles {
		if uc.isCancelled(job) {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(file TextFileEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️ 파일 다운로드 중 패닉 복구: %s - %v", file.FileName, r)
				}
			}()

			content, err := uc.downloadFileContent(ctx, file.FileID)
			if err != nil {
				log.Printf("⚠️ 파일 다운로드 실패 (건너뜀): %s - %v", file.FileName, err)
				// 개별 파일 실패는 건너뛰기
				return
			}

			if content == "" {
				return // 빈 파일 건너뛰기
			}

			// 해시 계산
			hash := sha256.Sum256([]byte(content))
			hashStr := hex.EncodeToString(hash[:])

			entriesMu.Lock()
			entries = append(entries, TextFileEntry{
				FileID:   file.FileID,
				FileName: file.FileName,
				FilePath: file.FilePath,
				Content:  content,
				Size:     int64(len(content)),
				Hash:     hashStr,
			})
			entriesMu.Unlock()

			downloaded := atomic.AddInt32(&job.Progress.DownloadedFiles, 1)
			atomic.AddInt64(&job.Progress.TotalInputSize, int64(len(content)))

			job.mu.Lock()
			job.Message = fmt.Sprintf("파일 다운로드 중... (%d/%d)", downloaded, len(textFiles))
			job.mu.Unlock()
		}(tf)
	}

	wg.Wait()

	return entries, nil
}

// downloadFileContent downloads a file and returns its text content
func (uc *DocumentConsolidationUseCase) downloadFileContent(
	ctx context.Context,
	fileID string,
) (string, error) {
	reader, err := uc.storageProvider.DownloadFile(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("파일 다운로드 실패: %w", err)
	}
	defer reader.Close()

	// 최대 50MB까지 읽기
	limitReader := io.LimitReader(reader, 50*1024*1024)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return "", fmt.Errorf("파일 읽기 실패: %w", err)
	}

	return string(data), nil
}

// deduplicateByContent removes duplicate files based on content hash
func (uc *DocumentConsolidationUseCase) deduplicateByContent(entries []TextFileEntry) []TextFileEntry {
	seen := make(map[string]bool)
	var unique []TextFileEntry

	for _, entry := range entries {
		if !seen[entry.Hash] {
			seen[entry.Hash] = true
			unique = append(unique, entry)
		}
	}

	return unique
}

// mergeToMarkdown merges unique text entries into a Markdown document
func (uc *DocumentConsolidationUseCase) mergeToMarkdown(
	entries []TextFileEntry,
	folderName string,
	totalFiles int,
	duplicatesRemoved int,
) string {
	var sb strings.Builder

	// 헤더
	sb.WriteString("# 문서 통합 결과\n\n")
	sb.WriteString(fmt.Sprintf("> 생성일: %s | 원본 폴더: %s\n",
		time.Now().Format("2006-01-02 15:04:05"), folderName))
	sb.WriteString(fmt.Sprintf("> 총 %d개 문서 중 %d개 고유 문서 통합 (중복 %d개 제거)\n\n",
		totalFiles, len(entries), duplicatesRemoved))
	sb.WriteString("---\n\n")

	// 각 문서 내용 추가
	for i, entry := range entries {
		sb.WriteString(fmt.Sprintf("## 📄 %s\n", entry.FileName))
		sb.WriteString(fmt.Sprintf("> 크기: %s\n\n", consolidateFormatSize(entry.Size)))

		// 파일 내용을 코드 블록 안에 넣기 (확장자에 따라 언어 힌트 추가)
		ext := getFileExtension(entry.FileName)
		langHint := getLanguageHint(ext)

		sb.WriteString(fmt.Sprintf("```%s\n", langHint))
		sb.WriteString(entry.Content)
		if !strings.HasSuffix(entry.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")

		if i < len(entries)-1 {
			sb.WriteString("---\n\n")
		}
	}

	return sb.String()
}

// splitBySize splits content into multiple files if it exceeds maxSize
func (uc *DocumentConsolidationUseCase) splitBySize(
	fullContent string,
	entries []TextFileEntry,
	maxSizeBytes int64,
	folderName string,
	totalFiles int,
	duplicatesRemoved int,
) []SplitFile {
	// 전체 내용이 제한 이내이면 분할 불필요
	if int64(len(fullContent)) <= maxSizeBytes {
		return []SplitFile{{
			FileName:      "consolidated_1.md",
			Content:       fullContent,
			DocumentCount: len(entries),
		}}
	}

	// 문서 단위로 분할
	var files []SplitFile
	fileNum := 1
	var currentContent strings.Builder
	currentDocCount := 0

	for i, entry := range entries {
		// 이 문서를 추가할 섹션 생성
		var section strings.Builder
		section.WriteString(fmt.Sprintf("## 📄 %s\n", entry.FileName))
		section.WriteString(fmt.Sprintf("> 크기: %s\n\n", consolidateFormatSize(entry.Size)))

		ext := getFileExtension(entry.FileName)
		langHint := getLanguageHint(ext)
		section.WriteString(fmt.Sprintf("```%s\n", langHint))
		section.WriteString(entry.Content)
		if !strings.HasSuffix(entry.Content, "\n") {
			section.WriteString("\n")
		}
		section.WriteString("```\n\n")
		if i < len(entries)-1 {
			section.WriteString("---\n\n")
		}

		sectionStr := section.String()

		// 현재 파일에 추가하면 크기 초과하는지 확인
		headerSize := uc.estimateHeaderSize(folderName, totalFiles, len(entries), duplicatesRemoved, fileNum)
		if currentContent.Len()+len(sectionStr)+headerSize > int(maxSizeBytes) && currentDocCount > 0 {
			// 현재까지의 내용으로 파일 생성
			header := uc.createSplitHeader(folderName, totalFiles, len(entries), duplicatesRemoved, fileNum, len(files)+1)
			files = append(files, SplitFile{
				FileName:      fmt.Sprintf("consolidated_%d.md", fileNum),
				Content:       header + currentContent.String(),
				DocumentCount: currentDocCount,
			})
			fileNum++
			currentContent.Reset()
			currentDocCount = 0
		}

		currentContent.WriteString(sectionStr)
		currentDocCount++
	}

	// 마지막 파일
	if currentDocCount > 0 {
		header := uc.createSplitHeader(folderName, totalFiles, len(entries), duplicatesRemoved, fileNum, len(files)+1)
		files = append(files, SplitFile{
			FileName:      fmt.Sprintf("consolidated_%d.md", fileNum),
			Content:       header + currentContent.String(),
			DocumentCount: currentDocCount,
		})
	}

	// 분할 파일 수 정보를 헤더에 추가 (최종 파일 수를 알게 된 후)
	totalParts := len(files)
	for i := range files {
		files[i].Content = strings.Replace(files[i].Content,
			fmt.Sprintf("파트 %d", i+1),
			fmt.Sprintf("파트 %d/%d", i+1, totalParts),
			1)
	}

	return files
}

// SplitFile represents a split output file
type SplitFile struct {
	FileName      string
	Content       string
	DocumentCount int
}

// uploadResults uploads the result files to Google Drive
func (uc *DocumentConsolidationUseCase) uploadResults(
	ctx context.Context,
	job *ConsolidationJob,
	folderID string,
	files []SplitFile,
) ([]OutputFileInfo, error) {
	var outputFiles []OutputFileInfo

	for i, file := range files {
		if uc.isCancelled(job) {
			break
		}

		job.mu.Lock()
		job.Message = fmt.Sprintf("Google Drive에 업로드 중... (%d/%d)", i+1, len(files))
		job.mu.Unlock()

		reader := strings.NewReader(file.Content)
		fileID, err := uc.storageProvider.UploadFile(
			ctx,
			folderID,
			file.FileName,
			"text/markdown",
			reader,
		)
		if err != nil {
			return nil, fmt.Errorf("파일 업로드 실패 (%s): %w", file.FileName, err)
		}

		// 업로드된 파일 정보 조회
		fileInfo, err := uc.storageProvider.GetFile(ctx, fileID)
		webViewLink := ""
		if err == nil && fileInfo != nil {
			webViewLink = fileInfo.WebViewLink
		}

		outputFiles = append(outputFiles, OutputFileInfo{
			FileID:        fileID,
			FileName:      file.FileName,
			FileSize:      int64(len(file.Content)),
			WebViewLink:   webViewLink,
			DocumentCount: file.DocumentCount,
		})

		log.Printf("📤 통합 파일 업로드 완료: %s (ID: %s, %s)", file.FileName, fileID, formatSize(int64(len(file.Content))))
	}

	return outputFiles, nil
}

// GetJobProgress returns the progress of a consolidation job
func (uc *DocumentConsolidationUseCase) GetJobProgress(jobID string) (*ConsolidationJob, bool) {
	uc.activeJobsMu.RLock()
	defer uc.activeJobsMu.RUnlock()

	job, exists := uc.activeJobs[jobID]
	return job, exists
}

// CancelJob cancels a consolidation job
func (uc *DocumentConsolidationUseCase) CancelJob(jobID string) error {
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

	log.Printf("🛑 문서 통합 작업 취소 요청: %s", jobID)
	return nil
}

// Helper functions

func (uc *DocumentConsolidationUseCase) failJob(job *ConsolidationJob, errMsg string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.Status = "failed"
	job.Error = errMsg
	job.Message = errMsg
	log.Printf("❌ 문서 통합 실패: %s - %s", job.ID, errMsg)
}

func (uc *DocumentConsolidationUseCase) isCancelled(job *ConsolidationJob) bool {
	return atomic.LoadInt32(&job.cancelled) == 1
}

func (uc *DocumentConsolidationUseCase) createSplitHeader(
	folderName string, totalFiles, uniqueFiles, duplicatesRemoved, partNum, _ int,
) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 문서 통합 결과 (파트 %d)\n\n", partNum))
	sb.WriteString(fmt.Sprintf("> 생성일: %s | 원본 폴더: %s\n",
		time.Now().Format("2006-01-02 15:04:05"), folderName))
	sb.WriteString(fmt.Sprintf("> 총 %d개 문서 중 %d개 고유 문서 통합 (중복 %d개 제거)\n\n",
		totalFiles, uniqueFiles, duplicatesRemoved))
	sb.WriteString("---\n\n")
	return sb.String()
}

func (uc *DocumentConsolidationUseCase) estimateHeaderSize(
	folderName string, totalFiles, uniqueFiles, duplicatesRemoved, partNum int,
) int {
	// 헤더 크기 대략적 추정
	return len(folderName) + 200 + len(fmt.Sprintf("%d%d%d%d", totalFiles, uniqueFiles, duplicatesRemoved, partNum))
}

func getFileExtension(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(name[idx:])
}

func getLanguageHint(ext string) string {
	hints := map[string]string{
		".py":         "python",
		".java":       "java",
		".js":         "javascript",
		".ts":         "typescript",
		".go":         "go",
		".json":       "json",
		".xml":        "xml",
		".html":       "html",
		".htm":        "html",
		".css":        "css",
		".yaml":       "yaml",
		".yml":        "yaml",
		".sh":         "bash",
		".bat":        "batch",
		".md":         "markdown",
		".csv":        "csv",
		".sql":        "sql",
		".properties": "properties",
		".ini":        "ini",
		".cfg":        "ini",
		".conf":       "conf",
	}
	if hint, ok := hints[ext]; ok {
		return hint
	}
	return ""
}

func consolidateFormatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	k := float64(1024)
	i := 0
	size := float64(bytes)
	for size >= k && i < len(sizes)-1 {
		size /= k
		i++
	}
	return fmt.Sprintf("%.1f %s", size, sizes[i])
}
