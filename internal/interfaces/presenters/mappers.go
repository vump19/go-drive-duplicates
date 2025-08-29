package presenters

import (
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/usecases"
	"time"
)

// Entity to DTO mappers

// ToProgressDTO converts a Progress entity to ProgressDTO
func ToProgressDTO(progress *entities.Progress) *ProgressDTO {
	if progress == nil {
		return nil
	}

	dto := &ProgressDTO{
		ID:             progress.ID,
		OperationType:  progress.OperationType,
		ProcessedItems: progress.ProcessedItems,
		TotalItems:     progress.TotalItems,
		Status:         progress.Status,
		CurrentStep:    progress.CurrentStep,
		ErrorMessage:   progress.ErrorMessage,
		Percentage:     progress.GetPercentage(),
		StartTime:      progress.StartTime,
		EndTime:        progress.EndTime,
		LastUpdated:    progress.LastUpdated,
		Duration:       formatDuration(progress.GetDuration()),
		Metadata:       progress.Metadata,
	}

	// Calculate ETA if available
	if eta := progress.GetETA(); eta != nil {
		dto.ETA = eta
	}

	return dto
}

// ToFileDTO converts a File entity to FileDTO
func ToFileDTO(file *entities.File) *FileDTO {
	if file == nil {
		return nil
	}

	return &FileDTO{
		ID:             file.ID,
		Name:           file.Name,
		Size:           file.Size,
		SizeFormatted:  formatFileSize(file.Size),
		MimeType:       file.MimeType,
		ModifiedTime:   file.ModifiedTime,
		Hash:           file.Hash,
		HashCalculated: file.HashCalculated,
		Parents:        file.Parents,
		Path:           file.Path,
		WebViewLink:    file.WebViewLink,
		Category:       file.GetFileCategory(),
		SizeCategory:   file.GetSizeCategory(),
		IsLargeFile:    file.IsLargeFile(),
	}
}

// ToFileDTOList converts a slice of File entities to FileDTO slice
func ToFileDTOList(files []*entities.File) []*FileDTO {
	if files == nil {
		return nil
	}

	dtos := make([]*FileDTO, len(files))
	for i, file := range files {
		dtos[i] = ToFileDTO(file)
	}
	return dtos
}

// ToDuplicateGroupDTO converts a DuplicateGroup entity to DuplicateGroupDTO
func ToDuplicateGroupDTO(group *entities.DuplicateGroup) *DuplicateGroupDTO {
	if group == nil {
		return nil
	}

	dto := &DuplicateGroupDTO{
		ID:                   group.ID,
		Hash:                 group.Hash,
		Files:                ToFileDTOList(group.Files),
		Count:                group.Count,
		TotalSize:            group.TotalSize,
		TotalSizeFormatted:   formatFileSize(group.TotalSize),
		WastedSpace:          group.GetWastedSpace(),
		WastedSpaceFormatted: formatFileSize(group.GetWastedSpace()),
		CreatedAt:            group.CreatedAt,
		UpdatedAt:            group.UpdatedAt,
	}

	// Add oldest and newest file information
	if oldest := group.GetOldestFile(); oldest != nil {
		dto.OldestFile = ToFileDTO(oldest)
	}
	if newest := group.GetNewestFile(); newest != nil {
		dto.NewestFile = ToFileDTO(newest)
	}

	return dto
}

// ToDuplicateGroupDTOList converts a slice of DuplicateGroup entities to DuplicateGroupDTO slice
func ToDuplicateGroupDTOList(groups []*entities.DuplicateGroup) []*DuplicateGroupDTO {
	if groups == nil {
		return nil
	}

	dtos := make([]*DuplicateGroupDTO, len(groups))
	for i, group := range groups {
		dtos[i] = ToDuplicateGroupDTO(group)
	}
	return dtos
}

// ToComparisonResultDTO converts a ComparisonResult entity to ComparisonResultDTO
func ToComparisonResultDTO(result *entities.ComparisonResult) *ComparisonResultDTO {
	if result == nil {
		return nil
	}

	return &ComparisonResultDTO{
		ID:                       result.ID,
		SourceFolderID:           result.SourceFolderID,
		TargetFolderID:           result.TargetFolderID,
		SourceFolderName:         result.SourceFolderName,
		TargetFolderName:         result.TargetFolderName,
		SourceFileCount:          result.SourceFileCount,
		TargetFileCount:          result.TargetFileCount,
		DuplicateCount:           result.DuplicateCount,
		SourceTotalSize:          result.SourceTotalSize,
		SourceTotalSizeFormatted: formatFileSize(result.SourceTotalSize),
		TargetTotalSize:          result.TargetTotalSize,
		TargetTotalSizeFormatted: formatFileSize(result.TargetTotalSize),
		DuplicateSize:            result.DuplicateSize,
		DuplicateSizeFormatted:   formatFileSize(result.DuplicateSize),
		DuplicateFiles:           ToFileDTOList(result.DuplicateFiles),
		CanDeleteTargetFolder:    result.CanDeleteTargetFolder,
		DuplicationPercentage:    result.DuplicationPercentage,
		UniqueFilesInTarget:      result.GetUniqueFilesInTarget(),
		UniqueFilesSize:          result.GetUniqueFilesSize(),
		UniqueFilesSizeFormatted: formatFileSize(result.GetUniqueFilesSize()),
		IsSignificantSavings:     result.IsSignificantSavings(),
		Summary:                  result.Summary(),
		CreatedAt:                result.CreatedAt,
		UpdatedAt:                result.UpdatedAt,
	}
}

// ToFileStatisticsDTO converts a FileStatistics entity to FileStatisticsDTO
func ToFileStatisticsDTO(stats *entities.FileStatistics) *FileStatisticsDTO {
	if stats == nil {
		return nil
	}

	dto := &FileStatisticsDTO{
		TotalFiles:               stats.TotalFiles,
		TotalSize:                stats.TotalSize,
		TotalSizeFormatted:       formatFileSize(stats.TotalSize),
		AverageFileSize:          stats.GetAverageFileSize(),
		AverageFileSizeFormatted: formatFileSize(stats.GetAverageFileSize()),
		FilesByType:              stats.FilesByType,
		SizesByType:              stats.SizesByType,
		FilesBySize:              stats.FilesBySize,
		SizesBySize:              stats.SizesBySize,
		FilesByMonth:             stats.FilesByMonth,
		SizesByMonth:             stats.SizesByMonth,
		TopFolders:               ToFolderStatsDTOList(stats.TopFolders),
		TopExtensions:            ToExtensionStatsDTOList(stats.TopExtensions),
		SpaceDistribution:        stats.GetSpaceDistribution(),
		LargestCategory:          stats.GetLargestCategory(),
		GeneratedAt:              stats.GeneratedAt,
	}

	return dto
}

// ToFolderStatsDTO converts a FolderStats entity to FolderStatsDTO
func ToFolderStatsDTO(stats *entities.FolderStats) *FolderStatsDTO {
	if stats == nil {
		return nil
	}

	return &FolderStatsDTO{
		FolderID:           stats.FolderID,
		FolderName:         stats.FolderName,
		FileCount:          stats.FileCount,
		TotalSize:          stats.TotalSize,
		TotalSizeFormatted: formatFileSize(stats.TotalSize),
		Path:               stats.Path,
	}
}

// ToFolderStatsDTOList converts a slice of FolderStats entities to FolderStatsDTO slice
func ToFolderStatsDTOList(statsList []*entities.FolderStats) []*FolderStatsDTO {
	if statsList == nil {
		return nil
	}

	dtos := make([]*FolderStatsDTO, len(statsList))
	for i, stats := range statsList {
		dtos[i] = ToFolderStatsDTO(stats)
	}
	return dtos
}

// ToExtensionStatsDTO converts an ExtensionStats entity to ExtensionStatsDTO
func ToExtensionStatsDTO(stats *entities.ExtensionStats) *ExtensionStatsDTO {
	if stats == nil {
		return nil
	}

	return &ExtensionStatsDTO{
		Extension:          stats.Extension,
		Count:              stats.Count,
		TotalSize:          stats.TotalSize,
		TotalSizeFormatted: formatFileSize(stats.TotalSize),
		AvgSize:            stats.AvgSize,
		AvgSizeFormatted:   formatFileSize(stats.AvgSize),
	}
}

// ToExtensionStatsDTOList converts a slice of ExtensionStats entities to ExtensionStatsDTO slice
func ToExtensionStatsDTOList(statsList []*entities.ExtensionStats) []*ExtensionStatsDTO {
	if statsList == nil {
		return nil
	}

	dtos := make([]*ExtensionStatsDTO, len(statsList))
	for i, stats := range statsList {
		dtos[i] = ToExtensionStatsDTO(stats)
	}
	return dtos
}

// UseCase response to DTO mappers

// ToScanResponseDTO converts a use case response to ScanResponseDTO
func ToScanResponseDTO(response interface{}) *ScanResponseDTO {
	switch r := response.(type) {
	case *usecases.ScanAllFilesResponse:
		return &ScanResponseDTO{
			Progress:       ToProgressDTO(r.Progress),
			TotalFiles:     r.TotalFiles,
			ProcessedFiles: r.ProcessedFiles,
			NewFiles:       r.NewFiles,
			UpdatedFiles:   r.UpdatedFiles,
			Errors:         r.Errors,
		}
	case *usecases.ScanFolderResponse:
		return &ScanResponseDTO{
			Progress:       ToProgressDTO(r.Progress),
			TotalFiles:     r.TotalFiles,
			ProcessedFiles: r.ProcessedFiles,
			NewFiles:       r.NewFiles,
			UpdatedFiles:   r.UpdatedFiles,
			FolderPath:     r.FolderPath,
			Errors:         r.Errors,
		}
	default:
		return nil
	}
}

// ToDuplicatesResponseDTO converts a FindDuplicatesResponse to DuplicatesResponseDTO
func ToDuplicatesResponseDTO(response *usecases.FindDuplicatesResponse) *DuplicatesResponseDTO {
	if response == nil {
		return nil
	}

	return &DuplicatesResponseDTO{
		Progress:                  ToProgressDTO(response.Progress),
		DuplicateGroups:           ToDuplicateGroupDTOList(response.DuplicateGroups),
		TotalGroups:               response.TotalGroups,
		TotalFiles:                response.TotalFiles,
		TotalWastedSpace:          response.TotalWastedSpace,
		TotalWastedSpaceFormatted: formatFileSize(response.TotalWastedSpace),
		HashesCalculated:          response.HashesCalculated,
		Errors:                    response.Errors,
	}
}

// ToComparisonResponseDTO converts a CompareFoldersResponse to ComparisonResponseDTO
func ToComparisonResponseDTO(response *usecases.CompareFoldersResponse) *ComparisonResponseDTO {
	if response == nil {
		return nil
	}

	return &ComparisonResponseDTO{
		Progress:         ToProgressDTO(response.Progress),
		ComparisonResult: ToComparisonResultDTO(response.ComparisonResult),
		Errors:           response.Errors,
	}
}

// ToDeleteResponseDTO converts a DeleteFilesResponse to DeleteResponseDTO
func ToDeleteResponseDTO(response *usecases.DeleteFilesResponse) *DeleteResponseDTO {
	if response == nil {
		return nil
	}

	return &DeleteResponseDTO{
		Progress:            ToProgressDTO(response.Progress),
		TotalFiles:          response.TotalFiles,
		DeletedFiles:        response.DeletedFiles,
		FailedFiles:         response.FailedFiles,
		DeletedFolders:      response.DeletedFolders,
		SpaceSaved:          response.SpaceSaved,
		SpaceSavedFormatted: formatFileSize(response.SpaceSaved),
		Errors:              response.Errors,
	}
}

// ToCleanupResponseDTO converts a CleanupEmptyFoldersResponse to CleanupResponseDTO
func ToCleanupResponseDTO(response *usecases.CleanupEmptyFoldersResponse) *CleanupResponseDTO {
	if response == nil {
		return nil
	}

	return &CleanupResponseDTO{
		Progress:       ToProgressDTO(response.Progress),
		DeletedFolders: response.DeletedFolders,
		Errors:         response.Errors,
	}
}

// ToHashCalculationResponseDTO converts a CalculateHashesResponse to HashCalculationResponseDTO
func ToHashCalculationResponseDTO(response *usecases.CalculateHashesResponse) *HashCalculationResponseDTO {
	if response == nil {
		return nil
	}

	return &HashCalculationResponseDTO{
		Progress:         ToProgressDTO(response.Progress),
		TotalFiles:       response.TotalFiles,
		ProcessedFiles:   response.ProcessedFiles,
		SuccessfulHashes: response.SuccessfulHashes,
		FailedHashes:     response.FailedHashes,
		Errors:           response.Errors,
	}
}

// Utility functions

// formatFileSize formats file size in human readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration formats duration in human readable format
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}

// CreateErrorResponse creates a standard error response
func CreateErrorResponse(err error, code string) *ErrorResponse {
	response := &ErrorResponse{
		Error: err.Error(),
		Code:  code,
	}
	return response
}

// CreateSuccessResponse creates a standard success response
func CreateSuccessResponse(message string, data interface{}) *SuccessResponse {
	return &SuccessResponse{
		Status:  "success",
		Message: message,
		Data:    data,
	}
}
