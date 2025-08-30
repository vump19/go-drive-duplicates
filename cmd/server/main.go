package main

import (
	"flag"
	"go-drive-duplicates/internal/infrastructure/config"
	"log"
	"os"
	"path/filepath"
)

const (
	defaultConfigPath = "./config/app.yaml"
	appName           = "Go Drive Duplicates"
	appVersion        = "1.0.0"
)

func main() {
	// Parse command line flags
	var (
		configPath = flag.String("config", defaultConfigPath, "Path to configuration file (supports .json, .yaml, .yml)")
		version    = flag.Bool("version", false, "Show version information")
		help       = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	// Show version information
	if *version {
		log.Printf("%s v%s", appName, appVersion)
		os.Exit(0)
	}

	// Show help information
	if *help {
		showHelp()
		os.Exit(0)
	}

	// Print startup banner
	printBanner()

	// Ensure config directory exists
	configDir := filepath.Dir(*configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("❌ Failed to create config directory: %v", err)
	}

	// Create and start the application
	app, err := config.NewApplication(*configPath)
	if err != nil {
		log.Fatalf("❌ Failed to create application: %v", err)
	}

	// Start the server
	if err := app.Run(); err != nil {
		log.Fatalf("❌ Application error: %v", err)
	}
}

func printBanner() {
	log.Printf("==========================================")
	log.Printf("        %s v%s        ", appName, appVersion)
	log.Printf("                                      ")
	log.Printf("    Google Drive Duplicate Finder    ")
	log.Printf("         Clean Architecture          ")
	log.Printf("==========================================")
	log.Printf("")
}

func showHelp() {
	log.Printf("%s v%s", appName, appVersion)
	log.Printf("Google Drive duplicate file finder and manager")
	log.Printf("")
	log.Printf("Usage:")
	log.Printf("  %s [options]", os.Args[0])
	log.Printf("")
	log.Printf("Options:")
	log.Printf("  -config string")
	log.Printf("        Path to configuration file (supports .json, .yaml, .yml) (default: %s)", defaultConfigPath)
	log.Printf("  -version")
	log.Printf("        Show version information")
	log.Printf("  -help")
	log.Printf("        Show this help message")
	log.Printf("")
	log.Printf("Environment Variables:")
	log.Printf("  SERVER_HOST                  Server host (default: localhost)")
	log.Printf("  SERVER_PORT                  Server port (default: 8080)")
	log.Printf("  DATABASE_PATH                SQLite database path")
	log.Printf("  GOOGLE_DRIVE_API_KEY         Google Drive API key")
	log.Printf("  GOOGLE_DRIVE_CREDENTIALS_PATH Google Drive service account credentials")
	log.Printf("  HASH_ALGORITHM               Hash algorithm (md5, sha1, sha256)")
	log.Printf("  HASH_WORKER_COUNT            Number of hash calculation workers")
	log.Printf("  PROCESSING_BATCH_SIZE        Batch size for processing operations")
	log.Printf("  LOG_LEVEL                    Log level (debug, info, warn, error)")
	log.Printf("  ENV                          Environment (development, production)")
	log.Printf("")
	log.Printf("Examples:")
	log.Printf("  # Start with default configuration")
	log.Printf("  %s", os.Args[0])
	log.Printf("")
	log.Printf("  # Start with custom configuration file")
	log.Printf("  %s -config /path/to/config.yaml", os.Args[0])
	log.Printf("  %s -config /path/to/config.json", os.Args[0])
	log.Printf("")
	log.Printf("  # Start with environment variables")
	log.Printf("  SERVER_PORT=9090 GOOGLE_DRIVE_API_KEY=your_key %s", os.Args[0])
	log.Printf("")
	log.Printf("API Endpoints:")
	log.Printf("  GET  /health                     - Health check")
	log.Printf("  GET  /health/db                  - Database health")
	log.Printf("  GET  /health/storage             - Storage health")
	log.Printf("")
	log.Printf("  POST /api/files/scan             - Scan all files")
	log.Printf("  POST /api/files/scan/folder      - Scan specific folder")
	log.Printf("  GET  /api/files/scan/progress    - Get scan progress")
	log.Printf("  POST /api/files/hash/calculate   - Calculate file hashes")
	log.Printf("  GET  /api/files/statistics       - Get file statistics")
	log.Printf("")
	log.Printf("  POST /api/duplicates/find        - Find duplicate files")
	log.Printf("  GET  /api/duplicates/groups      - Get duplicate groups")
	log.Printf("  GET  /api/duplicates/group?id=N  - Get specific duplicate group")
	log.Printf("  GET  /api/duplicates/progress    - Get duplicate search progress")
	log.Printf("")
	log.Printf("  POST /api/compare/folders        - Compare two folders")
	log.Printf("  GET  /api/compare/results        - Get comparison results")
	log.Printf("  GET  /api/compare/result?id=N    - Get specific comparison result")
	log.Printf("  GET  /api/compare/progress       - Get comparison progress")
	log.Printf("")
	log.Printf("  POST /api/compare/single-folder/duplicates - Find duplicates within single folder")
	log.Printf("  GET  /api/compare/single-folder/progress   - Get single folder duplicate search progress")
	log.Printf("")
	log.Printf("  POST /api/cleanup/files          - Delete specific files")
	log.Printf("  POST /api/cleanup/duplicates     - Delete duplicates from group")
	log.Printf("  POST /api/cleanup/pattern        - Bulk delete by pattern")
	log.Printf("  POST /api/cleanup/folders        - Cleanup empty folders")
	log.Printf("  GET  /api/cleanup/progress       - Get cleanup progress")
	log.Printf("  POST /api/cleanup/search         - Search files to delete (dry run)")
	log.Printf("")
}
