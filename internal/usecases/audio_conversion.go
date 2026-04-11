package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/services"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// AudioConversionUseCase handles audio file format conversion
type AudioConversionUseCase struct {
	storageProvider services.StorageProvider

	activeJobs   map[string]*AudioConversionJob
	activeJobsMu sync.RWMutex
}

// NewAudioConversionUseCase creates a new audio conversion use case
func NewAudioConversionUseCase(storageProvider services.StorageProvider) *AudioConversionUseCase {
	return &AudioConversionUseCase{
		storageProvider: storageProvider,
		activeJobs:      make(map[string]*AudioConversionJob),
	}
}

// AudioConversionOptions represents options for audio conversion
type AudioConversionOptions struct {
	OutputFormat      string   `json:"outputFormat"`      // mp3, wav, flac, aac, ogg, wma, m4a
	Bitrate           string   `json:"bitrate"`           // 128k, 192k, 256k, 320k (empty = default)
	SampleRate        string   `json:"sampleRate"`        // 44100, 48000 (empty = keep original)
	OutputFolderID    string   `json:"outputFolderId"`    // empty = same folder as original
	OutputFolderURL   string   `json:"outputFolderUrl"`
	DeleteOriginal    bool     `json:"deleteOriginal"`    // delete original after conversion
	IncludeSubfolders bool     `json:"includeSubfolders"` // include subfolders
	SourceExtensions  []string `json:"sourceExtensions"`  // e.g. ["3gp","amr","mp4"] - empty = audio/* MIME only
}

// AudioConversionJob represents a running audio conversion job
type AudioConversionJob struct {
	ID             string                  `json:"jobId"`
	Status         string                  `json:"status"` // running, completed, failed, cancelled
	TotalFiles     int                     `json:"totalFiles"`
	CompletedFiles int                     `json:"completedFiles"`
	FailedFiles    int                     `json:"failedFiles"`
	CurrentFile    *AudioFileProgress      `json:"currentFile,omitempty"`
	Results        []AudioConversionResult `json:"results"`
	OutputFormat   string                  `json:"outputFormat"`
	StartTime      time.Time               `json:"startTime"`
	EndTime        time.Time               `json:"endTime,omitempty"`

	cancelFunc context.CancelFunc
	cancelled  int32
	mu         sync.Mutex
}

// AudioFileProgress represents progress for a single file
type AudioFileProgress struct {
	FileName string `json:"fileName"`
	Phase    string `json:"phase"` // scanning, downloading, converting, uploading
	Message  string `json:"message"`
}

// AudioConversionResult represents the result for a single file
type AudioConversionResult struct {
	OriginalName   string `json:"originalName"`
	ConvertedName  string `json:"convertedName"`
	OriginalSize   int64  `json:"originalSize"`
	ConvertedSize  int64  `json:"convertedSize"`
	UploadedFileID string `json:"uploadedFileId,omitempty"`
	Status         string `json:"status"` // completed, failed, skipped
	Error          string `json:"error,omitempty"`
}

// AudioFormatInfo describes a supported audio format
type AudioFormatInfo struct {
	Extension string `json:"extension"`
	MimeType  string `json:"mimeType"`
	Label     string `json:"label"`
}

// audioMimeMap maps output format extensions to MIME types
var audioMimeMap = map[string]string{
	"mp3":  "audio/mpeg",
	"wav":  "audio/wav",
	"flac": "audio/flac",
	"aac":  "audio/aac",
	"ogg":  "audio/ogg",
	"m4a":  "audio/mp4",
	"wma":  "audio/x-ms-wma",
}

// GetSupportedFormats returns the list of supported audio formats
func (uc *AudioConversionUseCase) GetSupportedFormats() []AudioFormatInfo {
	return []AudioFormatInfo{
		{Extension: "mp3", MimeType: "audio/mpeg", Label: "MP3"},
		{Extension: "wav", MimeType: "audio/wav", Label: "WAV"},
		{Extension: "flac", MimeType: "audio/flac", Label: "FLAC"},
		{Extension: "aac", MimeType: "audio/aac", Label: "AAC"},
		{Extension: "ogg", MimeType: "audio/ogg", Label: "OGG Vorbis"},
		{Extension: "m4a", MimeType: "audio/mp4", Label: "M4A (AAC)"},
		{Extension: "wma", MimeType: "audio/x-ms-wma", Label: "WMA"},
	}
}

// GetSupportedSourceFormats returns the list of supported source audio formats
func (uc *AudioConversionUseCase) GetSupportedSourceFormats() []AudioFormatInfo {
	return []AudioFormatInfo{
		{Extension: "3gp", MimeType: "video/3gpp", Label: "3GP"},
		{Extension: "amr", MimeType: "audio/amr", Label: "AMR"},
		{Extension: "mp4", MimeType: "video/mp4", Label: "MP4 (Audio)"},
		{Extension: "webm", MimeType: "video/webm", Label: "WebM"},
		{Extension: "opus", MimeType: "audio/opus", Label: "Opus"},
		{Extension: "mp3", MimeType: "audio/mpeg", Label: "MP3"},
		{Extension: "wav", MimeType: "audio/wav", Label: "WAV"},
		{Extension: "flac", MimeType: "audio/flac", Label: "FLAC"},
		{Extension: "aac", MimeType: "audio/aac", Label: "AAC"},
		{Extension: "ogg", MimeType: "audio/ogg", Label: "OGG"},
		{Extension: "m4a", MimeType: "audio/mp4", Label: "M4A"},
		{Extension: "wma", MimeType: "audio/x-ms-wma", Label: "WMA"},
	}
}

// StartConversion starts an audio conversion job
func (uc *AudioConversionUseCase) StartConversion(
	ctx context.Context,
	folderURL string,
	folderID string,
	options *AudioConversionOptions,
) (*AudioConversionJob, error) {
	// Determine folder ID
	fID := folderID
	if fID == "" && folderURL != "" {
		id, err := extractFolderIDFromURL(folderURL)
		if err != nil {
			return nil, err
		}
		fID = id
	}
	if fID == "" {
		return nil, fmt.Errorf("폴더 ID 또는 URL이 필요합니다")
	}

	// Validate output format
	if _, ok := audioMimeMap[options.OutputFormat]; !ok {
		return nil, fmt.Errorf("지원하지 않는 출력 형식입니다: %s", options.OutputFormat)
	}

	// Check ffmpeg availability
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg가 설치되어 있지 않습니다. ffmpeg를 설치해주세요")
	}

	// Determine output folder ID
	outputFolderID := fID // default: same folder
	if options.OutputFolderID != "" {
		outputFolderID = options.OutputFolderID
	} else if options.OutputFolderURL != "" {
		id, err := extractFolderIDFromURL(options.OutputFolderURL)
		if err != nil {
			return nil, fmt.Errorf("출력 폴더 URL 파싱 실패: %w", err)
		}
		outputFolderID = id
	}

	// Create job
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &AudioConversionJob{
		ID:           fmt.Sprintf("audio_convert_%d", time.Now().UnixNano()),
		Status:       "running",
		OutputFormat: options.OutputFormat,
		StartTime:    time.Now(),
		Results:      make([]AudioConversionResult, 0),
		cancelFunc:   cancel,
	}

	// Register job
	uc.activeJobsMu.Lock()
	uc.activeJobs[job.ID] = job
	uc.activeJobsMu.Unlock()

	log.Printf("🎵 오디오 변환 작업 시작: %s (폴더: %s, 형식: %s)", job.ID, fID, options.OutputFormat)

	// Start processing in background
	go uc.processConversion(jobCtx, job, fID, outputFolderID, options)

	return job, nil
}

// processConversion processes audio files in the folder
func (uc *AudioConversionUseCase) processConversion(
	ctx context.Context,
	job *AudioConversionJob,
	sourceFolderID string,
	outputFolderID string,
	options *AudioConversionOptions,
) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("audio_convert_%s_", job.ID))
	if err != nil {
		job.mu.Lock()
		job.Status = "failed"
		job.EndTime = time.Now()
		job.mu.Unlock()
		log.Printf("❌ 임시 디렉토리 생성 실패: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	defer func() {
		job.mu.Lock()
		job.EndTime = time.Now()
		if atomic.LoadInt32(&job.cancelled) == 1 {
			job.Status = "cancelled"
		} else if job.FailedFiles > 0 && job.CompletedFiles == 0 {
			job.Status = "failed"
		} else {
			job.Status = "completed"
		}
		job.CurrentFile = nil
		job.mu.Unlock()
		log.Printf("🎵 오디오 변환 작업 완료: %s (성공: %d, 실패: %d)",
			job.ID, job.CompletedFiles, job.FailedFiles)
	}()

	// Phase 1: Scan for audio files
	job.mu.Lock()
	job.CurrentFile = &AudioFileProgress{
		Phase:   "scanning",
		Message: "오디오 파일 스캔 중...",
	}
	job.mu.Unlock()

	var files []*struct {
		ID       string
		Name     string
		Size     int64
		ParentID string
	}

	var allFiles []*struct {
		ID       string
		Name     string
		Size     int64
		MimeType string
		ParentID string
	}

	if options.IncludeSubfolders {
		driveFiles, err := uc.storageProvider.ListFilesRecursive(ctx, sourceFolderID, true)
		if err != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.mu.Unlock()
			log.Printf("❌ 파일 목록 조회 실패: %v", err)
			return
		}
		for _, f := range driveFiles {
			parentID := ""
			if len(f.Parents) > 0 {
				parentID = f.Parents[0]
			}
			allFiles = append(allFiles, &struct {
				ID       string
				Name     string
				Size     int64
				MimeType string
				ParentID string
			}{
				ID:       f.ID,
				Name:     f.Name,
				Size:     f.Size,
				MimeType: f.MimeType,
				ParentID: parentID,
			})
		}
	} else {
		driveFiles, err := uc.storageProvider.ListFiles(ctx, sourceFolderID)
		if err != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.mu.Unlock()
			log.Printf("❌ 파일 목록 조회 실패: %v", err)
			return
		}
		for _, f := range driveFiles {
			parentID := ""
			if len(f.Parents) > 0 {
				parentID = f.Parents[0]
			}
			allFiles = append(allFiles, &struct {
				ID       string
				Name     string
				Size     int64
				MimeType string
				ParentID string
			}{
				ID:       f.ID,
				Name:     f.Name,
				Size:     f.Size,
				MimeType: f.MimeType,
				ParentID: parentID,
			})
		}
	}

	// Build source extension set for fast lookup
	sourceExtSet := make(map[string]bool)
	for _, ext := range options.SourceExtensions {
		sourceExtSet[strings.ToLower(ext)] = true
	}

	// Filter audio files
	for _, f := range allFiles {
		matchesMime := strings.HasPrefix(f.MimeType, "audio/")
		matchesExt := false
		if len(sourceExtSet) > 0 {
			fileExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Name), "."))
			matchesExt = sourceExtSet[fileExt]
		}

		if matchesMime || matchesExt {
			files = append(files, &struct {
				ID       string
				Name     string
				Size     int64
				ParentID string
			}{
				ID:       f.ID,
				Name:     f.Name,
				Size:     f.Size,
				ParentID: f.ParentID,
			})
		}
	}

	if len(files) == 0 {
		job.mu.Lock()
		job.Status = "completed"
		job.mu.Unlock()
		log.Printf("⚠️ 오디오 파일이 없습니다 (폴더: %s)", sourceFolderID)
		return
	}

	job.mu.Lock()
	job.TotalFiles = len(files)
	job.mu.Unlock()

	log.Printf("🎵 오디오 파일 %d개 발견 (폴더: %s)", len(files), sourceFolderID)

	// Phase 2: Process each file
	for _, file := range files {
		// Check cancellation
		if atomic.LoadInt32(&job.cancelled) == 1 {
			log.Printf("🛑 오디오 변환 작업 취소됨: %s", job.ID)
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		result := uc.convertSingleFile(ctx, job, file.ID, file.Name, file.Size, file.ParentID, outputFolderID, tmpDir, options)

		job.mu.Lock()
		job.Results = append(job.Results, result)
		if result.Status == "completed" {
			job.CompletedFiles++
		} else if result.Status == "failed" {
			job.FailedFiles++
		}
		job.mu.Unlock()
	}
}

// convertSingleFile downloads, converts, and uploads a single audio file
func (uc *AudioConversionUseCase) convertSingleFile(
	ctx context.Context,
	job *AudioConversionJob,
	fileID, fileName string,
	fileSize int64,
	parentFolderID string,
	outputFolderID string,
	tmpDir string,
	options *AudioConversionOptions,
) AudioConversionResult {
	result := AudioConversionResult{
		OriginalName: fileName,
		OriginalSize: fileSize,
	}

	// Determine output file name
	ext := filepath.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)
	newFileName := baseName + "." + options.OutputFormat
	result.ConvertedName = newFileName

	// Skip if same format
	if strings.EqualFold(strings.TrimPrefix(ext, "."), options.OutputFormat) {
		result.Status = "skipped"
		result.Error = "이미 동일한 형식입니다"
		log.Printf("⏭️ 건너뜀 (동일 형식): %s", fileName)
		return result
	}

	// Phase: downloading
	job.mu.Lock()
	job.CurrentFile = &AudioFileProgress{
		FileName: fileName,
		Phase:    "downloading",
		Message:  fmt.Sprintf("다운로드 중: %s", fileName),
	}
	job.mu.Unlock()

	// Download file
	reader, err := uc.storageProvider.DownloadFile(ctx, fileID)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("다운로드 실패: %v", err)
		log.Printf("❌ 다운로드 실패: %s - %v", fileName, err)
		return result
	}

	inputPath := filepath.Join(tmpDir, fmt.Sprintf("input_%s_%s", fileID, fileName))
	inputFile, err := os.Create(inputPath)
	if err != nil {
		reader.Close()
		result.Status = "failed"
		result.Error = fmt.Sprintf("임시 파일 생성 실패: %v", err)
		return result
	}

	_, err = io.Copy(inputFile, reader)
	reader.Close()
	inputFile.Close()
	if err != nil {
		os.Remove(inputPath)
		result.Status = "failed"
		result.Error = fmt.Sprintf("파일 저장 실패: %v", err)
		return result
	}

	// Phase: converting
	job.mu.Lock()
	job.CurrentFile = &AudioFileProgress{
		FileName: fileName,
		Phase:    "converting",
		Message:  fmt.Sprintf("변환 중: %s → %s", fileName, options.OutputFormat),
	}
	job.mu.Unlock()

	outputPath := filepath.Join(tmpDir, fmt.Sprintf("output_%s_%s", fileID, newFileName))

	// Build ffmpeg command
	args := []string{"-i", inputPath, "-y"}
	if options.Bitrate != "" {
		args = append(args, "-b:a", options.Bitrate)
	}
	if options.SampleRate != "" {
		args = append(args, "-ar", options.SampleRate)
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmdOutput, err := cmd.CombinedOutput()
	os.Remove(inputPath) // Clean up input file

	if err != nil {
		os.Remove(outputPath)
		errMsg := string(cmdOutput)
		if len(errMsg) > 200 {
			errMsg = errMsg[len(errMsg)-200:]
		}
		result.Status = "failed"
		result.Error = fmt.Sprintf("변환 실패: %s", errMsg)
		log.Printf("❌ 변환 실패: %s - %v", fileName, err)
		return result
	}

	// Get converted file size
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		os.Remove(outputPath)
		result.Status = "failed"
		result.Error = fmt.Sprintf("변환 파일 정보 조회 실패: %v", err)
		return result
	}
	result.ConvertedSize = outputInfo.Size()

	// Phase: uploading
	job.mu.Lock()
	job.CurrentFile = &AudioFileProgress{
		FileName: fileName,
		Phase:    "uploading",
		Message:  fmt.Sprintf("업로드 중: %s", newFileName),
	}
	job.mu.Unlock()

	// Determine target folder
	targetFolderID := outputFolderID
	if targetFolderID == "" {
		targetFolderID = parentFolderID
	}

	// Upload converted file
	outputFile, err := os.Open(outputPath)
	if err != nil {
		os.Remove(outputPath)
		result.Status = "failed"
		result.Error = fmt.Sprintf("변환 파일 열기 실패: %v", err)
		return result
	}

	mimeType := audioMimeMap[options.OutputFormat]
	uploadedID, err := uc.storageProvider.UploadFile(ctx, targetFolderID, newFileName, mimeType, outputFile)
	outputFile.Close()
	os.Remove(outputPath) // Clean up output file

	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("업로드 실패: %v", err)
		log.Printf("❌ 업로드 실패: %s - %v", newFileName, err)
		return result
	}

	result.UploadedFileID = uploadedID
	result.Status = "completed"

	log.Printf("✅ 변환 완료: %s → %s (%.1f MB → %.1f MB)",
		fileName, newFileName,
		float64(fileSize)/(1024*1024),
		float64(result.ConvertedSize)/(1024*1024))

	// Delete original if requested
	if options.DeleteOriginal {
		if err := uc.storageProvider.TrashFile(ctx, fileID); err != nil {
			log.Printf("⚠️ 원본 파일 휴지통 이동 실패: %s - %v", fileName, err)
		} else {
			log.Printf("🗑️ 원본 파일 휴지통 이동: %s", fileName)
		}
	}

	return result
}

// GetJobProgress returns the progress of an audio conversion job
func (uc *AudioConversionUseCase) GetJobProgress(jobID string) (*AudioConversionJob, bool) {
	uc.activeJobsMu.RLock()
	defer uc.activeJobsMu.RUnlock()

	job, exists := uc.activeJobs[jobID]
	return job, exists
}

// CancelJob cancels an audio conversion job
func (uc *AudioConversionUseCase) CancelJob(jobID string) error {
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

	log.Printf("🛑 오디오 변환 작업 취소 요청: %s", jobID)
	return nil
}
