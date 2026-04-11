/**
 * 중복 그룹 대량 관리 JavaScript
 * 체크박스 기반 선택, 대량 삭제, 모든 그룹 삭제 기능 구현
 */

let bulkCurrentPage = 1;
let bulkPageSize = 10;
let selectedGroups = new Set();

// API 호출 중복 방지를 위한 디바운스 변수들
let loadBulkGroupsTimeout = null;
let isLoadingBulkGroups = false;

// 메인 함수: 대량 관리 모달 표시
async function showBulkDuplicateManagement() {
    const modal = document.getElementById('bulk-duplicate-modal');
    if (!modal) {
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('대량 관리 모달을 찾을 수 없습니다.');
        } else {
            alert('대량 관리 모달을 찾을 수 없습니다.');
        }
        return;
    }
    
    // 선택 초기화
    selectedGroups.clear();
    updateSelectedCount();
    
    // 모달 표시
    modal.classList.remove('hidden');
    
    // 데이터 로드
    await loadBulkDuplicateGroups();
}

// 대량 관리 모달 닫기
function closeBulkDuplicateModal() {
    const modal = document.getElementById('bulk-duplicate-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    // 선택 초기화
    selectedGroups.clear();
    updateSelectedCount();
}

// 중복 그룹 목록 로드 (디바운스 적용)
async function loadBulkDuplicateGroups() {
    const listContainer = document.getElementById('bulk-duplicate-list');
    if (!listContainer) return;
    
    // 이미 로딩 중이면 중복 호출 방지
    if (isLoadingBulkGroups) {
        console.log('🔄 이미 로딩 중이므로 중복 호출 무시');
        return;
    }
    
    isLoadingBulkGroups = true;
    
    try {
        listContainer.innerHTML = `
            <div class="loading">
                <i class="fas fa-spinner fa-spin"></i> 중복 그룹을 불러오는 중...
            </div>
        `;
        
        const response = await window.API.Duplicate.getDuplicateGroups(bulkCurrentPage, bulkPageSize);
        
        // 응답 형식 처리
        let groups = [];
        let totalGroups = 0;
        let totalPages = 1;
        let hasNext = false;
        let hasPrev = false;
        
        if (response && response.groups && Array.isArray(response.groups)) {
            groups = response.groups;
            totalGroups = response.totalGroups || 0;
            totalPages = response.totalPages || 1;
            hasNext = response.hasNext || false;
            hasPrev = response.hasPrev || false;
        } else if (Array.isArray(response)) {
            groups = response;
            totalGroups = groups.length;
        }
        
        if (groups.length === 0) {
            listContainer.innerHTML = `
                <div class="loading">
                    <i class="fas fa-inbox" style="font-size: 2rem; color: #6b7280; margin-bottom: 16px;"></i>
                    <div>중복 그룹이 없습니다</div>
                </div>
            `;
            return;
        }
        
        // 그룹 목록 표시
        displayBulkDuplicateGroups(groups);
        updateBulkPagination(totalGroups, bulkCurrentPage, totalPages, hasNext, hasPrev);
        
    } catch (error) {
        console.error('중복 그룹 로드 실패:', error);
        listContainer.innerHTML = `
            <div class="loading">
                <i class="fas fa-exclamation-triangle" style="font-size: 2rem; color: #ef4444; margin-bottom: 16px;"></i>
                <div>중복 그룹을 불러올 수 없습니다</div>
            </div>
        `;
    } finally {
        // 로딩 상태 해제
        isLoadingBulkGroups = false;
    }
}

// 중복 그룹 목록 표시
function displayBulkDuplicateGroups(groups) {
    const listContainer = document.getElementById('bulk-duplicate-list');
    if (!listContainer) return;
    
    const html = groups.map(group => {
        const fileName = group.files && group.files.length > 0 ? group.files[0].name : '이름 없음';
        const wastedSpace = group.files && group.files.length > 0 ? 
            (group.count - 1) * group.files[0].size : 0;
        const fileTypeIcon = getFileTypeIcon(group.files && group.files.length > 0 ? 
            group.files[0].mimeType || 'application/octet-stream' : 'application/octet-stream');
        const isSelected = selectedGroups.has(group.id);
        
        return `
            <div class="bulk-duplicate-item ${isSelected ? 'selected' : ''}" id="bulk-item-${group.id}">
                <input type="checkbox" 
                       class="bulk-item-checkbox" 
                       id="checkbox-${group.id}"
                       ${isSelected ? 'checked' : ''}
                       onchange="toggleGroupSelection('${group.id}')">
                
                <div class="bulk-item-info">
                    <div class="bulk-item-details">
                        <div class="bulk-item-name">
                            <span class="file-type-icon">${fileTypeIcon}</span>
                            <span title="${fileName}">${fileName}</span>
                        </div>
                        <div class="bulk-item-stats">
                            <span><i class="fas fa-copy"></i> ${group.count || 0}개 중복</span>
                            <span><i class="fas fa-save"></i> ${formatFileSize(wastedSpace)} 절약 가능</span>
                            <span><i class="fas fa-fingerprint"></i> ${group.hash ? group.hash.substring(0, 8) + '...' : 'N/A'}</span>
                        </div>
                    </div>
                    
                    <div class="bulk-item-actions">
                        <div class="bulk-item-size">${formatFileSize(group.totalSize || 0)}</div>
                        <button class="btn-sm btn-secondary" onclick="viewGroup('${group.id}')" title="상세보기">
                            <i class="fas fa-eye"></i>
                        </button>
                        <button class="btn-sm btn-danger" onclick="deleteSingleGroup('${group.id}')" title="개별 삭제">
                            <i class="fas fa-trash"></i>
                        </button>
                    </div>
                </div>
            </div>
        `;
    }).join('');
    
    listContainer.innerHTML = html;
}

// 그룹 선택/해제
function toggleGroupSelection(groupId) {
    const checkbox = document.getElementById(`checkbox-${groupId}`);
    const item = document.getElementById(`bulk-item-${groupId}`);
    
    if (checkbox && checkbox.checked) {
        selectedGroups.add(groupId);
        item?.classList.add('selected');
    } else {
        selectedGroups.delete(groupId);
        item?.classList.remove('selected');
    }
    
    updateSelectedCount();
}

// 전체 선택
function selectAllGroups() {
    const checkboxes = document.querySelectorAll('.bulk-item-checkbox');
    checkboxes.forEach(checkbox => {
        if (!checkbox.checked) {
            checkbox.checked = true;
            const groupId = checkbox.id.replace('checkbox-', '');
            selectedGroups.add(groupId);
            document.getElementById(`bulk-item-${groupId}`)?.classList.add('selected');
        }
    });
    updateSelectedCount();
}

// 전체 해제
function unselectAllGroups() {
    const checkboxes = document.querySelectorAll('.bulk-item-checkbox');
    checkboxes.forEach(checkbox => {
        if (checkbox.checked) {
            checkbox.checked = false;
            const groupId = checkbox.id.replace('checkbox-', '');
            selectedGroups.delete(groupId);
            document.getElementById(`bulk-item-${groupId}`)?.classList.remove('selected');
        }
    });
    updateSelectedCount();
}

// 선택된 개수 업데이트
function updateSelectedCount() {
    const countElement = document.getElementById('selected-count');
    const deleteButton = document.getElementById('delete-selected-btn');
    
    if (countElement) {
        countElement.textContent = `${selectedGroups.size}개 선택됨`;
    }
    
    if (deleteButton) {
        deleteButton.disabled = selectedGroups.size === 0;
    }
}

// 선택된 그룹들 삭제
function deleteSelectedGroups() {
    // 커스텀 확인 모달 사용
    if (typeof confirmDeleteSelectedGroups === 'function') {
        confirmDeleteSelectedGroups();
    } else {
        // 폴백: 기존 confirm 사용
        deleteSelectedGroupsFallback();
    }
}

async function deleteSelectedGroupsFallback() {
    if (selectedGroups.size === 0) {
        if (typeof showWarningMessage === 'function') {
            showWarningMessage('삭제할 그룹을 선택해주세요.');
        } else {
            alert('삭제할 그룹을 선택해주세요.');
        }
        return;
    }
    
    const groupIds = Array.from(selectedGroups);
    const confirmed = confirm(`선택된 ${groupIds.length}개의 중복 그룹을 삭제하시겠습니까?\n\n이 작업은 되돌릴 수 없습니다.`);
    
    if (!confirmed) return;
    
    try {
        // 로딩 상태 표시
        const deleteButton = document.getElementById('delete-selected-btn');
        if (deleteButton) {
            deleteButton.disabled = true;
            deleteButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 삭제 중...';
        }
        
        // 각 그룹을 순차적으로 삭제
        let deletedCount = 0;
        let failedCount = 0;
        
        for (const groupId of groupIds) {
            try {
                await window.API.Duplicate.deleteDuplicateGroup(groupId);
                deletedCount++;
                
                // UI에서 해당 항목 제거
                const item = document.getElementById(`bulk-item-${groupId}`);
                if (item) {
                    item.style.transition = 'opacity 0.3s ease';
                    item.style.opacity = '0.5';
                }
                
                selectedGroups.delete(groupId);
                
            } catch (error) {
                console.error(`그룹 ${groupId} 삭제 실패:`, error);
                failedCount++;
            }
        }
        
        // 결과 알림
        let message = `${deletedCount}개의 중복 그룹이 삭제되었습니다.`;
        if (failedCount > 0) {
            message += `\n${failedCount}개의 그룹 삭제에 실패했습니다.`;
        }
        
        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage(message);
        } else {
            alert(message);
        }
        
        // 목록 새로고침
        await loadBulkDuplicateGroups();
        updateSelectedCount();
        
    } catch (error) {
        console.error('대량 삭제 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('대량 삭제 중 오류가 발생했습니다: ' + error.message);
        } else {
            alert('대량 삭제 중 오류가 발생했습니다: ' + error.message);
        }
    } finally {
        // 버튼 상태 복원
        const deleteButton = document.getElementById('delete-selected-btn');
        if (deleteButton) {
            deleteButton.innerHTML = '<i class="fas fa-trash"></i> 선택된 그룹 삭제';
            updateSelectedCount(); // 버튼 상태 재설정
        }
    }
}

// 모든 그룹 삭제
function deleteAllGroups() {
    // 커스텀 확인 모달 사용
    if (typeof confirmDeleteAllGroups === 'function') {
        confirmDeleteAllGroups();
    } else {
        // 폴백: 기존 confirm 사용
        deleteAllGroupsFallback();
    }
}

async function deleteAllGroupsFallback() {
    const confirmed = confirm('⚠️ 경고: 모든 중복 그룹을 삭제하시겠습니까?\n\n이 작업은 모든 중복 파일 정보를 영구적으로 삭제하며 되돌릴 수 없습니다.\n\n정말로 계속하시겠습니까?');
    
    if (!confirmed) return;
    
    const doubleConfirmed = confirm('마지막 확인: 정말로 모든 중복 그룹을 삭제하시겠습니까?\n\n이 작업 후에는 중복 검사를 다시 실행해야 합니다.');
    
    if (!doubleConfirmed) return;
    
    try {
        // 현재 모든 그룹 조회
        const response = await window.API.Duplicate.getDuplicateGroups(1, 1000); // 최대 1000개까지
        
        let allGroups = [];
        if (response && response.groups && Array.isArray(response.groups)) {
            allGroups = response.groups;
        } else if (Array.isArray(response)) {
            allGroups = response;
        }
        
        if (allGroups.length === 0) {
            if (typeof showWarningMessage === 'function') {
                showWarningMessage('삭제할 중복 그룹이 없습니다.');
            } else {
                alert('삭제할 중복 그룹이 없습니다.');
            }
            return;
        }
        
        // 로딩 상태 표시
        const listContainer = document.getElementById('bulk-duplicate-list');
        if (listContainer) {
            listContainer.innerHTML = `
                <div class="loading">
                    <i class="fas fa-spinner fa-spin"></i> 모든 중복 그룹을 삭제하는 중... (${allGroups.length}개)
                </div>
            `;
        }
        
        // 모든 그룹 삭제
        let deletedCount = 0;
        let failedCount = 0;
        
        for (const group of allGroups) {
            try {
                await window.API.Duplicate.deleteDuplicateGroup(group.id);
                deletedCount++;
                
                // 진행 상황 업데이트
                if (listContainer) {
                    listContainer.innerHTML = `
                        <div class="loading">
                            <i class="fas fa-spinner fa-spin"></i> 중복 그룹 삭제 중... (${deletedCount}/${allGroups.length})
                        </div>
                    `;
                }
                
            } catch (error) {
                console.error(`그룹 ${group.id} 삭제 실패:`, error);
                failedCount++;
            }
        }
        
        // 결과 알림
        let message = `${deletedCount}개의 중복 그룹이 삭제되었습니다.`;
        if (failedCount > 0) {
            message += `\n${failedCount}개의 그룹 삭제에 실패했습니다.`;
        }
        
        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage(message);
        } else {
            alert(message);
        }
        
        // 선택 초기화 및 목록 새로고침
        selectedGroups.clear();
        await loadBulkDuplicateGroups();
        updateSelectedCount();
        
    } catch (error) {
        console.error('모든 그룹 삭제 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('모든 그룹 삭제 중 오류가 발생했습니다: ' + error.message);
        } else {
            alert('모든 그룹 삭제 중 오류가 발생했습니다: ' + error.message);
        }
        await loadBulkDuplicateGroups();
    }
}

// 개별 그룹 삭제 (인라인)
function deleteSingleGroup(groupId) {
    // 커스텀 확인 모달 사용 (빠른 삭제와 동일한 로직)
    if (typeof confirmQuickDelete === 'function') {
        confirmQuickDelete(groupId);
    } else {
        // 폴백: 기존 confirm 사용
        deleteSingleGroupFallback(groupId);
    }
}

async function deleteSingleGroupFallback(groupId) {
    if (!confirm('이 중복 그룹을 삭제하시겠습니까?')) return;
    
    try {
        await window.API.Duplicate.deleteDuplicateGroup(groupId);
        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage('중복 그룹이 삭제되었습니다.');
        } else {
            alert('중복 그룹이 삭제되었습니다.');
        }
        
        // 즉시 DOM에서 제거 (시각적 피드백)
        removeBulkItemFromDOM(groupId);
        
        // 디바운스된 목록 새로고침 (서버 상태 동기화)
        debouncedLoadBulkGroups(2000); // 2초 후 새로고침
        
    } catch (error) {
        console.error('그룹 삭제 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('그룹 삭제에 실패했습니다: ' + error.message);
        } else {
            alert('그룹 삭제에 실패했습니다: ' + error.message);
        }
    }
}

// 페이지네이션 업데이트
function updateBulkPagination(totalGroups, currentPage, totalPages, hasNext, hasPrev) {
    const paginationContainer = document.getElementById('bulk-pagination');
    if (!paginationContainer) return;
    
    const startItem = (currentPage - 1) * bulkPageSize + 1;
    const endItem = Math.min(currentPage * bulkPageSize, totalGroups);
    
    paginationContainer.innerHTML = `
        <button class="btn btn-secondary" ${!hasPrev ? 'disabled' : ''} onclick="changeBulkPage(${currentPage - 1})">
            <i class="fas fa-chevron-left"></i> 이전
        </button>
        
        <div class="pagination-info">
            ${startItem}-${endItem} / ${totalGroups}개 그룹 (${currentPage}/${totalPages} 페이지)
        </div>
        
        <button class="btn btn-secondary" ${!hasNext ? 'disabled' : ''} onclick="changeBulkPage(${currentPage + 1})">
            다음 <i class="fas fa-chevron-right"></i>
        </button>
    `;
}

// 페이지 변경
async function changeBulkPage(page) {
    bulkCurrentPage = page;
    await loadBulkDuplicateGroups();
}

// 목록 새로고침
async function refreshBulkList() {
    selectedGroups.clear();
    updateSelectedCount();
    await loadBulkDuplicateGroups();
}

// DOM에서 특정 그룹 항목 제거 (즉시 시각적 피드백용)
function removeBulkItemFromDOM(groupId) {
    const item = document.getElementById(`bulk-item-${groupId}`);
    if (item) {
        // 부드러운 애니메이션으로 제거
        item.style.transition = 'opacity 0.5s ease, transform 0.5s ease';
        item.style.opacity = '0';
        item.style.transform = 'translateX(-20px)';
        
        // 애니메이션 완료 후 DOM에서 제거
        setTimeout(() => {
            if (item.parentNode) {
                item.parentNode.removeChild(item);
            }
        }, 500);
        
        // 선택된 그룹에서도 제거
        selectedGroups.delete(groupId);
        updateSelectedCount();
        
        console.log(`✅ DOM에서 그룹 ${groupId} 항목 제거됨 (즉시 피드백)`);
    }
}

// 디바운스된 목록 새로고침 (중복 호출 방지)
function debouncedLoadBulkGroups(delay = 1000) {
    // 기존 타이머 취소
    if (loadBulkGroupsTimeout) {
        clearTimeout(loadBulkGroupsTimeout);
    }
    
    // 새 타이머 설정
    loadBulkGroupsTimeout = setTimeout(async () => {
        console.log('🔄 디바운스된 목록 새로고침 실행');
        await loadBulkDuplicateGroups();
    }, delay);
    
    console.log(`⏰ 목록 새로고침 ${delay}ms 후 예약됨`);
}

// ESC 키 이벤트 (기존 이벤트에 추가)
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        const bulkModal = document.getElementById('bulk-duplicate-modal');
        if (bulkModal && !bulkModal.classList.contains('hidden')) {
            closeBulkDuplicateModal();
            return;
        }
    }
});