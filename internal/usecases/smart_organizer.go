package usecases

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
)

// SmartOrganizerUseCase - 스마트 파일 정리 유스케이스
type SmartOrganizerUseCase struct {
	storageProvider services.StorageProvider
	aiService       services.AIService
	ruleSetRepo     repositories.OrganizationRuleSetRepository
	ruleRepo        repositories.OrganizationRuleRepository
	logRepo         repositories.OrganizationLogRepository
	chatRepo        repositories.ChatMessageRepository

	// 감시 폴더 폴링
	watchers   map[int]*FolderWatcher
	watchersMu sync.RWMutex

	// 정리 작업 관리
	activeJobs   map[string]*OrganizeJob
	activeJobsMu sync.RWMutex

	pollInterval   time.Duration
	maxFilesPerRun int
}

// FolderWatcher - 폴더 감시 정보
type FolderWatcher struct {
	RuleSetID  int
	Ticker     *time.Ticker
	Cancel     context.CancelFunc
	LastCheck  time.Time
	KnownFiles map[string]bool
}

// OrganizeJob - 정리 작업 상태
type OrganizeJob struct {
	ID             string    `json:"jobId"`
	RuleSetID      int       `json:"ruleSetId"`
	FolderID       string    `json:"folderId"`
	Status         string    `json:"status"` // running, completed, failed, cancelled
	TotalFiles     int       `json:"totalFiles"`
	ProcessedFiles int32     `json:"processedFiles"`
	MovedFiles     int32     `json:"movedFiles"`
	SkippedFiles   int32     `json:"skippedFiles"`
	FailedFiles    int32     `json:"failedFiles"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime,omitempty"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	cancelFunc     context.CancelFunc
}

// OrganizePreviewItem - 미리보기 항목
type OrganizePreviewItem struct {
	FileID     string `json:"fileId"`
	FileName   string `json:"fileName"`
	FileSize   int64  `json:"fileSize"`
	MimeType   string `json:"mimeType"`
	SourcePath string `json:"sourcePath"`
	TargetPath string `json:"targetPath"`
	RuleName   string `json:"ruleName"`
	Action     string `json:"action"`
}

// OrganizePreviewResult - 미리보기 결과
type OrganizePreviewResult struct {
	TotalFiles    int                   `json:"totalFiles"`
	MatchedFiles  int                   `json:"matchedFiles"`
	UnmatchedFiles int                  `json:"unmatchedFiles"`
	Items         []OrganizePreviewItem `json:"items"`
}

// NewSmartOrganizerUseCase creates a new SmartOrganizerUseCase
func NewSmartOrganizerUseCase(
	storageProvider services.StorageProvider,
	aiService services.AIService,
	ruleSetRepo repositories.OrganizationRuleSetRepository,
	ruleRepo repositories.OrganizationRuleRepository,
	logRepo repositories.OrganizationLogRepository,
	chatRepo repositories.ChatMessageRepository,
	pollInterval time.Duration,
	maxFilesPerRun int,
) *SmartOrganizerUseCase {
	if pollInterval == 0 {
		pollInterval = 5 * time.Minute
	}
	if maxFilesPerRun == 0 {
		maxFilesPerRun = 100
	}
	return &SmartOrganizerUseCase{
		storageProvider: storageProvider,
		aiService:       aiService,
		ruleSetRepo:     ruleSetRepo,
		ruleRepo:        ruleRepo,
		logRepo:         logRepo,
		chatRepo:        chatRepo,
		watchers:        make(map[int]*FolderWatcher),
		activeJobs:      make(map[string]*OrganizeJob),
		pollInterval:    pollInterval,
		maxFilesPerRun:  maxFilesPerRun,
	}
}

// ============================================================
// 채팅 & 규칙 생성
// ============================================================

const systemPromptTemplate = `당신은 Google Drive 파일 정리 전문가입니다. 사용자와 대화하면서 파일 정리 규칙을 만들어줍니다.

사용 가능한 규칙 조건:
- name: 파일 이름 (contains, equals, startsWith, endsWith, matches)
- extension: 확장자 (equals: .jpg, .pdf, .docx 등)
- mimeType: MIME 타입 (equals, contains)
- size: 파일 크기 바이트 (greaterThan, lessThan)
- modifiedDate: 수정일 (before, after)

사용 가능한 작업:
- move: 파일 이동
- copy: 파일 복사

응답에 규칙을 제안할 때는 반드시 다음 JSON 형식으로 제안하세요:
` + "```json" + `
{"ruleSuggestions": [{"name": "규칙이름", "description": "설명", "conditions": [{"field": "extension", "operator": "equals", "value": ".jpg"}], "action": "move", "targetPath": "대상폴더ID"}]}
` + "```" + `

현재 사용자의 폴더 구조:
%s

규칙을 제안할 때:
1. 한국어로 친절하게 설명하세요
2. 구체적인 조건과 대상 경로를 포함하세요
3. 사용자가 대상 폴더 ID를 아직 지정하지 않았다면 적절한 폴더 이름을 제안하세요
4. 여러 규칙이 필요한 경우 priority를 고려하여 정렬하세요`

// ChatWithAI sends a message to AI and returns the response
func (uc *SmartOrganizerUseCase) ChatWithAI(ctx context.Context, sessionID, userMessage, folderContext string) (*services.AIResponse, error) {
	return uc.ChatWithAIService(ctx, sessionID, userMessage, folderContext, nil)
}

// ChatWithAIService sends a message to AI using the provided service (or default)
func (uc *SmartOrganizerUseCase) ChatWithAIService(ctx context.Context, sessionID, userMessage, folderContext string, overrideAI services.AIService) (*services.AIResponse, error) {
	aiSvc := overrideAI
	if aiSvc == nil {
		aiSvc = uc.aiService
	}
	if aiSvc == nil {
		return nil, fmt.Errorf("AI 서비스가 설정되지 않았습니다. API 키를 설정하세요")
	}

	// Save user message
	userMsg := &entities.ChatMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   userMessage,
	}
	if err := uc.chatRepo.Create(ctx, userMsg); err != nil {
		log.Printf("⚠️ 사용자 메시지 저장 실패: %v", err)
	}

	// Load chat history
	history, err := uc.chatRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		log.Printf("⚠️ 채팅 기록 로드 실패: %v", err)
	}

	// Convert to AI messages
	aiMessages := make([]services.AIMessage, 0, len(history))
	for _, msg := range history {
		aiMessages = append(aiMessages, services.AIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Build system prompt
	systemPrompt := fmt.Sprintf(systemPromptTemplate, folderContext)

	// Call AI
	response, err := aiSvc.Chat(ctx, aiMessages, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI 응답 실패: %w", err)
	}

	// Save assistant message
	metadata := ""
	if len(response.RuleSuggestions) > 0 {
		if data, err := json.Marshal(response.RuleSuggestions); err == nil {
			metadata = string(data)
		}
	}

	assistantMsg := &entities.ChatMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   response.Content,
		Metadata:  metadata,
	}
	if err := uc.chatRepo.Create(ctx, assistantMsg); err != nil {
		log.Printf("⚠️ AI 응답 저장 실패: %v", err)
	}

	return response, nil
}

// GetChatHistory returns chat history for a session
func (uc *SmartOrganizerUseCase) GetChatHistory(ctx context.Context, sessionID string) ([]*entities.ChatMessage, error) {
	return uc.chatRepo.GetBySessionID(ctx, sessionID)
}

// DeleteChatSession deletes a chat session
func (uc *SmartOrganizerUseCase) DeleteChatSession(ctx context.Context, sessionID string) error {
	return uc.chatRepo.DeleteBySessionID(ctx, sessionID)
}

// ApplySuggestedRules adds AI-suggested rules to a rule set
func (uc *SmartOrganizerUseCase) ApplySuggestedRules(ctx context.Context, ruleSetID int, suggestions []entities.RuleSuggestion) error {
	// Verify rule set exists
	ruleSet, err := uc.ruleSetRepo.GetByID(ctx, ruleSetID)
	if err != nil {
		return fmt.Errorf("규칙 세트 조회 실패: %w", err)
	}
	if ruleSet == nil {
		return fmt.Errorf("규칙 세트를 찾을 수 없습니다: ID %d", ruleSetID)
	}

	// Get existing rules to determine next priority
	existing, err := uc.ruleRepo.GetByRuleSetID(ctx, ruleSetID)
	if err != nil {
		return fmt.Errorf("기존 규칙 조회 실패: %w", err)
	}
	nextPriority := len(existing)

	for i, suggestion := range suggestions {
		conditionsJSON, err := json.Marshal(suggestion.Conditions)
		if err != nil {
			return fmt.Errorf("조건 직렬화 실패: %w", err)
		}

		rule := &entities.OrganizationRule{
			RuleSetID:   ruleSetID,
			Priority:    nextPriority + i,
			Name:        suggestion.Name,
			Description: suggestion.Description,
			Conditions:  string(conditionsJSON),
			Action:      suggestion.Action,
			TargetPath:  suggestion.TargetPath,
			Enabled:     true,
		}

		if rule.Action == "" {
			rule.Action = "move"
		}

		if err := uc.ruleRepo.Create(ctx, rule); err != nil {
			return fmt.Errorf("규칙 생성 실패 (%s): %w", suggestion.Name, err)
		}
		log.Printf("✅ 규칙 추가: %s (세트 %d, 우선순위 %d)", suggestion.Name, ruleSetID, rule.Priority)
	}

	return nil
}

// ============================================================
// 규칙 세트 CRUD
// ============================================================

// CreateRuleSet creates a new rule set
func (uc *SmartOrganizerUseCase) CreateRuleSet(ctx context.Context, ruleSet *entities.OrganizationRuleSet) error {
	return uc.ruleSetRepo.Create(ctx, ruleSet)
}

// GetRuleSet returns a rule set with its rules
func (uc *SmartOrganizerUseCase) GetRuleSet(ctx context.Context, id int) (*entities.OrganizationRuleSet, error) {
	ruleSet, err := uc.ruleSetRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if ruleSet == nil {
		return nil, nil
	}

	rules, err := uc.ruleRepo.GetByRuleSetID(ctx, id)
	if err != nil {
		return nil, err
	}

	ruleSet.Rules = make([]entities.OrganizationRule, len(rules))
	for i, r := range rules {
		ruleSet.Rules[i] = *r
	}

	return ruleSet, nil
}

// ListRuleSets returns all rule sets
func (uc *SmartOrganizerUseCase) ListRuleSets(ctx context.Context) ([]*entities.OrganizationRuleSet, error) {
	return uc.ruleSetRepo.List(ctx)
}

// UpdateRuleSet updates a rule set
func (uc *SmartOrganizerUseCase) UpdateRuleSet(ctx context.Context, ruleSet *entities.OrganizationRuleSet) error {
	return uc.ruleSetRepo.Update(ctx, ruleSet)
}

// DeleteRuleSet deletes a rule set and its rules
func (uc *SmartOrganizerUseCase) DeleteRuleSet(ctx context.Context, id int) error {
	// Stop watcher if active
	uc.StopWatching(id)
	return uc.ruleSetRepo.Delete(ctx, id)
}

// ============================================================
// 규칙 CRUD
// ============================================================

// CreateRule creates a new rule
func (uc *SmartOrganizerUseCase) CreateRule(ctx context.Context, rule *entities.OrganizationRule) error {
	return uc.ruleRepo.Create(ctx, rule)
}

// UpdateRule updates a rule
func (uc *SmartOrganizerUseCase) UpdateRule(ctx context.Context, rule *entities.OrganizationRule) error {
	return uc.ruleRepo.Update(ctx, rule)
}

// DeleteRule deletes a rule
func (uc *SmartOrganizerUseCase) DeleteRule(ctx context.Context, id int) error {
	return uc.ruleRepo.Delete(ctx, id)
}

// ============================================================
// Google Drive 백업 / 복원
// ============================================================

// BackupRuleSetToGDrive backs up a rule set to Google Drive as JSON
func (uc *SmartOrganizerUseCase) BackupRuleSetToGDrive(ctx context.Context, ruleSetID int) (string, error) {
	ruleSet, err := uc.GetRuleSet(ctx, ruleSetID)
	if err != nil {
		return "", fmt.Errorf("규칙 세트 조회 실패: %w", err)
	}
	if ruleSet == nil {
		return "", fmt.Errorf("규칙 세트를 찾을 수 없습니다: ID %d", ruleSetID)
	}

	data, err := json.MarshalIndent(ruleSet, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON 직렬화 실패: %w", err)
	}

	fileName := fmt.Sprintf("organizer_ruleset_%d_%s.json", ruleSetID, time.Now().Format("20060102_150405"))
	fileID, err := uc.storageProvider.UploadFile(ctx, "root", fileName, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("Google Drive 업로드 실패: %w", err)
	}

	// Update backup file ID
	ruleSet.BackupFileID = fileID
	if err := uc.ruleSetRepo.Update(ctx, ruleSet); err != nil {
		log.Printf("⚠️ 백업 파일 ID 업데이트 실패: %v", err)
	}

	log.Printf("✅ 규칙 세트 백업 완료: %s (파일 ID: %s)", fileName, fileID)
	return fileID, nil
}

// RestoreRuleSetFromGDrive restores a rule set from Google Drive backup
func (uc *SmartOrganizerUseCase) RestoreRuleSetFromGDrive(ctx context.Context, fileID string) (*entities.OrganizationRuleSet, error) {
	reader, err := uc.storageProvider.DownloadFile(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("백업 파일 다운로드 실패: %w", err)
	}
	defer reader.Close()

	var data []byte
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	var ruleSet entities.OrganizationRuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	// Create new rule set
	ruleSet.ID = 0
	ruleSet.BackupFileID = fileID
	if err := uc.ruleSetRepo.Create(ctx, &ruleSet); err != nil {
		return nil, fmt.Errorf("규칙 세트 생성 실패: %w", err)
	}

	// Create rules
	for i := range ruleSet.Rules {
		ruleSet.Rules[i].ID = 0
		ruleSet.Rules[i].RuleSetID = ruleSet.ID
		rule := ruleSet.Rules[i]
		if err := uc.ruleRepo.Create(ctx, &rule); err != nil {
			log.Printf("⚠️ 규칙 복원 실패: %v", err)
		}
	}

	log.Printf("✅ 규칙 세트 복원 완료: %s (ID: %d, 규칙 수: %d)", ruleSet.Name, ruleSet.ID, len(ruleSet.Rules))
	return &ruleSet, nil
}

// ============================================================
// 규칙 적용 (미리보기 + 실행)
// ============================================================

// PreviewOrganize previews what would happen if rules are applied
func (uc *SmartOrganizerUseCase) PreviewOrganize(ctx context.Context, ruleSetID int, folderID string) (*OrganizePreviewResult, error) {
	rules, err := uc.ruleRepo.GetByRuleSetID(ctx, ruleSetID)
	if err != nil {
		return nil, fmt.Errorf("규칙 조회 실패: %w", err)
	}

	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("파일 목록 조회 실패: %w", err)
	}

	result := &OrganizePreviewResult{
		TotalFiles: len(files),
		Items:      make([]OrganizePreviewItem, 0),
	}

	for _, file := range files {
		if file.MimeType == "application/vnd.google-apps.folder" {
			continue
		}

		rule := uc.matchFile(file, rules)
		if rule != nil {
			result.MatchedFiles++
			result.Items = append(result.Items, OrganizePreviewItem{
				FileID:     file.ID,
				FileName:   file.Name,
				FileSize:   file.Size,
				MimeType:   file.MimeType,
				SourcePath: file.Path,
				TargetPath: rule.TargetPath,
				RuleName:   rule.Name,
				Action:     rule.Action,
			})
		} else {
			result.UnmatchedFiles++
		}
	}

	return result, nil
}

// ExecuteOrganize starts organizing files in the background
func (uc *SmartOrganizerUseCase) ExecuteOrganize(ctx context.Context, ruleSetID int, folderID string) (*OrganizeJob, error) {
	rules, err := uc.ruleRepo.GetByRuleSetID(ctx, ruleSetID)
	if err != nil {
		return nil, fmt.Errorf("규칙 조회 실패: %w", err)
	}

	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("파일 목록 조회 실패: %w", err)
	}

	// Filter out folders
	var targetFiles []*entities.File
	for _, f := range files {
		if f.MimeType != "application/vnd.google-apps.folder" {
			targetFiles = append(targetFiles, f)
		}
	}

	jobCtx, cancel := context.WithCancel(context.Background())
	job := &OrganizeJob{
		ID:         fmt.Sprintf("organize_%d_%d", ruleSetID, time.Now().UnixNano()),
		RuleSetID:  ruleSetID,
		FolderID:   folderID,
		Status:     "running",
		TotalFiles: len(targetFiles),
		StartTime:  time.Now(),
		cancelFunc: cancel,
	}

	uc.activeJobsMu.Lock()
	uc.activeJobs[job.ID] = job
	uc.activeJobsMu.Unlock()

	go uc.processOrganize(jobCtx, job, targetFiles, rules)

	log.Printf("🔄 파일 정리 시작: 작업 ID=%s, 규칙세트=%d, 폴더=%s, 파일=%d개", job.ID, ruleSetID, folderID, len(targetFiles))
	return job, nil
}

func (uc *SmartOrganizerUseCase) processOrganize(ctx context.Context, job *OrganizeJob, files []*entities.File, rules []*entities.OrganizationRule) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 파일 정리 중 패닉 복구: %v", r)
			job.Status = "failed"
			job.ErrorMessage = fmt.Sprintf("패닉: %v", r)
		}
		job.EndTime = time.Now()
	}()

	for _, file := range files {
		select {
		case <-ctx.Done():
			job.Status = "cancelled"
			log.Printf("⚠️ 파일 정리 취소됨: %s", job.ID)
			return
		default:
		}

		atomic.AddInt32(&job.ProcessedFiles, 1)

		rule := uc.matchFile(file, rules)
		if rule == nil {
			atomic.AddInt32(&job.SkippedFiles, 1)
			continue
		}

		err := uc.executeAction(ctx, file, rule)
		if err != nil {
			atomic.AddInt32(&job.FailedFiles, 1)
			log.Printf("❌ 파일 정리 실패 (%s): %v", file.Name, err)

			// Log failure
			uc.logAction(ctx, job.RuleSetID, rule, file, "failed", err.Error())
		} else {
			atomic.AddInt32(&job.MovedFiles, 1)
			action := "moved"
			if rule.Action == "copy" {
				action = "copied"
			}
			uc.logAction(ctx, job.RuleSetID, rule, file, action, "")
		}
	}

	job.Status = "completed"
	log.Printf("✅ 파일 정리 완료: %s (이동: %d, 건너뜀: %d, 실패: %d)",
		job.ID, atomic.LoadInt32(&job.MovedFiles), atomic.LoadInt32(&job.SkippedFiles), atomic.LoadInt32(&job.FailedFiles))
}

// GetOrganizeProgress returns the progress of an organize job
func (uc *SmartOrganizerUseCase) GetOrganizeProgress(jobID string) *OrganizeJob {
	uc.activeJobsMu.RLock()
	defer uc.activeJobsMu.RUnlock()
	return uc.activeJobs[jobID]
}

// CancelOrganize cancels an organize job
func (uc *SmartOrganizerUseCase) CancelOrganize(jobID string) error {
	uc.activeJobsMu.RLock()
	job, exists := uc.activeJobs[jobID]
	uc.activeJobsMu.RUnlock()

	if !exists {
		return fmt.Errorf("작업을 찾을 수 없습니다: %s", jobID)
	}

	if job.cancelFunc != nil {
		job.cancelFunc()
	}
	return nil
}

// ============================================================
// 규칙 엔진
// ============================================================

// matchFile returns the first matching rule for a file (by priority)
func (uc *SmartOrganizerUseCase) matchFile(file *entities.File, rules []*entities.OrganizationRule) *entities.OrganizationRule {
	// Rules should already be sorted by priority ASC from DB query
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		conditions, err := rule.GetConditions()
		if err != nil {
			log.Printf("⚠️ 규칙 조건 파싱 실패 (규칙 %d): %v", rule.ID, err)
			continue
		}

		if len(conditions) == 0 {
			continue
		}

		allMatch := true
		for _, cond := range conditions {
			if !evaluateCondition(file, cond) {
				allMatch = false
				break
			}
		}

		if allMatch {
			return rule
		}
	}

	return nil
}

// evaluateCondition evaluates a single condition against a file
func evaluateCondition(file *entities.File, cond entities.RuleCondition) bool {
	var fieldValue string

	switch cond.Field {
	case "name":
		fieldValue = file.Name
	case "extension":
		fieldValue = filepath.Ext(file.Name)
	case "mimeType":
		fieldValue = file.MimeType
	case "size":
		return evaluateSizeCondition(file.Size, cond.Operator, cond.Value)
	case "modifiedDate":
		return evaluateDateCondition(file.ModifiedTime, cond.Operator, cond.Value)
	default:
		return false
	}

	return evaluateStringCondition(fieldValue, cond.Operator, cond.Value)
}

func evaluateStringCondition(fieldValue, operator, value string) bool {
	fieldLower := strings.ToLower(fieldValue)
	valueLower := strings.ToLower(value)

	switch operator {
	case "equals":
		return fieldLower == valueLower
	case "contains":
		return strings.Contains(fieldLower, valueLower)
	case "startsWith":
		return strings.HasPrefix(fieldLower, valueLower)
	case "endsWith":
		return strings.HasSuffix(fieldLower, valueLower)
	case "matches":
		re, err := regexp.Compile("(?i)" + value)
		if err != nil {
			return false
		}
		return re.MatchString(fieldValue)
	default:
		return false
	}
}

func evaluateSizeCondition(fileSize int64, operator, value string) bool {
	targetSize, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false
	}

	switch operator {
	case "greaterThan":
		return fileSize > targetSize
	case "lessThan":
		return fileSize < targetSize
	case "equals":
		return fileSize == targetSize
	default:
		return false
	}
}

func evaluateDateCondition(modifiedTime time.Time, operator, value string) bool {
	targetDate, err := time.Parse("2006-01-02", value)
	if err != nil {
		targetDate, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return false
		}
	}

	switch operator {
	case "before":
		return modifiedTime.Before(targetDate)
	case "after":
		return modifiedTime.After(targetDate)
	default:
		return false
	}
}

// executeAction performs the file action (move or copy)
func (uc *SmartOrganizerUseCase) executeAction(ctx context.Context, file *entities.File, rule *entities.OrganizationRule) error {
	switch rule.Action {
	case "move":
		return uc.storageProvider.MoveFile(ctx, file.ID, rule.TargetPath)
	case "copy":
		_, err := uc.storageProvider.CopyFile(ctx, file.ID, rule.TargetPath)
		return err
	default:
		return fmt.Errorf("알 수 없는 작업: %s", rule.Action)
	}
}

// ============================================================
// 감시 폴더 (자동 정리)
// ============================================================

// StartWatching starts polling a watch folder
func (uc *SmartOrganizerUseCase) StartWatching(ctx context.Context, ruleSetID int) error {
	ruleSet, err := uc.ruleSetRepo.GetByID(ctx, ruleSetID)
	if err != nil {
		return fmt.Errorf("규칙 세트 조회 실패: %w", err)
	}
	if ruleSet == nil {
		return fmt.Errorf("규칙 세트를 찾을 수 없습니다: ID %d", ruleSetID)
	}
	if ruleSet.WatchFolder == "" {
		return fmt.Errorf("감시 폴더가 설정되지 않았습니다")
	}

	uc.watchersMu.Lock()
	if _, exists := uc.watchers[ruleSetID]; exists {
		uc.watchersMu.Unlock()
		return fmt.Errorf("이미 감시 중입니다: 규칙 세트 %d", ruleSetID)
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	watcher := &FolderWatcher{
		RuleSetID:  ruleSetID,
		Ticker:     time.NewTicker(uc.pollInterval),
		Cancel:     cancel,
		LastCheck:  time.Now(),
		KnownFiles: make(map[string]bool),
	}
	uc.watchers[ruleSetID] = watcher
	uc.watchersMu.Unlock()

	// Update rule set active status
	ruleSet.IsActive = true
	_ = uc.ruleSetRepo.Update(ctx, ruleSet)

	go uc.watchLoop(watchCtx, watcher, ruleSet)

	log.Printf("👀 폴더 감시 시작: 규칙 세트 %d, 폴더 %s, 간격 %v", ruleSetID, ruleSet.WatchFolder, uc.pollInterval)
	return nil
}

// StopWatching stops polling a watch folder
func (uc *SmartOrganizerUseCase) StopWatching(ruleSetID int) {
	uc.watchersMu.Lock()
	watcher, exists := uc.watchers[ruleSetID]
	if exists {
		watcher.Ticker.Stop()
		watcher.Cancel()
		delete(uc.watchers, ruleSetID)
	}
	uc.watchersMu.Unlock()

	if exists {
		log.Printf("🛑 폴더 감시 중지: 규칙 세트 %d", ruleSetID)
	}
}

// GetWatchStatus returns current watcher status
func (uc *SmartOrganizerUseCase) GetWatchStatus() []map[string]interface{} {
	uc.watchersMu.RLock()
	defer uc.watchersMu.RUnlock()

	var result []map[string]interface{}
	for id, w := range uc.watchers {
		result = append(result, map[string]interface{}{
			"ruleSetId":  id,
			"lastCheck":  w.LastCheck,
			"knownFiles": len(w.KnownFiles),
		})
	}
	return result
}

func (uc *SmartOrganizerUseCase) watchLoop(ctx context.Context, watcher *FolderWatcher, ruleSet *entities.OrganizationRuleSet) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-watcher.Ticker.C:
			uc.pollWatchFolder(ctx, watcher, ruleSet)
		}
	}
}

func (uc *SmartOrganizerUseCase) pollWatchFolder(ctx context.Context, watcher *FolderWatcher, ruleSet *entities.OrganizationRuleSet) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔴 폴더 감시 중 패닉 복구: %v", r)
		}
	}()

	rules, err := uc.ruleRepo.GetByRuleSetID(ctx, ruleSet.ID)
	if err != nil {
		log.Printf("❌ 감시 폴더 규칙 조회 실패: %v", err)
		return
	}

	files, err := uc.storageProvider.ListFiles(ctx, ruleSet.WatchFolder)
	if err != nil {
		log.Printf("❌ 감시 폴더 파일 조회 실패: %v", err)
		return
	}

	processed := 0
	for _, file := range files {
		if file.MimeType == "application/vnd.google-apps.folder" {
			continue
		}
		if watcher.KnownFiles[file.ID] {
			continue
		}

		// New file found
		rule := uc.matchFile(file, rules)
		if rule != nil {
			err := uc.executeAction(ctx, file, rule)
			action := "moved"
			if rule.Action == "copy" {
				action = "copied"
			}
			errMsg := ""
			if err != nil {
				action = "failed"
				errMsg = err.Error()
				log.Printf("❌ 자동 정리 실패 (%s): %v", file.Name, err)
			} else {
				log.Printf("✅ 자동 정리: %s → %s (규칙: %s)", file.Name, rule.TargetPath, rule.Name)
			}

			uc.logAction(ctx, ruleSet.ID, rule, file, action, errMsg)
		}

		watcher.KnownFiles[file.ID] = true
		processed++

		if processed >= uc.maxFilesPerRun {
			break
		}
	}

	watcher.LastCheck = time.Now()
}

// ============================================================
// 로그 & 스프레드시트 내보내기
// ============================================================

func (uc *SmartOrganizerUseCase) logAction(ctx context.Context, ruleSetID int, rule *entities.OrganizationRule, file *entities.File, action, errMsg string) {
	sourceFolder := ""
	if len(file.Parents) > 0 {
		sourceFolder = file.Parents[0]
	}

	logEntry := &entities.OrganizationLog{
		RuleSetID:    ruleSetID,
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		FileID:       file.ID,
		FileName:     file.Name,
		FileSize:     file.Size,
		FileMimeType: file.MimeType,
		SourceFolder: sourceFolder,
		SourcePath:   file.Path,
		TargetFolder: rule.TargetPath,
		TargetPath:   rule.TargetPath,
		Action:       action,
		ErrorMessage: errMsg,
	}

	if err := uc.logRepo.Create(ctx, logEntry); err != nil {
		log.Printf("⚠️ 정리 기록 저장 실패: %v", err)
	}
}

// GetLogs returns organization logs for a rule set
func (uc *SmartOrganizerUseCase) GetLogs(ctx context.Context, ruleSetID, limit, offset int) ([]*entities.OrganizationLog, int, error) {
	logs, err := uc.logRepo.GetByRuleSetID(ctx, ruleSetID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.logRepo.CountByRuleSetID(ctx, ruleSetID)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ExportLogsToSpreadsheet exports logs to Google Spreadsheet
func (uc *SmartOrganizerUseCase) ExportLogsToSpreadsheet(ctx context.Context, ruleSetID int, parentFolderID string) (string, string, error) {
	logs, err := uc.logRepo.GetByRuleSetID(ctx, ruleSetID, 10000, 0)
	if err != nil {
		return "", "", fmt.Errorf("로그 조회 실패: %w", err)
	}

	// Build CSV
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header
	writer.Write([]string{"실행일시", "규칙", "파일명", "크기", "타입", "원본경로", "대상경로", "결과", "오류"})

	for _, l := range logs {
		writer.Write([]string{
			l.ExecutedAt.Format("2006-01-02 15:04:05"),
			l.RuleName,
			l.FileName,
			fmt.Sprintf("%d", l.FileSize),
			l.FileMimeType,
			l.SourcePath,
			l.TargetPath,
			l.Action,
			l.ErrorMessage,
		})
	}
	writer.Flush()

	if parentFolderID == "" {
		parentFolderID = "root"
	}

	fileName := fmt.Sprintf("organizer_logs_%d_%s", ruleSetID, time.Now().Format("20060102_150405"))
	fileID, webLink, err := uc.storageProvider.UploadCSVAsSpreadsheet(ctx, parentFolderID, fileName, &buf)
	if err != nil {
		return "", "", fmt.Errorf("스프레드시트 업로드 실패: %w", err)
	}

	log.Printf("✅ 로그 스프레드시트 내보내기 완료: %s", webLink)
	return fileID, webLink, nil
}

// ============================================================
// 폴더 파일 목록 (규칙 매칭 미리보기용)
// ============================================================

// ListFolderFiles lists files in a folder for rule preview
func (uc *SmartOrganizerUseCase) ListFolderFiles(ctx context.Context, folderID string) ([]*entities.File, error) {
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("폴더 파일 조회 실패: %w", err)
	}

	// Sort: folders first, then by name
	sort.Slice(files, func(i, j int) bool {
		iFolder := files[i].MimeType == "application/vnd.google-apps.folder"
		jFolder := files[j].MimeType == "application/vnd.google-apps.folder"
		if iFolder != jFolder {
			return iFolder
		}
		return files[i].Name < files[j].Name
	})

	return files, nil
}
