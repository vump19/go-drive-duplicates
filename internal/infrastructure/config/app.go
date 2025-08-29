package config

import (
	"context"
	"fmt"
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
	container *Container
	server    *http.Server
	config    *Config
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
		container: container,
		server:    server,
		config:    config,
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
	// mux.HandleFunc("/api/files/statistics", app.wrapHandler(app.container.FileController.GetFileStatistics))

	// API routes - Duplicate operations
	mux.HandleFunc("/api/duplicates/find", app.wrapHandler(app.container.DuplicateController.FindDuplicates))
	mux.HandleFunc("/api/duplicates/groups", app.wrapHandler(app.container.DuplicateController.GetDuplicateGroups))
	mux.HandleFunc("/api/duplicates/group", app.wrapHandler(app.container.DuplicateController.GetDuplicateGroup))
	mux.HandleFunc("/api/duplicates/group/delete", app.wrapHandler(app.container.DuplicateController.DeleteDuplicateGroup))
	mux.HandleFunc("/api/duplicates/progress", app.wrapHandler(app.container.DuplicateController.GetDuplicateProgress))
	mux.HandleFunc("/api/duplicates/file/path", app.wrapHandler(app.container.DuplicateController.GetFilePath))

	// API routes - Comparison operations
	mux.HandleFunc("/api/compare/folders", app.wrapHandler(app.container.ComparisonController.CompareFolders))
	mux.HandleFunc("/api/compare/progress", app.wrapHandler(app.container.ComparisonController.GetComparisonProgress))
	mux.HandleFunc("/api/compare/results/recent", app.wrapHandler(app.container.ComparisonController.GetRecentComparisons))
	mux.HandleFunc("/api/compare/result/load", app.wrapHandler(app.container.ComparisonController.LoadSavedComparison))
	mux.HandleFunc("/api/compare/result/delete", app.wrapHandler(app.container.ComparisonController.DeleteComparisonResult))
	mux.HandleFunc("/api/compare/resume", app.wrapHandler(app.container.ComparisonController.ResumeComparison))
	mux.HandleFunc("/api/compare/pending", app.wrapHandler(app.container.ComparisonController.GetPendingComparisons))
	
	// API routes - Folder deletion operations
	mux.HandleFunc("/api/compare/delete/target-folder", app.wrapHandler(app.container.ComparisonController.DeleteTargetFolder))
	mux.HandleFunc("/api/compare/delete/duplicate-files", app.wrapHandler(app.container.ComparisonController.DeleteDuplicateFiles))
	
	// API routes - Utility operations
	mux.HandleFunc("/api/utils/extract-folder-id", app.wrapHandler(app.container.ComparisonController.ExtractFolderIdFromUrl))

	// API routes - Cleanup operations
	mux.HandleFunc("/api/cleanup/files", app.wrapHandler(app.container.CleanupController.DeleteFiles))
	mux.HandleFunc("/api/cleanup/duplicates", app.wrapHandler(app.container.CleanupController.DeleteDuplicatesFromGroup))
	mux.HandleFunc("/api/cleanup/pattern", app.wrapHandler(app.container.CleanupController.BulkDeleteByPattern))
	mux.HandleFunc("/api/cleanup/folders", app.wrapHandler(app.container.CleanupController.CleanupEmptyFolders))
	mux.HandleFunc("/api/cleanup/progress", app.wrapHandler(app.container.CleanupController.GetCleanupProgress))
	mux.HandleFunc("/api/cleanup/search", app.wrapHandler(app.container.CleanupController.SearchFilesToDelete))

	// Home page
	mux.HandleFunc("/", app.handleHomePage)

	// Apply middleware chain
	handler := app.applyMiddleware(mux)
	app.server.Handler = handler
}

// wrapHandler wraps controller handlers with common functionality
func (app *Application) wrapHandler(handler http.HandlerFunc) http.HandlerFunc {
	return handler
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
}
