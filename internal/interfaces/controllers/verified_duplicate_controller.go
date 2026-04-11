package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/interfaces/presenters"
	"go-drive-duplicates/internal/usecases"
)

// VerifiedDuplicateController handles HTTP requests for verified duplicates
type VerifiedDuplicateController struct {
	verifiedUseCase *usecases.VerifiedDuplicateUseCase
	presenter       *presenters.VerifiedDuplicatePresenter
}

// NewVerifiedDuplicateController creates a new controller
func NewVerifiedDuplicateController(
	verifiedUseCase *usecases.VerifiedDuplicateUseCase,
	presenter *presenters.VerifiedDuplicatePresenter,
) *VerifiedDuplicateController {
	return &VerifiedDuplicateController{
		verifiedUseCase: verifiedUseCase,
		presenter:       presenter,
	}
}

// MarkAsVerified marks a duplicate group as verified
// POST /api/verified-duplicates
func (c *VerifiedDuplicateController) MarkAsVerified(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 검증된 중복 마킹 요청 수신")

	var request entities.VerifiedDuplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("❌ 요청 디코딩 실패: %v", err)
		c.presenter.PresentError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if request.Hash == "" {
		c.presenter.PresentError(w, "Hash is required", http.StatusBadRequest)
		return
	}

	if request.Status == "" {
		c.presenter.PresentError(w, "Status is required", http.StatusBadRequest)
		return
	}

	if request.FileCount < 2 {
		c.presenter.PresentError(w, "File count must be at least 2", http.StatusBadRequest)
		return
	}

	if request.TotalSize < 1 {
		c.presenter.PresentError(w, "Total size must be greater than 0", http.StatusBadRequest)
		return
	}

	verified, err := c.verifiedUseCase.MarkAsVerified(r.Context(), &request)
	if err != nil {
		log.Printf("❌ 검증 마킹 실패: %v", err)
		c.presenter.PresentError(w, "Failed to mark as verified", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ 검증 마킹 성공: ID=%d", verified.ID)
	c.presenter.PresentVerifiedDuplicate(w, verified)
}

// GetVerifiedDuplicates retrieves verified duplicates with optional filtering
// GET /api/verified-duplicates
func (c *VerifiedDuplicateController) GetVerifiedDuplicates(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 검증된 중복 목록 조회 요청")

	// Parse query parameters for filtering
	query := r.URL.Query()

	filter := &entities.VerifiedDuplicateFilter{}

	// Status filter
	if status := query.Get("status"); status != "" {
		filter.Status = entities.VerifiedDuplicateStatus(status)
	}

	// Hash list filter
	if hashes := query["hash"]; len(hashes) > 0 {
		filter.HashList = hashes
	}

	// Created at filter
	if createdAtStr := query.Get("created_at"); createdAtStr != "" {
		if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			filter.CreatedAt = &createdAt
		} else {
			log.Printf("⚠️ 잘못된 created_at 형식: %s", createdAtStr)
		}
	}

	verified, err := c.verifiedUseCase.GetVerifiedDuplicates(r.Context(), filter)
	if err != nil {
		log.Printf("❌ 검증된 중복 목록 조회 실패: %v", err)
		c.presenter.PresentError(w, "Failed to get verified duplicates", http.StatusInternalServerError)
		return
	}

	c.presenter.PresentVerifiedDuplicateList(w, verified)
}

// UpdateVerificationStatus updates the status of a verified duplicate
// PUT /api/verified-duplicates/{id}
func (c *VerifiedDuplicateController) UpdateVerificationStatus(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL path
	idStr := r.URL.Path[len("/api/verified-duplicates/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.presenter.PresentError(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	log.Printf("🔍 검증 상태 업데이트 요청: ID=%d", id)

	var updateRequest struct {
		Status      entities.VerifiedDuplicateStatus `json:"status"`
		Description string                           `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		log.Printf("❌ 요청 디코딩 실패: %v", err)
		c.presenter.PresentError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if updateRequest.Status == "" {
		c.presenter.PresentError(w, "Status is required", http.StatusBadRequest)
		return
	}

	err = c.verifiedUseCase.UpdateVerificationStatus(r.Context(), id, updateRequest.Status, updateRequest.Description)
	if err != nil {
		log.Printf("❌ 검증 상태 업데이트 실패: %v", err)
		c.presenter.PresentError(w, "Failed to update verification status", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ 검증 상태 업데이트 성공: ID=%d", id)
	c.presenter.PresentSuccess(w, "Verification status updated successfully")
}

// RemoveVerification removes a verified duplicate record
// DELETE /api/verified-duplicates/{id}
func (c *VerifiedDuplicateController) RemoveVerification(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL path
	idStr := r.URL.Path[len("/api/verified-duplicates/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.presenter.PresentError(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	log.Printf("🔍 검증 기록 삭제 요청: ID=%d", id)

	err = c.verifiedUseCase.RemoveVerification(r.Context(), id)
	if err != nil {
		log.Printf("❌ 검증 기록 삭제 실패: %v", err)
		c.presenter.PresentError(w, "Failed to remove verification", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ 검증 기록 삭제 성공: ID=%d", id)
	c.presenter.PresentSuccess(w, "Verification removed successfully")
}

// GetExcludedHashes returns hashes that should be excluded from duplicate search
// GET /api/verified-duplicates/excluded-hashes
func (c *VerifiedDuplicateController) GetExcludedHashes(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 제외할 해시 목록 조회 요청")

	hashes, err := c.verifiedUseCase.GetExcludedHashes(r.Context())
	if err != nil {
		log.Printf("❌ 제외할 해시 목록 조회 실패: %v", err)
		c.presenter.PresentError(w, "Failed to get excluded hashes", http.StatusInternalServerError)
		return
	}

	c.presenter.PresentHashList(w, hashes)
}

// GetVerificationDetails gets detailed information about a verified duplicate
// GET /api/verified-duplicates/hash/{hash}
func (c *VerifiedDuplicateController) GetVerificationDetails(w http.ResponseWriter, r *http.Request) {
	// Extract hash from URL path
	hash := r.URL.Path[len("/api/verified-duplicates/hash/"):]
	if hash == "" {
		c.presenter.PresentError(w, "Hash is required", http.StatusBadRequest)
		return
	}

	log.Printf("🔍 검증 상세 정보 조회 요청: hash=%s", hash)

	verified, err := c.verifiedUseCase.GetVerificationDetails(r.Context(), hash)
	if err != nil {
		log.Printf("❌ 검증 상세 정보 조회 실패: %v", err)
		c.presenter.PresentError(w, "Failed to get verification details", http.StatusInternalServerError)
		return
	}

	if verified == nil {
		log.Printf("⚠️ 검증 정보 없음: hash=%s", hash)
		c.presenter.PresentError(w, "Verification not found", http.StatusNotFound)
		return
	}

	c.presenter.PresentVerifiedDuplicate(w, verified)
}

// BulkMarkAsVerified marks multiple duplicate groups as verified
// POST /api/verified-duplicates/bulk
func (c *VerifiedDuplicateController) BulkMarkAsVerified(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 대량 검증 마킹 요청 수신")

	var requests []*entities.VerifiedDuplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
		log.Printf("❌ 요청 디코딩 실패: %v", err)
		c.presenter.PresentError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(requests) == 0 {
		c.presenter.PresentError(w, "At least one request is required", http.StatusBadRequest)
		return
	}

	// Validate all requests
	for i, request := range requests {
		if request.Hash == "" {
			c.presenter.PresentError(w, fmt.Sprintf("Hash is required for request %d", i), http.StatusBadRequest)
			return
		}
		if request.Status == "" {
			c.presenter.PresentError(w, fmt.Sprintf("Status is required for request %d", i), http.StatusBadRequest)
			return
		}
	}

	successCount, err := c.verifiedUseCase.BulkMarkAsVerified(r.Context(), requests)
	if err != nil {
		log.Printf("❌ 대량 검증 마킹 실패: %v", err)
		c.presenter.PresentError(w, "Failed to bulk mark as verified", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ 대량 검증 마킹 성공: %d/%d", successCount, len(requests))

	response := map[string]interface{}{
		"message":       "Bulk verification completed",
		"total":         len(requests),
		"success_count": successCount,
		"failed_count":  len(requests) - successCount,
	}

	c.presenter.PresentData(w, response)
}
