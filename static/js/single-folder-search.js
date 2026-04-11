/**
 * 단일 폴더 중복 검색 관련 JavaScript
 * 단일 폴더 내 중복 파일 검색 및 관리 기능들
 */

// 단일 폴더 중복 검색 관련 변수들
let singleFolderProgressInterval;
let currentSingleFolderProgressId = null;

// 단일 폴더 중복 검색 모달 관리 함수들
function findSingleFolderDuplicates() {
    const modal = document.getElementById('singleFolderDuplicateModal');
    if (modal) {
        modal.classList.remove('hidden');
        resetSingleFolderDuplicateModal();
    }
}

function closeSingleFolderDuplicateModal() {
    const modal = document.getElementById('singleFolderDuplicateModal');
    if (modal) {
        modal.classList.add('hidden');
    }
    if (singleFolderProgressInterval) {
        clearInterval(singleFolderProgressInterval);
        singleFolderProgressInterval = null;
    }
    currentSingleFolderProgressId = null;
}

function resetSingleFolderDuplicateModal() {
    // 모든 단계 숨기기
    const singleFolderInput = document.getElementById('singleFolderInput');
    const singleFolderProgress = document.getElementById('singleFolderProgress');
    const singleFolderResult = document.getElementById('singleFolderResult');
    const singleFolderDeletionProgress = document.getElementById('singleFolderDeletionProgress');
    
    if (singleFolderInput) singleFolderInput.style.display = 'block';
    if (singleFolderProgress) singleFolderProgress.style.display = 'none';
    if (singleFolderResult) singleFolderResult.style.display = 'none';
    if (singleFolderDeletionProgress) singleFolderDeletionProgress.style.display = 'none';
    
    // 입력 필드 초기화
    const singleFolderUrl = document.getElementById('singleFolderUrl');
    const includeSubfoldersOption = document.getElementById('includeSubfoldersOption');
    const minFileSizeOption = document.getElementById('minFileSizeOption');
    const forceNewScanOption = document.getElementById('forceNewScanOption');
    
    if (singleFolderUrl) singleFolderUrl.value = '';
    if (includeSubfoldersOption) includeSubfoldersOption.checked = true;
    if (minFileSizeOption) minFileSizeOption.value = '1024';
    if (forceNewScanOption) forceNewScanOption.checked = false;
    
    // 진행 상황 초기화
    const singleFolderProgressBar = document.getElementById('singleFolderProgressBar');
    const singleFolderProgressText = document.getElementById('singleFolderProgressText');
    const singleFolderProcessedCount = document.getElementById('singleFolderProcessedCount');
    const singleFolderTotalCount = document.getElementById('singleFolderTotalCount');
    
    if (singleFolderProgressBar) singleFolderProgressBar.style.width = '0%';
    if (singleFolderProgressText) singleFolderProgressText.textContent = '검색 작업 준비 중...';
    if (singleFolderProcessedCount) singleFolderProcessedCount.textContent = '0';
    if (singleFolderTotalCount) singleFolderTotalCount.textContent = '0';
    
    // 로그 초기화
    const logContainer = document.getElementById('singleFolderProgressLog');
    if (logContainer) {
        logContainer.innerHTML = '<div class="log-entry">중복 검색을 시작합니다...</div>';
    }
}

// 단일 폴더 중복 검색 시작
async function startSingleFolderDuplicateSearch() {
    const folderUrl = document.getElementById('singleFolderUrl')?.value.trim();
    
    if (!folderUrl) {
        alert('폴더 URL을 입력해주세요.');
        return;
    }
    
    try {
        // 폴더 ID 추출
        addSingleFolderLogEntry('📂 폴더 URL에서 폴더 ID 추출 중...', 'info');
        const extractResponse = await window.API.Comparison.extractFolderIdFromUrl(folderUrl);
        const folderId = extractResponse.folderId;
        
        addSingleFolderLogEntry(`✅ 폴더 ID 추출 완료: ${folderId}`, 'success');
        
        // 중복 검색 시작
        addSingleFolderLogEntry('🔍 중복 파일 검색을 시작합니다...', 'info');
        
        const options = {
            includeSubfolders: document.getElementById('includeSubfoldersOption')?.checked || true,
            minFileSize: parseInt(parseFloat(document.getElementById('minFileSizeOption')?.value || '1024') * 1024), // KB to bytes
            forceNewScan: document.getElementById('forceNewScanOption')?.checked || false
        };
        
        console.log('단일 폴더 중복 검색 요청 데이터:', { folderId, ...options });
        
        const data = await window.API.Comparison.findSingleFolderDuplicates(folderId, options);
        console.log('단일 폴더 중복 검색 API 응답:', data);
        
        currentSingleFolderProgressId = data.progressId || data.progress?.ID;
        
        // 진행 상황 표시로 전환
        showSingleFolderProgressStep();
        
        // 진행 상황 모니터링 시작
        startSingleFolderProgressTracking();
        
        addSingleFolderLogEntry('⏳ 중복 검색 작업이 백그라운드에서 진행됩니다...', 'info');
        
    } catch (error) {
        console.error('단일 폴더 중복 검색 오류:', error);
        addSingleFolderLogEntry('❌ 중복 검색 중 오류가 발생했습니다: ' + error.message, 'error');
        alert('중복 검색 중 오류가 발생했습니다: ' + error.message);
    }
}

function showSingleFolderProgressStep() {
    const singleFolderInput = document.getElementById('singleFolderInput');
    const singleFolderProgress = document.getElementById('singleFolderProgress');
    const singleFolderResult = document.getElementById('singleFolderResult');
    const singleFolderDeletionProgress = document.getElementById('singleFolderDeletionProgress');
    
    if (singleFolderInput) singleFolderInput.style.display = 'none';
    if (singleFolderProgress) singleFolderProgress.style.display = 'block';
    if (singleFolderResult) singleFolderResult.style.display = 'none';
    if (singleFolderDeletionProgress) singleFolderDeletionProgress.style.display = 'none';
}

function showSingleFolderResultStep() {
    const singleFolderInput = document.getElementById('singleFolderInput');
    const singleFolderProgress = document.getElementById('singleFolderProgress');
    const singleFolderResult = document.getElementById('singleFolderResult');
    const singleFolderDeletionProgress = document.getElementById('singleFolderDeletionProgress');
    
    if (singleFolderInput) singleFolderInput.style.display = 'none';
    if (singleFolderProgress) singleFolderProgress.style.display = 'none';
    if (singleFolderResult) singleFolderResult.style.display = 'block';
    if (singleFolderDeletionProgress) singleFolderDeletionProgress.style.display = 'none';
}

function startSingleFolderProgressTracking() {
    if (singleFolderProgressInterval) {
        clearInterval(singleFolderProgressInterval);
    }
    
    // Validate progressId before starting
    if (!currentSingleFolderProgressId) {
        console.error('진행 상황 추적 오류: progressId가 없습니다');
        addSingleFolderLogEntry('❌ 진행 상황 추적을 시작할 수 없습니다: progressId가 없습니다', 'error');
        return;
    }
    
    singleFolderProgressInterval = setInterval(async () => {
        try {
            const progress = await window.API.Comparison.getSingleFolderProgress(currentSingleFolderProgressId);
            console.log('단일 폴더 중복 검색 진행 상황:', progress);
            
            updateSingleFolderProgress(progress);
            
            if (progress.status === 'completed' || progress.status === 'failed') {
                clearInterval(singleFolderProgressInterval);
                singleFolderProgressInterval = null;
                
                if (progress.status === 'completed') {
                    addSingleFolderLogEntry('✅ 중복 검색이 완료되었습니다!', 'success');
                    loadSingleFolderSearchResults();
                } else {
                    addSingleFolderLogEntry('❌ 중복 검색에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'), 'error');
                    alert('중복 검색에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'));
                }
            }
            
        } catch (error) {
            console.error('진행 상황 조회 오류:', error);
        }
    }, 1000);
}

function updateSingleFolderProgress(progress) {
    const percentage = progress.totalItems > 0 ? (progress.processedItems / progress.totalItems * 100) : 0;
    
    const singleFolderProgressBar = document.getElementById('singleFolderProgressBar');
    const singleFolderProgressText = document.getElementById('singleFolderProgressText');
    const singleFolderProcessedCount = document.getElementById('singleFolderProcessedCount');
    const singleFolderTotalCount = document.getElementById('singleFolderTotalCount');
    
    if (singleFolderProgressBar) singleFolderProgressBar.style.width = percentage + '%';
    if (singleFolderProgressText) singleFolderProgressText.textContent = progress.currentStep || '작업 진행 중...';
    if (singleFolderProcessedCount) singleFolderProcessedCount.textContent = progress.processedItems || 0;
    if (singleFolderTotalCount) singleFolderTotalCount.textContent = progress.totalItems || 0;
}

async function loadSingleFolderSearchResults() {
    try {
        // 진행 상황에서 결과 정보를 가져오기 위해 다시 API 호출
        const progress = await window.API.Comparison.getSingleFolderProgress(currentSingleFolderProgressId);
        
        // 결과는 progress.metadata.result 또는 progress.result에 저장됨
        const result = (progress.metadata && progress.metadata.result) || progress.result;
        if (result && result.duplicateGroups) {
            displaySingleFolderSearchResults(result);
        } else {
            addSingleFolderLogEntry('ℹ️ 중복 파일이 발견되지 않았습니다.', 'info');
            const singleFolderSummary = document.getElementById('singleFolderSummary');
            if (singleFolderSummary) {
                singleFolderSummary.innerHTML = `
                    <div class="alert alert-info">
                        <i class="fas fa-info-circle"></i>
                        <strong>검색 완료</strong><br>
                        선택한 폴더에서 중복 파일이 발견되지 않았습니다.
                    </div>
                `;
            }
            showSingleFolderResultStep();
        }
        
    } catch (error) {
        console.error('검색 결과 로딩 오류:', error);
        addSingleFolderLogEntry('❌ 검색 결과를 불러오는데 실패했습니다: ' + error.message, 'error');
    }
}

function displaySingleFolderSearchResults(result) {
    const duplicateGroups = result.duplicateGroups || [];
    const totalDuplicates = duplicateGroups.reduce((sum, group) => sum + group.files.length, 0);
    const totalSize = duplicateGroups.reduce((sum, group) => sum + (group.size * group.files.length), 0);
    
    // 요약 정보 표시
    const singleFolderSummary = document.getElementById('singleFolderSummary');
    if (singleFolderSummary) {
        singleFolderSummary.innerHTML = `
            <div class="alert alert-success">
                <i class="fas fa-check-circle"></i>
                <strong>중복 파일 검색 완료</strong><br>
                ${duplicateGroups.length}개의 중복 그룹에서 총 ${totalDuplicates}개의 중복 파일을 발견했습니다.<br>
                예상 절약 공간: ${formatFileSize(totalSize - duplicateGroups.reduce((sum, group) => sum + group.size, 0))}
            </div>
        `;
    }
    
    // 중복 파일 목록 표시
    displaySingleFolderDuplicatesList(duplicateGroups);
    
    // 결과 단계로 전환
    showSingleFolderResultStep();
}

function displaySingleFolderDuplicatesList(duplicateGroups) {
    const container = document.getElementById('singleFolderDuplicatesList');
    
    if (!container) return;
    
    if (duplicateGroups.length === 0) {
        container.innerHTML = '<div class="no-results">중복 파일이 없습니다.</div>';
        return;
    }
    
    let html = '<div class="duplicate-groups">';
    
    duplicateGroups.forEach((group, groupIndex) => {
        html += `
            <div class="duplicate-group">
                <div class="group-header">
                    <h4>
                        <i class="fas fa-layer-group"></i>
                        중복 그룹 ${groupIndex + 1}
                        <span class="group-info">(${group.files.length}개 파일, ${formatFileSize(group.size)} 각각)</span>
                    </h4>
                    <label class="select-all-label">
                        <input type="checkbox" class="group-select-all" onchange="toggleGroupSelection(${groupIndex}, this.checked)">
                        모두 선택
                    </label>
                </div>
                <div class="files-list">
        `;
        
        // 파일을 생성일 기준으로 정렬 (가장 오래된 것이 원본으로 추천)
        const sortedFiles = [...group.files].sort((a, b) => {
            const aTime = new Date(a.createdTime || a.modifiedTime || 0);
            const bTime = new Date(b.createdTime || b.modifiedTime || 0);
            return aTime - bTime;
        });
        
        sortedFiles.forEach((file, fileIndex) => {
            const isOriginal = fileIndex === 0; // 가장 오래된 파일을 원본으로 간주
            const createdDate = file.createdTime ? new Date(file.createdTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';
            const modifiedDate = file.modifiedTime ? new Date(file.modifiedTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';
            
            html += `
                <div class="duplicate-file-item ${isOriginal ? 'original-file' : ''}" data-group="${groupIndex}" data-file="${file.id}">
                    <div class="file-checkbox">
                        <input type="checkbox" class="file-select"
                               data-group="${groupIndex}" data-file-id="${file.id}">
                    </div>
                    <div class="file-info">
                        <div class="file-name">
                            ${isOriginal ? '<i class="fas fa-crown original-crown" title="원본 추천"></i>' : ''}
                            ${file.name}
                        </div>
                        ${file.path ? `<div class="file-path" style="font-size: 0.8rem; color: #6366f1; margin-bottom: 2px;" title="${file.path}"><i class="fas fa-folder-open" style="margin-right: 3px;"></i>${file.path.substring(0, file.path.lastIndexOf('/')) || '/'}</div>` : ''}
                        <div class="file-details">
                            <span><i class="fas fa-calendar-plus"></i> 생성: ${createdDate}</span>
                            <span><i class="fas fa-calendar"></i> 수정: ${modifiedDate}</span>
                            <span><i class="fas fa-hdd"></i> ${formatFileSize(file.size)}</span>
                        </div>
                    </div>
                    <div class="file-actions">
                        ${file.webViewLink ? `<a href="${file.webViewLink}" target="_blank" class="btn-sm btn-primary" title="Google Drive에서 열기"><i class="fas fa-external-link-alt"></i></a>` : ''}
                    </div>
                </div>
            `;
        });
        
        html += `
                </div>
            </div>
        `;
    });
    
    html += '</div>';
    
    // 삭제 버튼 추가
    html += `
        <div class="deletion-actions">
            <button class="action-btn tertiary" onclick="deleteSelectedFiles()" style="margin-top: 20px;">
                <i class="fas fa-trash"></i>
                선택한 파일 휴지통으로 이동
            </button>
            <div style="margin-top: 10px;">
                <label>
                    <input type="checkbox" id="deleteEmptyFoldersAfterDeletion" checked>
                    이동 후 빈 폴더도 함께 정리
                </label>
            </div>
        </div>
    `;
    
    container.innerHTML = html;
}

// 그룹 내 파일 전체 선택/해제
function toggleGroupSelection(groupIndex, checked) {
    const checkboxes = document.querySelectorAll(`input[data-group="${groupIndex}"].file-select:not([disabled])`);
    checkboxes.forEach(checkbox => {
        checkbox.checked = checked;
    });
}

// 선택된 파일 삭제
async function deleteSelectedFiles() {
    const selectedCheckboxes = document.querySelectorAll('.file-select:checked');
    
    if (selectedCheckboxes.length === 0) {
        alert('휴지통으로 이동할 파일을 선택해주세요.');
        return;
    }
    
    const fileIds = Array.from(selectedCheckboxes).map(cb => cb.getAttribute('data-file-id'));
    
    if (!confirm(`선택한 ${fileIds.length}개의 파일을 휴지통으로 이동하시겠습니까?`)) {
        return;
    }
    
    try {
        // 파일 삭제 API 호출 (여기서는 단순화된 버전)
        // 실제 구현에서는 적절한 API 엔드포인트를 사용해야 함
        await window.API.Cleanup.deleteFiles(fileIds);
        
        const deleteEmptyFolders = document.getElementById('deleteEmptyFoldersAfterDeletion')?.checked || false;
        if (deleteEmptyFolders) {
            await window.API.Cleanup.cleanupEmptyFolders();
        }
        
        alert('선택한 파일이 성공적으로 삭제되었습니다.');
        
        // UI에서 삭제된 파일 제거
        fileIds.forEach(fileId => {
            const fileItem = document.querySelector(`[data-file="${fileId}"]`);
            if (fileItem) {
                fileItem.remove();
            }
        });
        
    } catch (error) {
        console.error('파일 삭제 오류:', error);
        alert('파일 삭제 중 오류가 발생했습니다: ' + error.message);
    }
}

// 단일 폴더 로그 엔트리 추가
function addSingleFolderLogEntry(message, type = 'default') {
    const logContainer = document.getElementById('singleFolderProgressLog');
    if (!logContainer) return;
    
    const logEntry = document.createElement('div');
    logEntry.className = `log-entry ${type}`;
    
    // 현재 시간을 추가
    const timeStr = formatTime();
    
    logEntry.textContent = `[${timeStr}] ${message}`;
    
    // 새 로그를 맨 위에 추가
    logContainer.insertBefore(logEntry, logContainer.firstChild);
    
    // 로그가 너무 많으면 오래된 것들 삭제 (최대 20개 유지)
    while (logContainer.children.length > 20) {
        logContainer.removeChild(logContainer.lastChild);
    }
}