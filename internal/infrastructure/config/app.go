package config

import (
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/infrastructure/services"
	"go-drive-duplicates/internal/interfaces/middleware"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Application represents the main application
type Application struct {
	container  *Container
	server     *http.Server
	config     *Config
	configPath string
}

// NewApplication creates a new application instance
func NewApplication(configPath string) (*Application, error) {
	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	// Merge with environment variables
	envConfig := LoadConfigFromEnv()
	mergeConfigs(config, envConfig)

	// Create dependency injection container
	container, err := NewContainer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %v", err)
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         config.GetAddress(),
		Handler:      nil, // Will be set in setupRoutes
		ReadTimeout:  config.Server.GetReadTimeout(),
		WriteTimeout: config.Server.GetWriteTimeout(),
		IdleTimeout:  config.Server.GetIdleTimeout(),
	}

	app := &Application{
		container:  container,
		server:     server,
		config:     config,
		configPath: configPath,
	}

	// Setup HTTP routes
	app.setupRoutes()

	return app, nil
}

// setupRoutes configures all HTTP routes and middleware
func (app *Application) setupRoutes() {
	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", app.handleHealth)
	mux.HandleFunc("/health/db", app.handleDBHealth)
	mux.HandleFunc("/health/storage", app.handleStorageHealth)

	// API routes - File operations
	mux.HandleFunc("/api/files/scan", app.wrapHandler(app.container.FileController.ScanAllFiles))
	mux.HandleFunc("/api/files/scan/folder", app.wrapHandler(app.container.FileController.ScanFolder))
	mux.HandleFunc("/api/files/scan/progress", app.wrapHandler(app.container.FileController.GetScanProgress))
	mux.HandleFunc("/api/files/clear-failed-progress", app.wrapHandler(app.container.FileController.ClearFailedProgress))
	mux.HandleFunc("/api/files/hash/calculate", app.wrapHandler(app.container.DuplicateController.CalculateHashes))
	mux.HandleFunc("/api/files/hash/progress", app.wrapHandler(app.container.DuplicateController.GetDuplicateProgress))
	mux.HandleFunc("/api/files/statistics", app.wrapHandler(app.container.FileController.GetStatistics))

	// API routes - DB-based file explorer
	mux.HandleFunc("/api/files/db/list", app.wrapHandler(app.container.FileController.GetDBFileList))
	mux.HandleFunc("/api/files/db/tree", app.wrapHandler(app.container.FileController.GetDBFolderTree))
	mux.HandleFunc("/api/files/check-exists", app.wrapHandler(app.container.FileController.CheckFilesExist))

	// API routes - Duplicate operations
	mux.HandleFunc("/api/duplicates/find", app.wrapHandler(app.container.DuplicateController.FindDuplicates))
	mux.HandleFunc("/api/duplicates/groups", app.wrapHandler(app.container.DuplicateController.GetDuplicateGroups))
	mux.HandleFunc("/api/duplicates/group", app.wrapHandler(app.container.DuplicateController.GetDuplicateGroup))
	mux.HandleFunc("/api/duplicates/group/by-hash", app.wrapHandler(app.container.DuplicateController.GetDuplicateGroupByHash))
	mux.HandleFunc("/api/duplicates/group/delete", app.wrapHandler(app.container.DuplicateController.DeleteDuplicateGroup))
	mux.HandleFunc("/api/duplicates/progress", app.wrapHandler(app.container.DuplicateController.GetDuplicateProgress))
	mux.HandleFunc("/api/duplicates/resumable", app.wrapHandler(app.container.DuplicateController.GetResumableHashCalculation))
	mux.HandleFunc("/api/duplicates/resume-check", app.wrapHandler(app.container.DuplicateController.CheckResumableTasks))
	mux.HandleFunc("/api/duplicates/reset", app.wrapHandler(app.container.DuplicateController.ResetProgress))
	mux.HandleFunc("/api/duplicates/file/path", app.wrapHandler(app.container.DuplicateController.GetFilePath))
	mux.HandleFunc("/api/duplicates/file/trash", app.wrapHandler(app.container.DuplicateController.TrashFile))
	mux.HandleFunc("/api/duplicates/validate", app.wrapHandler(app.container.DuplicateController.ValidateAndCleanupDuplicateGroups))
	mux.HandleFunc("/api/duplicates/group/memo", app.wrapHandler(app.container.DuplicateController.UpdateGroupMemo))

	// API routes - Comparison operations
	mux.HandleFunc("/api/compare/folders", app.wrapHandler(app.container.ComparisonController.CompareFolders))
	mux.HandleFunc("/api/compare/progress", app.wrapHandler(app.container.ComparisonController.GetComparisonProgress))
	mux.HandleFunc("/api/compare/results/recent", app.wrapHandler(app.container.ComparisonController.GetRecentComparisons))
	mux.HandleFunc("/api/compare/result/load", app.wrapHandler(app.container.ComparisonController.LoadSavedComparison))
	mux.HandleFunc("/api/compare/result/delete", app.wrapHandler(app.container.ComparisonController.DeleteComparisonResult))
	mux.HandleFunc("/api/compare/resume", app.wrapHandler(app.container.ComparisonController.ResumeComparison))
	mux.HandleFunc("/api/compare/pending", app.wrapHandler(app.container.ComparisonController.GetPendingComparisons))

	// API routes - Single folder duplicate detection
	mux.HandleFunc("/api/compare/single-folder/duplicates", app.wrapHandler(app.container.ComparisonController.FindDuplicatesInSingleFolder))
	mux.HandleFunc("/api/compare/single-folder/progress", app.wrapHandler(app.container.ComparisonController.GetSingleFolderDuplicateProgress))
	mux.HandleFunc("/api/compare/single-folder/recent", app.wrapHandler(app.container.ComparisonController.GetRecentSingleFolderResults))

	// API routes - Folder deletion operations
	mux.HandleFunc("/api/compare/delete/target-folder", app.wrapHandler(app.container.ComparisonController.DeleteTargetFolder))
	mux.HandleFunc("/api/compare/delete/duplicate-files", app.wrapHandler(app.container.ComparisonController.DeleteDuplicateFiles))

	// API routes - Move unique files operations
	mux.HandleFunc("/api/compare/move-unique-files", app.wrapHandler(app.container.ComparisonController.MoveUniqueFiles))
	mux.HandleFunc("/api/compare/move-unique-files/progress", app.wrapHandler(app.container.ComparisonController.GetMoveUniqueFilesProgress))
	mux.HandleFunc("/api/compare/move-unique-files/cancel", app.wrapHandler(app.container.ComparisonController.CancelMoveUniqueFiles))
	mux.HandleFunc("/api/compare/move-unique-files/active", app.wrapHandler(app.container.ComparisonController.GetActiveMoveOperations))
	mux.HandleFunc("/api/compare/unique-files", app.wrapHandler(app.container.ComparisonController.GetUniqueFilesForComparison))

	// API routes - Cancel operation
	mux.HandleFunc("/api/compare/cancel", app.wrapHandler(app.container.ComparisonController.CancelComparison))

	// API routes - Utility operations
	mux.HandleFunc("/api/utils/extract-folder-id", app.wrapHandler(app.container.ComparisonController.ExtractFolderIdFromUrl))
	mux.HandleFunc("/api/utils/resolve-folder", app.wrapHandler(app.container.ComparisonController.ResolveFolderPath))

	// API routes - Folder analysis operations
	mux.HandleFunc("/api/folder/analyze", app.wrapHandler(app.container.FolderAnalysisController.AnalyzeFolder))
	mux.HandleFunc("/api/folder/analyze/cancel", app.wrapHandler(app.container.FolderAnalysisController.CancelFolderAnalysis))
	mux.HandleFunc("/api/folder/empty/delete", app.wrapHandler(app.container.FolderAnalysisController.DeleteEmptyFolder))

	// API routes - Verified duplicate operations
	mux.HandleFunc("/api/verified-duplicates", app.wrapHandler(app.handleVerifiedDuplicates))
	mux.HandleFunc("/api/verified-duplicates/bulk", app.wrapHandler(app.container.VerifiedDuplicateController.BulkMarkAsVerified))
	mux.HandleFunc("/api/verified-duplicates/excluded-hashes", app.wrapHandler(app.container.VerifiedDuplicateController.GetExcludedHashes))
	mux.HandleFunc("/api/verified-duplicates/hash/", app.wrapHandler(app.container.VerifiedDuplicateController.GetVerificationDetails))
	mux.HandleFunc("/api/verified-duplicates/", app.wrapHandler(app.handleVerifiedDuplicateByID))
	mux.HandleFunc("/api/folder/empty/delete-all", app.wrapHandler(app.container.FolderAnalysisController.DeleteAllEmptyFolders))
	mux.HandleFunc("/api/folder/cleanup-empty-subfolders", app.wrapHandler(app.container.FolderAnalysisController.CleanupEmptySubfolders))
	mux.HandleFunc("/api/folder/find-by-name", app.wrapHandler(app.container.FolderAnalysisController.FindFoldersByName))
	mux.HandleFunc("/api/folder/delete-by-name", app.wrapHandler(app.container.FolderAnalysisController.DeleteFoldersByName))

	// API routes - Cleanup operations
	mux.HandleFunc("/api/cleanup/files", app.wrapHandler(app.container.CleanupController.DeleteFiles))
	mux.HandleFunc("/api/cleanup/duplicates", app.wrapHandler(app.container.CleanupController.DeleteDuplicatesFromGroup))
	mux.HandleFunc("/api/cleanup/pattern", app.wrapHandler(app.container.CleanupController.BulkDeleteByPattern))
	mux.HandleFunc("/api/cleanup/folders", app.wrapHandler(app.container.CleanupController.CleanupEmptyFolders))
	mux.HandleFunc("/api/cleanup/progress", app.wrapHandler(app.container.CleanupController.GetCleanupProgress))
	mux.HandleFunc("/api/cleanup/search", app.wrapHandler(app.container.CleanupController.SearchFilesToDelete))

	// API routes - File Explorer operations
	mux.HandleFunc("/api/explorer/folder-tree", app.wrapHandler(app.container.FileExplorerController.GetFolderTree))
	mux.HandleFunc("/api/explorer/folder-contents", app.wrapHandler(app.container.FileExplorerController.ListFolderContents))
	mux.HandleFunc("/api/explorer/folder-size", app.wrapHandler(app.container.FileExplorerController.GetFolderSize))
	mux.HandleFunc("/api/explorer/copy-files", app.wrapHandler(app.container.FileExplorerController.CopyFiles))
	mux.HandleFunc("/api/explorer/move-files", app.wrapHandler(app.container.FileExplorerController.MoveFiles))
	mux.HandleFunc("/api/explorer/delete-files", app.wrapHandler(app.container.FileExplorerController.DeleteFiles))
	mux.HandleFunc("/api/explorer/rename-file", app.wrapHandler(app.container.FileExplorerController.RenameFile))
	mux.HandleFunc("/api/explorer/create-folder", app.wrapHandler(app.container.FileExplorerController.CreateFolder))
	mux.HandleFunc("/api/explorer/file-info", app.wrapHandler(app.container.FileExplorerController.GetFileInfo))
	mux.HandleFunc("/api/explorer/search", app.wrapHandler(app.container.FileExplorerController.SearchFiles))
	mux.HandleFunc("/api/explorer/operation/progress", app.wrapHandler(app.container.FileExplorerController.GetOperationProgress))
	mux.HandleFunc("/api/explorer/operation/cancel", app.wrapHandler(app.container.FileExplorerController.CancelOperation))

	// API routes - Folder Download operations (non-SSE)
	mux.HandleFunc("/api/folder/download/prepare", app.wrapHandler(app.container.FolderDownloadController.PrepareDownload))
	mux.HandleFunc("/api/folder/download/stream", app.wrapHandler(app.container.FolderDownloadController.StreamDownload))
	mux.HandleFunc("/api/folder/download/cancel", app.wrapHandler(app.container.FolderDownloadController.CancelDownload))
	mux.HandleFunc("/api/folder/download/active", app.wrapHandler(app.container.FolderDownloadController.GetActiveDownloads))

	// API routes - Folder Download polling (instead of SSE)
	mux.HandleFunc("/api/folder/download/prepare/start", app.wrapHandler(app.container.FolderDownloadController.StartPrepareDownload))
	mux.HandleFunc("/api/folder/download/prepare/progress", app.wrapHandler(app.container.FolderDownloadController.GetPrepareProgress))

	// API routes - Server-side Folder Download (downloads to server first, then serves ZIP)
	mux.HandleFunc("/api/folder/download/server/start", app.wrapHandler(app.container.FolderDownloadController.StartServerDownload))
	mux.HandleFunc("/api/folder/download/server/progress", app.wrapHandler(app.container.FolderDownloadController.GetServerDownloadProgress))
	mux.HandleFunc("/api/folder/download/server/download", app.wrapHandler(app.container.FolderDownloadController.DownloadCompletedZip))
	mux.HandleFunc("/api/folder/download/server/cleanup", app.wrapHandler(app.container.FolderDownloadController.CleanupServerDownload))

	// API routes - Folder Export (export folder contents to Google Spreadsheet)
	// Note: SSE endpoint registered in sseMux below

	// API routes - Batch Folder Archive (download folders, create ZIPs, upload to parent, trash originals)
	mux.HandleFunc("/api/batch-archive/start", app.wrapHandler(app.container.BatchArchiveController.StartBatchArchive))
	mux.HandleFunc("/api/batch-archive/progress", app.wrapHandler(app.container.BatchArchiveController.GetProgress))
	mux.HandleFunc("/api/batch-archive/cancel", app.wrapHandler(app.container.BatchArchiveController.CancelJob))
	mux.HandleFunc("/api/batch-archive/jobs", app.wrapHandler(app.container.BatchArchiveController.GetAllJobs))
	mux.HandleFunc("/api/batch-archive/list-subfolders", app.wrapHandler(app.container.BatchArchiveController.ListSubfolders))

	// API routes - Document Consolidation (merge text files into Markdown)
	mux.HandleFunc("/api/consolidate/start", app.wrapHandler(app.container.DocumentConsolidationController.StartConsolidation))
	mux.HandleFunc("/api/consolidate/progress", app.wrapHandler(app.container.DocumentConsolidationController.GetProgress))
	mux.HandleFunc("/api/consolidate/cancel", app.wrapHandler(app.container.DocumentConsolidationController.CancelConsolidation))

	// API routes - Smart File Organizer
	mux.HandleFunc("/api/organizer/chat", app.wrapHandler(app.container.SmartOrganizerController.Chat))
	mux.HandleFunc("/api/organizer/chat/history", app.wrapHandler(app.container.SmartOrganizerController.GetChatHistory))
	mux.HandleFunc("/api/organizer/chat/session", app.wrapHandler(app.container.SmartOrganizerController.DeleteChatSession))
	mux.HandleFunc("/api/organizer/rulesets", app.wrapHandler(app.handleOrganizerRuleSets))
	mux.HandleFunc("/api/organizer/rulesets/detail", app.wrapHandler(app.container.SmartOrganizerController.GetRuleSet))
	mux.HandleFunc("/api/organizer/rulesets/backup", app.wrapHandler(app.container.SmartOrganizerController.BackupRuleSet))
	mux.HandleFunc("/api/organizer/rulesets/restore", app.wrapHandler(app.container.SmartOrganizerController.RestoreRuleSet))
	mux.HandleFunc("/api/organizer/rules", app.wrapHandler(app.handleOrganizerRules))
	mux.HandleFunc("/api/organizer/rules/apply-suggestions", app.wrapHandler(app.container.SmartOrganizerController.ApplySuggestedRules))
	mux.HandleFunc("/api/organizer/organize/preview", app.wrapHandler(app.container.SmartOrganizerController.PreviewOrganize))
	mux.HandleFunc("/api/organizer/organize/execute", app.wrapHandler(app.container.SmartOrganizerController.ExecuteOrganize))
	mux.HandleFunc("/api/organizer/organize/progress", app.wrapHandler(app.container.SmartOrganizerController.GetOrganizeProgress))
	mux.HandleFunc("/api/organizer/organize/cancel", app.wrapHandler(app.container.SmartOrganizerController.CancelOrganize))
	mux.HandleFunc("/api/organizer/watch/start", app.wrapHandler(app.container.SmartOrganizerController.StartWatching))
	mux.HandleFunc("/api/organizer/watch/stop", app.wrapHandler(app.container.SmartOrganizerController.StopWatching))
	mux.HandleFunc("/api/organizer/watch/status", app.wrapHandler(app.container.SmartOrganizerController.GetWatchStatus))
	mux.HandleFunc("/api/organizer/logs", app.wrapHandler(app.container.SmartOrganizerController.GetLogs))
	mux.HandleFunc("/api/organizer/logs/export", app.wrapHandler(app.container.SmartOrganizerController.ExportLogs))
	mux.HandleFunc("/api/organizer/folders", app.wrapHandler(app.container.SmartOrganizerController.ListFolderFiles))

	// API routes - Audio Conversion
	mux.HandleFunc("/api/audio-convert/start", app.wrapHandler(app.container.AudioConversionController.StartConversion))
	mux.HandleFunc("/api/audio-convert/progress", app.wrapHandler(app.container.AudioConversionController.GetProgress))
	mux.HandleFunc("/api/audio-convert/cancel", app.wrapHandler(app.container.AudioConversionController.CancelJob))
	mux.HandleFunc("/api/audio-convert/formats", app.wrapHandler(app.container.AudioConversionController.GetFormats))

	// API routes - Sharing Management
	mux.HandleFunc("/api/sharing/scan/start", app.wrapHandler(app.container.SharingController.StartSharingScan))
	mux.HandleFunc("/api/sharing/scan/progress", app.wrapHandler(app.container.SharingController.GetScanProgress))
	mux.HandleFunc("/api/sharing/scan/results", app.wrapHandler(app.container.SharingController.GetScanResults))
	mux.HandleFunc("/api/sharing/scan/cancel", app.wrapHandler(app.container.SharingController.CancelScan))
	mux.HandleFunc("/api/sharing/permissions/change", app.wrapHandler(app.container.SharingController.ChangePermissions))
	mux.HandleFunc("/api/sharing/permissions/progress", app.wrapHandler(app.container.SharingController.GetChangeProgress))

	// API routes - Notification settings
	mux.HandleFunc("/api/notifications/telegram/settings", app.wrapHandler(app.handleTelegramSettings))
	mux.HandleFunc("/api/notifications/telegram/test", app.wrapHandler(app.handleTelegramTest))

	// Home page
	mux.HandleFunc("/", app.handleHomePage)

	// Apply middleware chain to main mux
	mainHandler := app.applyMiddleware(mux)

	// Create SSE mux (without response buffering middleware)
	sseMux := http.NewServeMux()
	sseMux.HandleFunc("/api/folder/download/prepare-sse", app.wrapSSEHandler(app.container.FolderDownloadController.PrepareDownloadWithSSE))
	sseMux.HandleFunc("/api/folder/download/progress", app.wrapSSEHandler(app.container.FolderDownloadController.GetDownloadProgress))
	sseMux.HandleFunc("/api/export/folder-to-spreadsheet", app.wrapSSEHandler(app.container.FolderExportController.ExportFolderToSpreadsheet))

	// Combine handlers: SSE routes bypass middleware
	app.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route SSE endpoints to SSE mux (without buffering middleware)
		if r.URL.Path == "/api/folder/download/prepare-sse" ||
			r.URL.Path == "/api/folder/download/progress" ||
			r.URL.Path == "/api/export/folder-to-spreadsheet" {
			sseMux.ServeHTTP(w, r)
			return
		}
		// All other routes go through main handler with middleware
		mainHandler.ServeHTTP(w, r)
	})
}

// wrapHandler wraps controller handlers with common functionality
func (app *Application) wrapHandler(handler http.HandlerFunc) http.HandlerFunc {
	return handler
}

// wrapSSEHandler wraps SSE handlers with minimal middleware (no response buffering)
func (app *Application) wrapSSEHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Apply only essential middleware for SSE (CORS, security headers)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Log SSE request start
		log.Printf("🔵 SSE %s %s - %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Call handler directly without response buffering
		handler(w, r)

		// Log SSE request end
		log.Printf("✅ SSE %s %s completed", r.Method, r.URL.Path)
	}
}

// applyMiddleware applies middleware to the handler
func (app *Application) applyMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in correct order (first applied = first executed)
	result := handler

	// Logging middleware (innermost)
	if app.config.IsDevelopment() {
		result = middleware.DetailedLoggingMiddleware(result.ServeHTTP)
	} else {
		result = middleware.LoggingMiddleware(result.ServeHTTP)
	}

	// Validation middleware
	result = middleware.ValidationMiddleware(result.ServeHTTP)

	// Rate limiting middleware (if enabled)
	if app.config.Security.EnableRateLimit {
		rateLimitMiddleware := middleware.RateLimitMiddleware(app.config.Security.RateLimit)
		result = rateLimitMiddleware(result.ServeHTTP)
	}

	// CORS middleware (if enabled)
	if app.config.Security.EnableCORS {
		result = middleware.CORSMiddleware(result.ServeHTTP)
	}

	// Security headers middleware
	result = middleware.SecurityHeadersMiddleware(result.ServeHTTP)

	// Error handling middleware (outermost)
	result = middleware.ErrorHandlerMiddleware(result.ServeHTTP)

	return result
}

// Run starts the application
func (app *Application) Run() error {
	log.Printf("🚀 Starting server on %s", app.server.Addr)
	log.Printf("📊 Environment: %s", app.getEnvironment())
	log.Printf("🗄️  Database: %s", app.config.Database.Path)
	log.Printf("🔧 Hash algorithm: %s", app.config.Hash.Algorithm)

	// Create a channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		var err error
		if app.config.Server.EnableTLS {
			log.Printf("🔒 Starting HTTPS server")
			err = app.server.ListenAndServeTLS(app.config.Server.CertFile, app.config.Server.KeyFile)
		} else {
			log.Printf("🌐 Starting HTTP server")
			err = app.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-stop
	log.Printf("📴 Shutting down server...")

	// Shutdown gracefully
	return app.Shutdown()
}

// Shutdown gracefully shuts down the application
func (app *Application) Shutdown() error {
	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := app.server.Shutdown(ctx); err != nil {
		log.Printf("❌ Error shutting down server: %v", err)
	}

	// Close database connections and other resources
	if err := app.container.Close(); err != nil {
		log.Printf("❌ Error closing container: %v", err)
		return err
	}

	log.Printf("✅ Server shut down successfully")
	return nil
}

// Health check handlers

func (app *Application) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

func (app *Application) handleDBHealth(w http.ResponseWriter, r *http.Request) {
	if err := app.container.CheckDatabaseHealth(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"unhealthy","error":"%s"}`, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"database"}`)
}

func (app *Application) handleStorageHealth(w http.ResponseWriter, r *http.Request) {
	if err := app.container.CheckStorageHealth(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"unhealthy","error":"%s"}`, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"storage"}`)
}

// Legacy handlers for backward compatibility

func (app *Application) handleHomePage(w http.ResponseWriter, r *http.Request) {
	// Only serve root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Show API documentation for clean architecture
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Go Drive Duplicates API - Clean Architecture</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; margin-bottom: 10px; }
        .subtitle { text-align: center; color: #666; margin-bottom: 40px; }
        .endpoint { background: #f8f9fa; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #007bff; }
        .method { font-weight: bold; color: #0066cc; display: inline-block; width: 60px; }
        .path { font-family: monospace; color: #333; }
        .description { color: #666; margin-top: 5px; font-size: 14px; }
        .section { margin: 30px 0; }
        .health { border-left-color: #28a745; }
        .files { border-left-color: #ffc107; }
        .duplicates { border-left-color: #dc3545; }
        .compare { border-left-color: #6f42c1; }
        .cleanup { border-left-color: #fd7e14; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Go Drive Duplicates Finder</h1>
        <p class="subtitle">Clean Architecture API Server v1.0.0</p>
        
        <div class="section">
            <h2>Health Check</h2>
            <div class="endpoint health">
                <span class="method">GET</span> <span class="path">/health</span>
                <div class="description">Server health check</div>
            </div>
            <div class="endpoint health">
                <span class="method">GET</span> <span class="path">/health/db</span>
                <div class="description">Database connectivity check</div>
            </div>
            <div class="endpoint health">
                <span class="method">GET</span> <span class="path">/health/storage</span>
                <div class="description">Google Drive storage connectivity check</div>
            </div>
        </div>
        
        <div class="section">
            <h2>File Operations</h2>
            <div class="endpoint files">
                <span class="method">POST</span> <span class="path">/api/files/scan</span>
                <div class="description">Start scanning all files in Google Drive</div>
            </div>
            <div class="endpoint files">
                <span class="method">POST</span> <span class="path">/api/files/scan/folder</span>
                <div class="description">Scan specific folder</div>
            </div>
            <div class="endpoint files">
                <span class="method">GET</span> <span class="path">/api/files/scan/progress</span>
                <div class="description">Get file scanning progress</div>
            </div>
            <div class="endpoint files">
                <span class="method">POST</span> <span class="path">/api/files/hash/calculate</span>
                <div class="description">Calculate file hashes for duplicate detection</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Duplicate Operations</h2>
            <div class="endpoint duplicates">
                <span class="method">POST</span> <span class="path">/api/duplicates/find</span>
                <div class="description">Find duplicate files based on content hash</div>
            </div>
            <div class="endpoint duplicates">
                <span class="method">GET</span> <span class="path">/api/duplicates/groups</span>
                <div class="description">Get paginated list of duplicate file groups</div>
            </div>
            <div class="endpoint duplicates">
                <span class="method">GET</span> <span class="path">/api/duplicates/group?id=N</span>
                <div class="description">Get specific duplicate group details</div>
            </div>
            <div class="endpoint duplicates">
                <span class="method">DELETE</span> <span class="path">/api/duplicates/group/delete?id=N</span>
                <div class="description">Delete a duplicate group from the list</div>
            </div>
            <div class="endpoint duplicates">
                <span class="method">GET</span> <span class="path">/api/duplicates/progress</span>
                <div class="description">Get duplicate finding progress</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Folder Comparison</h2>
            <div class="endpoint compare">
                <span class="method">POST</span> <span class="path">/api/compare/folders</span>
                <div class="description">Compare two folders for duplicates</div>
            </div>
            <div class="endpoint compare">
                <span class="method">GET</span> <span class="path">/api/compare/progress</span>
                <div class="description">Get folder comparison progress</div>
            </div>
            <div class="endpoint compare">
                <span class="method">GET</span> <span class="path">/api/compare/results/recent</span>
                <div class="description">Get recent comparison results</div>
            </div>
            <div class="endpoint compare">
                <span class="method">GET</span> <span class="path">/api/compare/result/load</span>
                <div class="description">Load saved comparison result</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Single Folder Duplicate Detection</h2>
            <div class="endpoint compare">
                <span class="method">POST</span> <span class="path">/api/compare/single-folder/duplicates</span>
                <div class="description">Find duplicates within a single folder</div>
            </div>
            <div class="endpoint compare">
                <span class="method">GET</span> <span class="path">/api/compare/single-folder/progress</span>
                <div class="description">Get single folder duplicate detection progress</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Folder Deletion Operations</h2>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/compare/delete/target-folder</span>
                <div class="description">Delete entire target folder (100% duplicated)</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/compare/delete/duplicate-files</span>
                <div class="description">Delete specific duplicate files from comparison</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Utility Operations</h2>
            <div class="endpoint files">
                <span class="method">POST</span> <span class="path">/api/utils/extract-folder-id</span>
                <div class="description">Extract Google Drive folder ID from URL</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Cleanup Operations</h2>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/cleanup/files</span>
                <div class="description">Delete specific files by ID</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/cleanup/duplicates</span>
                <div class="description">Delete duplicates from a group</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/cleanup/pattern</span>
                <div class="description">Bulk delete files matching pattern</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/cleanup/folders</span>
                <div class="description">Cleanup empty folders</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">POST</span> <span class="path">/api/cleanup/search</span>
                <div class="description">Search files to delete (dry run)</div>
            </div>
            <div class="endpoint cleanup">
                <span class="method">GET</span> <span class="path">/api/cleanup/progress</span>
                <div class="description">Get cleanup operation progress</div>
            </div>
        </div>
        
        <p style="text-align: center; margin-top: 40px; color: #666;">
            <strong>Environment:</strong> %s | 
            <strong>Clean Architecture</strong> | 
            <strong>SOLID Principles</strong>
        </p>
    </div>
</body>
</html>`, app.getEnvironment())
}

// Helper functions

func (app *Application) getEnvironment() string {
	if app.config.IsProduction() {
		return "production"
	}
	if app.config.IsDevelopment() {
		return "development"
	}
	return "unknown"
}

// Verified duplicate route handlers

func (app *Application) handleVerifiedDuplicates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.container.VerifiedDuplicateController.GetVerifiedDuplicates(w, r)
	case http.MethodPost:
		app.container.VerifiedDuplicateController.MarkAsVerified(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *Application) handleVerifiedDuplicateByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		app.container.VerifiedDuplicateController.UpdateVerificationStatus(w, r)
	case http.MethodDelete:
		app.container.VerifiedDuplicateController.RemoveVerification(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Smart Organizer route handlers (method-based routing)

func (app *Application) handleOrganizerRuleSets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.container.SmartOrganizerController.ListRuleSets(w, r)
	case http.MethodPost:
		app.container.SmartOrganizerController.CreateRuleSet(w, r)
	case http.MethodPut:
		app.container.SmartOrganizerController.UpdateRuleSet(w, r)
	case http.MethodDelete:
		app.container.SmartOrganizerController.DeleteRuleSet(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *Application) handleOrganizerRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		app.container.SmartOrganizerController.CreateRule(w, r)
	case http.MethodPut:
		app.container.SmartOrganizerController.UpdateRule(w, r)
	case http.MethodDelete:
		app.container.SmartOrganizerController.DeleteRule(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Telegram notification handlers

func (app *Application) handleTelegramSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		app.getTelegramSettings(w, r)
	case http.MethodPut:
		app.updateTelegramSettings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *Application) getTelegramSettings(w http.ResponseWriter, r *http.Request) {
	notifier, ok := app.container.NotificationService.(*services.TelegramNotifier)
	if !ok {
		http.Error(w, `{"error":"알림 서비스가 초기화되지 않았습니다"}`, http.StatusInternalServerError)
		return
	}

	enabled, maskedToken, chatID := notifier.GetConfig()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":  enabled,
		"botToken": maskedToken,
		"chatId":   chatID,
	})
}

func (app *Application) updateTelegramSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled  bool   `json:"enabled"`
		BotToken string `json:"botToken"`
		ChatID   string `json:"chatId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"잘못된 요청: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	notifier, ok := app.container.NotificationService.(*services.TelegramNotifier)
	if !ok {
		http.Error(w, `{"error":"알림 서비스가 초기화되지 않았습니다"}`, http.StatusInternalServerError)
		return
	}

	// 런타임 설정 업데이트
	notifier.SetEnabled(req.Enabled)
	if req.BotToken != "" && !isTokenMasked(req.BotToken) {
		notifier.UpdateConfig(req.BotToken, req.ChatID)
		app.config.Telegram.BotToken = req.BotToken
	} else if req.ChatID != "" {
		// 토큰이 마스킹 상태여도 Chat ID는 업데이트
		notifier.UpdateChatID(req.ChatID)
	}
	if req.ChatID != "" {
		app.config.Telegram.ChatID = req.ChatID
	}
	app.config.Telegram.Enabled = req.Enabled

	// YAML 파일에 저장
	if app.configPath != "" {
		if err := SaveConfig(app.config, app.configPath); err != nil {
			log.Printf("⚠️ 텔레그램 설정 파일 저장 실패: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":"설정 파일 저장 실패: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		log.Printf("✅ 텔레그램 설정이 파일에 저장되었습니다: %s", app.configPath)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "텔레그램 설정이 저장되었습니다",
	})
}

func (app *Application) handleTelegramTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := app.container.NotificationService.SendTestMessage(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "테스트 알림이 전송되었습니다",
	})
}

// isTokenMasked checks if a token string is masked (contains ****)
func isTokenMasked(token string) bool {
	if len(token) < 9 {
		return token == "****"
	}
	return token[4:len(token)-4] == "****"
}

// mergeConfigs merges environment configuration into the main configuration
func mergeConfigs(main, env *Config) {
	// Only override non-zero values from environment
	if env.Server.Host != "localhost" {
		main.Server.Host = env.Server.Host
	}
	if env.Server.Port != 8080 {
		main.Server.Port = env.Server.Port
	}
	if env.Database.Path != "./data/app.db" {
		main.Database.Path = env.Database.Path
	}
	if env.GoogleDrive.APIKey != "" {
		main.GoogleDrive.APIKey = env.GoogleDrive.APIKey
	}
	if env.GoogleDrive.CredentialsPath != "" {
		main.GoogleDrive.CredentialsPath = env.GoogleDrive.CredentialsPath
	}
	if env.Hash.Algorithm != "sha256" {
		main.Hash.Algorithm = env.Hash.Algorithm
	}
	if env.Hash.WorkerCount != 4 {
		main.Hash.WorkerCount = env.Hash.WorkerCount
	}
	if env.Processing.BatchSize != 100 {
		main.Processing.BatchSize = env.Processing.BatchSize
	}
	if env.Logging.Level != "info" {
		main.Logging.Level = env.Logging.Level
	}
	if env.AI.APIKey != "" {
		main.AI.APIKey = env.AI.APIKey
	}
	if env.AI.Model != "" {
		main.AI.Model = env.AI.Model
	}
	if env.Telegram.BotToken != "" {
		main.Telegram.BotToken = env.Telegram.BotToken
	}
	if env.Telegram.ChatID != "" {
		main.Telegram.ChatID = env.Telegram.ChatID
	}
	if env.Telegram.Enabled {
		main.Telegram.Enabled = env.Telegram.Enabled
	}
}
