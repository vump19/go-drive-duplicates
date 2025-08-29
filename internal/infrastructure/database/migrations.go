package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          func(*sqlx.DB) error
	Down        func(*sqlx.DB) error
}

// Migrator handles database migrations
type Migrator struct {
	db         *sqlx.DB
	migrations []Migration
}

// NewMigrator creates a new database migrator
func NewMigrator(db *sqlx.DB) *Migrator {
	migrator := &Migrator{
		db:         db,
		migrations: []Migration{},
	}

	// Add all migrations
	migrator.addMigrations()
	return migrator
}

// addMigrations adds all migration definitions
func (m *Migrator) addMigrations() {
	// Migration 1: Create schema_migrations table
	m.migrations = append(m.migrations, Migration{
		Version:     1,
		Description: "Create schema_migrations table",
		Up: func(db *sqlx.DB) error {
			query := `
				CREATE TABLE IF NOT EXISTS schema_migrations (
					version INTEGER PRIMARY KEY,
					description TEXT NOT NULL,
					applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			_, err := db.Exec(query)
			return err
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS schema_migrations")
			return err
		},
	})

	// Migration 2: Update files table structure
	m.migrations = append(m.migrations, Migration{
		Version:     2,
		Description: "Update files table for new architecture",
		Up: func(db *sqlx.DB) error {
			// Check if the new columns already exist
			var columnExists bool

			// Check for hash_calculated column
			err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='hash_calculated'").Scan(&columnExists)
			if err != nil {
				return err
			}

			if !columnExists {
				if _, err := db.Exec("ALTER TABLE files ADD COLUMN hash_calculated BOOLEAN DEFAULT FALSE"); err != nil {
					return err
				}
			}

			// Check for parents column
			err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='parents'").Scan(&columnExists)
			if err != nil {
				return err
			}

			if !columnExists {
				if _, err := db.Exec("ALTER TABLE files ADD COLUMN parents TEXT DEFAULT ''"); err != nil {
					return err
				}
			}

			// Check for web_view_link column (rename from web_view_link if exists)
			err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='web_view_link'").Scan(&columnExists)
			if err != nil {
				return err
			}

			if columnExists {
				// Add new column
				if _, err := db.Exec("ALTER TABLE files ADD COLUMN webview_link TEXT DEFAULT ''"); err != nil {
					return err
				}
				// Copy data from old column to new
				if _, err := db.Exec("UPDATE files SET webview_link = web_view_link"); err != nil {
					return err
				}
			}

			// Update modified_time format if needed
			_, err = db.Exec("UPDATE files SET modified_time = datetime(modified_time) WHERE modified_time NOT LIKE '____-__-__ __:__:__'")
			if err != nil {
				log.Printf("Warning: Could not update modified_time format: %v", err)
			}

			return nil
		},
		Down: func(db *sqlx.DB) error {
			// Reverse the changes (be careful with data loss)
			return fmt.Errorf("migration 2 down not implemented - would cause data loss")
		},
	})

	// Migration 3: Update progress table structure
	m.migrations = append(m.migrations, Migration{
		Version:     3,
		Description: "Update progress table for new architecture",
		Up: func(db *sqlx.DB) error {
			// Create new progress table with updated structure
			createNewTable := `
				CREATE TABLE IF NOT EXISTS progress_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					operation_type TEXT NOT NULL,
					processed_items INTEGER DEFAULT 0,
					total_items INTEGER DEFAULT 0,
					status TEXT DEFAULT 'pending',
					current_step TEXT,
					error_message TEXT,
					start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
					end_time DATETIME,
					last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
					metadata TEXT -- JSON
				)
			`
			if _, err := db.Exec(createNewTable); err != nil {
				return err
			}

			// Migrate data from old progress table
			migrateData := `
				INSERT INTO progress_new (operation_type, processed_items, total_items, status, last_updated)
				SELECT 
					CASE 
						WHEN status = 'scanning' THEN 'file_scan'
						WHEN status = 'processing' THEN 'duplicate_search'
						ELSE 'file_scan'
					END as operation_type,
					processed_files,
					total_files,
					CASE 
						WHEN status = 'idle' THEN 'pending'
						WHEN status = 'scanning' THEN 'running'
						WHEN status = 'processing' THEN 'running'
						WHEN status = 'completed' THEN 'completed'
						ELSE 'pending'
					END as status,
					last_updated
				FROM progress
			`
			if _, err := db.Exec(migrateData); err != nil {
				log.Printf("Warning: Could not migrate progress data: %v", err)
			}

			// Drop old table and rename new one
			if _, err := db.Exec("DROP TABLE IF EXISTS progress_old"); err != nil {
				return err
			}
			if _, err := db.Exec("ALTER TABLE progress RENAME TO progress_old"); err != nil {
				return err
			}
			if _, err := db.Exec("ALTER TABLE progress_new RENAME TO progress"); err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_progress_operation_type ON progress(operation_type)",
				"CREATE INDEX IF NOT EXISTS idx_progress_status ON progress(status)",
				"CREATE INDEX IF NOT EXISTS idx_progress_start_time ON progress(start_time)",
			}
			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			return nil
		},
		Down: func(db *sqlx.DB) error {
			return fmt.Errorf("migration 3 down not implemented")
		},
	})

	// Migration 4: Update duplicate tables structure
	m.migrations = append(m.migrations, Migration{
		Version:     4,
		Description: "Update duplicate tables for new architecture",
		Up: func(db *sqlx.DB) error {
			// Check if duplicate_groups table has the correct structure
			var hasCountColumn bool
			err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('duplicate_groups') WHERE name='count'").Scan(&hasCountColumn)
			if err != nil {
				return err
			}

			if !hasCountColumn {
				// Add missing columns to duplicate_groups with NULL default first
				if _, err := db.Exec("ALTER TABLE duplicate_groups ADD COLUMN count INTEGER"); err != nil {
					return err
				}
				if _, err := db.Exec("ALTER TABLE duplicate_groups ADD COLUMN total_size INTEGER"); err != nil {
					return err
				}
				if _, err := db.Exec("ALTER TABLE duplicate_groups ADD COLUMN wasted_space INTEGER"); err != nil {
					return err
				}
				if _, err := db.Exec("ALTER TABLE duplicate_groups ADD COLUMN last_updated DATETIME"); err != nil {
					return err
				}

				// Set default values after adding columns
				if _, err := db.Exec("UPDATE duplicate_groups SET count = 0 WHERE count IS NULL"); err != nil {
					return err
				}
				if _, err := db.Exec("UPDATE duplicate_groups SET total_size = 0 WHERE total_size IS NULL"); err != nil {
					return err
				}
				if _, err := db.Exec("UPDATE duplicate_groups SET wasted_space = 0 WHERE wasted_space IS NULL"); err != nil {
					return err
				}
				if _, err := db.Exec("UPDATE duplicate_groups SET last_updated = CURRENT_TIMESTAMP WHERE last_updated IS NULL"); err != nil {
					return err
				}

				// Update count and total_size based on existing data
				updateQuery := `
					UPDATE duplicate_groups 
					SET 
						count = (SELECT COUNT(*) FROM duplicate_files WHERE group_id = duplicate_groups.id),
						total_size = (SELECT COALESCE(SUM(f.size), 0) FROM duplicate_files df JOIN files f ON df.file_id = f.id WHERE df.group_id = duplicate_groups.id),
						wasted_space = (SELECT COALESCE(SUM(f.size), 0) - COALESCE(MIN(f.size), 0) FROM duplicate_files df JOIN files f ON df.file_id = f.id WHERE df.group_id = duplicate_groups.id)
				`
				if _, err := db.Exec(updateQuery); err != nil {
					log.Printf("Warning: Could not update duplicate group statistics: %v", err)
				}
			}

			// Rename duplicate_files to duplicate_group_files if needed
			var tableExists bool
			err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='duplicate_group_files'").Scan(&tableExists)
			if err != nil {
				return err
			}

			if !tableExists {
				// Create new table structure
				createTable := `
					CREATE TABLE duplicate_group_files (
						group_id INTEGER,
						file_id TEXT,
						PRIMARY KEY (group_id, file_id),
						FOREIGN KEY (group_id) REFERENCES duplicate_groups(id) ON DELETE CASCADE,
						FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
					)
				`
				if _, err := db.Exec(createTable); err != nil {
					return err
				}

				// Migrate data from old table
				if _, err := db.Exec("INSERT INTO duplicate_group_files SELECT group_id, file_id FROM duplicate_files"); err != nil {
					log.Printf("Warning: Could not migrate duplicate files data: %v", err)
				}
			}

			return nil
		},
		Down: func(db *sqlx.DB) error {
			return fmt.Errorf("migration 4 down not implemented")
		},
	})

	// Migration 5: Create comparison_results table
	m.migrations = append(m.migrations, Migration{
		Version:     5,
		Description: "Create comparison_results table for new architecture",
		Up: func(db *sqlx.DB) error {
			// Create comparison_results table
			createTable := `
				CREATE TABLE IF NOT EXISTS comparison_results (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					source_folder_id TEXT NOT NULL,
					target_folder_id TEXT NOT NULL,
					source_folder_name TEXT,
					target_folder_name TEXT,
					source_file_count INTEGER DEFAULT 0,
					target_file_count INTEGER DEFAULT 0,
					source_total_size INTEGER DEFAULT 0,
					target_total_size INTEGER DEFAULT 0,
					duplicate_count INTEGER DEFAULT 0,
					wasted_space INTEGER DEFAULT 0,
					duplication_percentage REAL DEFAULT 0.0,
					can_delete_target_folder BOOLEAN DEFAULT FALSE,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return err
			}

			// Create comparison_duplicate_files table
			createDuplicateTable := `
				CREATE TABLE IF NOT EXISTS comparison_duplicate_files (
					comparison_id INTEGER,
					file_id TEXT,
					PRIMARY KEY (comparison_id, file_id),
					FOREIGN KEY (comparison_id) REFERENCES comparison_results(id) ON DELETE CASCADE,
					FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
				)
			`
			if _, err := db.Exec(createDuplicateTable); err != nil {
				return err
			}

			// Migrate data from old folder_comparison_tasks table if it exists
			var tableExists bool
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='folder_comparison_tasks'").Scan(&tableExists)
			if err != nil {
				return err
			}

			if tableExists {
				migrateQuery := `
					INSERT INTO comparison_results (source_folder_id, target_folder_id, created_at, updated_at)
					SELECT source_folder_id, target_folder_id, created_at, updated_at
					FROM folder_comparison_tasks
					WHERE status = 'completed'
				`
				if _, err := db.Exec(migrateQuery); err != nil {
					log.Printf("Warning: Could not migrate comparison tasks: %v", err)
				}
			}

			// Create indexes
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_comparison_source_target ON comparison_results(source_folder_id, target_folder_id)",
				"CREATE INDEX IF NOT EXISTS idx_comparison_created_at ON comparison_results(created_at)",
			}
			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err1 := db.Exec("DROP TABLE IF EXISTS comparison_duplicate_files")
			_, err2 := db.Exec("DROP TABLE IF EXISTS comparison_results")
			if err1 != nil {
				return err1
			}
			return err2
		},
	})
}

// Run executes all pending migrations
func (m *Migrator) Run(ctx context.Context) error {
	log.Println("🔄 Starting database migrations...")

	// Get current version
	currentVersion, err := m.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	log.Printf("📊 Current schema version: %d", currentVersion)

	// Run pending migrations
	for _, migration := range m.migrations {
		if migration.Version <= currentVersion {
			continue
		}

		log.Printf("⬆️  Applying migration %d: %s", migration.Version, migration.Description)

		// Run migration without transaction for large databases
		if err := migration.Up(m.db); err != nil {
			return fmt.Errorf("migration %d failed: %w", migration.Version, err)
		}

		// Record migration
		if err := m.recordMigration(migration.Version, migration.Description); err != nil {
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		log.Printf("✅ Migration %d completed successfully", migration.Version)
	}

	newVersion, _ := m.getCurrentVersion()
	log.Printf("🎉 Database migrations completed. Schema version: %d", newVersion)

	return nil
}

// getCurrentVersion returns the current schema version
func (m *Migrator) getCurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		// If schema_migrations doesn't exist, this is version 0
		if err == sql.ErrNoRows {
			return 0, nil
		}
		// Check if the error is because the table doesn't exist
		if err.Error() == "no such table: schema_migrations" {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

// recordMigration records a successful migration
func (m *Migrator) recordMigration(version int, description string) error {
	query := "INSERT INTO schema_migrations (version, description) VALUES (?, ?)"
	_, err := m.db.Exec(query, version, description)
	return err
}

// BackupDatabase creates a backup of the current database
func (m *Migrator) BackupDatabase(backupPath string) error {
	log.Printf("💾 Creating database backup: %s", backupPath)

	// Simple file copy backup
	query := fmt.Sprintf("VACUUM INTO '%s'", backupPath)
	_, err := m.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create database backup: %w", err)
	}

	log.Printf("✅ Database backup created successfully")
	return nil
}
