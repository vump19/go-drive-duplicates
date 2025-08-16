package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type DriveFile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Size         int64    `json:"size"`
	WebViewLink  string   `json:"webViewLink"`
	MimeType     string   `json:"mimeType"`
	ModifiedTime string   `json:"modifiedTime"`
	Hash         string   `json:"hash,omitempty"`
	Parents      []string `json:"parents,omitempty"`
	Path         string   `json:"path,omitempty"`
}

type DriveService struct {
	service *drive.Service
	db      *sql.DB
}

type ProgressData struct {
	ProcessedFiles int                     `json:"processedFiles"`
	TotalFiles     int                     `json:"totalFiles"`
	CompletedGroups int                    `json:"completedGroups"`
	CurrentGroup   int                     `json:"currentGroup"`
	Duplicates     [][]*DriveFile          `json:"duplicates"`
	LastUpdated    time.Time               `json:"lastUpdated"`
	Status         string                  `json:"status"` // "running", "paused", "completed"
	LastPageToken  string                  `json:"lastPageToken,omitempty"`
	LastPageCount  int                     `json:"lastPageCount"`
}

const (
	dbFile = "drive_duplicates.db"
)

func initDB() (*sql.DB, error) {
	log.Println("🗄️ 데이터베이스 초기화 중...")
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, fmt.Errorf("데이터베이스 연결 오류: %v", err)
	}

	// 파일 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			size INTEGER NOT NULL,
			web_view_link TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			modified_time TEXT NOT NULL,
			hash TEXT DEFAULT '',
			hash_calculated BOOLEAN DEFAULT FALSE,
			parents TEXT DEFAULT '',
			path TEXT DEFAULT '',
			last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("files 테이블 생성 오류: %v", err)
	}

	// 진행 상태 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS progress (
			id INTEGER PRIMARY KEY,
			processed_files INTEGER DEFAULT 0,
			total_files INTEGER DEFAULT 0,
			completed_groups INTEGER DEFAULT 0,
			current_group INTEGER DEFAULT 0,
			status TEXT DEFAULT 'idle',
			last_page_token TEXT DEFAULT '',
			last_page_count INTEGER DEFAULT 0,
			last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("progress 테이블 생성 오류: %v", err)
	}

	// 폴더 비교 작업 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS folder_comparison_tasks (
			id INTEGER PRIMARY KEY,
			source_folder_id TEXT NOT NULL,
			target_folder_id TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			current_step TEXT DEFAULT '',
			source_folder_scanned INTEGER DEFAULT 0,
			source_folder_total INTEGER DEFAULT 0,
			target_folder_scanned INTEGER DEFAULT 0,
			target_folder_total INTEGER DEFAULT 0,
			hashes_calculated INTEGER DEFAULT 0,
			total_hashes_to_calc INTEGER DEFAULT 0,
			duplicates_found INTEGER DEFAULT 0,
			error_message TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("folder_comparison_tasks 테이블 생성 오류: %v", err)
	}

	// 폴더 비교 결과 파일 저장 테이블
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS comparison_result_files (
			id INTEGER PRIMARY KEY,
			task_id INTEGER NOT NULL,
			file_id TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_hash TEXT NOT NULL,
			file_path TEXT DEFAULT '',
			web_view_link TEXT DEFAULT '',
			mime_type TEXT DEFAULT '',
			modified_time TEXT DEFAULT '',
			is_duplicate BOOLEAN DEFAULT FALSE,
			folder_type TEXT NOT NULL, -- 'source' or 'target'
			FOREIGN KEY (task_id) REFERENCES folder_comparison_tasks (id)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("progress 테이블 생성 오류: %v", err)
	}

	// 설정 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("settings 테이블 생성 오류: %v", err)
	}

	// 기본 병렬 작업 개수 설정
	_, err = db.Exec(`
		INSERT OR IGNORE INTO settings (key, value) VALUES ('max_workers', '3')
	`)
	if err != nil {
		return nil, fmt.Errorf("기본 설정 추가 오류: %v", err)
	}

	// 중복 그룹 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS duplicate_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hash TEXT NOT NULL,
			group_size INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("duplicate_groups 테이블 생성 오류: %v", err)
	}

	// 중복 파일 매핑 테이블 생성
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS duplicate_files (
			group_id INTEGER,
			file_id TEXT,
			FOREIGN KEY (group_id) REFERENCES duplicate_groups(id),
			FOREIGN KEY (file_id) REFERENCES files(id)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("duplicate_files 테이블 생성 오류: %v", err)
	}

	// 데이터베이스 마이그레이션 실행
	err = migrateDatabaseSchema(db)
	if err != nil {
		return nil, fmt.Errorf("데이터베이스 마이그레이션 오류: %v", err)
	}

	log.Println("✅ 데이터베이스 초기화 완료")
	return db, nil
}

func migrateDatabaseSchema(db *sql.DB) error {
	log.Println("🔄 데이터베이스 스키마 마이그레이션 중...")
	
	// files 테이블에 parents, path 컬럼이 있는지 확인
	rows, err := db.Query("PRAGMA table_info(files)")
	if err != nil {
		return fmt.Errorf("테이블 정보 조회 오류: %v", err)
	}
	defer rows.Close()
	
	hasParents := false
	hasPath := false
	
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "parents" {
			hasParents = true
		}
		if name == "path" {
			hasPath = true
		}
	}
	
	// parents 컬럼 추가
	if !hasParents {
		log.Println("📝 parents 컬럼 추가 중...")
		_, err = db.Exec("ALTER TABLE files ADD COLUMN parents TEXT DEFAULT ''")
		if err != nil {
			return fmt.Errorf("parents 컬럼 추가 오류: %v", err)
		}
		log.Println("✅ parents 컬럼 추가 완료")
	}
	
	// path 컬럼 추가
	if !hasPath {
		log.Println("📝 path 컬럼 추가 중...")
		_, err = db.Exec("ALTER TABLE files ADD COLUMN path TEXT DEFAULT ''")
		if err != nil {
			return fmt.Errorf("path 컬럼 추가 오류: %v", err)
		}
		log.Println("✅ path 컬럼 추가 완료")
	}
	
	// progress 테이블에 새 컬럼 추가
	err = migrateProgressTable(db)
	if err != nil {
		return fmt.Errorf("progress 테이블 마이그레이션 오류: %v", err)
	}
	
	log.Println("✅ 데이터베이스 스키마 마이그레이션 완료")
	return nil
}

func migrateProgressTable(db *sql.DB) error {
	// progress 테이블 구조 확인
	rows, err := db.Query("PRAGMA table_info(progress)")
	if err != nil {
		return fmt.Errorf("progress 테이블 정보 조회 오류: %v", err)
	}
	defer rows.Close()
	
	hasLastPageToken := false
	hasLastPageCount := false
	
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "last_page_token" {
			hasLastPageToken = true
		}
		if name == "last_page_count" {
			hasLastPageCount = true
		}
	}
	
	// last_page_token 컬럼 추가
	if !hasLastPageToken {
		log.Println("📝 progress 테이블에 last_page_token 컬럼 추가 중...")
		_, err = db.Exec("ALTER TABLE progress ADD COLUMN last_page_token TEXT DEFAULT ''")
		if err != nil {
			return fmt.Errorf("last_page_token 컬럼 추가 오류: %v", err)
		}
		log.Println("✅ last_page_token 컬럼 추가 완료")
	}
	
	// last_page_count 컬럼 추가
	if !hasLastPageCount {
		log.Println("📝 progress 테이블에 last_page_count 컬럼 추가 중...")
		_, err = db.Exec("ALTER TABLE progress ADD COLUMN last_page_count INTEGER DEFAULT 0")
		if err != nil {
			return fmt.Errorf("last_page_count 컬럼 추가 오류: %v", err)
		}
		log.Println("✅ last_page_count 컬럼 추가 완료")
	}
	
	return nil
}

func NewDriveService(ctx context.Context) (*DriveService, error) {
	log.Println("🔧 OAuth 설정 파일 읽는 중...")
	config, err := getOAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("OAuth 설정 오류: %v", err)
	}
	log.Println("✅ OAuth 설정 파일 로드 완료")

	log.Println("🎫 액세스 토큰 확인 중...")
	token, err := getToken(config)
	if err != nil {
		return nil, fmt.Errorf("토큰 획득 오류: %v", err)
	}
	log.Println("✅ 액세스 토큰 확인 완료")

	log.Println("🌐 Google Drive 서비스 연결 중...")
	client := config.Client(ctx, token)
	
	// HTTP 클라이언트 최적화 (성능 향상)
	optimizedClient := optimizeHTTPClient(client)
	
	service, err := drive.NewService(ctx, option.WithHTTPClient(optimizedClient))
	if err != nil {
		return nil, fmt.Errorf("Drive 서비스 생성 오류: %v", err)
	}
	log.Println("✅ Google Drive 서비스 연결 성공")

	// DB 초기화
	db, err := initDB()
	if err != nil {
		return nil, err
	}

	return &DriveService{service: service, db: db}, nil
}

// HTTP 클라이언트 성능 최적화
func optimizeHTTPClient(client *http.Client) *http.Client {
	// 기존 Transport를 복제하여 설정 유지
	var baseTransport *http.Transport
	if client.Transport != nil {
		if transport, ok := client.Transport.(*http.Transport); ok {
			baseTransport = transport.Clone()
		} else if transport, ok := client.Transport.(*oauth2.Transport); ok {
			if innerTransport, ok := transport.Base.(*http.Transport); ok {
				baseTransport = innerTransport.Clone()
			}
		}
	}
	
	// 기본 Transport 설정 (백업)
	if baseTransport == nil {
		baseTransport = http.DefaultTransport.(*http.Transport).Clone()
	}
	
	// 기본 연결 설정 (타임아웃 제거)
	baseTransport.MaxIdleConns = 100          // 최대 유휴 연결 수
	baseTransport.MaxIdleConnsPerHost = 20    // 호스트당 최대 유휴 연결 수
	baseTransport.MaxConnsPerHost = 50        // 호스트당 최대 연결 수
	// 모든 타임아웃 설정 제거
	
	// 압축 활성화
	baseTransport.DisableCompression = false
	
	// OAuth Transport 래핑 유지
	var finalTransport http.RoundTripper = baseTransport
	if oauthTransport, ok := client.Transport.(*oauth2.Transport); ok {
		oauthTransport.Base = baseTransport
		finalTransport = oauthTransport
	}
	
	optimizedClient := &http.Client{
		Transport: finalTransport,
		// Timeout 제거 - 무제한 대기
	}
	
	log.Printf("🚀 HTTP 클라이언트 최적화 적용: MaxConns=%d, MaxIdleConns=%d, Timeout=무제한", 
		baseTransport.MaxConnsPerHost, baseTransport.MaxIdleConns)
	
	return optimizedClient
}

func getOAuthConfig() (*oauth2.Config, error) {
	credentialsFile := "credentials.json"
	
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("credentials.json 파일을 읽을 수 없습니다: %v\n구글 클라우드 콘솔에서 OAuth 2.0 클라이언트 ID를 생성하고 credentials.json으로 저장하세요", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("OAuth 설정 파싱 오류: %v", err)
	}

	// 데스크톱 애플리케이션의 경우 redirect URL 설정
	if config.RedirectURL == "" {
		config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	}

	return config, nil
}

func getToken(config *oauth2.Config) (*oauth2.Token, error) {
	tokenFile := "token.json"
	
	token, err := tokenFromFile(tokenFile)
	if err != nil {
		token = getTokenFromWeb(config)
		saveToken(tokenFile, token)
	}
	
	return token, nil
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	log.Println("🔐 Google 계정 인증이 필요합니다!")
	log.Println("📋 다음 단계를 따라주세요:")
	log.Println("1. 아래 링크를 복사하여 브라우저에서 열기")
	log.Println("2. Google 계정으로 로그인")
	log.Println("3. 권한 허용")
	log.Println("4. 표시되는 인증 코드를 복사")
	log.Println("5. 아래에 인증 코드 입력")
	log.Println("═══════════════════════════════════════")
	fmt.Printf("%v\n", authURL)
	log.Println("═══════════════════════════════════════")
	fmt.Print("인증 코드를 입력하세요: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("❌ 인증 코드 입력 오류: %v", err)
	}

	log.Println("🔄 인증 코드를 토큰으로 교환 중...")
	token, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("❌ 토큰 교환 오류: %v", err)
	}
	
	log.Println("✅ 인증 성공! 토큰이 저장되었습니다.")
	return token
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	
	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	
	return token, err
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("토큰 파일 저장 오류: %v", err)
	}
	defer f.Close()
	
	json.NewEncoder(f).Encode(token)
}

func (ds *DriveService) saveFilesToDB(files []*DriveFile) error {
	log.Println("🗄️ 파일 정보를 데이터베이스에 저장 중...")
	
	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 오류: %v", err)
	}
	defer tx.Rollback()

	// 테이블 구조 확인
	rows, err := tx.Query("PRAGMA table_info(files)")
	if err != nil {
		return fmt.Errorf("테이블 정보 조회 오류: %v", err)
	}
	
	hasParents := false
	hasPath := false
	
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "parents" {
			hasParents = true
		}
		if name == "path" {
			hasPath = true
		}
	}
	rows.Close()

	// 컬럼 유무에 따라 다른 INSERT 문 사용
	var stmt *sql.Stmt
	if hasParents && hasPath {
		stmt, err = tx.Prepare(`
			INSERT OR REPLACE INTO files 
			(id, name, size, web_view_link, mime_type, modified_time, parents, path, last_updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`)
	} else {
		stmt, err = tx.Prepare(`
			INSERT OR REPLACE INTO files 
			(id, name, size, web_view_link, mime_type, modified_time, last_updated)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`)
	}
	
	if err != nil {
		return fmt.Errorf("준비된 문장 생성 오류: %v", err)
	}
	defer stmt.Close()

	for _, file := range files {
		if hasParents && hasPath {
			parentsJSON, _ := json.Marshal(file.Parents)
			_, err = stmt.Exec(file.ID, file.Name, file.Size, file.WebViewLink, file.MimeType, file.ModifiedTime, string(parentsJSON), file.Path)
		} else {
			_, err = stmt.Exec(file.ID, file.Name, file.Size, file.WebViewLink, file.MimeType, file.ModifiedTime)
		}
		
		if err != nil {
			return fmt.Errorf("파일 저장 오류: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 오류: %v", err)
	}

	log.Printf("✅ %d개 파일 정보가 데이터베이스에 저장됨", len(files))
	return nil
}

func (ds *DriveService) loadFilesFromDB() ([]*DriveFile, error) {
	log.Println("🗄️ 데이터베이스에서 파일 목록 로드 중...")
	
	// 테이블 구조 확인
	infoRows, err := ds.db.Query("PRAGMA table_info(files)")
	if err != nil {
		return nil, fmt.Errorf("테이블 정보 조회 오류: %v", err)
	}
	
	hasParents := false
	hasPath := false
	
	for infoRows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := infoRows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "parents" {
			hasParents = true
		}
		if name == "path" {
			hasPath = true
		}
	}
	infoRows.Close()

	// 컬럼 유무에 따라 다른 SELECT 문 사용
	var query string
	if hasParents && hasPath {
		query = `
			SELECT id, name, size, web_view_link, mime_type, modified_time, hash, hash_calculated, parents, path
			FROM files
			ORDER BY size DESC
		`
	} else {
		query = `
			SELECT id, name, size, web_view_link, mime_type, modified_time, hash, hash_calculated
			FROM files
			ORDER BY size DESC
		`
	}
	
	rows, err := ds.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("파일 조회 오류: %v", err)
	}
	defer rows.Close()

	var files []*DriveFile
	for rows.Next() {
		file := &DriveFile{}
		var hashCalculated bool
		
		if hasParents && hasPath {
			var parentsJSON string
			err := rows.Scan(&file.ID, &file.Name, &file.Size, &file.WebViewLink, 
							&file.MimeType, &file.ModifiedTime, &file.Hash, &hashCalculated, &parentsJSON, &file.Path)
			if err != nil {
				return nil, fmt.Errorf("파일 스캔 오류: %v", err)
			}
			
			// Parents JSON을 파싱
			if parentsJSON != "" {
				json.Unmarshal([]byte(parentsJSON), &file.Parents)
			}
		} else {
			err := rows.Scan(&file.ID, &file.Name, &file.Size, &file.WebViewLink, 
							&file.MimeType, &file.ModifiedTime, &file.Hash, &hashCalculated)
			if err != nil {
				return nil, fmt.Errorf("파일 스캔 오류: %v", err)
			}
			file.Path = ""
			file.Parents = []string{}
		}
		
		files = append(files, file)
	}

	if len(files) > 0 {
		log.Printf("✅ 데이터베이스에서 %d개 파일 로드 완료", len(files))
	}
	return files, nil
}

func (ds *DriveService) updateFileHash(fileID, hash string) error {
	_, err := ds.db.Exec(`
		UPDATE files 
		SET hash = ?, hash_calculated = TRUE, last_updated = CURRENT_TIMESTAMP
		WHERE id = ?
	`, hash, fileID)
	return err
}

func (ds *DriveService) saveProgress(progress ProgressData) error {
	// 테이블 구조 확인
	rows, err := ds.db.Query("PRAGMA table_info(progress)")
	if err != nil {
		return fmt.Errorf("progress 테이블 정보 조회 오류: %v", err)
	}
	
	hasLastPageToken := false
	hasLastPageCount := false
	
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "last_page_token" {
			hasLastPageToken = true
		}
		if name == "last_page_count" {
			hasLastPageCount = true
		}
	}
	rows.Close()
	
	// 컬럼 유무에 따라 다른 INSERT 문 사용
	if hasLastPageToken && hasLastPageCount {
		_, err = ds.db.Exec(`
			INSERT OR REPLACE INTO progress (id, processed_files, total_files, completed_groups, current_group, status, last_page_token, last_page_count, last_updated)
			VALUES (1, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, progress.ProcessedFiles, progress.TotalFiles, progress.CompletedGroups, progress.CurrentGroup, progress.Status, progress.LastPageToken, progress.LastPageCount)
	} else {
		_, err = ds.db.Exec(`
			INSERT OR REPLACE INTO progress (id, processed_files, total_files, completed_groups, current_group, status, last_updated)
			VALUES (1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, progress.ProcessedFiles, progress.TotalFiles, progress.CompletedGroups, progress.CurrentGroup, progress.Status)
	}
	
	return err
}

func (ds *DriveService) loadProgress() (*ProgressData, error) {
	// 테이블 구조 확인
	rows, err := ds.db.Query("PRAGMA table_info(progress)")
	if err != nil {
		return nil, fmt.Errorf("progress 테이블 정보 조회 오류: %v", err)
	}
	
	hasLastPageToken := false
	hasLastPageCount := false
	
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "last_page_token" {
			hasLastPageToken = true
		}
		if name == "last_page_count" {
			hasLastPageCount = true
		}
	}
	rows.Close()
	
	var progress ProgressData
	
	// 컬럼 유무에 따라 다른 SELECT 문 사용
	if hasLastPageToken && hasLastPageCount {
		err = ds.db.QueryRow(`
			SELECT processed_files, total_files, completed_groups, current_group, status, last_page_token, last_page_count, last_updated
			FROM progress WHERE id = 1
		`).Scan(&progress.ProcessedFiles, &progress.TotalFiles, &progress.CompletedGroups, 
				&progress.CurrentGroup, &progress.Status, &progress.LastPageToken, &progress.LastPageCount, &progress.LastUpdated)
	} else {
		err = ds.db.QueryRow(`
			SELECT processed_files, total_files, completed_groups, current_group, status, last_updated
			FROM progress WHERE id = 1
		`).Scan(&progress.ProcessedFiles, &progress.TotalFiles, &progress.CompletedGroups, 
				&progress.CurrentGroup, &progress.Status, &progress.LastUpdated)
		
		// 기본값 설정
		progress.LastPageToken = ""
		progress.LastPageCount = 0
	}
	
	if err == sql.ErrNoRows {
		return &ProgressData{Status: "idle"}, nil
	}
	return &progress, err
}

func (ds *DriveService) saveSingleDuplicateGroup(group []*DriveFile) error {
	if len(group) < 2 {
		return nil
	}

	log.Printf("💾 중복 그룹 저장 중: %d개 파일, 해시: %s...", len(group), group[0].Hash[:8])

	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 오류: %v", err)
	}
	defer tx.Rollback()

	// 동일한 해시의 기존 그룹이 있는지 확인
	var existingGroupID int64
	err = tx.QueryRow("SELECT id FROM duplicate_groups WHERE hash = ?", group[0].Hash).Scan(&existingGroupID)
	
	if err == sql.ErrNoRows {
		// 새 그룹 생성
		log.Printf("🆕 새 중복 그룹 생성: 해시 %s...", group[0].Hash[:8])
		result, err := tx.Exec(`
			INSERT INTO duplicate_groups (hash, group_size)
			VALUES (?, ?)
		`, group[0].Hash, len(group))
		if err != nil {
			return fmt.Errorf("그룹 생성 오류: %v", err)
		}

		existingGroupID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("그룹 ID 조회 오류: %v", err)
		}
		log.Printf("✅ 새 그룹 ID: %d", existingGroupID)
	} else if err != nil {
		return fmt.Errorf("기존 그룹 확인 오류: %v", err)
	} else {
		// 기존 그룹의 파일들 삭제 후 새로 추가
		log.Printf("🔄 기존 그룹 업데이트: ID %d", existingGroupID)
		_, err = tx.Exec("DELETE FROM duplicate_files WHERE group_id = ?", existingGroupID)
		if err != nil {
			return fmt.Errorf("기존 파일 삭제 오류: %v", err)
		}
		
		// 그룹 크기 업데이트
		_, err = tx.Exec("UPDATE duplicate_groups SET group_size = ? WHERE id = ?", len(group), existingGroupID)
		if err != nil {
			return fmt.Errorf("그룹 크기 업데이트 오류: %v", err)
		}
	}

	// 그룹에 파일들 추가
	for _, file := range group {
		_, err = tx.Exec(`
			INSERT INTO duplicate_files (group_id, file_id)
			VALUES (?, ?)
		`, existingGroupID, file.ID)
		if err != nil {
			return fmt.Errorf("파일 추가 오류: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 오류: %v", err)
	}

	log.Printf("✅ 중복 그룹 저장 완료: 그룹 ID %d, %d개 파일", existingGroupID, len(group))
	return nil
}

func (ds *DriveService) saveDuplicateGroups(duplicates [][]*DriveFile) error {
	log.Println("🗄️ 중복 그룹을 데이터베이스에 저장 중...")
	
	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 오류: %v", err)
	}
	defer tx.Rollback()

	// 기존 중복 그룹 삭제
	_, err = tx.Exec("DELETE FROM duplicate_files")
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM duplicate_groups")
	if err != nil {
		return err
	}

	for _, group := range duplicates {
		if len(group) < 2 {
			continue
		}

		// 그룹 생성
		result, err := tx.Exec(`
			INSERT INTO duplicate_groups (hash, group_size)
			VALUES (?, ?)
		`, group[0].Hash, len(group))
		if err != nil {
			return err
		}

		groupID, err := result.LastInsertId()
		if err != nil {
			return err
		}

		// 그룹에 파일들 추가
		for _, file := range group {
			_, err = tx.Exec(`
				INSERT INTO duplicate_files (group_id, file_id)
				VALUES (?, ?)
			`, groupID, file.ID)
			if err != nil {
				return err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 오료: %v", err)
	}

	log.Printf("✅ %d개 중복 그룹이 데이터베이스에 저장됨", len(duplicates))
	return nil
}

func (ds *DriveService) loadDuplicateGroupsPaginated(page, limit int) ([][]*DriveFile, int, error) {
	log.Printf("🗄️ 페이지네이션된 중복 그룹 로드 중... (페이지 %d, 한계 %d)", page, limit)
	
	// 총 그룹 개수 조회
	var totalCount int
	err := ds.db.QueryRow("SELECT COUNT(*) FROM duplicate_groups").Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("그룹 개수 조회 오류: %v", err)
	}
	
	// OFFSET 계산
	offset := (page - 1) * limit
	
	rows, err := ds.db.Query(`
		SELECT dg.id, dg.hash, dg.group_size,
			   f.id, f.name, f.size, f.web_view_link, f.mime_type, f.modified_time, f.hash, f.parents, f.path
		FROM duplicate_groups dg
		JOIN duplicate_files df ON dg.id = df.group_id
		JOIN files f ON df.file_id = f.id
		WHERE dg.id IN (
			SELECT id FROM duplicate_groups 
			ORDER BY id 
			LIMIT ? OFFSET ?
		)
		ORDER BY dg.id, f.name
	`, limit, offset)
	
	if err != nil {
		return nil, 0, fmt.Errorf("중복 그룹 조회 오류: %v", err)
	}
	defer rows.Close()

	groupMap := make(map[int64][]*DriveFile)
	fileCount := 0
	for rows.Next() {
		var groupID int64
		var groupHash string
		var groupSize int
		var parentsJSON string
		file := &DriveFile{}
		
		err := rows.Scan(&groupID, &groupHash, &groupSize,
						&file.ID, &file.Name, &file.Size, &file.WebViewLink,
						&file.MimeType, &file.ModifiedTime, &file.Hash, &parentsJSON, &file.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("중복 그룹 스캔 오류: %v", err)
		}
		
		// Parents JSON을 파싱
		if parentsJSON != "" {
			json.Unmarshal([]byte(parentsJSON), &file.Parents)
		}
		
		groupMap[groupID] = append(groupMap[groupID], file)
		fileCount++
	}

	var duplicates [][]*DriveFile
	for groupID, group := range groupMap {
		log.Printf("📋 그룹 ID %d: %d개 파일", groupID, len(group))
		duplicates = append(duplicates, group)
	}

	log.Printf("✅ 페이지네이션된 중복 그룹 로드 완료: %d개 그룹 (총 %d개 중)", len(duplicates), totalCount)
	return duplicates, totalCount, nil
}

func (ds *DriveService) loadDuplicateGroups() ([][]*DriveFile, error) {
	log.Println("🗄️ 데이터베이스에서 중복 그룹 로드 중...")
	
	// 먼저 중복 그룹 개수 확인
	var groupCount int
	err := ds.db.QueryRow("SELECT COUNT(*) FROM duplicate_groups").Scan(&groupCount)
	if err != nil {
		log.Printf("⚠️ 중복 그룹 개수 조회 실패: %v", err)
	} else {
		log.Printf("📊 데이터베이스에 %d개 중복 그룹 존재", groupCount)
	}
	
	rows, err := ds.db.Query(`
		SELECT dg.id, dg.hash, dg.group_size,
			   f.id, f.name, f.size, f.web_view_link, f.mime_type, f.modified_time, f.hash, f.parents, f.path
		FROM duplicate_groups dg
		JOIN duplicate_files df ON dg.id = df.group_id
		JOIN files f ON df.file_id = f.id
		ORDER BY dg.id, f.name
	`)
	if err != nil {
		log.Printf("❌ 중복 그룹 조회 쿼리 실패: %v", err)
		return nil, fmt.Errorf("중복 그룹 조회 오류: %v", err)
	}
	defer rows.Close()

	groupMap := make(map[int64][]*DriveFile)
	fileCount := 0
	for rows.Next() {
		var groupID int64
		var groupHash string
		var groupSize int
		var parentsJSON string
		file := &DriveFile{}
		
		err := rows.Scan(&groupID, &groupHash, &groupSize,
						&file.ID, &file.Name, &file.Size, &file.WebViewLink,
						&file.MimeType, &file.ModifiedTime, &file.Hash, &parentsJSON, &file.Path)
		if err != nil {
			log.Printf("❌ 중복 그룹 스캔 오류: %v", err)
			return nil, fmt.Errorf("중복 그룹 스캔 오류: %v", err)
		}
		
		// Parents JSON을 파싱
		log.Printf("🔍 파일 %s의 parentsJSON: '%s'", file.Name, parentsJSON)
		if parentsJSON != "" {
			err := json.Unmarshal([]byte(parentsJSON), &file.Parents)
			if err != nil {
				log.Printf("⚠️ Parents JSON 파싱 실패: %v", err)
			} else {
				log.Printf("📋 파싱된 Parents: %v", file.Parents)
			}
		} else {
			log.Printf("⚠️ 파일 %s의 parents 정보가 비어있음", file.Name)
		}
		
		groupMap[groupID] = append(groupMap[groupID], file)
		fileCount++
	}

	log.Printf("📄 총 %d개 중복 파일 로드됨", fileCount)

	var duplicates [][]*DriveFile
	for groupID, group := range groupMap {
		log.Printf("📋 그룹 ID %d: %d개 파일", groupID, len(group))
		duplicates = append(duplicates, group)
	}

	log.Printf("✅ 데이터베이스에서 %d개 중복 그룹 로드 완료", len(duplicates))
	return duplicates, nil
}

func (ds *DriveService) deleteFileFromDB(fileID string) error {
	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 오류: %v", err)
	}
	defer tx.Rollback()

	// duplicate_files에서 삭제
	_, err = tx.Exec("DELETE FROM duplicate_files WHERE file_id = ?", fileID)
	if err != nil {
		return fmt.Errorf("duplicate_files 삭제 오류: %v", err)
	}

	// files에서 삭제
	_, err = tx.Exec("DELETE FROM files WHERE id = ?", fileID)
	if err != nil {
		return fmt.Errorf("files 삭제 오류: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 오류: %v", err)
	}

	return nil
}

func (ds *DriveService) cleanupEmptyGroups() error {
	// 파일이 1개 이하인 그룹들을 찾아서 삭제
	rows, err := ds.db.Query(`
		SELECT dg.id, COUNT(df.file_id) as file_count
		FROM duplicate_groups dg
		LEFT JOIN duplicate_files df ON dg.id = df.group_id
		GROUP BY dg.id
		HAVING file_count <= 1
	`)
	if err != nil {
		return fmt.Errorf("빈 그룹 조회 오류: %v", err)
	}
	defer rows.Close()

	var emptyGroupIDs []int64
	for rows.Next() {
		var groupID int64
		var fileCount int
		if err := rows.Scan(&groupID, &fileCount); err != nil {
			return fmt.Errorf("빈 그룹 스캔 오류: %v", err)
		}
		emptyGroupIDs = append(emptyGroupIDs, groupID)
	}

	// 빈 그룹들 삭제
	for _, groupID := range emptyGroupIDs {
		_, err = ds.db.Exec("DELETE FROM duplicate_files WHERE group_id = ?", groupID)
		if err != nil {
			log.Printf("⚠️ duplicate_files 삭제 실패 (group_id: %d): %v", groupID, err)
		}
		_, err = ds.db.Exec("DELETE FROM duplicate_groups WHERE id = ?", groupID)
		if err != nil {
			log.Printf("⚠️ duplicate_groups 삭제 실패 (id: %d): %v", groupID, err)
		}
	}

	if len(emptyGroupIDs) > 0 {
		log.Printf("🗑️ %d개의 빈 중복 그룹 정리 완료", len(emptyGroupIDs))
	}

	return nil
}

func (ds *DriveService) getMaxWorkers() int {
	var value string
	err := ds.db.QueryRow("SELECT value FROM settings WHERE key = 'max_workers'").Scan(&value)
	if err != nil {
		log.Printf("⚠️ max_workers 설정 조회 실패, 기본값 3 사용: %v", err)
		return 3
	}
	
	maxWorkers := 3
	if _, err := fmt.Sscanf(value, "%d", &maxWorkers); err != nil {
		log.Printf("⚠️ max_workers 파싱 실패, 기본값 3 사용: %v", err)
		return 3
	}
	
	// 최소 1개, 최대 20개로 제한
	if maxWorkers < 1 {
		maxWorkers = 1
	} else if maxWorkers > 20 {
		maxWorkers = 20
	}
	
	return maxWorkers
}

func (ds *DriveService) setMaxWorkers(workers int) error {
	if workers < 1 {
		workers = 1
	} else if workers > 20 {
		workers = 20
	}
	
	_, err := ds.db.Exec(`
		INSERT OR REPLACE INTO settings (key, value, updated_at) 
		VALUES ('max_workers', ?, CURRENT_TIMESTAMP)
	`, fmt.Sprintf("%d", workers))
	
	if err != nil {
		return fmt.Errorf("max_workers 설정 저장 실패: %v", err)
	}
	
	log.Printf("⚙️ 병렬 작업 개수가 %d개로 설정되었습니다", workers)
	return nil
}

func (ds *DriveService) getAllSettings() map[string]string {
	settings := make(map[string]string)
	
	rows, err := ds.db.Query("SELECT key, value FROM settings")
	if err != nil {
		log.Printf("⚠️ 설정 조회 실패: %v", err)
		return settings
	}
	defer rows.Close()
	
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		settings[key] = value
	}
	
	return settings
}

// 폴더 비교 및 중복 파일 검출 기능
type FolderComparisonResult struct {
	SourceFolder      string      `json:"sourceFolder"`
	TargetFolder      string      `json:"targetFolder"`
	SourceFiles       []*DriveFile `json:"sourceFiles"`
	TargetFiles       []*DriveFile `json:"targetFiles"`
	DuplicatesInTarget []*DriveFile `json:"duplicatesInTarget"`
	TotalDuplicates   int         `json:"totalDuplicates"`
	
	// 폴더 삭제 권장 정보
	CanDeleteTargetFolder  bool    `json:"canDeleteTargetFolder"`  // 대상 폴더 전체 삭제 가능 여부
	TargetFolderName      string  `json:"targetFolderName"`       // 대상 폴더 이름
	TargetFolderID        string  `json:"targetFolderID"`         // 대상 폴더 ID
	DuplicationPercentage float64 `json:"duplicationPercentage"`  // 중복 비율
}

// 폴더 비교 진행 상황 추적
type FolderComparisonProgress struct {
	Status              string `json:"status"` // "running", "completed", "error"
	CurrentStep         string `json:"currentStep"`
	SourceFolderScanned int    `json:"sourceFolderScanned"`
	SourceFolderTotal   int    `json:"sourceFolderTotal"`
	TargetFolderScanned int    `json:"targetFolderScanned"`
	TargetFolderTotal   int    `json:"targetFolderTotal"`
	HashesCalculated    int    `json:"hashesCalculated"`
	TotalHashesToCalc   int    `json:"totalHashesToCalc"`
	DuplicatesFound     int    `json:"duplicatesFound"`
	ErrorMessage        string `json:"errorMessage,omitempty"`
	StartTime           time.Time `json:"startTime"`
	LastUpdated         time.Time `json:"lastUpdated"`
}

// 폴더 비교 작업 구조체
type FolderComparisonTask struct {
	ID                  int    `json:"id"`
	SourceFolderID      string `json:"sourceFolderId"`
	TargetFolderID      string `json:"targetFolderId"`
	Status              string `json:"status"`
	CurrentStep         string `json:"currentStep"`
	SourceFolderScanned int    `json:"sourceFolderScanned"`
	SourceFolderTotal   int    `json:"sourceFolderTotal"`
	TargetFolderScanned int    `json:"targetFolderScanned"`
	TargetFolderTotal   int    `json:"targetFolderTotal"`
	HashesCalculated    int    `json:"hashesCalculated"`
	TotalHashesToCalc   int    `json:"totalHashesToCalc"`
	DuplicatesFound     int    `json:"duplicatesFound"`
	ErrorMessage        string `json:"errorMessage"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// 전역 폴더 비교 진행 상황 (메모리에 저장)
var (
	currentComparisonProgress *FolderComparisonProgress
	comparisonProgressMutex   sync.RWMutex
	lastComparisonResult      *FolderComparisonResult
	comparisonResultMutex     sync.RWMutex
	currentComparisonTask     *FolderComparisonTask
	currentTaskMutex          sync.RWMutex
)

// 폴더 비교 진행 상황 업데이트 함수들
func updateComparisonProgress(update func(*FolderComparisonProgress)) {
	comparisonProgressMutex.Lock()
	defer comparisonProgressMutex.Unlock()
	
	if currentComparisonProgress != nil {
		update(currentComparisonProgress)
		currentComparisonProgress.LastUpdated = time.Now()
	}
}

func initComparisonProgress(sourceFolderID, targetFolderID string) {
	comparisonProgressMutex.Lock()
	defer comparisonProgressMutex.Unlock()
	
	currentComparisonProgress = &FolderComparisonProgress{
		Status:      "running",
		CurrentStep: "초기화 중...",
		StartTime:   time.Now(),
		LastUpdated: time.Now(),
	}
}

func getComparisonProgress() *FolderComparisonProgress {
	comparisonProgressMutex.RLock()
	defer comparisonProgressMutex.RUnlock()
	
	if currentComparisonProgress == nil {
		return nil
	}
	
	// 복사본 반환 (동시성 안전)
	progress := *currentComparisonProgress
	return &progress
}

func saveComparisonResult(result *FolderComparisonResult) {
	comparisonResultMutex.Lock()
	defer comparisonResultMutex.Unlock()
	
	lastComparisonResult = result
}

func getComparisonResult() *FolderComparisonResult {
	comparisonResultMutex.RLock()
	defer comparisonResultMutex.RUnlock()
	
	return lastComparisonResult
}

// 폴더 비교 작업을 데이터베이스에 저장
func (ds *DriveService) saveComparisonTask(task *FolderComparisonTask) error {
	if task.ID == 0 {
		// 새로운 작업 생성
		result, err := ds.db.Exec(`
			INSERT INTO folder_comparison_tasks 
			(source_folder_id, target_folder_id, status, current_step, 
			 source_folder_scanned, source_folder_total, target_folder_scanned, target_folder_total,
			 hashes_calculated, total_hashes_to_calc, duplicates_found, error_message, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, task.SourceFolderID, task.TargetFolderID, task.Status, task.CurrentStep,
			task.SourceFolderScanned, task.SourceFolderTotal, task.TargetFolderScanned, task.TargetFolderTotal,
			task.HashesCalculated, task.TotalHashesToCalc, task.DuplicatesFound, task.ErrorMessage)
		
		if err != nil {
			return fmt.Errorf("작업 저장 실패: %v", err)
		}
		
		taskID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("작업 ID 조회 실패: %v", err)
		}
		task.ID = int(taskID)
		
		log.Printf("📝 새로운 폴더 비교 작업 저장됨: ID=%d", task.ID)
	} else {
		// 기존 작업 업데이트
		_, err := ds.db.Exec(`
			UPDATE folder_comparison_tasks SET
			status = ?, current_step = ?, 
			source_folder_scanned = ?, source_folder_total = ?, 
			target_folder_scanned = ?, target_folder_total = ?,
			hashes_calculated = ?, total_hashes_to_calc = ?, 
			duplicates_found = ?, error_message = ?,
			updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, task.Status, task.CurrentStep,
			task.SourceFolderScanned, task.SourceFolderTotal,
			task.TargetFolderScanned, task.TargetFolderTotal,
			task.HashesCalculated, task.TotalHashesToCalc,
			task.DuplicatesFound, task.ErrorMessage, task.ID)
		
		if err != nil {
			return fmt.Errorf("작업 업데이트 실패: %v", err)
		}
	}
	
	return nil
}

// 미완료된 폴더 비교 작업 조회
func (ds *DriveService) getIncompleteComparisonTask() (*FolderComparisonTask, error) {
	var task FolderComparisonTask
	
	err := ds.db.QueryRow(`
		SELECT id, source_folder_id, target_folder_id, status, current_step,
		       source_folder_scanned, source_folder_total, target_folder_scanned, target_folder_total,
		       hashes_calculated, total_hashes_to_calc, duplicates_found, error_message,
		       created_at, updated_at
		FROM folder_comparison_tasks 
		WHERE status IN ('pending', 'running') 
		ORDER BY updated_at DESC 
		LIMIT 1
	`).Scan(
		&task.ID, &task.SourceFolderID, &task.TargetFolderID, &task.Status, &task.CurrentStep,
		&task.SourceFolderScanned, &task.SourceFolderTotal, &task.TargetFolderScanned, &task.TargetFolderTotal,
		&task.HashesCalculated, &task.TotalHashesToCalc, &task.DuplicatesFound, &task.ErrorMessage,
		&task.CreatedAt, &task.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil // 미완료된 작업 없음
	} else if err != nil {
		return nil, fmt.Errorf("작업 조회 실패: %v", err)
	}
	
	return &task, nil
}

// 작업에 파일 정보 저장
func (ds *DriveService) saveComparisonFiles(taskID int, files []*DriveFile, folderType string, isDuplicates []bool) error {
	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 실패: %v", err)
	}
	defer tx.Rollback()
	
	// 기존 파일 정보 삭제 (해당 폴더 타입만)
	_, err = tx.Exec("DELETE FROM comparison_result_files WHERE task_id = ? AND folder_type = ?", taskID, folderType)
	if err != nil {
		return fmt.Errorf("기존 파일 정보 삭제 실패: %v", err)
	}
	
	// 새 파일 정보 저장
	stmt, err := tx.Prepare(`
		INSERT INTO comparison_result_files 
		(task_id, file_id, file_name, file_size, file_hash, file_path, web_view_link, mime_type, modified_time, is_duplicate, folder_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepared statement 생성 실패: %v", err)
	}
	defer stmt.Close()
	
	for i, file := range files {
		isDup := false
		if i < len(isDuplicates) {
			isDup = isDuplicates[i]
		}
		
		_, err = stmt.Exec(taskID, file.ID, file.Name, file.Size, file.Hash, file.Path, 
			file.WebViewLink, file.MimeType, file.ModifiedTime, isDup, folderType)
		if err != nil {
			return fmt.Errorf("파일 정보 저장 실패: %v", err)
		}
	}
	
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("트랜잭션 커밋 실패: %v", err)
	}
	
	log.Printf("💾 파일 정보 저장 완료: %d개 파일 (%s)", len(files), folderType)
	return nil
}

// 저장된 파일 정보 불러오기
func (ds *DriveService) loadComparisonFiles(taskID int, folderType string) ([]*DriveFile, error) {
	rows, err := ds.db.Query(`
		SELECT file_id, file_name, file_size, file_hash, file_path, web_view_link, mime_type, modified_time, is_duplicate
		FROM comparison_result_files 
		WHERE task_id = ? AND folder_type = ?
		ORDER BY file_name
	`, taskID, folderType)
	if err != nil {
		return nil, fmt.Errorf("파일 정보 조회 실패: %v", err)
	}
	defer rows.Close()
	
	var files []*DriveFile
	for rows.Next() {
		var file DriveFile
		var isDuplicate bool
		
		err := rows.Scan(&file.ID, &file.Name, &file.Size, &file.Hash, &file.Path,
			&file.WebViewLink, &file.MimeType, &file.ModifiedTime, &isDuplicate)
		if err != nil {
			return nil, fmt.Errorf("파일 정보 스캔 실패: %v", err)
		}
		
		files = append(files, &file)
	}
	
	return files, nil
}

// 두 폴더를 비교하여 대상 폴더의 중복 파일을 찾는다
func (ds *DriveService) compareFolders(sourceFolderID, targetFolderID string) (*FolderComparisonResult, error) {
	log.Printf("🔍 폴더 비교 시작: 기준 폴더 %s vs 대상 폴더 %s", sourceFolderID, targetFolderID)
	
	// 기존 미완료 작업 확인
	existingTask, err := ds.getIncompleteComparisonTask()
	if err != nil {
		log.Printf("⚠️ 기존 작업 확인 실패: %v", err)
	}
	
	var task *FolderComparisonTask
	var sourceFiles, targetFiles []*DriveFile
	
	if existingTask != nil && existingTask.SourceFolderID == sourceFolderID && existingTask.TargetFolderID == targetFolderID {
		// 기존 작업 재개
		log.Printf("🔄 기존 작업 재개: ID=%d", existingTask.ID)
		task = existingTask
		task.Status = "running"
		task.CurrentStep = "기존 작업 재개 중..."
		ds.saveComparisonTask(task)
		
		// 저장된 파일 정보 불러오기
		if task.SourceFolderTotal > 0 {
			sourceFiles, err = ds.loadComparisonFiles(task.ID, "source")
			if err != nil {
				log.Printf("⚠️ 기준 폴더 파일 정보 불러오기 실패: %v", err)
				sourceFiles = nil
			} else {
				log.Printf("📂 저장된 기준 폴더 파일 정보 불러옴: %d개", len(sourceFiles))
			}
		}
		
		if task.TargetFolderTotal > 0 {
			targetFiles, err = ds.loadComparisonFiles(task.ID, "target")
			if err != nil {
				log.Printf("⚠️ 대상 폴더 파일 정보 불러오기 실패: %v", err)
				targetFiles = nil
			} else {
				log.Printf("📂 저장된 대상 폴더 파일 정보 불러옴: %d개", len(targetFiles))
			}
		}
	} else {
		// 새로운 작업 시작
		task = &FolderComparisonTask{
			SourceFolderID: sourceFolderID,
			TargetFolderID: targetFolderID,
			Status:         "running",
			CurrentStep:    "초기화 중...",
		}
		err = ds.saveComparisonTask(task)
		if err != nil {
			return nil, fmt.Errorf("작업 저장 실패: %v", err)
		}
		log.Printf("📝 새로운 폴더 비교 작업 생성: ID=%d", task.ID)
	}
	
	// 현재 작업을 전역 변수에 저장
	currentTaskMutex.Lock()
	currentComparisonTask = task
	currentTaskMutex.Unlock()
	
	// 진행 상황 초기화
	initComparisonProgress(sourceFolderID, targetFolderID)

	// 기준 폴더 파일 스캔 (저장된 데이터가 없는 경우에만)
	if sourceFiles == nil {
		updateComparisonProgress(func(p *FolderComparisonProgress) {
			p.CurrentStep = "기준 폴더 스캔 중..."
		})
		
		sourceFiles, err = ds.getFilesInFolderWithProgress(sourceFolderID, true, "source")
		if err != nil {
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				p.Status = "error"
				p.ErrorMessage = fmt.Sprintf("기준 폴더 파일 조회 실패: %v", err)
			})
			return nil, fmt.Errorf("기준 폴더 파일 조회 실패: %v", err)
		}
		log.Printf("📁 기준 폴더에서 %d개 파일 발견", len(sourceFiles))
		
		// 스캔 완료 후 파일 정보 저장
		err = ds.saveComparisonFiles(task.ID, sourceFiles, "source", nil)
		if err != nil {
			log.Printf("⚠️ 기준 폴더 파일 정보 저장 실패: %v", err)
		}
		
		// 작업 진행 상황 업데이트
		task.SourceFolderTotal = len(sourceFiles)
		task.SourceFolderScanned = len(sourceFiles)
		ds.saveComparisonTask(task)
	} else {
		log.Printf("📂 저장된 기준 폴더 파일 정보 사용: %d개", len(sourceFiles))
	}

	// 대상 폴더 파일 스캔 (저장된 데이터가 없는 경우에만)
	if targetFiles == nil {
		updateComparisonProgress(func(p *FolderComparisonProgress) {
			p.CurrentStep = "대상 폴더 스캔 중..."
			p.SourceFolderTotal = len(sourceFiles)
		})
		
		targetFiles, err = ds.getFilesInFolderWithProgress(targetFolderID, true, "target")
		if err != nil {
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				p.Status = "error"
				p.ErrorMessage = fmt.Sprintf("대상 폴더 파일 조회 실패: %v", err)
			})
			return nil, fmt.Errorf("대상 폴더 파일 조회 실패: %v", err)
		}
		log.Printf("📁 대상 폴더에서 %d개 파일 발견", len(targetFiles))
		
		// 스캔 완료 후 파일 정보 저장
		err = ds.saveComparisonFiles(task.ID, targetFiles, "target", nil)
		if err != nil {
			log.Printf("⚠️ 대상 폴더 파일 정보 저장 실패: %v", err)
		}
		
		// 작업 진행 상황 업데이트
		task.TargetFolderTotal = len(targetFiles)
		task.TargetFolderScanned = len(targetFiles)
		ds.saveComparisonTask(task)
	} else {
		log.Printf("📂 저장된 대상 폴더 파일 정보 사용: %d개", len(targetFiles))
		
		updateComparisonProgress(func(p *FolderComparisonProgress) {
			p.SourceFolderTotal = len(sourceFiles)
		})
	}

	// 기준 폴더 파일들의 해시 맵 생성
	updateComparisonProgress(func(p *FolderComparisonProgress) {
		p.CurrentStep = "해시 맵 생성 중..."
		p.TargetFolderTotal = len(targetFiles)
	})
	
	sourceFileHashes := make(map[string]*DriveFile)
	for _, file := range sourceFiles {
		if file.Hash != "" {
			sourceFileHashes[file.Hash] = file
		}
	}
	log.Printf("🔑 기준 폴더에서 %d개 파일의 해시 생성", len(sourceFileHashes))

	// 대상 폴더에서 중복 파일 찾기
	updateComparisonProgress(func(p *FolderComparisonProgress) {
		p.CurrentStep = "중복 파일 검출 중..."
	})
	
	var duplicatesInTarget []*DriveFile
	for i, targetFile := range targetFiles {
		if targetFile.Hash != "" {
			if sourceFile, exists := sourceFileHashes[targetFile.Hash]; exists {
				log.Printf("🔄 중복 발견: %s (대상) = %s (기준)", targetFile.Name, sourceFile.Name)
				// 추가 정보를 포함한 중복 파일 정보 설정
				duplicateFile := &DriveFile{
					ID:           targetFile.ID,
					Name:         targetFile.Name,
					Size:         targetFile.Size,
					WebViewLink:  targetFile.WebViewLink,
					MimeType:     targetFile.MimeType,
					ModifiedTime: targetFile.ModifiedTime,
					Hash:         targetFile.Hash,
					Parents:      targetFile.Parents,
					Path:         targetFile.Path,
				}
				duplicatesInTarget = append(duplicatesInTarget, duplicateFile)
				
				// 진행 상황 업데이트
				updateComparisonProgress(func(p *FolderComparisonProgress) {
					p.DuplicatesFound = len(duplicatesInTarget)
				})
			}
		}
		
		// 진행률 업데이트 및 주기적 저장 (50개마다)
		if i%50 == 0 || i == len(targetFiles)-1 {
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				p.CurrentStep = fmt.Sprintf("중복 파일 검출 중... (%d/%d)", i+1, len(targetFiles))
			})
			
			// 작업 진행 상황을 데이터베이스에 저장
			task.DuplicatesFound = len(duplicatesInTarget)
			ds.saveComparisonTask(task)
		}
	}

	// 완료 상태 업데이트
	updateComparisonProgress(func(p *FolderComparisonProgress) {
		p.Status = "completed"
		p.CurrentStep = "완료"
		p.DuplicatesFound = len(duplicatesInTarget)
	})

	// 데이터베이스 작업을 완료로 표시
	task.Status = "completed"
	task.CurrentStep = "완료"
	task.DuplicatesFound = len(duplicatesInTarget)
	ds.saveComparisonTask(task)

	// 폴더 삭제 권장 분석
	canDeleteTargetFolder := false
	duplicationPercentage := 0.0
	var targetFolderName string
	
	if len(targetFiles) > 0 {
		duplicationPercentage = float64(len(duplicatesInTarget)) / float64(len(targetFiles)) * 100
		// 100% 중복이면 폴더 전체 삭제 권장
		canDeleteTargetFolder = duplicationPercentage >= 100.0
		
		if canDeleteTargetFolder {
			log.Printf("🎯 폴더 전체 삭제 권장: 대상 폴더의 %.1f%% (%d/%d)가 중복됨", 
				duplicationPercentage, len(duplicatesInTarget), len(targetFiles))
		}
	}
	
	// 대상 폴더 정보 조회
	if canDeleteTargetFolder {
		folderInfo, err := ds.service.Files.Get(targetFolderID).Fields("id,name").Do()
		if err != nil {
			log.Printf("⚠️ 대상 폴더 정보 조회 실패: %v", err)
		} else {
			targetFolderName = folderInfo.Name
		}
	}

	result := &FolderComparisonResult{
		SourceFolder:       sourceFolderID,
		TargetFolder:       targetFolderID,
		SourceFiles:        sourceFiles,
		TargetFiles:        targetFiles,
		DuplicatesInTarget: duplicatesInTarget,
		TotalDuplicates:    len(duplicatesInTarget),
		
		// 폴더 삭제 권장 정보
		CanDeleteTargetFolder:  canDeleteTargetFolder,
		TargetFolderName:      targetFolderName,
		TargetFolderID:        targetFolderID,
		DuplicationPercentage: duplicationPercentage,
	}

	// 결과 저장
	saveComparisonResult(result)
	
	log.Printf("✅ 폴더 비교 완료: 대상 폴더에서 %d개 중복 파일 발견", len(duplicatesInTarget))
	return result, nil
}

// 진행 상황 추적과 함께 폴더 스캔
func (ds *DriveService) getFilesInFolderWithProgress(folderID string, calculateHashes bool, folderType string) ([]*DriveFile, error) {
	var allFiles []*DriveFile
	pageToken := ""

	for {
		query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
		listCall := ds.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, webViewLink, parents)").
			PageSize(1000)

		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}

		response, err := listCall.Do()
		if err != nil {
			return nil, fmt.Errorf("폴더 파일 목록 조회 실패: %v", err)
		}

		for i, file := range response.Files {
			// 폴더는 제외, 파일만 처리
			if file.MimeType != "application/vnd.google-apps.folder" {
				// 빈 파일만 필터링
				if file.Size <= 0 {
					log.Printf("⏭️ 빈 파일 스킵: %s", file.Name)
					continue
				}
				
				driveFile := &DriveFile{
					ID:           file.Id,
					Name:         file.Name,
					Size:         file.Size,
					WebViewLink:  file.WebViewLink,
					MimeType:     file.MimeType,
					ModifiedTime: file.ModifiedTime,
					Parents:      file.Parents,
				}

				// 경로 계산
				if len(file.Parents) > 0 {
					driveFile.Path = ds.buildFullPath(file.Parents[0])
				} else {
					driveFile.Path = "/"
				}

				// 해시는 나중에 병렬로 계산

				allFiles = append(allFiles, driveFile)
				
				// 진행률 업데이트
				if folderType == "source" {
					updateComparisonProgress(func(p *FolderComparisonProgress) {
						p.SourceFolderScanned = len(allFiles)
					})
				} else {
					updateComparisonProgress(func(p *FolderComparisonProgress) {
						p.TargetFolderScanned = len(allFiles)
					})
				}
			} else {
				// 하위 폴더가 있는 경우 재귀적으로 처리
				subFiles, err := ds.getFilesInFolderWithProgress(file.Id, calculateHashes, folderType)
				if err != nil {
					log.Printf("⚠️ 하위 폴더 처리 실패 (%s): %v", file.Name, err)
					continue
				}
				allFiles = append(allFiles, subFiles...)
			}
			
			// 100개마다 진행 상황 로그
			if (i+1)%100 == 0 {
				log.Printf("📄 %s 폴더 스캔 중: %d개 파일 처리됨", folderType, len(allFiles))
			}
		}

		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}

	// 파일 스캔 완료 알림
	log.Printf("✅ 폴더 스캔 완료: %s 타입, %d개 파일 발견", folderType, len(allFiles))
	
	// 해시 계산이 필요한 경우 충분한 검증 후 병렬 처리
	if calculateHashes {
		log.Printf("🔍 해시 계산 준비 중: %s 폴더", folderType)
		
		// 파일 스캔이 완전히 끝날 때까지 잠시 대기
		time.Sleep(500 * time.Millisecond)
		
		// 크기가 0보다 큰 파일들만 필터링하고 유효성 검증
		var filesToHash []*DriveFile
		for _, file := range allFiles {
			if file.Size > 0 && file.ID != "" && file.Name != "" {
				// 파일 정보가 완전한지 한 번 더 검증
				if len(file.ID) > 10 { // Google Drive 파일 ID는 일반적으로 28자 이상
					filesToHash = append(filesToHash, file)
				} else {
					log.Printf("⚠️ 불완전한 파일 정보 건너뜀: ID=%s, Name=%s", file.ID, file.Name)
				}
			}
		}
		
		log.Printf("📊 해시 계산 대상: 전체 %d개 중 %d개 파일 (크기 > 0, 유효한 ID)", len(allFiles), len(filesToHash))
		
		if len(filesToHash) > 0 {
			// 진행률 업데이트를 더 명확하게
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				if folderType == "source" {
					p.CurrentStep = fmt.Sprintf("📁 기준 폴더 스캔 완료 (%d개) → 🔑 해시 계산 시작...", len(allFiles))
				} else {
					p.CurrentStep = fmt.Sprintf("📁 대상 폴더 스캔 완료 (%d개) → 🔑 해시 계산 시작...", len(allFiles))
				}
				p.TotalHashesToCalc = len(filesToHash)
			})
			
			// 해시 계산 시작 전 한 번 더 대기
			time.Sleep(1 * time.Second)
			log.Printf("🚀 해시 계산 시작: %d개 파일", len(filesToHash))
			
			// 병렬 해시 계산 (설정된 워커 개수 사용)
			maxWorkers := getMaxWorkers()
			err := ds.calculateHashesInParallel(filesToHash, maxWorkers)
			if err != nil {
				log.Printf("⚠️ 병렬 해시 계산 실패: %v", err)
			} else {
				log.Printf("✅ 해시 계산 완료: %s 폴더", folderType)
			}
		} else {
			log.Printf("ℹ️ 해시 계산할 파일이 없음: %s 폴더", folderType)
		}
	}

	return allFiles, nil
}

// 폴더 내의 모든 파일을 재귀적으로 조회하고 해시를 계산한다 (기존 함수 유지)
func (ds *DriveService) getFilesInFolder(folderID string, calculateHashes bool) ([]*DriveFile, error) {
	var allFiles []*DriveFile
	pageToken := ""

	for {
		query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
		listCall := ds.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, size, mimeType, modifiedTime, webViewLink, parents)").
			PageSize(1000)

		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}

		response, err := listCall.Do()
		if err != nil {
			return nil, fmt.Errorf("폴더 파일 목록 조회 실패: %v", err)
		}

		for _, file := range response.Files {
			// 폴더는 제외, 파일만 처리
			if file.MimeType != "application/vnd.google-apps.folder" {
				driveFile := &DriveFile{
					ID:           file.Id,
					Name:         file.Name,
					Size:         file.Size,
					WebViewLink:  file.WebViewLink,
					MimeType:     file.MimeType,
					ModifiedTime: file.ModifiedTime,
					Parents:      file.Parents,
				}

				// 경로 계산
				if len(file.Parents) > 0 {
					driveFile.Path = ds.buildFullPath(file.Parents[0])
				} else {
					driveFile.Path = "/"
				}

				// 해시 계산이 필요한 경우
				if calculateHashes && file.Size > 0 {
					hash, err := ds.calculateFileHash(file.Id)
					if err != nil {
						log.Printf("⚠️ 해시 계산 실패 (%s): %v", file.Name, err)
						continue
					}
					driveFile.Hash = hash
				}

				allFiles = append(allFiles, driveFile)
			} else {
				// 하위 폴더가 있는 경우 재귀적으로 처리
				subFiles, err := ds.getFilesInFolder(file.Id, calculateHashes)
				if err != nil {
					log.Printf("⚠️ 하위 폴더 처리 실패 (%s): %v", file.Name, err)
					continue
				}
				allFiles = append(allFiles, subFiles...)
			}
		}

		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return allFiles, nil
}

// 폴더 ID를 URL에서 추출하는 헬퍼 함수
func extractFolderIDFromURL(folderURL string) string {
	// https://drive.google.com/drive/folders/1ABC123 형태에서 ID 추출
	re := regexp.MustCompile(`folders/([a-zA-Z0-9-_]+)`)
	matches := re.FindStringSubmatch(folderURL)
	if len(matches) > 1 {
		return matches[1]
	}
	return folderURL // URL이 아닌 경우 그대로 반환 (이미 ID일 수 있음)
}

func (ds *DriveService) clearAllData() error {
	log.Println("🗑️ 모든 데이터 삭제 중...")
	
	_, err := ds.db.Exec("DELETE FROM duplicate_files")
	if err != nil {
		return err
	}
	_, err = ds.db.Exec("DELETE FROM duplicate_groups")
	if err != nil {
		return err
	}
	_, err = ds.db.Exec("DELETE FROM files")
	if err != nil {
		return err
	}
	_, err = ds.db.Exec("DELETE FROM progress")
	if err != nil {
		return err
	}
	
	log.Println("✅ 모든 데이터 삭제 완료")
	return nil
}

func (ds *DriveService) ListAllFiles() ([]*DriveFile, error) {
	// DB에서 파일 목록 로드 시도
	dbFiles, err := ds.loadFilesFromDB()
	if err == nil && len(dbFiles) > 0 {
		log.Printf("✅ 데이터베이스에서 %d개 파일 로드 완료", len(dbFiles))
		return dbFiles, nil
	}
	log.Println("ℹ️ 데이터베이스가 비어있거나 오류 발생. 새로 조회합니다.")

	// 진행 상태 확인 (재개 가능한지)
	progress, err := ds.loadProgress()
	if err == nil && progress.Status == "scanning" && progress.LastPageToken != "" {
		log.Printf("🔄 이전 스캔을 %d 페이지부터 재개합니다", progress.LastPageCount+1)
		return ds.resumeFileScanning(progress)
	}

	// 새로운 스캔 시작
	return ds.startNewFileScanning()
}

func (ds *DriveService) startNewFileScanning() ([]*DriveFile, error) {
	var allFiles []*DriveFile
	pageToken := ""
	pageCount := 0

	log.Println("📄 Google Drive 파일 목록을 페이지별로 조회하고 있습니다...")

	// 진행 상태 초기화
	progress := ProgressData{
		Status:        "scanning",
		LastPageCount: 0,
		LastPageToken: "",
	}
	ds.saveProgress(progress)

	for {
		pageCount++
		log.Printf("📑 페이지 %d 조회 중...", pageCount)
		
		query := ds.service.Files.List().
			Q("trashed=false").
			PageSize(1000).
			Fields("nextPageToken, files(id, name, size, webViewLink, mimeType, modifiedTime, parents)")

		if pageToken != "" {
			query = query.PageToken(pageToken)
		}

		var result *drive.FileList
		err := ds.retryWithBackoff(func() error {
			var err error
			result, err = query.Do()
			return err
		})
		
		if err != nil {
			log.Printf("❌ 페이지 %d 조회 실패: %v", pageCount, err)
			
			// 진행 상태 저장 (재개 가능하도록)
			progress.LastPageCount = pageCount - 1
			progress.LastPageToken = pageToken
			progress.Status = "interrupted"
			ds.saveProgress(progress)
			
			// 5페이지 이상 성공적으로 조회했다면 부분 결과라도 반환
			if pageCount > 5 && len(allFiles) > 0 {
				log.Printf("⚠️ %d개 페이지까지 성공한 부분 결과를 반환합니다 (총 %d개 파일)", pageCount-1, len(allFiles))
				
				// 부분 결과를 DB에 저장
				if err := ds.saveFilesToDB(allFiles); err != nil {
					log.Printf("⚠️ 데이터베이스 저장 실패: %v", err)
				}
				
				return allFiles, nil
			}
			
			return nil, fmt.Errorf("파일 목록 조회 오류: %v", err)
		}

		validFiles := 0
		for _, file := range result.Files {
			if file.Size > 0 {
				log.Printf("🔍 API에서 받은 파일: %s, Parents: %v", file.Name, file.Parents)
				driveFile := &DriveFile{
					ID:           file.Id,
					Name:         file.Name,
					Size:         file.Size,
					WebViewLink:  file.WebViewLink,
					MimeType:     file.MimeType,
					ModifiedTime: file.ModifiedTime,
					Parents:      file.Parents,
					Path:         "", // 나중에 필요할 때 계산
				}
				
				allFiles = append(allFiles, driveFile)
				validFiles++
			}
		}
		
		log.Printf("✅ 페이지 %d: %d개 파일 발견 (크기가 있는 파일 %d개)", pageCount, len(result.Files), validFiles)

		// 진행 상태 업데이트
		progress.LastPageCount = pageCount
		progress.LastPageToken = result.NextPageToken
		ds.saveProgress(progress)

		// 20페이지마다 중간 저장 (20,000개 파일마다)
		if pageCount%20 == 0 {
			log.Printf("🔄 중간 저장: %d개 페이지까지 %d개 파일", pageCount, len(allFiles))
			if err := ds.saveFilesToDB(allFiles); err != nil {
				log.Printf("⚠️ 중간 저장 실패: %v", err)
			}
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	log.Printf("📋 전체 조회 완료: %d개 페이지에서 총 %d개 파일", pageCount, len(allFiles))
	
	// 완료 상태로 업데이트
	progress.Status = "scan_completed"
	ds.saveProgress(progress)
	
	// 파일 목록을 DB에 저장
	if err := ds.saveFilesToDB(allFiles); err != nil {
		log.Printf("⚠️ 데이터베이스 저장 실패: %v", err)
	}
	
	return allFiles, nil
}

func (ds *DriveService) resumeFileScanning(progress *ProgressData) ([]*DriveFile, error) {
	// 기존 파일들 로드
	allFiles, err := ds.loadFilesFromDB()
	if err != nil {
		log.Printf("⚠️ 기존 파일 로드 실패, 새로 시작합니다: %v", err)
		return ds.startNewFileScanning()
	}

	pageToken := progress.LastPageToken
	pageCount := progress.LastPageCount

	log.Printf("🔄 페이지 %d부터 파일 스캔을 재개합니다 (기존 %d개 파일)", pageCount+1, len(allFiles))

	for pageToken != "" {
		pageCount++
		log.Printf("📑 페이지 %d 조회 중... (재개 모드)", pageCount)
		
		query := ds.service.Files.List().
			Q("trashed=false").
			PageSize(1000).
			Fields("nextPageToken, files(id, name, size, webViewLink, mimeType, modifiedTime, parents)").
			PageToken(pageToken)

		var result *drive.FileList
		err := ds.retryWithBackoff(func() error {
			var err error
			result, err = query.Do()
			return err
		})
		
		if err != nil {
			log.Printf("❌ 페이지 %d 조회 실패: %v", pageCount, err)
			
			// 진행 상태 저장
			progress.LastPageCount = pageCount - 1
			progress.LastPageToken = pageToken
			progress.Status = "interrupted"
			ds.saveProgress(*progress)
			
			return nil, fmt.Errorf("파일 목록 조회 오류: %v", err)
		}

		validFiles := 0
		var newFiles []*DriveFile
		for _, file := range result.Files {
			if file.Size > 0 {
				log.Printf("🔍 API에서 받은 파일 (resume): %s, Parents: %v", file.Name, file.Parents)
				driveFile := &DriveFile{
					ID:           file.Id,
					Name:         file.Name,
					Size:         file.Size,
					WebViewLink:  file.WebViewLink,
					MimeType:     file.MimeType,
					ModifiedTime: file.ModifiedTime,
					Parents:      file.Parents,
					Path:         "", // 나중에 필요할 때 계산
				}
				
				allFiles = append(allFiles, driveFile)
				newFiles = append(newFiles, driveFile)
				validFiles++
			}
		}
		
		log.Printf("✅ 페이지 %d: %d개 파일 발견 (크기가 있는 파일 %d개)", pageCount, len(result.Files), validFiles)

		// 새 파일들을 DB에 추가
		if len(newFiles) > 0 {
			if err := ds.saveFilesToDB(newFiles); err != nil {
				log.Printf("⚠️ 새 파일 저장 실패: %v", err)
			}
		}

		// 진행 상태 업데이트
		progress.LastPageCount = pageCount
		progress.LastPageToken = result.NextPageToken
		ds.saveProgress(*progress)

		pageToken = result.NextPageToken
	}

	log.Printf("📋 스캔 재개 완료: %d개 페이지에서 총 %d개 파일", pageCount, len(allFiles))
	
	// 완료 상태로 업데이트
	progress.Status = "scan_completed"
	ds.saveProgress(*progress)
	
	return allFiles, nil
}

func (ds *DriveService) retryWithBackoff(operation func() error) error {
	maxRetries := 5
	baseDelay := time.Second
	maxDelay := 64 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		// Google API 에러 체크
		if apiErr, ok := err.(*googleapi.Error); ok {
			switch apiErr.Code {
			case 500, 502, 503, 504: // 서버 에러
				if attempt == maxRetries-1 {
					return err
				}
				delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
				if delay > maxDelay {
					delay = maxDelay
				}
				log.Printf("⚠️ API 서버 오류 (시도 %d/%d): %v - %v 후 재시도", attempt+1, maxRetries, apiErr.Code, delay)
				time.Sleep(delay)
				continue
			case 429: // Rate limit exceeded
				if attempt == maxRetries-1 {
					return err
				}
				delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
				if delay > maxDelay {
					delay = maxDelay
				}
				log.Printf("⚠️ API 속도 제한 (시도 %d/%d): %v 후 재시도", attempt+1, maxRetries, delay)
				time.Sleep(delay)
				continue
			default:
				return err
			}
		}

		// 기타 일시적 오류 체크
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "timeout") || 
		   strings.Contains(errStr, "connection reset") || 
		   strings.Contains(errStr, "temporary failure") {
			if attempt == maxRetries-1 {
				return err
			}
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			if delay > maxDelay {
				delay = maxDelay
			}
			log.Printf("⚠️ 일시적 네트워크 오류 (시도 %d/%d): %v - %v 후 재시도", attempt+1, maxRetries, err, delay)
			time.Sleep(delay)
			continue
		}

		return err
	}

	return fmt.Errorf("최대 재시도 횟수 초과")
}

func (ds *DriveService) DownloadFileContent(fileID string) ([]byte, error) {
	var content []byte
	err := ds.retryWithBackoff(func() error {
		resp, err := ds.service.Files.Get(fileID).Download()
		if err != nil {
			return fmt.Errorf("파일 다운로드 오류: %v", err)
		}
		defer resp.Body.Close()

		content, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("파일 내용 읽기 오류: %v", err)
		}
		return nil
	})

	return content, err
}

func calculateHash(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}

// 파일 ID로부터 파일을 다운로드하여 해시를 계산하는 메서드 (최적화 버전)
func (ds *DriveService) calculateFileHash(fileID string) (string, error) {
	return ds.calculateFileHashWithRetry(fileID, 3)
}

func (ds *DriveService) calculateFileHashWithRetry(fileID string, maxRetries int) (string, error) {
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		hash, err := ds.calculateFileHashOnce(fileID)
		if err == nil {
			return hash, nil
		}
		
		lastErr = err
		if attempt < maxRetries {
			// 지수 백오프: 1초, 2초, 4초 대기
			waitTime := time.Duration(attempt) * time.Second
			log.Printf("🔄 해시 계산 재시도 %d/%d (%s): %v, %v 대기", attempt, maxRetries, fileID, err, waitTime)
			time.Sleep(waitTime)
		}
	}
	
	return "", fmt.Errorf("최대 재시도 후 실패: %v", lastErr)
}

func (ds *DriveService) calculateFileHashOnce(fileID string) (string, error) {
	// 파일 ID 유효성 검증
	if fileID == "" || len(fileID) < 10 {
		return "", fmt.Errorf("유효하지 않은 파일 ID: %s", fileID)
	}
	
	log.Printf("🔍 해시 계산 시작: %s", fileID)
	
	// Google Drive API를 통해 파일 내용 다운로드 (타임아웃 없음)
	resp, err := ds.service.Files.Get(fileID).Download()
	if err != nil {
		return "", fmt.Errorf("파일 다운로드 실패: %v", err)
	}
	defer resp.Body.Close()
	
	// 간단한 해시 계산
	hasher := sha256.New()
	
	// io.Copy를 사용한 간단한 복사
	_, err = io.Copy(hasher, resp.Body)
	if err != nil {
		return "", fmt.Errorf("파일 읽기 실패: %v", err)
	}
	
	log.Printf("✅ 해시 계산 완료: %s", fileID)
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// 중복 그룹을 데이터베이스에서 제거
func (ds *DriveService) removeDuplicateGroup(groupHash string) error {
	if groupHash == "" {
		return fmt.Errorf("그룹 해시가 필요합니다")
	}

	log.Printf("🗑️ 중복 그룹 제거 요청: %s", groupHash)

	// 해당 해시를 가진 모든 파일을 데이터베이스에서 삭제
	query := `DELETE FROM files WHERE hash = ?`
	result, err := ds.db.Exec(query, groupHash)
	if err != nil {
		return fmt.Errorf("데이터베이스에서 중복 그룹 삭제 실패: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("⚠️ 삭제된 행 수 확인 실패: %v", err)
	} else {
		log.Printf("✅ 중복 그룹 제거 완료: %d개 파일 제거됨", rowsAffected)
	}

	return nil
}

// 삭제된 파일들을 데이터베이스에서 정리
func (ds *DriveService) cleanupDeletedFiles() (int, error) {
	log.Printf("🧹 삭제된 파일 정리 시작...")

	// 데이터베이스에서 모든 파일 ID 가져오기
	query := `SELECT id, name FROM files`
	rows, err := ds.db.Query(query)
	if err != nil {
		return 0, fmt.Errorf("파일 목록 조회 실패: %v", err)
	}
	defer rows.Close()

	var filesToCheck []struct {
		ID   string
		Name string
	}

	for rows.Next() {
		var file struct {
			ID   string
			Name string
		}
		if err := rows.Scan(&file.ID, &file.Name); err != nil {
			log.Printf("⚠️ 파일 정보 스캔 실패: %v", err)
			continue
		}
		filesToCheck = append(filesToCheck, file)
	}

	log.Printf("📊 총 %d개 파일 확인 중...", len(filesToCheck))

	deletedCount := 0
	for i, file := range filesToCheck {
		if i%100 == 0 {
			log.Printf("📋 진행률: %d/%d (%d개 삭제됨)", i, len(filesToCheck), deletedCount)
		}

		// Google Drive API로 파일 존재 여부 확인
		_, err := ds.service.Files.Get(file.ID).Fields("id,trashed").Do()
		if err != nil {
			// 파일이 존재하지 않는 경우
			log.Printf("❌ 삭제된 파일 발견: %s (%s)", file.Name, file.ID)
			
			// 데이터베이스에서 제거
			deleteErr := ds.deleteFileFromDB(file.ID)
			if deleteErr != nil {
				log.Printf("⚠️ DB에서 파일 삭제 실패: %v", deleteErr)
			} else {
				deletedCount++
			}
		}
	}

	log.Printf("✅ 삭제된 파일 정리 완료: %d개 파일 제거됨", deletedCount)
	return deletedCount, nil
}

// 빈 폴더인지 확인
func (ds *DriveService) isFolderEmpty(folderID string) (bool, error) {
	query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
	listCall := ds.service.Files.List().
		Q(query).
		Fields("files(id)").
		PageSize(1) // 하나라도 있으면 빈 폴더가 아님

	response, err := listCall.Do()
	if err != nil {
		return false, fmt.Errorf("폴더 내용 확인 실패: %v", err)
	}

	return len(response.Files) == 0, nil
}

// 폴더 삭제
func (ds *DriveService) deleteFolder(folderID string) error {
	// 폴더 정보 먼저 가져오기
	folderInfo, err := ds.service.Files.Get(folderID).Fields("id,name,parents").Do()
	if err != nil {
		return fmt.Errorf("폴더 정보 조회 실패: %v", err)
	}

	log.Printf("🗑️ 빈 폴더 삭제: %s (%s)", folderInfo.Name, folderID)

	// 폴더 삭제
	err = ds.service.Files.Delete(folderID).Do()
	if err != nil {
		return fmt.Errorf("폴더 삭제 실패: %v", err)
	}

	log.Printf("✅ 폴더 삭제 완료: %s", folderInfo.Name)

	// 상위 폴더도 빈 폴더인지 재귀적으로 확인
	if len(folderInfo.Parents) > 0 {
		parentFolderID := folderInfo.Parents[0]
		return ds.checkAndDeleteEmptyFolder(parentFolderID)
	}

	return nil
}

// 폴더가 비어있으면 삭제하고 상위 폴더도 재귀적으로 확인
func (ds *DriveService) checkAndDeleteEmptyFolder(folderID string) error {
	// 루트 폴더나 특수 폴더는 삭제하지 않음
	if folderID == "" || folderID == "root" {
		return nil
	}

	// 폴더 정보 확인
	folderInfo, err := ds.service.Files.Get(folderID).Fields("id,name,mimeType,parents").Do()
	if err != nil {
		// 폴더가 이미 삭제되었거나 접근할 수 없는 경우
		log.Printf("⚠️ 폴더 정보 조회 실패 (%s): %v", folderID, err)
		return nil
	}

	// 폴더가 아닌 경우 건너뛰기
	if folderInfo.MimeType != "application/vnd.google-apps.folder" {
		return nil
	}

	// 빈 폴더인지 확인
	isEmpty, err := ds.isFolderEmpty(folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 빈 상태 확인 실패 (%s): %v", folderInfo.Name, err)
		return nil
	}

	if isEmpty {
		log.Printf("📂 빈 폴더 발견: %s (%s)", folderInfo.Name, folderID)
		
		// 폴더 삭제
		err = ds.service.Files.Delete(folderID).Do()
		if err != nil {
			log.Printf("⚠️ 폴더 삭제 실패 (%s): %v", folderInfo.Name, err)
			return nil
		}

		log.Printf("✅ 빈 폴더 삭제 완료: %s", folderInfo.Name)

		// 상위 폴더도 빈 폴더인지 재귀적으로 확인
		if len(folderInfo.Parents) > 0 {
			return ds.checkAndDeleteEmptyFolder(folderInfo.Parents[0])
		}
	}

	return nil
}

// 파일 삭제 후 상위 폴더들의 빈 상태 확인 및 삭제
func (ds *DriveService) cleanupEmptyFoldersAfterFileDeletion(fileID string) error {
	// 삭제된 파일의 상위 폴더 정보 조회
	fileInfo, err := ds.service.Files.Get(fileID).Fields("id,name,parents").Do()
	if err != nil {
		// 파일이 이미 삭제되었으므로 데이터베이스에서 상위 폴더 정보를 가져와야 함
		log.Printf("⚠️ 삭제된 파일의 상위 폴더 확인 불가: %s", fileID)
		return nil
	}

	// 상위 폴더가 있으면 빈 폴더인지 확인
	if len(fileInfo.Parents) > 0 {
		return ds.checkAndDeleteEmptyFolder(fileInfo.Parents[0])
	}

	return nil
}

// 전체 드라이브에서 빈 폴더들을 찾아서 정리
func (ds *DriveService) cleanupAllEmptyFolders() (int, error) {
	log.Printf("📂 전체 드라이브 빈 폴더 정리 시작...")

	// 모든 폴더 조회
	query := "mimeType='application/vnd.google-apps.folder' and trashed=false"
	var allFolders []*drive.File
	pageToken := ""

	for {
		listCall := ds.service.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, parents)").
			PageSize(1000)

		if pageToken != "" {
			listCall = listCall.PageToken(pageToken)
		}

		response, err := listCall.Do()
		if err != nil {
			return 0, fmt.Errorf("폴더 목록 조회 실패: %v", err)
		}

		allFolders = append(allFolders, response.Files...)

		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}

	log.Printf("📊 총 %d개 폴더 확인 중...", len(allFolders))

	deletedCount := 0
	
	// 하위 폴더부터 정리하기 위해 역순으로 처리
	for i := len(allFolders) - 1; i >= 0; i-- {
		folder := allFolders[i]
		
		if i%100 == 0 {
			log.Printf("📋 진행률: %d/%d (%d개 삭제됨)", len(allFolders)-i, len(allFolders), deletedCount)
		}

		// 루트 폴더나 특수 폴더는 건너뛰기
		if folder.Id == "root" || len(folder.Parents) == 0 {
			continue
		}

		// 빈 폴더인지 확인
		isEmpty, err := ds.isFolderEmpty(folder.Id)
		if err != nil {
			log.Printf("⚠️ 폴더 빈 상태 확인 실패 (%s): %v", folder.Name, err)
			continue
		}

		if isEmpty {
			log.Printf("📂 빈 폴더 발견: %s (%s)", folder.Name, folder.Id)
			
			// 폴더 삭제
			err = ds.service.Files.Delete(folder.Id).Do()
			if err != nil {
				log.Printf("⚠️ 폴더 삭제 실패 (%s): %v", folder.Name, err)
				continue
			}

			log.Printf("✅ 빈 폴더 삭제 완료: %s", folder.Name)
			deletedCount++

			// API 제한을 위한 짧은 대기
			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("✅ 빈 폴더 정리 완료: %d개 폴더 삭제됨", deletedCount)
	return deletedCount, nil
}

// 대상 폴더 전체 삭제 (중복 비교 후)
func (ds *DriveService) deleteTargetFolder(folderID, folderName string) error {
	log.Printf("🗑️ 대상 폴더 삭제 시작: %s (%s)", folderName, folderID)

	// 폴더 존재 여부 확인
	folderInfo, err := ds.service.Files.Get(folderID).Fields("id,name,trashed").Do()
	if err != nil {
		return fmt.Errorf("폴더 정보 조회 실패: %v", err)
	}

	if folderInfo.Trashed {
		return fmt.Errorf("폴더가 이미 휴지통에 있습니다: %s", folderName)
	}

	// 폴더 내용 확인 (마지막 안전 확인)
	query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
	listCall := ds.service.Files.List().
		Q(query).
		Fields("files(id,name,mimeType)").
		PageSize(10) // 몇 개만 확인

	response, err := listCall.Do()
	if err != nil {
		return fmt.Errorf("폴더 내용 확인 실패: %v", err)
	}

	log.Printf("📂 폴더 '%s'에 %d개 항목 확인됨", folderName, len(response.Files))

	// 빈 폴더가 아닌 경우 경고 (하지만 계속 진행)
	if len(response.Files) > 0 {
		log.Printf("⚠️ 주의: 폴더 '%s'에 %d개 파일/폴더가 있습니다. 모두 삭제됩니다.", folderName, len(response.Files))
	}

	// 폴더 삭제 실행
	err = ds.service.Files.Delete(folderID).Do()
	if err != nil {
		return fmt.Errorf("폴더 삭제 실패: %v", err)
	}

	log.Printf("✅ 대상 폴더 삭제 완료: %s", folderName)
	return nil
}

// 병렬 해시 계산을 위한 구조체
type hashJob struct {
	file   *DriveFile
	result chan hashResult
}

type hashResult struct {
	fileID string
	hash   string
	err    error
}

// 병렬 해시 계산 (최적화된 워커 풀 패턴)
func (ds *DriveService) calculateHashesInParallel(files []*DriveFile, maxWorkers int) error {
	if len(files) == 0 {
		return nil
	}
	
	// 최적화된 워커 수 계산 (너무 많으면 오히려 느려짐)
	optimalWorkers := maxWorkers
	if maxWorkers > 8 && len(files) < maxWorkers*4 {
		optimalWorkers = max(4, len(files)/4)
		log.Printf("⚙️ 워커 수 최적화: %d -> %d (파일 수 대비 조정)", maxWorkers, optimalWorkers)
	}
	
	// 파일 유효성 재검증
	var validFiles []*DriveFile
	for _, file := range files {
		if file.ID != "" && len(file.ID) > 10 && file.Size > 0 {
			validFiles = append(validFiles, file)
		} else {
			log.Printf("⚠️ 해시 계산에서 유효하지 않은 파일 제외: ID=%s, Size=%d", file.ID, file.Size)
		}
	}
	
	if len(validFiles) != len(files) {
		log.Printf("📋 파일 유효성 검증: %d개 → %d개 파일로 필터링됨", len(files), len(validFiles))
		files = validFiles
	}
	
	log.Printf("🚀 최적화된 병렬 해시 계산 시작: %d개 검증된 파일, %d개 워커", len(files), optimalWorkers)
	
	// 채널 버퍼 크기 최적화
	jobs := make(chan hashJob, min(optimalWorkers*3, 1000))
	results := make(chan hashResult, optimalWorkers*2)
	
	// 성능 모니터링
	startTime := time.Now()
	
	// 워커 시작
	var wg sync.WaitGroup
	for w := 0; w < optimalWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			localCount := 0
			localStart := time.Now()
			
			for job := range jobs {
				hash, err := ds.calculateFileHash(job.file.ID)
				results <- hashResult{
					fileID: job.file.ID,
					hash:   hash,
					err:    err,
				}
				
				localCount++
				// 워커별 성능 로깅 (50개마다)
				if localCount%50 == 0 {
					elapsed := time.Since(localStart)
					rate := float64(localCount) / elapsed.Seconds()
					log.Printf("📊 워커 %d: %d개 처리, 속도: %.1f/s", workerID, localCount, rate)
				}
			}
			
			elapsed := time.Since(localStart)
			rate := float64(localCount) / elapsed.Seconds()
			log.Printf("✅ 워커 %d 완료: %d개 처리, 평균 %.1f/s", workerID, localCount, rate)
		}(w)
	}
	
	// 작업 큐에 추가
	go func() {
		defer close(jobs)
		for i, file := range files {
			jobs <- hashJob{file: file}
			
			// 큐 진행 상황 로깅 (1000개마다)
			if i > 0 && i%1000 == 0 {
				log.Printf("📤 큐 진행: %d/%d개 추가됨", i+1, len(files))
			}
		}
		log.Printf("📤 모든 작업이 큐에 추가됨: %d개", len(files))
	}()
	
	// 워커 완료 대기
	go func() {
		wg.Wait()
		close(results)
	}()
	
	// 결과 수집
	hashMap := make(map[string]string)
	completedCount := 0
	lastUpdateTime := time.Now()
	var failCount int
	
	for result := range results {
		completedCount++
		
		if result.err != nil {
			failCount++
			log.Printf("⚠️ 해시 계산 실패 (%s): %v", result.fileID, result.err)
			continue
		}
		
		hashMap[result.fileID] = result.hash
		
		// 최적화된 진행 상황 업데이트 (5초마다 또는 20개마다)
		now := time.Now()
		if completedCount%20 == 0 || now.Sub(lastUpdateTime) >= 5*time.Second {
			percentage := float64(completedCount) / float64(len(files)) * 100
			elapsed := time.Since(startTime)
			rate := float64(completedCount) / elapsed.Seconds()
			
			var eta string
			if rate > 0 {
				etaSeconds := float64(len(files)-completedCount) / rate
				eta = time.Duration(etaSeconds * float64(time.Second)).Round(time.Second).String()
			} else {
				eta = "계산 중..."
			}
			
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				p.HashesCalculated = len(hashMap)
				p.CurrentStep = fmt.Sprintf("해시 계산 중... (%d/%d, %.1f%%, %.1f/s, ETA: %s)", 
					completedCount, len(files), percentage, rate, eta)
			})
			
			log.Printf("📈 진행: %d/%d (%.1f%%), 성공: %d, 실패: %d, 속도: %.1f/s, 예상완료: %s", 
				completedCount, len(files), percentage, len(hashMap), failCount, rate, eta)
			lastUpdateTime = now
		}
	}
	
	// 파일 객체에 해시 할당
	successCount := 0
	for _, file := range files {
		if hash, exists := hashMap[file.ID]; exists {
			file.Hash = hash
			successCount++
		}
	}
	
	// 최종 통계
	elapsed := time.Since(startTime)
	avgRate := float64(completedCount) / elapsed.Seconds()
	
	// 최종 진행 상황 업데이트
	updateComparisonProgress(func(p *FolderComparisonProgress) {
		p.HashesCalculated = successCount
		p.CurrentStep = fmt.Sprintf("해시 계산 완료 (%d 성공, %d 실패, 평균 %.1f/s)", 
			successCount, failCount, avgRate)
	})
	
	log.Printf("🏁 병렬 해시 계산 완료!")
	log.Printf("📊 총 %d개 파일 중 %d개 성공, %d개 실패", len(files), successCount, failCount)
	log.Printf("⏱️ 총 소요시간: %v, 평균 속도: %.2f 파일/초", elapsed, avgRate)
	
	if avgRate < 1.0 {
		log.Printf("💡 성능 팁: 워커 수를 줄이거나 네트워크 연결을 확인해보세요")
	}
	
	return nil
}

// min 함수 (Go 1.18 이전 버전 호환성)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 함수 (Go 1.18 이전 버전 호환성)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// 폴더 이름 캐시
var folderCache = make(map[string]string)
var folderCacheMutex sync.RWMutex

func (ds *DriveService) getFilePath(fileID string, parents []string) string {
	if len(parents) == 0 {
		return "/"
	}
	
	return ds.buildFullPath(parents[0])
}

// 폴더 정보 구조체 (캐시용)
type FolderInfo struct {
	Name     string
	Parents  []string
}

// 폴더 캐시 (이름과 부모 정보 모두 포함)
var folderInfoCache = make(map[string]*FolderInfo)

func (ds *DriveService) buildFullPath(folderID string) string {
	log.Printf("🔍 경로 구성 시작: 폴더 ID %s", folderID)
	
	if folderID == "root" || folderID == "0AEBGrCpGtL_PUk9PVA" {
		log.Printf("📁 루트 폴더 감지: %s -> /", folderID)
		return "/"
	}
	
	// 캐시에서 폴더 정보 확인
	folderCacheMutex.RLock()
	if folderInfo, exists := folderInfoCache[folderID]; exists {
		folderCacheMutex.RUnlock()
		log.Printf("📋 캐시에서 폴더 정보 발견: %s (부모: %v)", folderInfo.Name, folderInfo.Parents)
		// 부모 폴더 경로 재귀적으로 구성
		if len(folderInfo.Parents) > 0 {
			parentPath := ds.buildFullPath(folderInfo.Parents[0])
			if parentPath == "/" {
				result := "/" + folderInfo.Name
				log.Printf("📁 경로 완성 (루트 하위): %s", result)
				return result
			}
			result := parentPath + "/" + folderInfo.Name
			log.Printf("📁 경로 완성 (중첩): %s", result)
			return result
		}
		result := "/" + folderInfo.Name
		log.Printf("📁 경로 완성 (부모 없음): %s", result)
		return result
	}
	folderCacheMutex.RUnlock()
	
	// 캐시에 없으면 API 호출
	log.Printf("🌐 API 호출로 폴더 정보 조회: %s", folderID)
	folder, err := ds.service.Files.Get(folderID).Fields("id, name, parents").Do()
	if err != nil {
		log.Printf("⚠️ 폴더 정보 조회 실패 (ID: %s): %v", folderID, err)
		return "/"
	}
	
	log.Printf("📋 API 응답: 이름='%s', 부모=%v", folder.Name, folder.Parents)
	
	// 캐시에 저장 (이름과 부모 정보 모두)
	folderCacheMutex.Lock()
	folderCache[folderID] = folder.Name // 기존 캐시 호환성
	folderInfoCache[folderID] = &FolderInfo{
		Name:    folder.Name,
		Parents: folder.Parents,
	}
	folderCacheMutex.Unlock()
	
	// 부모 폴더 경로 재귀적으로 구성
	if len(folder.Parents) > 0 {
		parentPath := ds.buildFullPath(folder.Parents[0])
		if parentPath == "/" {
			result := "/" + folder.Name
			log.Printf("📁 경로 완성 (루트 하위): %s", result)
			return result
		}
		result := parentPath + "/" + folder.Name
		log.Printf("📁 경로 완성 (중첩): %s", result)
		return result
	}
	
	result := "/" + folder.Name
	log.Printf("📁 경로 완성 (부모 없음): %s", result)
	return result
}

// 중복 파일들의 경로를 간단히 설정 (실시간 표시를 위해)
func (ds *DriveService) enrichDuplicatesWithPaths(duplicates [][]*DriveFile) [][]*DriveFile {
	log.Println("📁 중복 파일 경로 설정 중...")
	
	for _, group := range duplicates {
		for _, file := range group {
			// 경로가 비어있으면 "경로 미확인"으로 표시
			if file.Path == "" || file.Path == "/" {
				file.Path = "경로 미확인"
			}
		}
	}
	
	log.Println("✅ 중복 파일 기본 경로 설정 완료")
	return duplicates
}

// 데이터베이스의 모든 파일에 parents 정보를 업데이트하는 함수
func (ds *DriveService) updateAllFileParents() error {
	log.Println("🔄 데이터베이스 파일들의 parents 정보 업데이트 시작...")
	
	// parents 정보가 없는 파일들 조회
	rows, err := ds.db.Query("SELECT id, name FROM files WHERE parents IS NULL OR parents = ''")
	if err != nil {
		return fmt.Errorf("파일 조회 오류: %v", err)
	}
	defer rows.Close()
	
	var filesToUpdate []struct {
		ID   string
		Name string
	}
	
	for rows.Next() {
		var file struct {
			ID   string
			Name string
		}
		if err := rows.Scan(&file.ID, &file.Name); err != nil {
			continue
		}
		filesToUpdate = append(filesToUpdate, file)
	}
	
	log.Printf("📊 업데이트할 파일 수: %d개", len(filesToUpdate))
	
	// 배치 크기 제한
	batchSize := 50
	for i := 0; i < len(filesToUpdate); i += batchSize {
		end := i + batchSize
		if end > len(filesToUpdate) {
			end = len(filesToUpdate)
		}
		
		log.Printf("🔄 배치 %d-%d 처리 중...", i+1, end)
		
		for j := i; j < end; j++ {
			file := filesToUpdate[j]
			
			// API에서 parents 정보 조회
			fileInfo, err := ds.service.Files.Get(file.ID).Fields("id, name, parents").Do()
			if err != nil {
				log.Printf("⚠️ 파일 정보 조회 실패: %s", file.Name)
				continue
			}
			
			// parents 정보 업데이트
			parentsJSON, _ := json.Marshal(fileInfo.Parents)
			_, err = ds.db.Exec("UPDATE files SET parents = ? WHERE id = ?", string(parentsJSON), file.ID)
			if err != nil {
				log.Printf("⚠️ 파일 업데이트 실패: %s", file.Name)
			}
		}
		
		// API 제한을 위한 대기
		time.Sleep(time.Second)
	}
	
	log.Println("✅ parents 정보 업데이트 완료")
	return nil
}

// 특정 폴더의 파일들을 정규표현식으로 필터링하여 검색
func (ds *DriveService) searchFilesInFolder(folderID, regexPattern string) ([]*DriveFile, error) {
	log.Printf("🔍 폴더 %s에서 패턴 '%s'에 맞는 파일 검색 중...", folderID, regexPattern)
	
	// 정규표현식 컴파일
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("잘못된 정규표현식: %v", err)
	}
	
	var allFiles []*DriveFile
	pageToken := ""
	
	for {
		query := fmt.Sprintf("'%s' in parents and trashed=false", folderID)
		
		result, err := ds.service.Files.List().
			Q(query).
			PageSize(1000).
			PageToken(pageToken).
			Fields("nextPageToken, files(id, name, size, webViewLink, mimeType, modifiedTime, parents)").
			Do()
			
		if err != nil {
			return nil, fmt.Errorf("폴더 파일 조회 오류: %v", err)
		}
		
		for _, file := range result.Files {
			// 정규표현식과 매치되는 파일만 추가
			if regex.MatchString(file.Name) {
				driveFile := &DriveFile{
					ID:           file.Id,
					Name:         file.Name,
					Size:         file.Size,
					WebViewLink:  file.WebViewLink,
					MimeType:     file.MimeType,
					ModifiedTime: file.ModifiedTime,
					Parents:      file.Parents,
				}
				allFiles = append(allFiles, driveFile)
			}
		}
		
		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}
	
	log.Printf("✅ 총 %d개 파일이 패턴과 일치함", len(allFiles))
	return allFiles, nil
}

// 여러 파일을 일괄 삭제
func (ds *DriveService) bulkDeleteFiles(fileIDs []string) (int, error) {
	log.Printf("🗑️ %d개 파일 일괄 삭제 시작...", len(fileIDs))
	
	deletedCount := 0
	var parentFoldersToCheck []string
	
	for i, fileID := range fileIDs {
		log.Printf("삭제 중 (%d/%d): %s", i+1, len(fileIDs), fileID)
		
		// 파일 삭제 전에 상위 폴더 정보 저장
		fileInfo, err := ds.service.Files.Get(fileID).Fields("id,name,parents").Do()
		if err != nil {
			log.Printf("⚠️ 파일 정보 조회 실패 (%s): %v", fileID, err)
		} else {
			// 상위 폴더들을 나중에 확인할 목록에 추가
			for _, parentID := range fileInfo.Parents {
				// 중복 방지를 위해 이미 목록에 있는지 확인
				found := false
				for _, existingParent := range parentFoldersToCheck {
					if existingParent == parentID {
						found = true
						break
					}
				}
				if !found {
					parentFoldersToCheck = append(parentFoldersToCheck, parentID)
				}
			}
			log.Printf("🗑️ 파일 삭제: %s (%s)", fileInfo.Name, fileID)
		}
		
		err = ds.service.Files.Delete(fileID).Do()
		if err != nil {
			log.Printf("⚠️ 파일 삭제 실패: %s - %v", fileID, err)
			continue
		}
		
		// 데이터베이스에서도 삭제
		ds.deleteFileFromDB(fileID)
		
		deletedCount++
		
		// API 제한을 위한 짧은 대기
		time.Sleep(100 * time.Millisecond)
	}
	
	log.Printf("✅ 일괄 삭제 완료: %d개 성공, %d개 실패", deletedCount, len(fileIDs)-deletedCount)
	
	// 모든 파일 삭제 후 빈 폴더들 정리 (백그라운드에서 실행)
	if len(parentFoldersToCheck) > 0 {
		log.Printf("📂 빈 폴더 정리 시작: %d개 폴더 확인", len(parentFoldersToCheck))
		go func() {
			for _, parentID := range parentFoldersToCheck {
				err := ds.checkAndDeleteEmptyFolder(parentID)
				if err != nil {
					log.Printf("⚠️ 빈 폴더 정리 실패: %v", err)
				}
			}
			log.Printf("✅ 빈 폴더 정리 완료")
		}()
	}
	
	return deletedCount, nil
}

func (ds *DriveService) bulkDeleteFilesWithCleanup(fileIDs []string, cleanupEmptyFolders bool) (int, error) {
	log.Printf("🗑️ %d개 파일 일괄 삭제 시작 (빈 폴더 정리: %v)...", len(fileIDs), cleanupEmptyFolders)
	
	deletedCount := 0
	var parentFoldersToCheck []string
	
	for i, fileID := range fileIDs {
		log.Printf("삭제 중 (%d/%d): %s", i+1, len(fileIDs), fileID)
		
		// 빈 폴더 정리가 활성화된 경우에만 상위 폴더 정보 저장
		if cleanupEmptyFolders {
			fileInfo, err := ds.service.Files.Get(fileID).Fields("id,name,parents").Do()
			if err != nil {
				log.Printf("⚠️ 파일 정보 조회 실패 (%s): %v", fileID, err)
			} else {
				// 상위 폴더들을 나중에 확인할 목록에 추가
				for _, parentID := range fileInfo.Parents {
					// 중복 방지를 위해 이미 목록에 있는지 확인
					found := false
					for _, existingParent := range parentFoldersToCheck {
						if existingParent == parentID {
							found = true
							break
						}
					}
					if !found {
						parentFoldersToCheck = append(parentFoldersToCheck, parentID)
					}
				}
				log.Printf("🗑️ 파일 삭제: %s (%s)", fileInfo.Name, fileID)
			}
		}
		
		err := ds.service.Files.Delete(fileID).Do()
		if err != nil {
			log.Printf("⚠️ 파일 삭제 실패: %s - %v", fileID, err)
			continue
		}
		
		// 데이터베이스에서도 삭제
		ds.deleteFileFromDB(fileID)
		
		deletedCount++
		
		// API 제한을 위한 짧은 대기
		time.Sleep(100 * time.Millisecond)
	}
	
	log.Printf("✅ 일괄 삭제 완료: %d개 성공, %d개 실패", deletedCount, len(fileIDs)-deletedCount)
	
	// 빈 폴더 정리가 활성화된 경우에만 실행
	if cleanupEmptyFolders && len(parentFoldersToCheck) > 0 {
		log.Printf("📂 빈 폴더 정리 시작: %d개 폴더 확인", len(parentFoldersToCheck))
		go func() {
			for _, parentID := range parentFoldersToCheck {
				err := ds.checkAndDeleteEmptyFolder(parentID)
				if err != nil {
					log.Printf("⚠️ 빈 폴더 정리 실패: %v", err)
				}
			}
			log.Printf("✅ 빈 폴더 정리 완료")
		}()
	} else if !cleanupEmptyFolders {
		log.Printf("📂 빈 폴더 정리 건너뜀 (사용자 옵션)")
	}
	
	return deletedCount, nil
}

func FindDuplicates(files []*DriveFile, ds *DriveService) ([][]*DriveFile, error) {
	// 기존 진행 상태 확인
	progress, err := ds.loadProgress()
	if err != nil {
		log.Printf("⚠️ 진행 상태 로드 실패: %v", err)
		progress = &ProgressData{Status: "idle"}
	}

	// 이미 완료된 중복 그룹이 있으면 반환
	if progress.Status == "completed" {
		log.Println("✅ 이전에 완료된 중복 검사 결과를 로드합니다.")
		return ds.loadDuplicateGroups()
	}

	log.Println("🔢 파일을 크기별로 그룹화하는 중...")
	sizeGroups := make(map[int64][]*DriveFile)
	
	for _, file := range files {
		sizeGroups[file.Size] = append(sizeGroups[file.Size], file)
	}

	// 크기가 같은 파일들만 필터링
	var sizeGroupsSlice [][]*DriveFile
	for _, group := range sizeGroups {
		if len(group) >= 2 {
			sizeGroupsSlice = append(sizeGroupsSlice, group)
		}
	}

	potentialDuplicates := 0
	for _, group := range sizeGroupsSlice {
		potentialDuplicates += len(group)
	}
	log.Printf("📊 잠재적 중복 후보: %d개 파일 (크기가 같은 파일들)", potentialDuplicates)

	// 진행 상태 초기화
	progress.TotalFiles = potentialDuplicates
	progress.Status = "running"
	progress.ProcessedFiles = 0
	progress.CompletedGroups = 0
	progress.CurrentGroup = 0
	ds.saveProgress(*progress)

	var duplicateGroups [][]*DriveFile
	processedFiles := 0
	
	for groupIndex, sameFiles := range sizeGroupsSlice {
		progress.CurrentGroup = groupIndex
		ds.saveProgress(*progress)

		log.Printf("🔍 그룹 %d/%d: 크기 %d bytes인 파일 %d개의 해시 계산 중...", 
			groupIndex+1, len(sizeGroupsSlice), sameFiles[0].Size, len(sameFiles))
		
		hashGroups := make(map[string][]*DriveFile)
		
		// 병렬 처리를 위한 워커 풀 (동적 설정)
		maxWorkers := ds.getMaxWorkers()
		log.Printf("⚙️ 병렬 작업 개수: %d개", maxWorkers)
		semaphore := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup
		var mu sync.Mutex
		
		for i, file := range sameFiles {
			// 이미 해시가 계산된 파일은 건너뛰기
			if file.Hash != "" {
				mu.Lock()
				hashGroups[file.Hash] = append(hashGroups[file.Hash], file)
				mu.Unlock()
				processedFiles++
				continue
			}

			wg.Add(1)
			go func(file *DriveFile, index int) {
				defer wg.Done()
				
				// 세마포어로 동시 다운로드 제한
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				
				log.Printf("⬇️  파일 다운로드 중 (%d/%d): %s", index+1, len(sameFiles), file.Name)
				
				content, err := ds.DownloadFileContent(file.ID)
				if err != nil {
					log.Printf("❌ 파일 %s 다운로드 실패: %v", file.Name, err)
					return
				}
				
				file.Hash = calculateHash(content)
				
				// DB에 해시 저장
				if err := ds.updateFileHash(file.ID, file.Hash); err != nil {
					log.Printf("⚠️ 해시 저장 실패: %v", err)
				}
				
				mu.Lock()
				hashGroups[file.Hash] = append(hashGroups[file.Hash], file)
				processedFiles++
				progress.ProcessedFiles = processedFiles
				mu.Unlock()
				
				log.Printf("✅ 해시 계산 완료 (%d/%d): %s", processedFiles, potentialDuplicates, file.Name)
				
				// 10개마다 진행 상태 저장
				if processedFiles%10 == 0 {
					ds.saveProgress(*progress)
				}
			}(file, i)
		}
		
		wg.Wait()
		
		// 해시별 그룹에서 중복 찾기 및 즉시 저장
		for hash, hashFiles := range hashGroups {
			if len(hashFiles) >= 2 {
				log.Printf("🎯 중복 발견! 해시 %s... : %d개 파일", hash[:8], len(hashFiles))
				duplicateGroups = append(duplicateGroups, hashFiles)
				
				// 즉시 중복 그룹을 DB에 저장
				if err := ds.saveSingleDuplicateGroup(hashFiles); err != nil {
					log.Printf("⚠️ 중복 그룹 저장 실패: %v", err)
				}
			}
		}
		
		progress.CompletedGroups = groupIndex + 1
		ds.saveProgress(*progress)
	}

	// 완료 상태로 업데이트
	progress.Status = "completed"
	progress.ProcessedFiles = processedFiles
	ds.saveProgress(*progress)

	log.Printf("🏁 중복 검사 완료: %d개 파일 처리됨", processedFiles)
	log.Printf("📊 총 %d개 중복 그룹 발견", len(duplicateGroups))
	return duplicateGroups, nil
}

// 저장된 폴더 비교 작업 조회
func (ds *DriveService) getSavedComparisonTasks() ([]*FolderComparisonTask, error) {
	rows, err := ds.db.Query(`
		SELECT id, source_folder_id, target_folder_id, status, current_step,
		       source_folder_scanned, source_folder_total, target_folder_scanned, target_folder_total,
		       hashes_calculated, total_hashes_to_calc, duplicates_found, error_message,
		       created_at, updated_at
		FROM folder_comparison_tasks
		WHERE status IN ('pending', 'running', 'paused', 'completed')
		ORDER BY updated_at DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("저장된 작업 조회 실패: %v", err)
	}
	defer rows.Close()

	var tasks []*FolderComparisonTask
	for rows.Next() {
		task := &FolderComparisonTask{}
		err := rows.Scan(
			&task.ID, &task.SourceFolderID, &task.TargetFolderID, &task.Status, &task.CurrentStep,
			&task.SourceFolderScanned, &task.SourceFolderTotal, &task.TargetFolderScanned, &task.TargetFolderTotal,
			&task.HashesCalculated, &task.TotalHashesToCalc, &task.DuplicatesFound, &task.ErrorMessage,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			log.Printf("⚠️ 작업 정보 스캔 실패: %v", err)
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// 저장된 폴더 비교 작업 모두 삭제
func (ds *DriveService) clearSavedComparisonTasks() error {
	tx, err := ds.db.Begin()
	if err != nil {
		return fmt.Errorf("트랜잭션 시작 실패: %v", err)
	}
	defer tx.Rollback()

	// 관련 테이블들의 데이터 삭제
	_, err = tx.Exec("DELETE FROM comparison_result_files")
	if err != nil {
		return fmt.Errorf("비교 결과 파일 삭제 실패: %v", err)
	}

	_, err = tx.Exec("DELETE FROM folder_comparison_tasks")
	if err != nil {
		return fmt.Errorf("비교 작업 삭제 실패: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("트랜잭션 커밋 실패: %v", err)
	}

	log.Printf("✅ 모든 저장된 폴더 비교 작업이 삭제됨")
	return nil
}