package usecases

import (
	"context"
	"fmt"
	"log"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
)

// VerifiedDuplicateUseCase handles business logic for verified duplicates
type VerifiedDuplicateUseCase struct {
	verifiedRepo  repositories.VerifiedDuplicateRepository
	duplicateRepo repositories.DuplicateRepository
}

// NewVerifiedDuplicateUseCase creates a new use case instance
func NewVerifiedDuplicateUseCase(
	verifiedRepo repositories.VerifiedDuplicateRepository,
	duplicateRepo repositories.DuplicateRepository,
) *VerifiedDuplicateUseCase {
	return &VerifiedDuplicateUseCase{
		verifiedRepo:  verifiedRepo,
		duplicateRepo: duplicateRepo,
	}
}

// MarkAsVerified marks a duplicate group as verified to exclude from future searches
func (uc *VerifiedDuplicateUseCase) MarkAsVerified(ctx context.Context, request *entities.VerifiedDuplicateRequest) (*entities.VerifiedDuplicate, error) {
	log.Printf("🔍 중복 그룹을 검증됨으로 마킹: hash=%s, status=%s", request.Hash, request.Status)

	// Check if this hash is already verified
	existing, err := uc.verifiedRepo.GetByHash(ctx, request.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing verified duplicate: %w", err)
	}

	if existing != nil {
		// Update existing record
		existing.Status = string(request.Status)
		existing.Description = request.Description

		if err := uc.verifiedRepo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update verified duplicate: %w", err)
		}

		log.Printf("✅ 기존 검증 기록 업데이트 완료: ID=%d, hash=%s", existing.ID, existing.Hash)
		return existing, nil
	}

	// Create new verified duplicate record
	verified := &entities.VerifiedDuplicate{
		Hash:        request.Hash,
		FileCount:   request.FileCount,
		TotalSize:   request.TotalSize,
		Status:      string(request.Status),
		Description: request.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := uc.verifiedRepo.Create(ctx, verified); err != nil {
		return nil, fmt.Errorf("failed to create verified duplicate: %w", err)
	}

	log.Printf("✅ 새로운 검증 기록 생성 완료: ID=%d, hash=%s", verified.ID, verified.Hash)
	return verified, nil
}

// GetVerifiedDuplicates retrieves verified duplicates with optional filtering
func (uc *VerifiedDuplicateUseCase) GetVerifiedDuplicates(ctx context.Context, filter *entities.VerifiedDuplicateFilter) ([]*entities.VerifiedDuplicate, error) {
	log.Printf("🔍 검증된 중복 목록 조회 시작")

	results, err := uc.verifiedRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get verified duplicates: %w", err)
	}

	log.Printf("✅ 검증된 중복 목록 조회 완료: %d개 항목", len(results))
	return results, nil
}

// UpdateVerificationStatus updates the status of a verified duplicate
func (uc *VerifiedDuplicateUseCase) UpdateVerificationStatus(ctx context.Context, id int, status entities.VerifiedDuplicateStatus, description string) error {
	log.Printf("🔍 검증 상태 업데이트 시작: ID=%d, status=%s", id, status)

	// Get existing record
	existing, err := uc.verifiedRepo.GetByHash(ctx, "") // We need to modify this to get by ID
	if err != nil {
		return fmt.Errorf("failed to get verified duplicate: %w", err)
	}

	if existing == nil {
		return fmt.Errorf("verified duplicate with ID %d not found", id)
	}

	// Update status and description
	existing.Status = string(status)
	existing.Description = description

	if err := uc.verifiedRepo.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update verification status: %w", err)
	}

	log.Printf("✅ 검증 상태 업데이트 완료: ID=%d", id)
	return nil
}

// RemoveVerification removes a verified duplicate record
func (uc *VerifiedDuplicateUseCase) RemoveVerification(ctx context.Context, id int) error {
	log.Printf("🔍 검증 기록 삭제 시작: ID=%d", id)

	if err := uc.verifiedRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to remove verification: %w", err)
	}

	log.Printf("✅ 검증 기록 삭제 완료: ID=%d", id)
	return nil
}

// GetExcludedHashes returns hashes that should be excluded from duplicate search
func (uc *VerifiedDuplicateUseCase) GetExcludedHashes(ctx context.Context) ([]string, error) {
	log.Printf("🔍 제외할 해시 목록 조회 시작")

	hashes, err := uc.verifiedRepo.GetVerifiedHashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get excluded hashes: %w", err)
	}

	log.Printf("✅ 제외할 해시 목록 조회 완료: %d개 해시", len(hashes))
	return hashes, nil
}

// FilterExistingVerified filters out duplicate groups that have already been verified
func (uc *VerifiedDuplicateUseCase) FilterExistingVerified(ctx context.Context, duplicateGroups []*entities.DuplicateGroup) ([]*entities.DuplicateGroup, error) {
	if len(duplicateGroups) == 0 {
		return duplicateGroups, nil
	}

	log.Printf("🔍 기존 검증된 중복 필터링 시작: %d개 그룹", len(duplicateGroups))

	// Extract hashes from duplicate groups
	hashes := make([]string, len(duplicateGroups))
	for i, group := range duplicateGroups {
		hashes[i] = group.Hash
	}

	// Batch check which hashes are verified
	verifiedMap, err := uc.verifiedRepo.BatchCheckVerified(ctx, hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to batch check verified duplicates: %w", err)
	}

	// Filter out verified groups
	var filteredGroups []*entities.DuplicateGroup
	excludedCount := 0
	verifiedCount := 0

	for _, group := range duplicateGroups {
		if verified, exists := verifiedMap[group.Hash]; exists {
			// Skip only ignored groups, keep verified groups for display with mark
			if verified.Status == string(entities.StatusIgnored) {
				excludedCount++
				continue
			} else if verified.Status == string(entities.StatusVerified) {
				// Add verification info to the group for frontend display
				group.VerificationStatus = verified.Status
				group.VerificationDescription = verified.Description
				group.VerificationDate = verified.CreatedAt
				verifiedCount++
			}
		}
		filteredGroups = append(filteredGroups, group)
	}

	log.Printf("✅ 검증된 중복 필터링 완료: %d개 무시 제외, %d개 검증됨 표시, %d개 유지", excludedCount, verifiedCount, len(filteredGroups))
	return filteredGroups, nil
}

// GetVerificationDetails gets detailed information about a verified duplicate
func (uc *VerifiedDuplicateUseCase) GetVerificationDetails(ctx context.Context, hash string) (*entities.VerifiedDuplicate, error) {
	log.Printf("🔍 검증 상세 정보 조회: hash=%s", hash)

	verified, err := uc.verifiedRepo.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get verification details: %w", err)
	}

	if verified == nil {
		log.Printf("⚠️  검증 정보 없음: hash=%s", hash)
		return nil, nil
	}

	log.Printf("✅ 검증 상세 정보 조회 완료: ID=%d, status=%s", verified.ID, verified.Status)
	return verified, nil
}

// BulkMarkAsVerified marks multiple duplicate groups as verified
func (uc *VerifiedDuplicateUseCase) BulkMarkAsVerified(ctx context.Context, requests []*entities.VerifiedDuplicateRequest) (int, error) {
	log.Printf("🔍 대량 검증 마킹 시작: %d개 요청", len(requests))

	successCount := 0
	for i, request := range requests {
		_, err := uc.MarkAsVerified(ctx, request)
		if err != nil {
			log.Printf("❌ 검증 마킹 실패 [%d/%d]: hash=%s, error=%v", i+1, len(requests), request.Hash, err)
			continue
		}
		successCount++

		if (i+1)%10 == 0 || i+1 == len(requests) {
			log.Printf("📊 진행 상황: %d/%d 완료", i+1, len(requests))
		}
	}

	log.Printf("✅ 대량 검증 마킹 완료: %d/%d 성공", successCount, len(requests))
	return successCount, nil
}
