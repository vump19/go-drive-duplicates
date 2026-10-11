package config

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/infrastructure/database"
	infraRepos "go-drive-duplicates/internal/infrastructure/repositories"
	"go-drive-duplicates/internal/infrastructure/repositories/sqlite"
	infraServices "go-drive-duplicates/internal/infrastructure/services"
	"go-drive-duplicates/internal/interfaces/controllers"
	"go-drive-duplicates/internal/interfaces/presenters"
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
	FileRepo              repositories.FileRepository
	DuplicateGroupRepo    repositories.DuplicateRepository
	ProgressRepo          repositories.ProgressRepository
	ComparisonResultRepo  repositories.ComparisonRepository
	VerifiedDuplicateRepo repositories.VerifiedDuplicateRepository
	FileOperationRepo     repositories.FileOperationRepository
	OrgRuleSetRepo        repositories.OrganizationRuleSetRepository
	OrgRuleRepo           repositories.OrganizationRuleRepository
	OrgLogRepo            repositories.OrganizationLogRepository
	ChatRepo              repositories.ChatMessageRepository

	// Services
	StorageProvider     services.StorageProvider
	HashService         services.HashService
	FileService         services.FileService
	ProgressService     services.ProgressService
	AIService           services.AIService
	NotificationService services.NotificationService

	// Use Cases
	FileScanningUseCase          *usecases.FileScanningUseCase
	DuplicateFindingUseCase      *usecases.DuplicateFindingUseCase
	FolderComparisonUseCase      *usecases.FolderComparisonUseCase
	FileCleanupUseCase           *usecases.FileCleanupUseCase
	FolderAnalysisUseCase        *usecases.FolderAnalysisUseCase
	VerifiedDuplicateUseCase     *usecases.VerifiedDuplicateUseCase
	FileExplorerUseCase          *usecases.FileExplorerUseCase
	FolderDownloadUseCase        *usecases.FolderDownloadUseCase
	BatchArchiveUseCase          *usecases.BatchArchiveUseCase
	FolderExportUseCase          *usecases.FolderExportUseCase
	DocumentConsolidationUseCase *usecases.DocumentConsolidationUseCase
	SmartOrganizerUseCase        *usecases.SmartOrganizerUseCase
	SharingManagerUseCase        *usecases.SharingManagerUseCase
	AudioConversionUseCase       *usecases.AudioConversionUseCase

	// Controllers
	FileController                  *controllers.FileController
	DuplicateController             *controllers.DuplicateController
	ComparisonController            *controllers.ComparisonController
	CleanupController               *controllers.CleanupController
	FolderAnalysisController        *controllers.FolderAnalysisController
	VerifiedDuplicateController     *controllers.VerifiedDuplicateController
	FileExplorerController          *controllers.FileExplorerController
	FolderDownloadController        *controllers.FolderDownloadController
	BatchArchiveController          *controllers.BatchArchiveController
	FolderExportController          *controllers.FolderExportController
	DocumentConsolidationController *controllers.DocumentConsolidationController
	SmartOrganizerController        *controllers.SmartOrganizerController
	SharingController               *controllers.SharingController
	AudioConversionController       *controllers.AudioConversionController
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

	// Run database migrations
	migrator := database.NewMigrator(c.DB)
	if err := migrator.Run(context.Background()); err != nil {
		return fmt.Errorf("failed to run database migrations: %v", err)
	}

	return nil
}

func (c *Container) initializeRepositories() error {
	// Initialize repositories
	c.FileRepo = sqlite.NewFileRepository(c.DB)
	c.ProgressRepo = sqlite.NewProgressRepository(c.DB)
	c.DuplicateGroupRepo = sqlite.NewDuplicateGroupRepository(c.DB, c.FileRepo)
	c.ComparisonResultRepo = sqlite.NewComparisonResultRepository(c.DB, c.FileRepo)
	c.VerifiedDuplicateRepo = infraRepos.NewVerifiedDuplicateRepository(c.DB.DB)
	c.FileOperationRepo = sqlite.NewFileOperationRepository(c.DB.DB)
	c.OrgRuleSetRepo = infraRepos.NewOrganizationRuleSetRepository(c.DB.DB)
	c.OrgRuleRepo = infraRepos.NewOrganizationRuleRepository(c.DB.DB)
	c.OrgLogRepo = infraRepos.NewOrganizationLogRepository(c.DB.DB)
	c.ChatRepo = infraRepos.NewChatMessageRepository(c.DB.DB)

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

	// Initialize notification service
	c.NotificationService = infraServices.NewTelegramNotifier(
		c.Config.Telegram.BotToken,
		c.Config.Telegram.ChatID,
		c.Config.Telegram.Enabled,
	)

	// Initialize domain services (these would be implemented as needed)
	c.FileService = NewFileService(c.FileRepo, c.StorageProvider)
	c.ProgressService = NewProgressService(c.ProgressRepo, c.NotificationService)

	// Initialize AI service
	if len(c.Config.AI.Providers) > 0 {
		// 멀티 프로바이더 모드
		providers := []infraServices.NamedProvider{}
		for _, p := range c.Config.AI.Providers {
			switch p.Name {
			case "claude":
				providers = append(providers, infraServices.NamedProvider{
					Name:    "claude",
					Service: infraServices.NewClaudeAPIAdapter(p.APIKey, p.Model),
				})
			case "openai":
				providers = append(providers, infraServices.NamedProvider{
					Name:    "openai",
					Service: infraServices.NewOpenAIAPIAdapter(p.APIKey, p.Model),
				})
			case "gemini":
				providers = append(providers, infraServices.NamedProvider{
					Name:    "gemini",
					Service: infraServices.NewGeminiAPIAdapter(p.APIKey, p.Model),
				})
			default:
				log.Printf("⚠️ 알 수 없는 AI 프로바이더: %s (건너뜀)", p.Name)
			}
		}
		if len(providers) > 0 {
			c.AIService = infraServices.NewMultiProviderAIService(providers)
			names := make([]string, len(providers))
			for i, p := range providers {
				names[i] = p.Name
			}
			log.Printf("✅ 멀티 AI 서비스 초기화 완료 (프로바이더: %v)", names)
		} else {
			log.Printf("⚠️ AI 프로바이더 설정이 있으나 유효한 프로바이더 없음")
		}
	} else if c.Config.AI.APIKey != "" {
		// 기존 단일 Claude 모드 (하위 호환)
		c.AIService = infraServices.NewClaudeAPIAdapter(c.Config.AI.APIKey, c.Config.AI.Model)
		log.Printf("✅ AI 서비스 초기화 완료 (Claude, 모델: %s)", c.Config.AI.Model)
	} else {
		log.Printf("⚠️ AI 서비스 미설정 (API 키 필요)")
	}

	return nil
}

func (c *Container) initializeUseCases() error {
	// Initialize use cases with proper dependencies
	c.FileScanningUseCase = usecases.NewFileScanningUseCase(
		c.FileRepo,
		c.ProgressRepo,
		c.DuplicateGroupRepo,
		c.StorageProvider,
		c.FileService,
		c.ProgressService,
	)

	c.DuplicateFindingUseCase = usecases.NewDuplicateFindingUseCase(
		c.FileRepo,
		c.DuplicateGroupRepo,
		c.ProgressRepo,
		c.VerifiedDuplicateRepo,
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

	c.FolderAnalysisUseCase = usecases.NewFolderAnalysisUseCase(
		c.FileRepo,
		c.ProgressRepo,
		c.StorageProvider,
		c.ProgressService,
	)

	c.VerifiedDuplicateUseCase = usecases.NewVerifiedDuplicateUseCase(
		c.VerifiedDuplicateRepo,
		c.DuplicateGroupRepo,
	)

	c.FileExplorerUseCase = usecases.NewFileExplorerUseCase(
		c.StorageProvider,
		c.FileOperationRepo,
	)

	c.FolderDownloadUseCase = usecases.NewFolderDownloadUseCase(
		c.FileRepo,
		c.ProgressRepo,
		c.StorageProvider,
		c.ProgressService,
	)

	c.BatchArchiveUseCase = usecases.NewBatchArchiveUseCase(
		c.StorageProvider,
		c.FolderDownloadUseCase,
	)

	c.FolderExportUseCase = usecases.NewFolderExportUseCase(
		c.StorageProvider,
	)

	c.DocumentConsolidationUseCase = usecases.NewDocumentConsolidationUseCase(
		c.StorageProvider,
	)

	c.SharingManagerUseCase = usecases.NewSharingManagerUseCase(
		c.StorageProvider,
	)

	c.AudioConversionUseCase = usecases.NewAudioConversionUseCase(
		c.StorageProvider,
	)

	c.SmartOrganizerUseCase = usecases.NewSmartOrganizerUseCase(
		c.StorageProvider,
		c.AIService,
		c.OrgRuleSetRepo,
		c.OrgRuleRepo,
		c.OrgLogRepo,
		c.ChatRepo,
		c.Config.SmartOrganizer.GetPollInterval(),
		c.Config.SmartOrganizer.MaxFilesPerRun,
	)

	return nil
}

func (c *Container) initializeControllers() error {
	// Initialize controllers with use cases
	c.FileController = controllers.NewFileController(c.FileScanningUseCase, c.FileRepo, c.StorageProvider, c.Config.Server.GetAPITimeout())
	c.DuplicateController = controllers.NewDuplicateController(c.DuplicateFindingUseCase, c.Config.Server.GetAPITimeout())
	c.ComparisonController = controllers.NewComparisonController(c.FolderComparisonUseCase, c.StorageProvider)
	c.CleanupController = controllers.NewCleanupController(c.FileCleanupUseCase)
	c.FolderAnalysisController = controllers.NewFolderAnalysisController(c.FolderAnalysisUseCase)

	// Initialize verified duplicate controller with presenter
	verifiedPresenter := presenters.NewVerifiedDuplicatePresenter()
	c.VerifiedDuplicateController = controllers.NewVerifiedDuplicateController(
		c.VerifiedDuplicateUseCase,
		verifiedPresenter,
	)

	// Initialize file explorer controller
	c.FileExplorerController = controllers.NewFileExplorerController(c.FileExplorerUseCase)

	// Initialize folder download controller
	c.FolderDownloadController = controllers.NewFolderDownloadController(c.FolderDownloadUseCase)

	// Initialize batch archive controller
	c.BatchArchiveController = controllers.NewBatchArchiveController(c.BatchArchiveUseCase)

	// Initialize folder export controller
	c.FolderExportController = controllers.NewFolderExportController(c.FolderExportUseCase)

	// Initialize document consolidation controller
	c.DocumentConsolidationController = controllers.NewDocumentConsolidationController(c.DocumentConsolidationUseCase)

	// Initialize sharing controller
	c.SharingController = controllers.NewSharingController(c.SharingManagerUseCase)

	// Initialize audio conversion controller
	c.AudioConversionController = controllers.NewAudioConversionController(c.AudioConversionUseCase)

	// Initialize smart organizer controller
	c.SmartOrganizerController = controllers.NewSmartOrganizerController(c.SmartOrganizerUseCase)

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
func NewProgressService(progressRepo repositories.ProgressRepository, notificationService services.NotificationService) services.ProgressService {
	return &progressService{
		progressRepo:        progressRepo,
		notificationService: notificationService,
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

// EnrichFileMetadata refreshes the file's metadata with the latest values from the storage provider.
// Path is computed locally and kept as is. The hash is replaced only when the file has no hash yet
// or its content may have changed (size or modified time differs); if the provider returns no hash
// in that case, it is cleared so it gets recalculated.
func (fs *fileService) EnrichFileMetadata(ctx context.Context, file *entities.File) error {
	if file == nil || file.ID == "" {
		return fmt.Errorf("file ID is required")
	}

	latest, err := fs.storageProvider.GetFile(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch file metadata: %w", err)
	}

	contentChanged := latest.Size != file.Size || !latest.ModifiedTime.Equal(file.ModifiedTime)

	file.Name = latest.Name
	file.Size = latest.Size
	file.MimeType = latest.MimeType
	file.ModifiedTime = latest.ModifiedTime
	file.Parents = latest.Parents
	file.WebViewLink = latest.WebViewLink

	if contentChanged || !file.HashCalculated {
		file.Hash = latest.Hash
		file.HashCalculated = latest.HashCalculated
	}

	file.LastUpdated = time.Now()
	return nil
}

func (fs *fileService) IsFileValid(file *entities.File) bool {
	return fs.ValidateFile(file) == nil
}

// RefreshFileMetadata fetches the latest metadata of the file from the storage provider and stores it.
// A file that is not in the repository yet is added.
func (fs *fileService) RefreshFileMetadata(ctx context.Context, fileID string) (*entities.File, error) {
	file, err := fs.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to load file: %w", err)
	}
	if file == nil {
		file = &entities.File{ID: fileID}
	}

	if err := fs.EnrichFileMetadata(ctx, file); err != nil {
		return nil, err
	}

	if err := fs.fileRepo.Save(ctx, file); err != nil {
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}
	return file, nil
}

func (fs *fileService) UpdateFilePaths(ctx context.Context, files []*entities.File) error {
	// update file paths
	for _, file := range files {
		err := fs.fileRepo.Update(ctx, file)
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateFileAccess checks with the storage provider that the file or folder exists,
// is accessible and is not in the trash. It does not depend on the local database,
// so folders that have never been scanned can be validated too.
func (fs *fileService) ValidateFileAccess(ctx context.Context, fileID string) error {
	if fileID == "" {
		return fmt.Errorf("file ID is required")
	}

	exists, err := fs.storageProvider.FileExists(ctx, fileID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("file not found or not accessible: %s", fileID)
	}
	return nil
}

// progressService implements services.ProgressService
type progressService struct {
	progressRepo        repositories.ProgressRepository
	notificationService services.NotificationService
}

// CancelOperation marks the operation as cancelled in the database.
// Stopping the running work itself is the caller's responsibility (e.g. cancelling its context).
func (ps *progressService) CancelOperation(ctx context.Context, progressID int) error {
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		log.Printf("❌ CancelOperation: 진행 상태 조회 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}
	if progress == nil {
		return fmt.Errorf("progress not found: %d", progressID)
	}
	if progress.IsCompleted() || progress.IsCancelled() {
		return nil
	}

	progress.Cancel()
	progress.CurrentStep = "취소됨"
	if err := ps.progressRepo.Update(ctx, progress); err != nil {
		log.Printf("❌ CancelOperation: 진행 상태 업데이트 실패 - ID: %d, Error: %v", progressID, err)
		return err
	}

	log.Printf("⏹️ 작업 취소 처리 완료 - Progress ID: %d", progressID)
	return nil
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

	// 비동기로 알림 전송 (fire-and-forget)
	if ps.notificationService != nil && ps.notificationService.IsEnabled() {
		progressCopy := *progress
		go func() {
			notifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := ps.notificationService.NotifyOperationCompleted(notifyCtx, &progressCopy); err != nil {
				log.Printf("⚠️ 텔레그램 완료 알림 전송 실패: %v", err)
			}
		}()
	}

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

	// 취소된 작업은 취소로 인해 뒤늦게 발생한 오류로 실패 처리하지 않음
	if progress.IsCancelled() {
		log.Printf("⚠️ FailOperation: 이미 취소된 작업이므로 상태 유지 - ID: %d", progressID)
		return nil
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

	// 비동기로 알림 전송 (fire-and-forget)
	if ps.notificationService != nil && ps.notificationService.IsEnabled() {
		progressCopy := *progress
		go func() {
			notifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := ps.notificationService.NotifyOperationFailed(notifyCtx, &progressCopy); err != nil {
				log.Printf("⚠️ 텔레그램 실패 알림 전송 실패: %v", err)
			}
		}()
	}

	return nil
}

func (ps *progressService) GetActiveOperations(ctx context.Context) ([]*entities.Progress, error) {
	// get active operations from repository
	return ps.progressRepo.GetActiveOperations(ctx)
}

func (ps *progressService) GetOperationsByType(ctx context.Context, operationType string) ([]*entities.Progress, error) {
	// delegate to repository
	return ps.progressRepo.GetByOperationType(ctx, operationType)
}

func (ps *progressService) GetProgress(ctx context.Context, progressID int) (*entities.Progress, error) {
	// delegate to repository
	return ps.progressRepo.GetByID(ctx, progressID)
}

func (ps *progressService) StartOperation(ctx context.Context, operationType string, totalItems int) (*entities.Progress, error) {
	// start operation
	progress := entities.NewProgress(operationType, totalItems)
	err := ps.progressRepo.Save(ctx, progress)
	return progress, err
}

func (ps *progressService) UpdateOperation(ctx context.Context, progressID int, processedItems int, currentStep string) error {
	// update operation
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
	progress, err := ps.progressRepo.GetByID(ctx, progressID)
	if err != nil {
		return err
	}
	if progress != nil {
		// 기존 메타데이터에 새 메타데이터를 병합 (기존 값 유지)
		if progress.Metadata == nil {
			progress.Metadata = make(map[string]interface{})
		}
		for k, v := range metadata {
			progress.Metadata[k] = v
		}
		return ps.progressRepo.Update(ctx, progress)
	}
	return nil
}
