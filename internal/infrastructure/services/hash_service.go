package services

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	"hash"
	"io"
	"strings"
	"sync"
	"time"
)

type HashService struct {
	storageProvider services.StorageProvider
	algorithm       string
	bufferSize      int
	workerCount     int
	maxFileSize     int64 // Skip files larger than this size (0 = no limit)
}

type HashCalculationResult struct {
	File  *entities.File
	Hash  string
	Error error
}

func NewHashService(storageProvider services.StorageProvider, algorithm string) services.HashService {
	return &HashService{
		storageProvider: storageProvider,
		algorithm:       algorithm,
		bufferSize:      64 * 1024, // 64KB buffer
		workerCount:     4,
		maxFileSize:     100 * 1024 * 1024, // 100MB default limit
	}
}

func (h *HashService) CalculateFileHash(ctx context.Context, file *entities.File) (string, error) {
	return h.CalculateHash(ctx, file)
}

func (h *HashService) CalculateHash(ctx context.Context, file *entities.File) (string, error) {
	// Skip files that are too large
	if h.maxFileSize > 0 && file.Size > h.maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes (limit: %d bytes)", file.Size, h.maxFileSize)
	}

	// Skip folders and Google Apps files
	if h.isNonHashableFile(file) {
		return "", fmt.Errorf("file type not hashable: %s", file.MimeType)
	}

	// Download file content
	reader, err := h.storageProvider.DownloadFile(ctx, file.ID)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %v", err)
	}
	defer reader.Close()

	// Create hasher based on algorithm
	hasher, err := h.createHasher()
	if err != nil {
		return "", err
	}

	// Calculate hash with buffer
	buffer := make([]byte, h.bufferSize)
	totalRead := int64(0)

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			hasher.Write(buffer[:n])
			totalRead += int64(n)

			// Check file size limit during reading
			if h.maxFileSize > 0 && totalRead > h.maxFileSize {
				return "", fmt.Errorf("file size exceeded limit during reading: %d bytes", totalRead)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading file: %v", err)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}

	// Return hash as hex string
	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}

func (h *HashService) CalculateHashForFiles(ctx context.Context, files []*entities.File, progressCallback func(processed, total int)) ([]*HashCalculationResult, error) {
	if len(files) == 0 {
		return nil, nil
	}

	// Filter out non-hashable files
	hashableFiles := make([]*entities.File, 0, len(files))
	for _, file := range files {
		if !h.isNonHashableFile(file) && (h.maxFileSize == 0 || file.Size <= h.maxFileSize) {
			hashableFiles = append(hashableFiles, file)
		}
	}

	if len(hashableFiles) == 0 {
		return nil, nil
	}

	// Create worker pool
	jobChan := make(chan *entities.File, len(hashableFiles))
	resultChan := make(chan *HashCalculationResult, len(hashableFiles))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < h.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.hashWorker(ctx, jobChan, resultChan)
		}()
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for _, file := range hashableFiles {
			select {
			case jobChan <- file:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	var results []*HashCalculationResult
	processed := 0

	go func() {
		defer close(resultChan)
		wg.Wait()
	}()

	for result := range resultChan {
		results = append(results, result)
		processed++

		if progressCallback != nil {
			progressCallback(processed, len(hashableFiles))
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
	}

	return results, nil
}

func (h *HashService) ValidateFileIntegrity(ctx context.Context, file *entities.File, expectedHash string) (bool, error) {
	calculatedHash, err := h.CalculateHash(ctx, file)
	if err != nil {
		return false, err
	}

	return calculatedHash == expectedHash, nil
}

func (h *HashService) GetSupportedAlgorithms() []string {
	return []string{"md5", "sha1", "sha256"}
}

func (h *HashService) SetAlgorithm(algorithm string) error {
	supportedAlgorithms := h.GetSupportedAlgorithms()
	for _, supported := range supportedAlgorithms {
		if algorithm == supported {
			h.algorithm = algorithm
			return nil
		}
	}
	return fmt.Errorf("unsupported algorithm: %s", algorithm)
}

func (h *HashService) SetWorkerCount(count int) {
	if count > 0 {
		h.workerCount = count
	}
}

func (h *HashService) SetMaxFileSize(size int64) {
	h.maxFileSize = size
}

func (h *HashService) SetBufferSize(size int) {
	if size > 0 {
		h.bufferSize = size
	}
}

// Worker function for hash calculation
func (h *HashService) hashWorker(ctx context.Context, jobChan <-chan *entities.File, resultChan chan<- *HashCalculationResult) {
	for {
		select {
		case file, ok := <-jobChan:
			if !ok {
				return
			}

			hash, err := h.CalculateHash(ctx, file)
			result := &HashCalculationResult{
				File:  file,
				Hash:  hash,
				Error: err,
			}

			select {
			case resultChan <- result:
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// Helper functions

func (h *HashService) createHasher() (hash.Hash, error) {
	switch h.algorithm {
	case "md5":
		return md5.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", h.algorithm)
	}
}

func (h *HashService) isNonHashableFile(file *entities.File) bool {
	// Skip folders
	if file.MimeType == "application/vnd.google-apps.folder" {
		return true
	}

	// Skip Google Apps native files (they don't have content to hash)
	googleAppsMimeTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.script",
		"application/vnd.google-apps.site",
		"application/vnd.google-apps.jam",
		"application/vnd.google-apps.shortcut",
	}

	for _, googleType := range googleAppsMimeTypes {
		if file.MimeType == googleType {
			return true
		}
	}

	// Skip zero-size files
	if file.Size == 0 {
		return true
	}

	return false
}

// Additional utility methods

func (h *HashService) EstimateHashingTime(files []*entities.File) time.Duration {
	hashableFiles := make([]*entities.File, 0, len(files))
	totalSize := int64(0)

	for _, file := range files {
		if !h.isNonHashableFile(file) && (h.maxFileSize == 0 || file.Size <= h.maxFileSize) {
			hashableFiles = append(hashableFiles, file)
			totalSize += file.Size
		}
	}

	if len(hashableFiles) == 0 {
		return 0
	}

	// Estimate based on typical processing speed (adjust based on testing)
	// Assuming ~50MB/s processing speed with network overhead
	estimatedSpeed := int64(50 * 1024 * 1024) // 50MB/s
	estimatedSeconds := totalSize / estimatedSpeed

	// Add overhead for worker coordination and API calls
	overhead := time.Duration(len(hashableFiles)/10) * time.Second

	return time.Duration(estimatedSeconds)*time.Second + overhead
}

func (h *HashService) GetHashingStats(files []*entities.File) map[string]interface{} {
	hashableCount := 0
	skippedCount := 0
	totalSize := int64(0)
	largeFileCount := 0

	for _, file := range files {
		if h.isNonHashableFile(file) {
			skippedCount++
		} else if h.maxFileSize > 0 && file.Size > h.maxFileSize {
			largeFileCount++
			skippedCount++
		} else {
			hashableCount++
			totalSize += file.Size
		}
	}

	return map[string]interface{}{
		"total_files":    len(files),
		"hashable_files": hashableCount,
		"skipped_files":  skippedCount,
		"large_files":    largeFileCount,
		"total_size":     totalSize,
		"estimated_time": h.EstimateHashingTime(files),
		"algorithm":      h.algorithm,
		"worker_count":   h.workerCount,
		"max_file_size":  h.maxFileSize,
		"buffer_size":    h.bufferSize,
	}
}

// Quick hash for small files or preview purposes
func (h *HashService) CalculateQuickHash(ctx context.Context, file *entities.File, sampleSize int64) (string, error) {
	if sampleSize <= 0 {
		sampleSize = 1024 // 1KB sample
	}

	if file.Size <= sampleSize {
		// For small files, calculate full hash
		return h.CalculateHash(ctx, file)
	}

	// For large files, sample from beginning
	reader, err := h.storageProvider.DownloadFile(ctx, file.ID)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %v", err)
	}
	defer reader.Close()

	hasher, err := h.createHasher()
	if err != nil {
		return "", err
	}

	// Read only the sample size
	buffer := make([]byte, sampleSize)
	n, err := io.ReadFull(reader, buffer)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("error reading file sample: %v", err)
	}

	hasher.Write(buffer[:n])
	hashBytes := hasher.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// CalculateFileHashes calculates hashes for multiple files
func (h *HashService) CalculateFileHashes(ctx context.Context, files []*entities.File) error {
	results, err := h.CalculateHashForFiles(ctx, files, nil)
	if err != nil {
		return err
	}

	// Update file hashes (this would typically be done via repository)
	for _, result := range results {
		if result.Error == nil {
			result.File.Hash = result.Hash
			result.File.HashCalculated = true
		}
	}

	return nil
}

// CalculateHashFromReader calculates hash from an io.Reader
func (h *HashService) CalculateHashFromReader(ctx context.Context, reader io.Reader) (string, error) {
	hasher, err := h.createHasher()
	if err != nil {
		return "", err
	}

	buffer := make([]byte, h.bufferSize)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			hasher.Write(buffer[:n])
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading: %v", err)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}

// CalculateHashesBatch calculates hashes for files in batches
func (h *HashService) CalculateHashesBatch(ctx context.Context, files []*entities.File, workerCount int) error {
	h.SetWorkerCount(workerCount)
	return h.CalculateFileHashes(ctx, files)
}

// CalculateHashesParallel calculates hashes in parallel
func (h *HashService) CalculateHashesParallel(ctx context.Context, files []*entities.File, workerCount int) error {
	h.SetWorkerCount(workerCount)
	return h.CalculateFileHashes(ctx, files)
}

// CalculateHashesWithProgress calculates hashes with progress callback
func (h *HashService) CalculateHashesWithProgress(ctx context.Context, files []*entities.File, workerCount int, progressCallback func(processed, total int)) error {
	h.SetWorkerCount(workerCount)
	_, err := h.CalculateHashForFiles(ctx, files, progressCallback)
	return err
}

// SetHashAlgorithm sets the hash algorithm (alias for SetAlgorithm)
func (h *HashService) SetHashAlgorithm(algorithm string) error {
	return h.SetAlgorithm(algorithm)
}

// GetCurrentAlgorithm returns the current algorithm
func (h *HashService) GetCurrentAlgorithm() string {
	return h.algorithm
}

// ValidateHash validates a hash string format
func (h *HashService) ValidateHash(hash string) error {
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}

	var expectedLength int
	switch h.algorithm {
	case "md5":
		expectedLength = 32
	case "sha1":
		expectedLength = 40
	case "sha256":
		expectedLength = 64
	default:
		return fmt.Errorf("unknown algorithm: %s", h.algorithm)
	}

	if len(hash) != expectedLength {
		return fmt.Errorf("invalid hash length for %s: expected %d, got %d", h.algorithm, expectedLength, len(hash))
	}

	// Check if hash contains only hex characters
	for _, char := range hash {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return fmt.Errorf("hash contains invalid character: %c", char)
		}
	}

	return nil
}

// CompareHashes compares two hash strings
func (h *HashService) CompareHashes(hash1, hash2 string) bool {
	return strings.ToLower(hash1) == strings.ToLower(hash2)
}

// GetOptimalWorkerCount returns optimal worker count based on CPU cores
func (h *HashService) GetOptimalWorkerCount() int {
	return h.workerCount
}

// EstimateCalculationTime estimates time needed for hash calculation
func (h *HashService) EstimateCalculationTime(files []*entities.File) int {
	duration := h.EstimateHashingTime(files)
	return int(duration.Seconds())
}

// CalculateWithRetry calculates hash with retry logic
func (h *HashService) CalculateWithRetry(ctx context.Context, file *entities.File, maxRetries int) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		hash, err := h.CalculateHash(ctx, file)
		if err == nil {
			return hash, nil
		}

		lastErr = err
		if attempt < maxRetries {
			// Exponential backoff
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}
	}

	return "", fmt.Errorf("hash calculation failed after %d retries: %v", maxRetries, lastErr)
}
