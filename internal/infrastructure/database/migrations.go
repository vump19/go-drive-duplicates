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

	// Migration 2: Create or update files table structure
	m.migrations = append(m.migrations, Migration{
		Version:     2,
		Description: "Create or update files table for new architecture",
		Up: func(db *sqlx.DB) error {
			// First, check if files table exists
			var tableExists bool
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='files'").Scan(&tableExists)
			if err != nil {
				return err
			}

			// If table doesn't exist, create it
			if !tableExists {
				createTable := `
					CREATE TABLE IF NOT EXISTS files (
						id TEXT PRIMARY KEY,
						name TEXT NOT NULL,
						size INTEGER NOT NULL,
						mime_type TEXT,
						modified_time DATETIME,
						hash TEXT,
						hash_calculated BOOLEAN DEFAULT FALSE,
						parents TEXT,
						path TEXT,
						web_view_link TEXT,
						last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
					);

					CREATE INDEX IF NOT EXISTS idx_files_size ON files(size);
					CREATE INDEX IF NOT EXISTS idx_files_hash ON files(hash);
					CREATE INDEX IF NOT EXISTS idx_files_name ON files(name);
					CREATE INDEX IF NOT EXISTS idx_files_modified ON files(modified_time);
					CREATE INDEX IF NOT EXISTS idx_files_hash_calculated ON files(hash_calculated);
				`
				if _, err := db.Exec(createTable); err != nil {
					return err
				}
				return nil
			}

			// Table exists, check and add missing columns
			var columnExists bool

			// Check for hash_calculated column
			err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('files') WHERE name='hash_calculated'").Scan(&columnExists)
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

	// Migration 3: Create or update progress table structure
	m.migrations = append(m.migrations, Migration{
		Version:     3,
		Description: "Create or update progress table for new architecture",
		Up: func(db *sqlx.DB) error {
			// Check if progress table exists
			var tableExists bool
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='progress'").Scan(&tableExists)
			if err != nil {
				return err
			}

			// If table doesn't exist, create it directly
			if !tableExists {
				createTable := `
					CREATE TABLE IF NOT EXISTS progress (
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
						metadata TEXT
					)
				`
				if _, err := db.Exec(createTable); err != nil {
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
			}

			// Table exists, create new progress table with updated structure
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

	// Migration 4: Create or update duplicate tables structure
	m.migrations = append(m.migrations, Migration{
		Version:     4,
		Description: "Create or update duplicate tables for new architecture",
		Up: func(db *sqlx.DB) error {
			// Check if duplicate_groups table exists
			var tableExists bool
			err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='duplicate_groups'").Scan(&tableExists)
			if err != nil {
				return err
			}

			// If table doesn't exist, create it
			if !tableExists {
				createTable := `
					CREATE TABLE IF NOT EXISTS duplicate_groups (
						id INTEGER PRIMARY KEY AUTOINCREMENT,
						hash TEXT NOT NULL UNIQUE,
						count INTEGER NOT NULL DEFAULT 0,
						total_size INTEGER NOT NULL DEFAULT 0,
						wasted_space INTEGER NOT NULL DEFAULT 0,
						created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
						updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
						last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
					);

					CREATE INDEX IF NOT EXISTS idx_duplicate_groups_hash ON duplicate_groups(hash);
					CREATE INDEX IF NOT EXISTS idx_duplicate_groups_total_size ON duplicate_groups(total_size);
				`
				if _, err := db.Exec(createTable); err != nil {
					return err
				}

				// Create duplicate_group_files table
				createFilesTable := `
					CREATE TABLE IF NOT EXISTS duplicate_group_files (
						group_id INTEGER,
						file_id TEXT,
						PRIMARY KEY (group_id, file_id),
						FOREIGN KEY (group_id) REFERENCES duplicate_groups(id) ON DELETE CASCADE,
						FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
					)
				`
				if _, err := db.Exec(createFilesTable); err != nil {
					return err
				}
				return nil
			}

			// Check if duplicate_groups table has the correct structure
			var hasCountColumn bool
			err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('duplicate_groups') WHERE name='count'").Scan(&hasCountColumn)
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
			tableExists = false // Reuse the variable instead of redeclaring
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

	// Migration 6: Create verified_duplicates table
	m.migrations = append(m.migrations, Migration{
		Version:     6,
		Description: "Create verified_duplicates table for verified duplicate tracking",
		Up: func(db *sqlx.DB) error {
			// Create verified_duplicates table
			createTable := `
				CREATE TABLE IF NOT EXISTS verified_duplicates (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					hash TEXT NOT NULL UNIQUE,
					file_count INTEGER NOT NULL DEFAULT 0,
					total_size INTEGER NOT NULL DEFAULT 0,
					status TEXT NOT NULL DEFAULT 'verified',
					description TEXT DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create verified_duplicates table: %w", err)
			}

			// Create indexes for efficient querying
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_verified_duplicates_hash ON verified_duplicates(hash)",
				"CREATE INDEX IF NOT EXISTS idx_verified_duplicates_status ON verified_duplicates(status)",
				"CREATE INDEX IF NOT EXISTS idx_verified_duplicates_created_at ON verified_duplicates(created_at)",
			}

			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ verified_duplicates 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS verified_duplicates")
			return err
		},
	})

	// Migration 7: Create file_operations table for file explorer
	m.migrations = append(m.migrations, Migration{
		Version:     7,
		Description: "Create file_operations table for file explorer operations tracking",
		Up: func(db *sqlx.DB) error {
			// Create file_operations table
			createTable := `
				CREATE TABLE IF NOT EXISTS file_operations (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					operation_type TEXT NOT NULL,
					source_folder_id TEXT NOT NULL,
					target_folder_id TEXT,
					file_ids TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'pending',
					total_files INTEGER NOT NULL DEFAULT 0,
					processed_files INTEGER NOT NULL DEFAULT 0,
					total_bytes INTEGER NOT NULL DEFAULT 0,
					processed_bytes INTEGER NOT NULL DEFAULT 0,
					error_message TEXT,
					started_at DATETIME,
					completed_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create file_operations table: %w", err)
			}

			// Create indexes for efficient querying
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_file_operations_status ON file_operations(status)",
				"CREATE INDEX IF NOT EXISTS idx_file_operations_created_at ON file_operations(created_at)",
				"CREATE INDEX IF NOT EXISTS idx_file_operations_operation_type ON file_operations(operation_type)",
			}

			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ file_operations 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS file_operations")
			return err
		},
	})

	// Migration 8: Create comparison_unique_files table for unique file tracking
	m.migrations = append(m.migrations, Migration{
		Version:     8,
		Description: "Create comparison_unique_files table for tracking unique files in folder comparison",
		Up: func(db *sqlx.DB) error {
			// Create comparison_unique_files table
			createTable := `
				CREATE TABLE IF NOT EXISTS comparison_unique_files (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					comparison_id INTEGER NOT NULL,
					file_id TEXT NOT NULL,
					file_name TEXT NOT NULL,
					file_path TEXT,
					file_size INTEGER DEFAULT 0,
					folder_type TEXT NOT NULL,
					moved BOOLEAN DEFAULT FALSE,
					moved_at DATETIME,
					new_file_id TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (comparison_id) REFERENCES comparison_results(id) ON DELETE CASCADE
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create comparison_unique_files table: %w", err)
			}

			// Create indexes for efficient querying
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_comparison_unique_files_comparison_id ON comparison_unique_files(comparison_id)",
				"CREATE INDEX IF NOT EXISTS idx_comparison_unique_files_folder_type ON comparison_unique_files(folder_type)",
				"CREATE INDEX IF NOT EXISTS idx_comparison_unique_files_moved ON comparison_unique_files(moved)",
			}

			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ comparison_unique_files 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS comparison_unique_files")
			return err
		},
	})

	// Migration 9: Create organization_rule_sets table
	m.migrations = append(m.migrations, Migration{
		Version:     9,
		Description: "Create organization_rule_sets table for smart file organizer",
		Up: func(db *sqlx.DB) error {
			createTable := `
				CREATE TABLE IF NOT EXISTS organization_rule_sets (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					description TEXT DEFAULT '',
					watch_folder TEXT DEFAULT '',
					is_active BOOLEAN DEFAULT 0,
					backup_file_id TEXT DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create organization_rule_sets table: %w", err)
			}

			log.Printf("✅ organization_rule_sets 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS organization_rule_sets")
			return err
		},
	})

	// Migration 10: Create organization_rules table
	m.migrations = append(m.migrations, Migration{
		Version:     10,
		Description: "Create organization_rules table for smart file organizer",
		Up: func(db *sqlx.DB) error {
			createTable := `
				CREATE TABLE IF NOT EXISTS organization_rules (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					rule_set_id INTEGER NOT NULL,
					priority INTEGER DEFAULT 0,
					name TEXT NOT NULL,
					description TEXT DEFAULT '',
					conditions TEXT NOT NULL DEFAULT '[]',
					action TEXT NOT NULL DEFAULT 'move',
					target_path TEXT NOT NULL,
					enabled BOOLEAN DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (rule_set_id) REFERENCES organization_rule_sets(id) ON DELETE CASCADE
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create organization_rules table: %w", err)
			}

			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_org_rules_rule_set ON organization_rules(rule_set_id)",
			}
			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ organization_rules 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS organization_rules")
			return err
		},
	})

	// Migration 11: Create organization_logs table
	m.migrations = append(m.migrations, Migration{
		Version:     11,
		Description: "Create organization_logs table for smart file organizer",
		Up: func(db *sqlx.DB) error {
			createTable := `
				CREATE TABLE IF NOT EXISTS organization_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					rule_set_id INTEGER NOT NULL,
					rule_id INTEGER NOT NULL,
					rule_name TEXT DEFAULT '',
					file_id TEXT NOT NULL,
					file_name TEXT NOT NULL,
					file_size INTEGER DEFAULT 0,
					file_mime_type TEXT DEFAULT '',
					source_folder TEXT DEFAULT '',
					source_path TEXT DEFAULT '',
					target_folder TEXT DEFAULT '',
					target_path TEXT DEFAULT '',
					action TEXT NOT NULL,
					error_message TEXT DEFAULT '',
					executed_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create organization_logs table: %w", err)
			}

			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_org_logs_rule_set ON organization_logs(rule_set_id)",
				"CREATE INDEX IF NOT EXISTS idx_org_logs_executed ON organization_logs(executed_at)",
			}
			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ organization_logs 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS organization_logs")
			return err
		},
	})

	// Migration 12: Create chat_messages table
	m.migrations = append(m.migrations, Migration{
		Version:     12,
		Description: "Create chat_messages table for smart file organizer AI chat",
		Up: func(db *sqlx.DB) error {
			createTable := `
				CREATE TABLE IF NOT EXISTS chat_messages (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					session_id TEXT NOT NULL,
					role TEXT NOT NULL,
					content TEXT NOT NULL,
					metadata TEXT DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)
			`
			if _, err := db.Exec(createTable); err != nil {
				return fmt.Errorf("failed to create chat_messages table: %w", err)
			}

			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id)",
			}
			for _, index := range indexes {
				if _, err := db.Exec(index); err != nil {
					log.Printf("Warning: Could not create index: %v", err)
				}
			}

			log.Printf("✅ chat_messages 테이블 생성 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			_, err := db.Exec("DROP TABLE IF EXISTS chat_messages")
			return err
		},
	})

	// Migration 13: Add memo column to duplicate_groups
	m.migrations = append(m.migrations, Migration{
		Version:     13,
		Description: "Add memo column to duplicate_groups table",
		Up: func(db *sqlx.DB) error {
			_, err := db.Exec("ALTER TABLE duplicate_groups ADD COLUMN memo TEXT DEFAULT ''")
			if err != nil {
				return fmt.Errorf("failed to add memo column: %w", err)
			}
			log.Printf("✅ duplicate_groups 테이블에 memo 컬럼 추가 완료")
			return nil
		},
		Down: func(db *sqlx.DB) error {
			// SQLite doesn't support DROP COLUMN before 3.35.0
			return nil
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
