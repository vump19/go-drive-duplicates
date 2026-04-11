/**
 * 삭제 확인 모달 JavaScript
 * confirm() 대신 사용하는 커스텀 확인 모달 시스템
 */

let confirmationCallback = null;
let confirmationData = null;

// 확인 모달 표시
// showDeleteMethodOptions: true면 휴지통/완전삭제 옵션 표시 (Google Drive 파일 삭제 시)
//                          false면 옵션 숨김 (DB 레코드 삭제 시)
function showDeleteConfirmation(message, onConfirm, data = null, showDeleteMethodOptions = false) {
    const modal = document.getElementById('delete-confirmation-modal');
    const messageElement = document.getElementById('delete-confirmation-content');
    const confirmButton = document.getElementById('confirm-delete-btn');
    const deleteMethodOptions = document.getElementById('delete-method-options');

    if (!modal || !messageElement || !confirmButton) {
        console.error('확인 모달 요소를 찾을 수 없습니다');
        return;
    }

    // 메시지 설정
    messageElement.innerHTML = message;

    // 콜백과 데이터 저장
    confirmationCallback = onConfirm;
    confirmationData = data;

    // 삭제 방식 옵션 표시/숨김
    if (deleteMethodOptions) {
        if (showDeleteMethodOptions) {
            deleteMethodOptions.style.display = 'block';
            // 휴지통 이동을 기본값으로
            const trashRadio = deleteMethodOptions.querySelector('input[value="trash"]');
            if (trashRadio) {
                trashRadio.checked = true;
            }
        } else {
            deleteMethodOptions.style.display = 'none';
        }
    }

    // 버튼 상태 초기화
    confirmButton.disabled = false;
    confirmButton.innerHTML = '<i class="fas fa-trash"></i> 삭제';

    // 모달 표시
    modal.classList.remove('hidden');
}

// 확인 모달 닫기
function closeDeleteConfirmationModal() {
    const modal = document.getElementById('delete-confirmation-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    
    // 콜백과 데이터 초기화
    confirmationCallback = null;
    confirmationData = null;
}

// 선택된 삭제 방식 가져오기
function getSelectedDeleteMethod() {
    const deleteMethodOptions = document.getElementById('delete-method-options');
    if (deleteMethodOptions) {
        const selectedRadio = deleteMethodOptions.querySelector('input[name="deleteMethod"]:checked');
        if (selectedRadio) {
            return selectedRadio.value === 'permanent';
        }
    }
    return false; // 기본값: 휴지통 이동 (permanentDelete = false)
}

// 확인된 삭제 실행
async function executeDelete() {
    if (!confirmationCallback) return;

    const confirmButton = document.getElementById('confirm-delete-btn');
    const permanentDelete = getSelectedDeleteMethod();

    try {
        // 버튼 비활성화 및 로딩 표시
        if (confirmButton) {
            confirmButton.disabled = true;
            const actionText = permanentDelete ? '삭제 중...' : '휴지통으로 이동 중...';
            confirmButton.innerHTML = `<i class="fas fa-spinner fa-spin"></i> ${actionText}`;
        }

        // confirmationData에 permanentDelete 옵션 추가
        const dataWithDeleteMethod = {
            ...confirmationData,
            permanentDelete: permanentDelete
        };

        // 콜백 실행 (삭제 방식 정보 포함)
        await confirmationCallback(dataWithDeleteMethod);

        // 모달 닫기
        closeDeleteConfirmationModal();

    } catch (error) {
        console.error('삭제 실행 중 오류:', error);

        // 버튼 상태 복원
        if (confirmButton) {
            confirmButton.disabled = false;
            confirmButton.innerHTML = '<i class="fas fa-trash"></i> 삭제';
        }

        // 에러 메시지 표시
        showErrorMessage('삭제 중 오류가 발생했습니다: ' + error.message, '삭제 실패');
    }
}

// ESC 키로 모달 닫기 (기존 이벤트에 추가)
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        // 알림 모달이 열려있으면 먼저 닫기
        const notificationModal = document.getElementById('result-notification-modal');
        if (notificationModal && !notificationModal.classList.contains('hidden')) {
            closeResultNotificationModal();
            return;
        }
        
        // 확인 모달이 열려있으면 닫기
        const confirmModal = document.getElementById('delete-confirmation-modal');
        if (confirmModal && !confirmModal.classList.contains('hidden')) {
            closeDeleteConfirmationModal();
            return;
        }
    }
});

// 그룹 삭제를 위한 헬퍼 함수들
function confirmDeleteCurrentGroup() {
    if (!currentGroupId) return;
    
    const message = `
        <div style="margin-bottom: 16px;">
            <strong>중복 그룹을 삭제하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.95rem; color: #6b7280;">
            그룹 ID: <code style="background: #f3f4f6; padding: 2px 6px; border-radius: 3px;">${currentGroupId}</code>
        </div>
    `;
    
    showDeleteConfirmation(message, async () => {
        await deleteCurrentGroupDirectly();
    });
}

async function deleteCurrentGroupDirectly() {
    if (!currentGroupId) return;
    
    try {
        await window.API.Duplicate.deleteDuplicateGroup(currentGroupId);
        
        // 성공 알림 (대시보드 새로고침 플래그 설정)
        showSuccessMessage('✅ 중복 그룹이 성공적으로 삭제되었습니다.');
        const modal = document.getElementById('result-notification-modal');
        if (modal) {
            modal.dataset.refreshDashboard = 'true';
        }
        
        // 그룹 상세 모달 닫기
        closeGroupDetailsModal();
        
        // 최적화된 업데이트: 즉시 DOM 제거 후 지연된 목록 새로고침
        if (typeof refreshDuplicates === 'function') {
            await refreshDuplicates();
        } else if (typeof loadBulkDuplicateGroups === 'function') {
            // 즉시 DOM에서 해당 항목 제거 (시각적 피드백)
            removeBulkItemFromDOM(currentGroupId);
            
            // 디바운스된 목록 새로고침 (서버 상태와 동기화)
            if (typeof debouncedLoadBulkGroups === 'function') {
                debouncedLoadBulkGroups(3000); // 3초 후 새로고침
            }
        } else {
            // 대시보드에서 삭제된 그룹 선택 상태 제거
            if (typeof selectedRecentGroups !== 'undefined') {
                selectedRecentGroups.delete(currentGroupId);
                console.log('🗑️ 삭제된 그룹 선택 상태 제거:', currentGroupId);
            }
            
            // 대시보드 페이지에서 상태 카드와 최근 중복 파일 목록 업데이트
            if (typeof updateStatusCards === 'function') {
                setTimeout(() => {
                    updateStatusCards();
                }, 2000); // 2초 후 업데이트
            }
            
            // 최근 발견된 중복 파일 목록 즉시 갱신
            if (typeof updateRecentDuplicates === 'function') {
                await updateRecentDuplicates(true);
            }
        }
        
    } catch (error) {
        console.error('그룹 삭제 실패:', error);
        throw error; // 상위로 에러 전달
    }
}

// 빠른 삭제 확인
function confirmQuickDelete(groupId) {
    const message = `
        <div style="margin-bottom: 16px;">
            <strong>이 중복 그룹을 삭제하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.95rem; color: #6b7280;">
            그룹 ID: <code style="background: #f3f4f6; padding: 2px 6px; border-radius: 3px;">${groupId}</code>
        </div>
    `;
    
    showDeleteConfirmation(message, async () => {
        await quickDeleteDirectly(groupId);
    });
}

async function quickDeleteDirectly(groupId) {
    try {
        await window.API.Duplicate.deleteDuplicateGroup(groupId);
        
        // 성공 알림 (대시보드 새로고침 플래그 설정)
        showSuccessMessage('✅ 중복 그룹이 성공적으로 삭제되었습니다.');
        const modal = document.getElementById('result-notification-modal');
        if (modal) {
            modal.dataset.refreshDashboard = 'true';
        }
        
        // 최적화된 업데이트: DOM 업데이트 우선, API 호출 최소화 (디바운스 적용)
        if (typeof updateStatusCards === 'function') {
            // 디바운스된 업데이트 (대시보드 페이지에서만)
            setTimeout(() => {
                updateStatusCards();
            }, 1000); // 1초 후 업데이트
        }
        
        // 대량 관리 모달이 열려있는 경우에만 DOM 업데이트
        if (typeof removeBulkItemFromDOM === 'function') {
            removeBulkItemFromDOM(groupId);
        }
        
        // 선택 상태에서 제거 (dashboard.js에서 사용하는 경우)
        if (typeof selectedRecentGroups !== 'undefined') {
            selectedRecentGroups.delete(groupId);
        }
        if (typeof updateRecentSelectionUI === 'function') {
            updateRecentSelectionUI();
        }
        
        // 최근 중복 목록 즉시 업데이트 (강제)
        if (typeof updateRecentDuplicates === 'function') {
            console.log(`🔄 그룹 ${groupId} 삭제 후 최근 중복 목록 강제 새로고침`);
            await updateRecentDuplicates(true);
            console.log('✅ 최근 중복 목록 강제 새로고침 완료');
        }
        
    } catch (error) {
        console.error('빠른 삭제 실패:', error);
        
        // 404나 500 오류인 경우 이미 삭제된 것으로 간주
        if (error.message && (error.message.includes('404') || error.message.includes('500') || error.message.includes('찾을 수 없습니다'))) {
            console.log(`그룹 ${groupId}는 이미 삭제된 것으로 간주`);
            
            if (typeof showSuccessMessage === 'function') {
                showSuccessMessage('✅ 중복 그룹이 이미 삭제되었습니다.');
                const modal = document.getElementById('result-notification-modal');
                if (modal) {
                    modal.dataset.refreshDashboard = 'true';
                }
            } else {
                alert('✅ 중복 그룹이 이미 삭제되었습니다.');
            }
            
            // 선택 상태에서 제거 (dashboard.js에서 사용하는 경우)
            if (typeof selectedRecentGroups !== 'undefined') {
                selectedRecentGroups.delete(groupId);
            }
            if (typeof updateRecentSelectionUI === 'function') {
                updateRecentSelectionUI();
            }
            
            // 목록 새로고침 (강제)
            if (typeof updateRecentDuplicates === 'function') {
                await updateRecentDuplicates(true);
            }
            if (typeof updateStatusCards === 'function') {
                await updateStatusCards();
            }
            
            return; // 성공으로 처리
        } else {
            // 실제 오류인 경우
            throw error;
        }
    }
}

// 선택된 그룹들 삭제 확인
function confirmDeleteSelectedGroups() {
    if (!selectedGroups || selectedGroups.size === 0) {
        alert('삭제할 그룹을 선택해주세요.');
        return;
    }
    
    const groupIds = Array.from(selectedGroups);
    const message = `
        <div style="margin-bottom: 16px;">
            <strong>선택된 ${groupIds.length}개의 중복 그룹을 삭제하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; max-height: 150px; overflow-y: auto; background: #f8f9fa; padding: 8px; border-radius: 4px;">
            ${groupIds.map(id => `• 그룹 ID: ${id}`).join('<br>')}
        </div>
    `;
    
    showDeleteConfirmation(message, async () => {
        await deleteSelectedGroupsDirectly();
    });
}

async function deleteSelectedGroupsDirectly() {
    if (!selectedGroups || selectedGroups.size === 0) return;
    
    const groupIds = Array.from(selectedGroups);
    let deletedCount = 0;
    let failedCount = 0;
    
    for (const groupId of groupIds) {
        try {
            await window.API.Duplicate.deleteDuplicateGroup(groupId);
            deletedCount++;
            selectedGroups.delete(groupId);
            
            // 즉시 DOM에서 해당 항목 제거 (시각적 피드백)
            if (typeof removeBulkItemFromDOM === 'function') {
                removeBulkItemFromDOM(groupId);
            }
            
        } catch (error) {
            console.error(`그룹 ${groupId} 삭제 실패:`, error);
            failedCount++;
        }
    }
    
    // 결과 메시지
    let message = `✅ ${deletedCount}개의 중복 그룹이 삭제되었습니다.`;
    if (failedCount > 0) {
        message += `\n❌ ${failedCount}개의 그룹 삭제에 실패했습니다.`;
    }
    
    showSuccessMessage(message);
    
    // UI 즉시 업데이트 (API 호출 없이)
    if (typeof updateSelectedCount === 'function') {
        updateSelectedCount();
    }
    
    // 디바운스된 목록 새로고침 (대량 삭제 후 서버 상태 동기화)
    if (typeof debouncedLoadBulkGroups === 'function') {
        debouncedLoadBulkGroups(3000); // 3초 후 새로고침 (대량 작업이므로 좀 더 여유있게)
    }
}

// 모든 그룹 삭제 확인
function confirmDeleteAllGroups() {
    const message = `
        <div style="margin-bottom: 16px;">
            <strong style="color: #ef4444;">⚠️ 경고: 모든 중복 그룹을 삭제하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; line-height: 1.5;">
            이 작업은 모든 중복 파일 정보를 영구적으로 삭제합니다.<br>
            삭제 후에는 중복 검사를 다시 실행해야 합니다.
        </div>
    `;
    
    showDeleteConfirmation(message, async () => {
        // 두 번째 확인
        const doubleConfirmMessage = `
            <div style="margin-bottom: 16px;">
                <strong style="color: #dc2626;">마지막 확인</strong>
            </div>
            <div style="font-size: 0.9rem; color: #6b7280;">
                정말로 모든 중복 그룹을 삭제하시겠습니까?
            </div>
        `;
        
        showDeleteConfirmation(doubleConfirmMessage, async () => {
            await deleteAllGroupsDirectly();
        });
    });
}

async function deleteAllGroupsDirectly() {
    try {
        // 현재 모든 그룹 조회
        const response = await window.API.Duplicate.getDuplicateGroups(1, 1000);
        
        let allGroups = [];
        if (response && response.groups && Array.isArray(response.groups)) {
            allGroups = response.groups;
        } else if (Array.isArray(response)) {
            allGroups = response;
        }
        
        if (allGroups.length === 0) {
            showSuccessMessage('삭제할 중복 그룹이 없습니다.');
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
        
        // 결과 메시지
        let message = `✅ ${deletedCount}개의 중복 그룹이 삭제되었습니다.`;
        if (failedCount > 0) {
            message += `\n❌ ${failedCount}개의 그룹 삭제에 실패했습니다.`;
        }
        
        showSuccessMessage(message);
        
        // 선택 초기화 및 UI 업데이트
        if (typeof selectedGroups !== 'undefined') {
            selectedGroups.clear();
        }
        if (typeof updateSelectedCount === 'function') {
            updateSelectedCount();
        }
        
        // 모든 그룹 삭제 후에는 목록 즉시 새로고침 (전체 삭제이므로)
        if (typeof loadBulkDuplicateGroups === 'function') {
            await loadBulkDuplicateGroups();
        }
        
    } catch (error) {
        console.error('모든 그룹 삭제 실패:', error);
        throw error;
    }
}

// 결과 알림 모달 표시
function showResultNotification(message, type = 'success', title = null) {
    const modal = document.getElementById('result-notification-modal');
    const titleElement = document.getElementById('notification-title');
    const titleTextElement = document.getElementById('notification-title-text');
    const iconElement = document.getElementById('notification-icon');
    const messageElement = document.getElementById('notification-message');
    
    if (!modal || !titleElement || !titleTextElement || !iconElement || !messageElement) {
        console.error('알림 모달 요소를 찾을 수 없습니다');
        // 폴백: 기존 alert 사용
        alert(message);
        return;
    }
    
    // 타입별 설정
    titleElement.className = type; // success, error, warning 클래스 적용
    messageElement.className = `notification-message ${type}`;
    
    switch (type) {
        case 'success':
            iconElement.className = 'fas fa-check-circle';
            titleTextElement.textContent = title || '성공';
            break;
        case 'error':
            iconElement.className = 'fas fa-exclamation-circle';
            titleTextElement.textContent = title || '오류';
            break;
        case 'warning':
            iconElement.className = 'fas fa-exclamation-triangle';
            titleTextElement.textContent = title || '경고';
            break;
        default:
            iconElement.className = 'fas fa-info-circle';
            titleTextElement.textContent = title || '알림';
    }
    
    // 메시지 설정
    messageElement.innerHTML = message;
    
    // 모달 표시
    modal.classList.remove('hidden');
}

// 결과 알림 모달 닫기
function closeResultNotificationModal() {
    const modal = document.getElementById('result-notification-modal');
    if (modal) {
        modal.classList.add('hidden');
        
        // 삭제 관련 성공 메시지였다면 대시보드 새로고침
        if (modal.dataset.refreshDashboard === 'true') {
            console.log('🔄 삭제 완료 후 대시보드 새로고침');
            
            // 대시보드의 최근 중복 파일 목록 강제 새로고침
            if (typeof updateRecentDuplicates === 'function') {
                updateRecentDuplicates(true);
            }
            
            // 데이터 속성 초기화
            delete modal.dataset.refreshDashboard;
        }
    }
}

// 성공 메시지 표시 (기존 함수 대체)
function showSuccessMessage(message, title = null) {
    showResultNotification(message, 'success', title);
}

// 에러 메시지 표시
function showErrorMessage(message, title = null) {
    showResultNotification(message, 'error', title);
}

// 경고 메시지 표시
function showWarningMessage(message, title = null) {
    showResultNotification(message, 'warning', title);
}