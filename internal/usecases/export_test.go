package usecases

import "context"

// SetRunningOperation registers a cancel function as if the operation were running in this process.
func SetRunningOperation(uc *FolderComparisonUseCase, progressID int, cancel context.CancelFunc) {
	uc.runningOpsMu.Lock()
	defer uc.runningOpsMu.Unlock()
	uc.runningOps[progressID] = cancel
}
