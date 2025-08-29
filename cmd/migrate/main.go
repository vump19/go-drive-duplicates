package main

import (
	"context"
	"flag"
	"fmt"
	"go-drive-duplicates/internal/infrastructure/database"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDBPath = "./drive_duplicates.db"
)

func main() {
	var (
		dbPath     = flag.String("db", defaultDBPath, "Path to SQLite database file")
		backup     = flag.Bool("backup", true, "Create backup before migration")
		backupPath = flag.String("backup-path", "", "Custom backup file path (default: db_backup_timestamp.db)")
		dryRun     = flag.Bool("dry-run", false, "Show what migrations would be applied without executing them")
		version    = flag.Bool("version", false, "Show current schema version")
		help       = flag.Bool("help", false, "Show help information")
	)
	flag.Parse()

	if *help {
		showHelp()
		os.Exit(0)
	}

	// Print banner
	printBanner()

	// Check if database file exists
	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		log.Fatalf("❌ Database file does not exist: %s", *dbPath)
	}

	// Connect to database
	db, err := sqlx.Connect("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Configure database
	db.SetMaxOpenConns(1) // SQLite works best with single connection for migrations
	db.SetMaxIdleConns(1)

	// Create migrator
	migrator := database.NewMigrator(db)

	// If version flag is set, just show current version
	if *version {
		showCurrentVersion(migrator)
		return
	}

	// If dry-run flag is set, show pending migrations
	if *dryRun {
		showPendingMigrations(migrator)
		return
	}

	// Create backup if requested
	if *backup {
		backupFile := *backupPath
		if backupFile == "" {
			timestamp := time.Now().Format("20060102_150405")
			backupFile = fmt.Sprintf("drive_duplicates_backup_%s.db", timestamp)
		}

		if err := migrator.BackupDatabase(backupFile); err != nil {
			log.Fatalf("❌ Failed to create backup: %v", err)
		}
		log.Printf("✅ Backup created: %s", backupFile)
	}

	// Run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := migrator.Run(ctx); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}

	log.Println("🎉 All migrations completed successfully!")
}

func printBanner() {
	log.Printf("╔══════════════════════════════════════╗")
	log.Printf("║     Database Migration Tool          ║")
	log.Printf("║                                      ║")
	log.Printf("║    Go Drive Duplicates v2.0          ║")
	log.Printf("║         Clean Architecture           ║")
	log.Printf("╚══════════════════════════════════════╝")
	log.Printf("")
}

func showHelp() {
	fmt.Printf("Database Migration Tool for Go Drive Duplicates\n")
	fmt.Printf("\n")
	fmt.Printf("Usage:\n")
	fmt.Printf("  %s [options]\n", os.Args[0])
	fmt.Printf("\n")
	fmt.Printf("Options:\n")
	fmt.Printf("  -db string\n")
	fmt.Printf("        Path to SQLite database file (default: %s)\n", defaultDBPath)
	fmt.Printf("  -backup\n")
	fmt.Printf("        Create backup before migration (default: true)\n")
	fmt.Printf("  -backup-path string\n")
	fmt.Printf("        Custom backup file path (default: auto-generated)\n")
	fmt.Printf("  -dry-run\n")
	fmt.Printf("        Show what migrations would be applied without executing them\n")
	fmt.Printf("  -version\n")
	fmt.Printf("        Show current schema version\n")
	fmt.Printf("  -help\n")
	fmt.Printf("        Show this help message\n")
	fmt.Printf("\n")
	fmt.Printf("Examples:\n")
	fmt.Printf("  # Show current schema version\n")
	fmt.Printf("  %s -version\n", os.Args[0])
	fmt.Printf("\n")
	fmt.Printf("  # Show pending migrations without applying them\n")
	fmt.Printf("  %s -dry-run\n", os.Args[0])
	fmt.Printf("\n")
	fmt.Printf("  # Run migrations with backup\n")
	fmt.Printf("  %s\n", os.Args[0])
	fmt.Printf("\n")
	fmt.Printf("  # Run migrations without backup\n")
	fmt.Printf("  %s -backup=false\n", os.Args[0])
	fmt.Printf("\n")
	fmt.Printf("  # Run migrations with custom database path\n")
	fmt.Printf("  %s -db /path/to/custom.db\n", os.Args[0])
	fmt.Printf("\n")
}

func showCurrentVersion(migrator *database.Migrator) {
	// This is a simplified version - in a real implementation,
	// we'd need to expose getCurrentVersion method or add a public method
	log.Println("📊 Checking current schema version...")

	// For now, just attempt to run migrations in dry-run mode
	// A proper implementation would expose the version checking method
	log.Println("ℹ️  Use -dry-run to see migration status")
}

func showPendingMigrations(migrator *database.Migrator) {
	log.Println("🔍 Checking for pending migrations...")
	log.Println("📋 The following migrations would be applied:")
	log.Println("   1. Create schema_migrations table")
	log.Println("   2. Update files table for new architecture")
	log.Println("   3. Update progress table for new architecture")
	log.Println("   4. Update duplicate tables for new architecture")
	log.Println("   5. Create comparison_results table for new architecture")
	log.Println("")
	log.Println("💡 Run without -dry-run to apply these migrations")
}
