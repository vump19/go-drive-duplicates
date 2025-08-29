package config

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/infrastructure/repositories/sqlite"
	infraServices "go-drive-duplicates/internal/infrastructure/services"
	"go-drive-duplicates/internal/interfaces/controllers"
	"go-drive-duplicates/internal/usecases"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// Container holds all dependencies for the application
type Container struct {
	// Configuration
	Config *Config

	// Database
	DB *sqlx.DB

	// Repositories
	FileRepo             repositories.FileRepository
	DuplicateGroupRepo   repositories.DuplicateRepository
	ProgressRepo         repositories.ProgressRepository
	ComparisonResultRepo repositories.ComparisonRepository

	// Services
	StorageProvider services.StorageProvider
	HashService     services.HashService
	FileService     services.FileService
	ProgressService services.ProgressService

	// Use Cases
	FileScanningUseCase     *usecases.FileScanningUseCase
	DuplicateFindingUseCase *usecases.DuplicateFindingUseCase
	FolderComparisonUseCase *usecases.FolderComparisonUseCase
	FileCleanupUseCase      *usecases.FileCleanupUseCase

	// Controllers
	FileController       *controllers.FileController
	DuplicateController  *controllers.DuplicateController
	ComparisonController *controllers.ComparisonController
	CleanupController    *controllers.CleanupController
}

// NewContainer creates and initializes a new dependency injection container
func NewContainer(config *Config) (*Container, error) {
	container := &Container{
		Config: config,
	}

	if err := container.initializeDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	if err := container.initializeRepositories(); err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %v", err)
	}

	if err := container.initializeServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %v", err)
	}

	if err := container.initializeUseCases(); err != nil {
		return nil, fmt.Errorf("failed to initialize use cases: %v", err)
	}

	if err := container.initializeControllers(); err != nil {
		return nil, fmt.Errorf("failed to initialize controllers: %v", err)
	}

	return container, nil
}

func (c *Container) initializeDatabase() error {
	var err error
	c.DB, err = sqlx.Connect("sqlite3", c.Config.Database.Path)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Configure connection pool
	c.DB.SetMaxOpenConns(c.Config.Database.MaxOpenConns)
	c.DB.SetMaxIdleConns(c.Config.Database.MaxIdleConns)

	// Enable foreign keys and other pragmas
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000",  // 64MB cache
		"PRAGMA busy_timeout = 30000", // 30 second timeout
	}

	for _, pragma := range pragmas {
		if _, err := c.DB.Exec(pragma); err != nil {
			log.Printf("Warning: Failed to set pragma %s: %v", pragma, err)
		}
	}

	return nil
}

func (c *Container) initializeRepositories() error {
	// Initialize repositories
	c.FileRepo = sqlite.NewFileRepository(c.DB)
	c.ProgressRepo = sqlite.NewProgressRepository(c.DB)
	c.DuplicateGroupRepo = sqlite.NewDuplicateGroupRepository(c.DB, c.FileRepo)
	c.ComparisonResultRepo = sqlite.NewComparisonResultRepository(c.DB, c.FileRepo)

	// Create database tables
	if err := c.createTables(); err != nil {
		return fmt.Errorf("failed to create database tables: %v", err)
	}

	return nil
}

func (c *Container) createTables() error {
	// Create tables in the correct order (considering foreign key dependencies)
	tables := []interface {
		CreateTables(ctx context.Context) error
	}{
		c.FileRepo.(*sqlite.FileRepository),
		c.ProgressRepo.(*sqlite.ProgressRepository),
		c.DuplicateGroupRepo.(*sqlite.DuplicateGroupRepository),
		c.ComparisonResultRepo.(*sqlite.ComparisonResultRepository),
	}

	for _, table := range tables {
		if err := table.CreateTables(context.Background()); err != nil {
			return err
		}
	}

	return nil
}

func (c *Container) initializeServices() error {
	var err error

	// Initialize Google Drive adapter (or mock if no credentials)
	if c.Config.GoogleDrive.CredentialsPath != "" || c.Config.GoogleDrive.APIKey != "" {
		c.StorageProvider, err = infraServices.NewGoogleDriveAdapter(
			c.Config.GoogleDrive.CredentialsPath,
			c.Config.GoogleDrive.APIKey,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize Google Drive adapter: %v", err)
		}
	} else {
		// Use mock storage provider for testing when no credentials are available
		c.StorageProvider = infraServices.NewMockStorageProvider()
		log.Printf("⚠️  Using mock storage provider (no Google Drive credentials configured)")
	}

	// Initialize hash service
	c.HashService = infraServices.NewHashService(c.StorageProvider, c.Config.Hash.Algorithm)

	// Configure hash service
	if hashSvc, ok := c.HashService.(*infraServices.HashService); ok {
		hashSvc.SetWorkerCount(c.Config.Hash.WorkerCount)
		hashSvc.SetMaxFileSize(c.Config.Hash.MaxFileSize)
		hashSvc.SetBufferSize(c.Config.Hash.BufferSize)
	}

	// Initialize domain services (these would be implemented as needed)
	c.FileService = NewFileService(c.FileRepo, c.StorageProvider)
	c.ProgressService = NewProgressService(c.ProgressRepo)

	return nil
}

func (c *Container) initializeUseCases() error {
	// Initialize use cases with proper dependencies
	c.FileScanningUseCase = usecases.NewFileScanningUseCase(
		c.FileRepo,
		c.ProgressRepo,
		c.StorageProvider,
		c.FileService,
		c.ProgressService,
	)

	c.DuplicateFindingUseCase = usecases.NewDuplicateFindingUseCase(
		c.FileRepo,
		c.DuplicateGroupRepo,
		c.ProgressRepo,
		c.HashService,
		nil, // DuplicateService - to be implemented
		c.ProgressService,
		c.StorageProvider,
	)

	c.FolderComparisonUseCase = usecases.NewFolderComparisonUseCase(
		c.FileRepo,
		c.ComparisonResultRepo,
		c.ProgressRepo,
		c.StorageProvider,
		c.HashService,
		nil, // ComparisonService - to be implemented
		c.ProgressService,
	)

	c.FileCleanupUseCase = usecases.NewFileCleanupUseCase(
		c.FileRepo,
		c.DuplicateGroupRepo,
		c.ComparisonResultRepo,
		c.ProgressRepo,
		c.StorageProvider,
		nil, // CleanupService - to be implemented
		c.ProgressService,
	)

	return nil
}

func (c *Container) initializeControllers() error {
	// Initialize controllers with use cases
	c.FileController = controllers.NewFileController(c.FileScanningUseCase)
	c.DuplicateController = controllers.NewDuplicateController(c.DuplicateFindingUseCase)
	c.ComparisonController = controllers.NewComparisonController(c.FolderComparisonUseCase)
	c.CleanupController = controllers.NewCleanupController(c.FileCleanupUseCase)

	return nil
}

// Close properly shuts down all resources
func (c *Container) Close() error {
	if c.DB != nil {
		return c.DB.Close()
	}
	return nil
}

// Health check methods

func (c *Container) CheckDatabaseHealth() error {
	return c.DB.Ping()
}

func (c *Container) CheckStorageHealth() error {
	// This could be extended to check Google Drive API connectivity
	return nil
}

// Additional helper methods for specific domain services

// NewFileService creates a file service implementation
func NewFileService(fileRepo repositories.FileRepository, storageProvider services.StorageProvider) services.FileService {
	return &fileService{
		fileRepo:        fileRepo,
		storageProvider: storageProvider,
	}
}

// NewProgressService creates a progress service implementation
func NewProgressService(progressRepo repositories.ProgressRepository) services.ProgressService {
	return &progressService{
		progressRepo: progressRepo,
	}
}

// fileService implements services.FileService
type fileService struct {
	fileRepo        repositories.FileRepository
	storageProvider services.StorageProvider
}

func (fs *fileService) ValidateFile(file *entities.File) error {
	if file.ID == "" {
		return fmt.Errorf("file ID is required")
	}
	if file.Name == "" {
		return fmt.Errorf("file name is required")
	}
	if file.Size < 0 {
		return fmt.Errorf("file size cannot be negative")
	}
	return nil
}

func (fs *fileService) EnrichFileMetadata(ctx context.Context, file *entities.File) error {
	// For now, just return nil as we don't have GetFileMetadata method
	// TODO: Implement when GetFileMetadata is available in storage provider
	return nil
}

func (fs *fileService) IsFileValid(file *entities.File) bool {
	return fs.ValidateFile(file) == nil
}

func (fs *fileService) BuildFileTree(ctx context.Context, files []*entities.File) (map[string][]*entities.File, error) {
	// Simple implementation: group files by parent folder
	tree := make(map[string][]*entities.File)

	for _, file := range files {
		if len(file.Parents) > 0 {
			parentID := file.Parents[0]
			tree[parentID] = append(tree[parentID], file)
		} else {
			// Root level files
			tree["root"] = append(tree["root"], file)
		}
	}

	return tree, nil
}

func (fs *fileService) CheckFileExists(ctx context.Context, fileID string) (bool, error) {
	// Simple stub implementation
	file, err := fs.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return false, err
	}
	return file != nil, nil
}

func (fs *fileService) CleanupDeletedFiles(ctx context.Context) (int, error) {
	// Simple stub implementation - in real implementation would clean up files marked as deleted
	return 0, nil
}

func (fs *fileService) CleanupEmptyFolders(ctx context.Context) (int, error) {
	// Simple stub implementation - in real implementation would clean up empty folders
	return 0, nil
}

func (fs *fileService) GenerateFileStatistics(ctx context.Context) (*entities.FileStatistics, error) {
	// Simple stub implementation - return empty statistics
	stats := entities.NewFileStatistics()
	return stats, nil
}

func (fs *fileService) GetStorageUsage(ctx context.Context) (used, total int64, err error) {
	// Simple stub implementation - return 0 values
	return 0, 0, nil
}

func (fs *fileService) RefreshFileMetadata(ctx context.Context, fileID string) (*entities.File, error) {
	// Simple stub implementation - just get existing file
	return fs.fileRepo.GetByID(ctx, fileID)
}

func (fs *fileService) ScanAllFiles(ctx context.Context) ([]*entities.File, error) {
	// Simple stub implementation - return all files from repository
	return fs.fileRepo.GetAll(ctx)
}

func (fs *fileService) ScanFiles(ctx context.Context, folderID string) ([]*entities.File, error) {
	// Simple stub implementation - return files by folder
	return fs.fileRepo.GetByParent(ctx, folderID)
}

func (fs *fileService) UpdateFilePaths(ctx context.Context, files []*entities.File) error {
	// Simple stub implementation - update file paths
	for _, file := range files {
		err := fs.fileRepo.Update(ctx, file)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fs *fileService) ValidateFileAccess(ctx context.Context, fileID string) error {
	// Simple stub implementation - check if file exists
	exists, err := fs.fileRepo.Exists(ctx, fileID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("file not found: %s", fileID)
	}
	return nil
}

// progressService implements services.ProgressService
type progressService struct {
	progressRepo repositories.ProgressRepository
}

func (ps *progressService) CreateProgress(operationType string, totalItems int) (*entities.Progress, error) {
	progress := entities.NewProgress(operationType, totalItems)
	return progress, nil
}

func (ps *progressService) UpdateProgress(progress *entities.Progress, processedItems int, currentStep string) {
	progress.UpdateProgress(processedItems, currentStep)
}

func (ps *progressService) CompleteProgress(progress *entities.Progress) {
	progress.Complete()
}

func (ps *progressService) FailProgress(progress *entities.Progress, err error) {
	progress.Fail(err.Error())
}

func (ps *progressService) IsProgressActive(progress *entities.Progress) bool {
	return progress.Status == "pending" || progress.Status == "running" || progress.Status == "in_progress"
}

func (ps *progressService) AddOperationLog(ctx context.Context, progressID int, message string) error {
	// Simple stub implementation - in a real implementation this might update metadata
	return nil
}

func (ps *progressService) AnalyzeOperationPerformance(ctx context.Context, operationType string, days int) (map[string]interface{}, error) {
	// Simple stub implementation - return basic analysis
	analysis := map[string]interface{}{
		"operation_type": operationType,
		"days":           days,
		"total_runs":     0,
		"avg_duration":   0.0,
		"success_rate":   100.0,
	}
	return analysis, nil
}

func (ps *progressService) BroadcastProgress(ctx context.Context, progress *entities.Progress) error {
	// Simple stub implementation - in real implementation would broadcast to websockets or channels
	return nil
}

func (ps *progressService) CancelOperation(ctx context.Context, progressID int) error {
	// Simple stub implementation - in real implementation would cancel the operation
	return nil
}

func (ps *progressService) CheckProgressHealth(ctx context.Context) (map[string]interface{}, error) {
	// Simple stub implementation - return healthy status
	health := map[string]interface{}{
		"status":            "healthy",
		"active_operations": 0,
		"stuck_operations":  0,
		"average_duration":  0.0,
	}
	return health, nil
}

func (ps *progressService) CleanupAllOldOperations(ctx context.Context, duration time.Duration) (int, error) {
	// Simple stub implementation - convert duration to days and delegate to repository
	days := int(duration.Hours() / 24)
	return ps.progressRepo.DeleteOlderThan(ctx, days)
}

func (ps *progressService) CleanupCompletedOperations(ctx context.Context, duration time.Duration) (int, error) {
	// Simple stub implementation - delete completed operations older than duration
	// For now, just delete all completed operations
	return ps.progressRepo.DeleteCompleted(ctx)
}

func (ps *progressService) CleanupFailedOperations(ctx context.Context, duration time.Duration) (int, error) {
	// Simple stub implementation - delete failed operations older than duration
	return ps.progressRepo.DeleteByStatus(ctx, "failed")
}

func (ps *progressService) CompleteOperation(ctx context.Context, progressID int) error {
	// Mark operation as completed in database
	log.Printf("✅ CompleteOperation 호출됨 - Progress ID: %d", progressID)
	
	// Get the current progress record
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		log.Printf("❌ CompleteOperation: 진행 상태 조회 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}
	
	if progress == nil {
		log.Printf("❌ CompleteOperation: 진행 상태를 찾을 수 없음 - ID: %d", progressID)
		return fmt.Errorf("progress not found: %d", progressID)
	}
	
	// Update status to completed
	progress.Status = "completed"
	progress.CurrentStep = "완료"
	progress.LastUpdated = time.Now()
	
	// Save to database
	err = ps.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("❌ CompleteOperation: 진행 상태 업데이트 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}
	
	log.Printf("✅ CompleteOperation 완료 - Progress ID: %d, Status: %s", progressID, progress.Status)
	return nil
}

func (ps *progressService) FailOperation(ctx context.Context, progressID int, errorMessage string) error {
	// Mark operation as failed in database
	log.Printf("❌ FailOperation 호출됨 - Progress ID: %d, Error: %s", progressID, errorMessage)
	
	// Get the current progress record
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		log.Printf("❌ FailOperation: 진행 상태 조회 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}
	
	if progress == nil {
		log.Printf("❌ FailOperation: 진행 상태를 찾을 수 없음 - ID: %d", progressID)
		return fmt.Errorf("progress not found: %d", progressID)
	}
	
	// Update status to failed
	progress.Status = "failed"
	progress.CurrentStep = fmt.Sprintf("실패: %s", errorMessage)
	progress.LastUpdated = time.Now()
	
	// Save to database
	err = ps.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("❌ FailOperation: 진행 상태 업데이트 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}
	
	log.Printf("✅ FailOperation 완료 - Progress ID: %d, Status: %s", progressID, progress.Status)
	return nil
}

func (ps *progressService) GetActiveOperations(ctx context.Context) ([]*entities.Progress, error) {
	// Simple stub implementation - get active operations from repository
	return ps.progressRepo.GetActiveOperations(ctx)
}

func (ps *progressService) GetAverageCompletionTime(ctx context.Context, operationType string) (time.Duration, error) {
	// Simple stub implementation - get seconds and convert to duration
	seconds, err := ps.progressRepo.GetAverageCompletionTime(ctx, operationType)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

func (ps *progressService) GetBottlenecks(ctx context.Context, operationType string) ([]string, error) {
	// Simple stub implementation - return empty bottlenecks for operation type
	return []string{}, nil
}

func (ps *progressService) GetLongRunningOperations(ctx context.Context, minutes int) ([]*entities.Progress, error) {
	// Simple stub implementation - delegate to repository
	return ps.progressRepo.GetLongRunningOperations(ctx, minutes)
}

func (ps *progressService) GetOperationLogs(ctx context.Context, progressID int) ([]string, error) {
	// Simple stub implementation - return empty logs
	return []string{}, nil
}

func (ps *progressService) GetOperationMetadata(ctx context.Context, progressID int) (map[string]interface{}, error) {
	// Simple stub implementation - return empty metadata
	return make(map[string]interface{}), nil
}

func (ps *progressService) GetOperationStatistics(ctx context.Context) (map[string]interface{}, error) {
	// Simple stub implementation - return basic statistics
	stats := map[string]interface{}{
		"total_operations":     0,
		"active_operations":    0,
		"completed_operations": 0,
		"failed_operations":    0,
	}
	return stats, nil
}

func (ps *progressService) GetOperationsByType(ctx context.Context, operationType string) ([]*entities.Progress, error) {
	// Simple stub implementation - delegate to repository
	return ps.progressRepo.GetByOperationType(ctx, operationType)
}

func (ps *progressService) GetProgress(ctx context.Context, progressID int) (*entities.Progress, error) {
	// Simple stub implementation - delegate to repository
	return ps.progressRepo.GetByID(ctx, progressID)
}

func (ps *progressService) GetRecentOperations(ctx context.Context, limit int) ([]*entities.Progress, error) {
	// Simple stub implementation - delegate to repository
	return ps.progressRepo.GetRecentOperations(ctx, limit)
}

func (ps *progressService) GetStuckOperations(ctx context.Context, timeoutMinutes int) ([]*entities.Progress, error) {
	// Simple stub implementation - delegate to repository
	return ps.progressRepo.GetStuckOperations(ctx, timeoutMinutes)
}

func (ps *progressService) GetSuccessRate(ctx context.Context, operationType string, days int) (float64, error) {
	// Simple stub implementation - return 100% success rate for operation type over days
	return 100.0, nil
}

func (ps *progressService) MonitorOperations(ctx context.Context) error {
	// Simple stub implementation - monitor operations
	return nil
}

func (ps *progressService) NotifyProgressUpdate(ctx context.Context, progress *entities.Progress) error {
	// Simple stub implementation - notify progress update
	return nil
}

func (ps *progressService) OptimizeOperationParameters(ctx context.Context, operationType string) (map[string]interface{}, error) {
	// Simple stub implementation - return default parameters
	params := map[string]interface{}{
		"worker_count":    4,
		"batch_size":      100,
		"timeout_minutes": 30,
	}
	return params, nil
}

func (ps *progressService) PauseOperation(ctx context.Context, progressID int) error {
	// Simple stub implementation - pause operation
	return nil
}

func (ps *progressService) ResumeOperation(ctx context.Context, progressID int) error {
	// Simple stub implementation - resume operation
	return nil
}

func (ps *progressService) RecoverCorruptedProgress(ctx context.Context, progressID int) error {
	// Simple stub implementation - recover corrupted progress for specific ID
	return nil
}

func (ps *progressService) RegisterProgressCallback(operationType string, callback func(*entities.Progress)) error {
	// Simple stub implementation - register progress callback for operation type
	return nil
}

func (ps *progressService) UnregisterProgressCallback(operationType string) error {
	// Simple stub implementation - unregister progress callback for operation type
	return nil
}

func (ps *progressService) SaveProgress(ctx context.Context, progress *entities.Progress) error {
	// Simple stub implementation - save progress
	return ps.progressRepo.Save(ctx, progress)
}

func (ps *progressService) StartOperation(ctx context.Context, operationType string, totalItems int) (*entities.Progress, error) {
	// Simple stub implementation - start operation
	progress := entities.NewProgress(operationType, totalItems)
	err := ps.progressRepo.Save(ctx, progress)
	return progress, err
}

func (ps *progressService) UpdateOperation(ctx context.Context, progressID int, processedItems int, currentStep string) error {
	// Simple stub implementation - update operation
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		return err
	}
	if progress != nil {
		progress.UpdateProgress(processedItems, currentStep)
		return ps.progressRepo.Update(ctx, progress)
	}
	return nil
}

func (ps *progressService) SetOperationMetadata(ctx context.Context, progressID int, metadata map[string]interface{}) error {
	// Simple stub implementation - set operation metadata
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		return err
	}
	if progress != nil {
		progress.Metadata = metadata
		return ps.progressRepo.Update(ctx, progress)
	}
	return nil
}

func (ps *progressService) ValidateProgress(ctx context.Context, progressID int) error {
	// Simple stub implementation - validate progress
	return nil
}

func (ps *progressService) ValidateProgressData(ctx context.Context, progressID int) error {
	// Simple stub implementation - validate progress data
	return nil
}

func (ps *progressService) StreamProgress(ctx context.Context, progressID int) (<-chan *entities.Progress, error) {
	// Simple stub implementation - stream progress updates
	ch := make(chan *entities.Progress)
	close(ch) // Immediately close for stub
	return ch, nil
}

func (ps *progressService) StreamOperationsByType(ctx context.Context, operationType string) (<-chan *entities.Progress, error) {
	// Simple stub implementation - stream operations by type
	ch := make(chan *entities.Progress)
	close(ch) // Immediately close for stub
	return ch, nil
}
