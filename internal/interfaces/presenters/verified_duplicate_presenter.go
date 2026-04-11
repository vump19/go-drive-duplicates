package presenters

import (
	"encoding/json"
	"net/http"

	"go-drive-duplicates/internal/domain/entities"
)

// VerifiedDuplicatePresenter handles response formatting for verified duplicates
type VerifiedDuplicatePresenter struct{}

// NewVerifiedDuplicatePresenter creates a new presenter
func NewVerifiedDuplicatePresenter() *VerifiedDuplicatePresenter {
	return &VerifiedDuplicatePresenter{}
}

// VerifiedDuplicateResponse represents the API response for a verified duplicate
type VerifiedDuplicateResponse struct {
	ID          int    `json:"id"`
	Hash        string `json:"hash"`
	FileCount   int    `json:"file_count"`
	TotalSize   int64  `json:"total_size"`
	Status      string `json:"status"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// VerifiedDuplicateListResponse represents the API response for a list of verified duplicates
type VerifiedDuplicateListResponse struct {
	VerifiedDuplicates []*VerifiedDuplicateResponse `json:"verified_duplicates"`
	Count              int                          `json:"count"`
}

// HashListResponse represents the API response for a list of hashes
type HashListResponse struct {
	Hashes []string `json:"hashes"`
	Count  int      `json:"count"`
}

// PresentVerifiedDuplicate presents a single verified duplicate
func (p *VerifiedDuplicatePresenter) PresentVerifiedDuplicate(w http.ResponseWriter, verified *entities.VerifiedDuplicate) {
	response := &VerifiedDuplicateResponse{
		ID:          verified.ID,
		Hash:        verified.Hash,
		FileCount:   verified.FileCount,
		TotalSize:   verified.TotalSize,
		Status:      string(verified.Status),
		Description: verified.Description,
		CreatedAt:   verified.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   verified.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PresentVerifiedDuplicateList presents a list of verified duplicates
func (p *VerifiedDuplicatePresenter) PresentVerifiedDuplicateList(w http.ResponseWriter, verified []*entities.VerifiedDuplicate) {
	verifiedResponses := make([]*VerifiedDuplicateResponse, len(verified))

	for i, v := range verified {
		verifiedResponses[i] = &VerifiedDuplicateResponse{
			ID:          v.ID,
			Hash:        v.Hash,
			FileCount:   v.FileCount,
			TotalSize:   v.TotalSize,
			Status:      string(v.Status),
			Description: v.Description,
			CreatedAt:   v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	response := &VerifiedDuplicateListResponse{
		VerifiedDuplicates: verifiedResponses,
		Count:              len(verified),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PresentHashList presents a list of hashes
func (p *VerifiedDuplicatePresenter) PresentHashList(w http.ResponseWriter, hashes []string) {
	response := &HashListResponse{
		Hashes: hashes,
		Count:  len(hashes),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PresentSuccess presents a success message
func (p *VerifiedDuplicatePresenter) PresentSuccess(w http.ResponseWriter, message string) {
	response := map[string]interface{}{
		"success": true,
		"message": message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PresentError presents an error response
func (p *VerifiedDuplicatePresenter) PresentError(w http.ResponseWriter, message string, statusCode int) {
	response := map[string]interface{}{
		"error":   message,
		"success": false,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// PresentData presents arbitrary data
func (p *VerifiedDuplicatePresenter) PresentData(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}
