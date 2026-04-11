package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
	infraServices "go-drive-duplicates/internal/infrastructure/services"
	"go-drive-duplicates/internal/usecases"
)

// SmartOrganizerController - 스마트 파일 정리 컨트롤러
type SmartOrganizerController struct {
	useCase *usecases.SmartOrganizerUseCase
}

// NewSmartOrganizerController creates a new SmartOrganizerController
func NewSmartOrganizerController(uc *usecases.SmartOrganizerUseCase) *SmartOrganizerController {
	return &SmartOrganizerController{useCase: uc}
}

// ============================================================
// 채팅 엔드포인트
// ============================================================

// aiProviderRequest - 프론트엔드에서 전달하는 AI 프로바이더 설정
type aiProviderRequest struct {
	Name   string `json:"name"`   // claude, openai, gemini
	APIKey string `json:"apiKey"`
	Model  string `json:"model"`
}

// buildAIServiceFromProviders creates an AI service from frontend-provided API keys
func buildAIServiceFromProviders(providers []aiProviderRequest) services.AIService {
	if len(providers) == 0 {
		return nil
	}

	named := []infraServices.NamedProvider{}
	for _, p := range providers {
		if p.APIKey == "" {
			continue
		}
		switch p.Name {
		case "claude":
			named = append(named, infraServices.NamedProvider{
				Name:    "claude",
				Service: infraServices.NewClaudeAPIAdapter(p.APIKey, p.Model),
			})
		case "openai":
			named = append(named, infraServices.NamedProvider{
				Name:    "openai",
				Service: infraServices.NewOpenAIAPIAdapter(p.APIKey, p.Model),
			})
		case "gemini":
			named = append(named, infraServices.NamedProvider{
				Name:    "gemini",
				Service: infraServices.NewGeminiAPIAdapter(p.APIKey, p.Model),
			})
		}
	}

	if len(named) == 0 {
		return nil
	}
	return infraServices.NewMultiProviderAIService(named)
}

// Chat handles POST /api/organizer/chat
func (c *SmartOrganizerController) Chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID     string               `json:"sessionId"`
		Message       string               `json:"message"`
		FolderContext string               `json:"folderContext"`
		AIProviders   []aiProviderRequest  `json:"aiProviders,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Message == "" {
		http.Error(w, "세션 ID와 메시지가 필요합니다", http.StatusBadRequest)
		return
	}

	// Build override AI service from frontend-provided keys (if any)
	var overrideAI services.AIService
	if len(req.AIProviders) > 0 {
		overrideAI = buildAIServiceFromProviders(req.AIProviders)
	}

	response, err := c.useCase.ChatWithAIService(r.Context(), req.SessionID, req.Message, req.FolderContext, overrideAI)
	if err != nil {
		log.Printf("❌ AI 채팅 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetChatHistory handles GET /api/organizer/chat/history?sessionId=xxx
func (c *SmartOrganizerController) GetChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "세션 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	history, err := c.useCase.GetChatHistory(r.Context(), sessionID)
	if err != nil {
		log.Printf("❌ 채팅 기록 조회 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// DeleteChatSession handles DELETE /api/organizer/chat/session?sessionId=xxx
func (c *SmartOrganizerController) DeleteChatSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "세션 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.DeleteChatSession(r.Context(), sessionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "세션이 삭제되었습니다"})
}

// ============================================================
// 규칙 세트 엔드포인트
// ============================================================

// CreateRuleSet handles POST /api/organizer/rulesets
func (c *SmartOrganizerController) CreateRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ruleSet entities.OrganizationRuleSet
	if err := json.NewDecoder(r.Body).Decode(&ruleSet); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if ruleSet.Name == "" {
		http.Error(w, "규칙 세트 이름이 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.CreateRuleSet(r.Context(), &ruleSet); err != nil {
		log.Printf("❌ 규칙 세트 생성 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ruleSet)
}

// ListRuleSets handles GET /api/organizer/rulesets
func (c *SmartOrganizerController) ListRuleSets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ruleSets, err := c.useCase.ListRuleSets(r.Context())
	if err != nil {
		log.Printf("❌ 규칙 세트 목록 조회 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ruleSets)
}

// GetRuleSet handles GET /api/organizer/rulesets/detail?id=N
func (c *SmartOrganizerController) GetRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "유효한 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	ruleSet, err := c.useCase.GetRuleSet(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ruleSet == nil {
		http.Error(w, "규칙 세트를 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ruleSet)
}

// UpdateRuleSet handles PUT /api/organizer/rulesets
func (c *SmartOrganizerController) UpdateRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ruleSet entities.OrganizationRuleSet
	if err := json.NewDecoder(r.Body).Decode(&ruleSet); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.UpdateRuleSet(r.Context(), &ruleSet); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ruleSet)
}

// DeleteRuleSet handles DELETE /api/organizer/rulesets?id=N
func (c *SmartOrganizerController) DeleteRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "유효한 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.DeleteRuleSet(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "규칙 세트가 삭제되었습니다"})
}

// BackupRuleSet handles POST /api/organizer/rulesets/backup
func (c *SmartOrganizerController) BackupRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID int `json:"ruleSetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	fileID, err := c.useCase.BackupRuleSetToGDrive(r.Context(), req.RuleSetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"fileId": fileID, "message": "백업이 완료되었습니다"})
}

// RestoreRuleSet handles POST /api/organizer/rulesets/restore
func (c *SmartOrganizerController) RestoreRuleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FileID string `json:"fileId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	ruleSet, err := c.useCase.RestoreRuleSetFromGDrive(r.Context(), req.FileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ruleSet)
}

// ============================================================
// 규칙 엔드포인트
// ============================================================

// CreateRule handles POST /api/organizer/rules
func (c *SmartOrganizerController) CreateRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rule entities.OrganizationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if rule.Name == "" || rule.TargetPath == "" {
		http.Error(w, "규칙 이름과 대상 경로가 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.CreateRule(r.Context(), &rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

// UpdateRule handles PUT /api/organizer/rules
func (c *SmartOrganizerController) UpdateRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rule entities.OrganizationRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.UpdateRule(r.Context(), &rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// DeleteRule handles DELETE /api/organizer/rules?id=N
func (c *SmartOrganizerController) DeleteRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "유효한 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.DeleteRule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "규칙이 삭제되었습니다"})
}

// ApplySuggestedRules handles POST /api/organizer/rules/apply-suggestions
func (c *SmartOrganizerController) ApplySuggestedRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID   int                       `json:"ruleSetId"`
		Suggestions []entities.RuleSuggestion `json:"suggestions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.ApplySuggestedRules(r.Context(), req.RuleSetID, req.Suggestions); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": fmt.Sprintf("%d개의 규칙이 추가되었습니다", len(req.Suggestions))})
}

// ============================================================
// 파일 정리 엔드포인트
// ============================================================

// PreviewOrganize handles POST /api/organizer/organize/preview
func (c *SmartOrganizerController) PreviewOrganize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID int    `json:"ruleSetId"`
		FolderID  string `json:"folderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	result, err := c.useCase.PreviewOrganize(r.Context(), req.RuleSetID, req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ExecuteOrganize handles POST /api/organizer/organize/execute
func (c *SmartOrganizerController) ExecuteOrganize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID int    `json:"ruleSetId"`
		FolderID  string `json:"folderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	job, err := c.useCase.ExecuteOrganize(r.Context(), req.RuleSetID, req.FolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// GetOrganizeProgress handles GET /api/organizer/organize/progress?jobId=xxx
func (c *SmartOrganizerController) GetOrganizeProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "작업 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	job := c.useCase.GetOrganizeProgress(jobID)
	if job == nil {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// CancelOrganize handles POST /api/organizer/organize/cancel?jobId=xxx
func (c *SmartOrganizerController) CancelOrganize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "작업 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.CancelOrganize(jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "작업이 취소되었습니다"})
}

// ============================================================
// 감시 폴더 엔드포인트
// ============================================================

// StartWatching handles POST /api/organizer/watch/start
func (c *SmartOrganizerController) StartWatching(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID int `json:"ruleSetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if err := c.useCase.StartWatching(r.Context(), req.RuleSetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "폴더 감시가 시작되었습니다"})
}

// StopWatching handles POST /api/organizer/watch/stop
func (c *SmartOrganizerController) StopWatching(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID int `json:"ruleSetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	c.useCase.StopWatching(req.RuleSetID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "폴더 감시가 중지되었습니다"})
}

// GetWatchStatus handles GET /api/organizer/watch/status
func (c *SmartOrganizerController) GetWatchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := c.useCase.GetWatchStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// ============================================================
// 로그 엔드포인트
// ============================================================

// GetLogs handles GET /api/organizer/logs?ruleSetId=N&limit=50&offset=0
func (c *SmartOrganizerController) GetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ruleSetID, err := strconv.Atoi(r.URL.Query().Get("ruleSetId"))
	if err != nil {
		http.Error(w, "유효한 규칙 세트 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	logs, total, err := c.useCase.GetLogs(r.Context(), ruleSetID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ExportLogs handles POST /api/organizer/logs/export
func (c *SmartOrganizerController) ExportLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RuleSetID      int    `json:"ruleSetId"`
		ParentFolderID string `json:"parentFolderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	fileID, webLink, err := c.useCase.ExportLogsToSpreadsheet(r.Context(), req.RuleSetID, req.ParentFolderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"fileId":  fileID,
		"webLink": webLink,
		"message": "스프레드시트로 내보내기가 완료되었습니다",
	})
}

// ============================================================
// 폴더 탐색 엔드포인트
// ============================================================

// ListFolderFiles handles GET /api/organizer/folders?folderId=xxx
func (c *SmartOrganizerController) ListFolderFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		folderID = "root"
	}

	files, err := c.useCase.ListFolderFiles(r.Context(), folderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}
