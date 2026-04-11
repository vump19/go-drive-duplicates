package controllers

import (
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
)

// FolderExportController handles folder export HTTP requests
type FolderExportController struct {
	folderExportUseCase *usecases.FolderExportUseCase
}

// NewFolderExportController creates a new folder export controller
func NewFolderExportController(folderExportUseCase *usecases.FolderExportUseCase) *FolderExportController {
	return &FolderExportController{
		folderExportUseCase: folderExportUseCase,
	}
}

// ExportFolderToSpreadsheet handles the request to export folder contents to a Google Spreadsheet
// POST /api/export/folder-to-spreadsheet - SSE 스트림으로 진행 상황 전달
func (c *FolderExportController) ExportFolderToSpreadsheet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req usecases.FolderExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if req.FolderID == "" {
		http.Error(w, "폴더 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("📊 폴더 스프레드시트 내보내기 요청: %s", req.FolderID)

	// SSE 헤더 설정
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "스트리밍을 지원하지 않습니다", http.StatusInternalServerError)
		return
	}

	// 초기 heartbeat
	fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()

	// 진행 상황 콜백
	progressCb := func(event usecases.ExportProgressEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			log.Printf("⚠️ 진행 상황 JSON 마샬링 실패: %v", err)
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
		flusher.Flush()
	}

	// 내보내기 실행
	result, err := c.folderExportUseCase.ExportFolderToSpreadsheet(r.Context(), req, progressCb)
	if err != nil {
		log.Printf("❌ 폴더 스프레드시트 내보내기 실패: %v", err)
		errorData, _ := json.Marshal(map[string]string{
			"type":    "error",
			"message": err.Error(),
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(errorData))
		flusher.Flush()
		return
	}

	// 완료 이벤트 전송
	resultData, _ := json.Marshal(result)
	fmt.Fprintf(w, "event: complete\ndata: %s\n\n", string(resultData))
	flusher.Flush()

	log.Printf("✅ 폴더 스프레드시트 내보내기 완료: %s → %s", req.FolderID, result.SpreadsheetURL)
}
