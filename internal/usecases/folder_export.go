package usecases

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"time"
)

// FolderExportUseCase handles exporting folder contents to a spreadsheet
type FolderExportUseCase struct {
	storageProvider services.StorageProvider
}

// NewFolderExportUseCase creates a new folder export use case
func NewFolderExportUseCase(storageProvider services.StorageProvider) *FolderExportUseCase {
	return &FolderExportUseCase{
		storageProvider: storageProvider,
	}
}

// FolderExportRequest represents a request to export folder contents
type FolderExportRequest struct {
	FolderID          string `json:"folderId"`
	IncludeSubfolders bool   `json:"includeSubfolders"`
}

// FolderExportResult represents the result of a folder export
type FolderExportResult struct {
	SpreadsheetID  string `json:"spreadsheetId"`
	SpreadsheetURL string `json:"spreadsheetUrl"`
	TotalFiles     int    `json:"totalFiles"`
	TotalFolders   int    `json:"totalFolders"`
	FolderName     string `json:"folderName"`
}

// ExportProgressCallback is called to report progress during export
type ExportProgressCallback func(event ExportProgressEvent)

// ExportProgressEvent represents a progress event during export
type ExportProgressEvent struct {
	Type           string `json:"type"`           // scanning, generating, uploading, complete, error
	Message        string `json:"message"`        // 진행 상황 메시지
	ScannedFiles   int    `json:"scannedFiles"`   // 스캔된 파일 수
	ScannedFolders int    `json:"scannedFolders"` // 스캔된 폴더 수
	TotalFiles     int    `json:"totalFiles"`     // 총 파일 수
	TotalFolders   int    `json:"totalFolders"`   // 총 폴더 수
}

// fileEntry represents a file/folder entry collected during scanning
type fileEntry struct {
	entryType    string // "폴더" or "파일"
	name         string
	path         string
	size         int64
	sizeReadable string
	mimeType     string
	modifiedTime string
	webViewLink  string
	fileID       string
}

// ExportFolderToSpreadsheet exports folder contents to a Google Spreadsheet
func (uc *FolderExportUseCase) ExportFolderToSpreadsheet(
	ctx context.Context,
	req FolderExportRequest,
	progressCb ExportProgressCallback,
) (*FolderExportResult, error) {
	log.Printf("📊 폴더 스프레드시트 내보내기 시작: %s", req.FolderID)

	// 1. 폴더 정보 조회
	progressCb(ExportProgressEvent{
		Type:    "scanning",
		Message: "폴더 정보 조회 중...",
	})

	folderInfo, err := uc.storageProvider.GetFile(ctx, req.FolderID)
	if err != nil {
		return nil, fmt.Errorf("폴더 정보 조회 실패: %w", err)
	}

	log.Printf("📁 폴더명: %s (ID: %s)", folderInfo.Name, folderInfo.ID)

	// 2. 상위 폴더 ID 조회 (스프레드시트를 상위 폴더에 저장)
	parentFolderID := req.FolderID // 기본값: 현재 폴더에 저장
	parents, err := uc.storageProvider.GetFileParents(ctx, req.FolderID)
	if err == nil && len(parents) > 0 {
		parentFolderID = parents[0]
		log.Printf("📁 상위 폴더 ID: %s", parentFolderID)
	}

	// 3. 재귀적 파일/폴더 목록 수집
	progressCb(ExportProgressEvent{
		Type:    "scanning",
		Message: "파일 및 폴더 목록 수집 중...",
	})

	entries, err := uc.collectEntries(ctx, req.FolderID, folderInfo.Name, req.IncludeSubfolders, progressCb)
	if err != nil {
		return nil, fmt.Errorf("파일 목록 수집 실패: %w", err)
	}

	totalFiles := 0
	totalFolders := 0
	for _, entry := range entries {
		if entry.entryType == "파일" {
			totalFiles++
		} else {
			totalFolders++
		}
	}

	log.Printf("📊 수집 완료: 파일 %d개, 폴더 %d개", totalFiles, totalFolders)

	// 4. CSV 생성
	progressCb(ExportProgressEvent{
		Type:         "generating",
		Message:      "CSV 파일 생성 중...",
		TotalFiles:   totalFiles,
		TotalFolders: totalFolders,
	})

	csvData, err := uc.generateCSV(entries)
	if err != nil {
		return nil, fmt.Errorf("CSV 생성 실패: %w", err)
	}

	log.Printf("📄 CSV 생성 완료: %d 바이트", csvData.Len())

	// 5. Google Drive 업로드 (CSV → Spreadsheet 변환)
	progressCb(ExportProgressEvent{
		Type:         "uploading",
		Message:      "Google 스프레드시트로 업로드 중...",
		TotalFiles:   totalFiles,
		TotalFolders: totalFolders,
	})

	spreadsheetName := fmt.Sprintf("%s_파일목록_%s", folderInfo.Name, time.Now().Format("20060102_150405"))
	spreadsheetID, webViewLink, err := uc.storageProvider.UploadCSVAsSpreadsheet(
		ctx,
		parentFolderID,
		spreadsheetName,
		csvData,
	)
	if err != nil {
		return nil, fmt.Errorf("스프레드시트 업로드 실패: %w", err)
	}

	log.Printf("✅ 스프레드시트 생성 완료: %s (ID: %s)", spreadsheetName, spreadsheetID)

	result := &FolderExportResult{
		SpreadsheetID:  spreadsheetID,
		SpreadsheetURL: webViewLink,
		TotalFiles:     totalFiles,
		TotalFolders:   totalFolders,
		FolderName:     folderInfo.Name,
	}

	progressCb(ExportProgressEvent{
		Type:         "complete",
		Message:      "내보내기 완료!",
		TotalFiles:   totalFiles,
		TotalFolders: totalFolders,
	})

	return result, nil
}

// collectEntries recursively collects file/folder entries
func (uc *FolderExportUseCase) collectEntries(
	ctx context.Context,
	folderID, basePath string,
	includeSubfolders bool,
	progressCb ExportProgressCallback,
) ([]fileEntry, error) {
	var entries []fileEntry
	return uc.collectEntriesRecursive(ctx, folderID, basePath, includeSubfolders, progressCb, entries, 0, 0)
}

func (uc *FolderExportUseCase) collectEntriesRecursive(
	ctx context.Context,
	folderID, currentPath string,
	includeSubfolders bool,
	progressCb ExportProgressCallback,
	entries []fileEntry,
	fileCount, folderCount int,
) ([]fileEntry, error) {
	select {
	case <-ctx.Done():
		return entries, ctx.Err()
	default:
	}

	// 현재 폴더의 파일 목록 조회
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 내용 조회 실패 (%s): %v - 계속 진행", folderID, err)
		return entries, nil
	}

	for _, file := range files {
		filePath := currentPath + "/" + file.Name

		if file.MimeType == "application/vnd.google-apps.folder" {
			// 폴더 엔트리 추가
			folderCount++
			entries = append(entries, fileEntry{
				entryType:    "폴더",
				name:         file.Name,
				path:         filePath,
				size:         0,
				sizeReadable: "-",
				mimeType:     file.MimeType,
				modifiedTime: formatTimeForCSV(file.ModifiedTime),
				webViewLink:  file.WebViewLink,
				fileID:       file.ID,
			})

			// 진행 상황 보고
			progressCb(ExportProgressEvent{
				Type:           "scanning",
				Message:        fmt.Sprintf("스캔 중: %s", filePath),
				ScannedFiles:   fileCount,
				ScannedFolders: folderCount,
			})

			// 하위 폴더 재귀 탐색
			if includeSubfolders {
				var newErr error
				entries, newErr = uc.collectEntriesRecursive(ctx, file.ID, filePath, true, progressCb, entries, fileCount, folderCount)
				if newErr != nil {
					return entries, newErr
				}
				// 재귀 후 카운트 업데이트
				fileCount = 0
				folderCount = 0
				for _, e := range entries {
					if e.entryType == "파일" {
						fileCount++
					} else {
						folderCount++
					}
				}
			}
		} else {
			// 파일 엔트리 추가
			fileCount++
			entries = append(entries, fileEntry{
				entryType:    "파일",
				name:         file.Name,
				path:         filePath,
				size:         file.Size,
				sizeReadable: formatFileSizeReadable(file.Size),
				mimeType:     file.MimeType,
				modifiedTime: formatTimeForCSV(file.ModifiedTime),
				webViewLink:  file.WebViewLink,
				fileID:       file.ID,
			})
		}
	}

	return entries, nil
}

// generateCSV creates a CSV file from entries
func (uc *FolderExportUseCase) generateCSV(entries []fileEntry) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	// UTF-8 BOM 추가 (Excel 호환성)
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(buf)

	// 헤더 작성
	header := []string{"유형", "이름", "경로", "크기", "크기(읽기)", "MIME 타입", "수정일", "Google Drive 링크", "파일 ID"}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("CSV 헤더 작성 실패: %w", err)
	}

	// 데이터 작성
	for _, entry := range entries {
		row := []string{
			entry.entryType,
			entry.name,
			entry.path,
			fmt.Sprintf("%d", entry.size),
			entry.sizeReadable,
			entry.mimeType,
			entry.modifiedTime,
			entry.webViewLink,
			entry.fileID,
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("CSV 행 작성 실패: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV 플러시 실패: %w", err)
	}

	return buf, nil
}

// formatFileSizeReadable converts bytes to human-readable size string
func formatFileSizeReadable(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(bytes)
	unitIndex := 0

	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", size, units[unitIndex])
}

// formatTimeForCSV formats time for CSV output
func formatTimeForCSV(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
