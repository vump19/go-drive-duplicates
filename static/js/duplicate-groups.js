/**
 * 중복 그룹 관련 JavaScript
 * 중복 그룹 상세보기, 파일 경로 조회, 모달 관리 기능들
 */

let currentGroupId = null;

// 중복 그룹 상세보기
async function viewGroup(groupId) {
    try {
        const response = await window.API.Duplicate.getDuplicateGroup(groupId);
        if (response) {
            currentGroupId = groupId;
            displayGroupDetailsModal(response);
        }
    } catch (error) {
        console.error('그룹 상세 정보 조회 실패:', error);
        
        // 404 오류 (그룹이 삭제됨)인 경우 자동으로 목록 새로고침
        if (error.message && error.message.includes('404')) {
            showSuccessMessage('✅ 해당 중복 그룹이 이미 삭제되었습니다. 목록을 새로고침합니다.');
            
            // 대시보드의 최근 중복 파일 목록 강제 새로고침
            if (typeof updateRecentDuplicates === 'function') {
                await updateRecentDuplicates(true);
            }
            
            // 선택 상태에서 제거
            if (typeof selectedRecentGroups !== 'undefined') {
                selectedRecentGroups.delete(groupId);
            }
            
            return;
        }
        
        alert('그룹 상세 정보를 불러올 수 없습니다: ' + error.message);
    }
}

// 그룹 상세보기 모달 표시
function displayGroupDetailsModal(group) {
    const modal = document.getElementById('group-details-modal');
    const content = document.getElementById('group-details-content');

    if (!modal || !content) {
        console.error('모달 요소를 찾을 수 없습니다');
        return;
    }

    // 현재 그룹 데이터 저장 (검증 기능을 위해)
    currentGroupData = group;
    allPathsVisible = false;

    // 그룹 요약 정보 생성
    const wastedSpace = group.files && group.files.length > 0 ?
        (group.count - 1) * group.files[0].size : 0;

    const totalSize = group.totalSize || (group.files && group.files.length > 0 ?
        group.files[0].size * group.count : 0);

    // 검증 상태 표시 부분
    const verificationBadge = group.verificationStatus === 'verified' ? `
        <div style="background: #d1fae5; border: 1px solid #10b981; border-radius: 8px; padding: 12px; margin-bottom: 16px; display: flex; align-items: center; gap: 8px;">
            <i class="fas fa-check-circle" style="color: #10b981; font-size: 1.2rem;"></i>
            <div>
                <div style="font-weight: 600; color: #065f46;">✅ 검증된 중복 그룹</div>
                <div style="font-size: 0.875rem; color: #047857; margin-top: 2px;">
                    ${group.verificationDescription || '사용자가 검증된 중복으로 표시'}
                    ${group.verificationDate ? ` • ${new Date(group.verificationDate).toLocaleDateString('ko-KR')}` : ''}
                </div>
            </div>
        </div>
    ` : '';

    let html = `
        ${verificationBadge}
        <div class="group-info">
            <div class="group-summary">
                <div class="group-summary-grid">
                    <div class="summary-item">
                        <div class="summary-value">${group.count || 0}</div>
                        <div class="summary-label">중복 파일 수</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value">${formatFileSize(totalSize)}</div>
                        <div class="summary-label">총 크기</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value" style="color: #ef4444;">${formatFileSize(wastedSpace)}</div>
                        <div class="summary-label">절약 가능 용량</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value" style="font-size: 0.9rem; font-family: monospace;">${group.hash ? group.hash.substring(0, 16) + '...' : 'N/A'}</div>
                        <div class="summary-label">파일 해시</div>
                    </div>
                </div>
            </div>
        </div>

        <div class="group-memo-section" style="margin-bottom: 16px;">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;">
                <h3 style="font-size: 1rem; font-weight: 600; color: #1f2937; margin: 0;">
                    <i class="fas fa-sticky-note" style="color: #f59e0b;"></i> 메모
                </h3>
                <div id="memo-actions">
                    <button class="btn-sm btn-secondary" onclick="toggleMemoEdit()" id="memo-edit-btn" title="메모 편집">
                        <i class="fas fa-pen"></i> 편집
                    </button>
                </div>
            </div>
            <div id="memo-display" style="${group.memo ? '' : 'display:none;'}">
                <div style="background: #fffbeb; border: 1px solid #fcd34d; border-radius: 8px; padding: 12px; color: #92400e; font-size: 0.9rem; white-space: pre-wrap;">${group.memo || ''}</div>
            </div>
            <div id="memo-empty" style="${group.memo ? 'display:none;' : ''}">
                <div style="color: #9ca3af; font-size: 0.875rem; cursor: pointer;" onclick="toggleMemoEdit()">
                    <i class="fas fa-plus-circle"></i> 메모를 추가하려면 클릭하세요
                </div>
            </div>
            <div id="memo-edit" style="display:none;">
                <textarea id="memo-textarea" rows="3" style="width: 100%; border: 1px solid #d1d5db; border-radius: 8px; padding: 10px; font-size: 0.9rem; resize: vertical; font-family: inherit;" placeholder="처리가 어려운 이유나 참고할 내용을 메모하세요...">${group.memo || ''}</textarea>
                <div style="display: flex; gap: 8px; margin-top: 8px; justify-content: flex-end;">
                    <button class="btn-sm btn-secondary" onclick="cancelMemoEdit()">취소</button>
                    <button class="btn-sm btn-primary" onclick="saveMemo()"><i class="fas fa-save"></i> 저장</button>
                </div>
            </div>
        </div>

        <div class="files-list">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px;">
                <h3 style="font-size: 1.125rem; font-weight: 600; color: #1f2937; margin: 0;">
                    <i class="fas fa-list"></i> 중복 파일 목록 (${group.files ? group.files.length : 0}개)
                </h3>
                <button class="btn-sm btn-secondary" onclick="toggleAllFilePaths()" id="toggle-all-paths-btn" title="전체 경로 한번에 보기">
                    <i class="fas fa-map-marked-alt"></i> 전체 경로 보기
                </button>
            </div>
    `;

    if (group.files && group.files.length > 0) {
        group.files.forEach((file, index) => {
            const fileTypeIcon = getFileTypeIcon(file.mimeType || 'application/octet-stream');
            const modifiedDate = file.modifiedTime ? new Date(file.modifiedTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';

            html += `
                <div class="file-item-modal" id="file-item-${file.id}">
                    <div class="file-name-row">
                        <div class="file-name-left">
                            <span class="file-type-icon">${fileTypeIcon}</span>
                            <span class="file-name-text" title="${file.name || '알 수 없는 파일'}">${file.name || '알 수 없는 파일'}</span>
                            <span class="file-exists-status" id="file-exists-${file.id}">
                                <i class="fas fa-spinner fa-pulse" style="color: #6b7280; font-size: 0.75rem;"></i>
                            </span>
                        </div>
                        <div class="file-name-right">
                            <span class="file-size-modal">${formatFileSize(file.size || 0)}</span>
                            <div class="file-actions-modal" id="file-actions-${file.id}">
                                <button class="btn-icon-only btn-danger-sm" onclick="trashFileFromModal('${file.id}', '${(file.name || '').replace(/'/g, "\\'")}')" title="휴지통으로 이동" id="trash-btn-${file.id}">
                                    <i class="fas fa-trash-alt"></i>
                                </button>
                                <button class="btn-icon-only btn-secondary-sm" onclick="toggleFilePath('${file.id}')" title="전체 경로 보기" id="path-btn-${file.id}">
                                    <i class="fas fa-map-marker-alt"></i>
                                </button>
                                ${file.parents && file.parents.length > 0 ? `
                                    <a href="explorer.html?folderId=${file.parents[0]}&fileId=${file.id}" class="btn-icon-only btn-explorer" title="Explorer에서 보기">
                                        <i class="fas fa-folder-tree"></i>
                                    </a>
                                    <a href="https://drive.google.com/drive/folders/${file.parents[0]}" target="_blank" class="btn-icon-only btn-drive-folder" title="Google Drive에서 폴더 열기">
                                        <i class="fas fa-folder-open"></i>
                                    </a>
                                ` : ''}
                                ${file.webViewLink ? `
                                    <a href="${file.webViewLink}" target="_blank" class="btn-icon-only btn-drive-file" title="Google Drive에서 파일 열기">
                                        <i class="fas fa-file"></i>
                                    </a>
                                ` : ''}
                            </div>
                        </div>
                    </div>
                    <div class="file-details-modal" id="file-details-${file.id}">
                        <span><i class="fas fa-calendar"></i> ${modifiedDate}</span>
                        <span><i class="fas fa-tag"></i> ${file.mimeType || 'application/octet-stream'}</span>
                        <span class="file-folder-info" id="file-folder-${file.id}">
                            <i class="fas fa-spinner fa-pulse"></i> 폴더 로딩 중...
                        </span>
                    </div>
                    <div class="file-path-display" id="file-path-${file.id}" style="display: none;">
                        <!-- 경로 정보가 여기에 표시됩니다 -->
                    </div>
                </div>
            `;
        });
    } else {
        html += '<p style="text-align: center; color: #6b7280; padding: 20px;">파일 정보를 불러올 수 없습니다.</p>';
    }

    html += '</div>';

    content.innerHTML = html;
    modal.classList.remove('hidden');

    // 각 파일의 폴더 정보를 자동으로 로드
    if (group.files && group.files.length > 0) {
        loadAllFolderNames(group.files);
        // 파일 존재 여부 확인 (Google Drive에서)
        checkFilesExistence(group.files);
    }

    // 검증 상태 확인 및 표시 (비동기)
    if (typeof updateGroupVerificationStatus === 'function') {
        updateGroupVerificationStatus(group);
    }
}

// 모든 파일의 폴더 이름을 자동으로 로드
async function loadAllFolderNames(files) {
    for (const file of files) {
        await loadFileFolderName(file.id, file);
    }
}

// 단일 파일의 폴더 이름 로드
async function loadFileFolderName(fileId, file) {
    const folderElement = document.getElementById(`file-folder-${fileId}`);
    if (!folderElement) return;

    try {
        // 파일 경로 API 호출
        const pathInfo = await window.API.Duplicate.getFilePath(fileId);

        if (pathInfo && pathInfo.path) {
            // 전체 경로에서 폴더 경로만 추출 (파일명 제외)
            const fullPath = pathInfo.path;
            const fileName = file.name || pathInfo.name;
            let folderPath = fullPath;

            // 파일명이 경로 끝에 있다면 제거
            if (fullPath.endsWith('/' + fileName)) {
                folderPath = fullPath.substring(0, fullPath.length - fileName.length - 1);
            }

            // 폴더 경로가 비어있으면 루트
            if (!folderPath || folderPath === '') {
                folderPath = '/';
            }

            // 폴더 경로 표시 (최대 40자로 제한)
            const displayPath = folderPath.length > 40
                ? '...' + folderPath.substring(folderPath.length - 37)
                : folderPath;

            folderElement.innerHTML = `<i class="fas fa-folder"></i> ${displayPath}`;
            folderElement.title = folderPath; // 전체 경로는 툴팁으로
        } else {
            folderElement.innerHTML = '<i class="fas fa-folder"></i> 폴더 정보 없음';
        }
    } catch (error) {
        console.error(`파일 ${fileId}의 폴더 정보 로드 실패:`, error);
        folderElement.innerHTML = '<i class="fas fa-folder"></i> 폴더 정보 없음';
    }
}

// 파일 존재 여부 확인 (Google Drive에서)
async function checkFilesExistence(files) {
    if (!files || files.length === 0) return;

    try {
        const fileIds = files.map(f => f.id);
        const response = await window.API.File.checkFilesExist(fileIds);

        if (response && response.results) {
            for (const file of files) {
                const exists = response.results[file.id];
                updateFileExistsStatus(file.id, exists);
            }
        }
    } catch (error) {
        console.error('파일 존재 여부 확인 실패:', error);
        // 오류 발생 시 모든 파일의 상태를 "확인 실패"로 표시
        for (const file of files) {
            updateFileExistsStatus(file.id, null);
        }
    }
}

// 파일 존재 상태 UI 업데이트
function updateFileExistsStatus(fileId, exists) {
    const statusElement = document.getElementById(`file-exists-${fileId}`);
    const fileItem = document.getElementById(`file-item-${fileId}`);
    const fileActions = document.getElementById(`file-actions-${fileId}`);

    if (!statusElement) return;

    if (exists === true) {
        // 파일 존재 - 체크 표시만
        statusElement.innerHTML = '<i class="fas fa-check-circle" style="color: #10b981; font-size: 0.85rem;" title="파일이 Google Drive에 존재합니다"></i>';
    } else if (exists === false) {
        // 파일 삭제됨 - 빨간색 삭제 표시
        statusElement.innerHTML = '<span class="deleted-badge" style="background: #fee2e2; color: #dc2626; padding: 2px 8px; border-radius: 4px; font-size: 0.75rem; font-weight: 600;"><i class="fas fa-trash-alt"></i> 삭제됨</span>';

        // 파일 아이템 스타일 변경
        if (fileItem) {
            fileItem.style.opacity = '0.6';
            fileItem.style.background = '#fef2f2';
        }

        // 파일 액션 버튼들 비활성화
        if (fileActions) {
            const actionButtons = fileActions.querySelectorAll('.btn-icon-only, .file-action-btn');
            actionButtons.forEach(btn => {
                btn.style.pointerEvents = 'none';
                btn.style.opacity = '0.4';
                btn.title = '삭제된 파일입니다';
            });
        }
    } else {
        // 확인 실패
        statusElement.innerHTML = '<i class="fas fa-question-circle" style="color: #f59e0b; font-size: 0.85rem;" title="파일 존재 여부를 확인할 수 없습니다"></i>';
    }
}

// 그룹 상세보기 모달 닫기
function closeGroupDetailsModal() {
    const modal = document.getElementById('group-details-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    currentGroupId = null;
    currentGroupData = null; // 검증 데이터도 초기화
}

// 현재 그룹 삭제 (커스텀 모달 사용)
function deleteCurrentGroup() {
    if (!currentGroupId) return;
    
    // 커스텀 확인 모달 사용
    if (typeof confirmDeleteCurrentGroup === 'function') {
        confirmDeleteCurrentGroup();
    } else {
        // 폴백: 기존 confirm 사용
        deleteCurrentGroupFallback();
    }
}

// 폴백 함수 (기존 로직)
async function deleteCurrentGroupFallback() {
    if (!currentGroupId) return;
    
    if (!confirm(`이 중복 그룹(ID: ${currentGroupId})을 삭제하시겠습니까?\n\n⚠️ 이 작업은 되돌릴 수 없습니다.`)) {
        return;
    }
    
    try {
        // 삭제 버튼 비활성화 및 로딩 표시
        const deleteButton = document.getElementById('delete-group-btn');
        if (deleteButton) {
            deleteButton.disabled = true;
            deleteButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 삭제 중...';
        }
        
        await window.API.Duplicate.deleteDuplicateGroup(currentGroupId);
        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage('중복 그룹이 성공적으로 삭제되었습니다!');
        } else {
            alert('✅ 중복 그룹이 성공적으로 삭제되었습니다.');
        }
        closeGroupDetailsModal();
        
        // 페이지에 대시보드 새로고침 함수가 있다면 호출, 없으면 페이지 새로고침
        if (typeof updateRecentDuplicates === 'function') {
            await updateRecentDuplicates();
        }
        if (typeof updateStatusCards === 'function') {
            await updateStatusCards();
        }
        if (typeof refreshDuplicates === 'function') {
            await refreshDuplicates();
        } else {
            location.reload(); // 마지막 대안
        }
        
    } catch (error) {
        console.error('그룹 삭제 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('그룹 삭제 실패: ' + error.message);
        } else {
            alert('❌ 그룹 삭제 실패: ' + error.message);
        }
        
        // 버튼 상태 복원
        const deleteButton = document.getElementById('delete-group-btn');
        if (deleteButton) {
            deleteButton.disabled = false;
            deleteButton.innerHTML = '그룹 삭제';
        }
    }
}

// 파일 경로 토글
async function toggleFilePath(fileId) {
    const pathDisplay = document.getElementById(`file-path-${fileId}`);
    const pathButton = document.getElementById(`path-btn-${fileId}`);
    
    if (!pathDisplay || !pathButton) return;
    
    if (pathDisplay.style.display === 'none' || pathDisplay.style.display === '') {
        // 경로가 숨겨져 있는 경우 - 로드하고 표시
        try {
            pathButton.disabled = true;
            pathButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 로딩중';
            
            const response = await window.API.Duplicate.getFilePath(fileId);
            if (response) {
                const pathInfo = `
                    <div style="margin-bottom: 8px;"><strong><i class="fas fa-folder-open"></i> 전체 경로:</strong></div>
                    <div style="background: #ffffff; padding: 6px 8px; border-radius: 3px; border-left: 3px solid #007bff; font-family: monospace; font-size: 0.9rem;">
                        ${response.path || '경로 정보 없음'}
                    </div>
                    ${response.parentId ? `<div style="margin-top: 6px; font-size: 0.75rem; color: #6c757d;"><i class="fas fa-info-circle"></i> 부모 폴더 ID: ${response.parentId}</div>` : ''}
                `;
                
                pathDisplay.innerHTML = pathInfo;
                pathDisplay.style.display = 'block';
                pathButton.innerHTML = '<i class="fas fa-eye-slash"></i>';
                pathButton.title = '경로 숨기기';
            } else {
                pathDisplay.innerHTML = '<div style="color: #dc3545;"><i class="fas fa-exclamation-triangle"></i> 파일 경로 정보를 가져올 수 없습니다.</div>';
                pathDisplay.style.display = 'block';
                pathButton.innerHTML = '<i class="fas fa-eye-slash"></i>';
                pathButton.title = '경로 숨기기';
            }
        } catch (error) {
            console.error('파일 경로 조회 실패:', error);
            pathDisplay.innerHTML = `<div style="color: #dc3545;"><i class="fas fa-exclamation-triangle"></i> 경로 조회 실패: ${error.message}</div>`;
            pathDisplay.style.display = 'block';
            pathButton.innerHTML = '<i class="fas fa-eye-slash"></i>';
            pathButton.title = '경로 숨기기';
        } finally {
            pathButton.disabled = false;
        }
    } else {
        // 경로가 표시되어 있는 경우 - 숨기기
        pathDisplay.style.display = 'none';
        pathButton.innerHTML = '<i class="fas fa-map-marker-alt"></i>';
        pathButton.title = '전체 경로 보기';
    }
}

// 모달에서 개별 파일 휴지통 이동
async function trashFileFromModal(fileId, fileName) {
    if (!confirm(`"${fileName}" 파일을 휴지통으로 이동하시겠습니까?`)) {
        return;
    }

    const trashBtn = document.getElementById(`trash-btn-${fileId}`);
    if (trashBtn) {
        trashBtn.disabled = true;
        trashBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';
    }

    try {
        await window.API.Duplicate.trashFile(fileId);
        updateFileExistsStatus(fileId, false);

        if (trashBtn) {
            trashBtn.innerHTML = '<i class="fas fa-check"></i>';
            trashBtn.style.pointerEvents = 'none';
        }

        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage(`"${fileName}" 파일이 휴지통으로 이동되었습니다.`);
        }
    } catch (error) {
        console.error('파일 휴지통 이동 실패:', error);

        if (trashBtn) {
            trashBtn.disabled = false;
            trashBtn.innerHTML = '<i class="fas fa-trash-alt"></i>';
        }

        if (typeof showErrorMessage === 'function') {
            showErrorMessage('파일 휴지통 이동 실패: ' + error.message);
        } else {
            alert('❌ 파일 휴지통 이동 실패: ' + error.message);
        }
    }
}

// 전체 경로 한번에 보기/숨기기
let allPathsVisible = false;

async function toggleAllFilePaths() {
    if (!currentGroupData || !currentGroupData.files) return;

    const btn = document.getElementById('toggle-all-paths-btn');
    allPathsVisible = !allPathsVisible;

    if (allPathsVisible) {
        btn.innerHTML = '<i class="fas fa-eye-slash"></i> 전체 경로 숨기기';
        // 각 파일의 경로를 순차적으로 표시
        for (const file of currentGroupData.files) {
            const pathDisplay = document.getElementById(`file-path-${file.id}`);
            if (pathDisplay && (pathDisplay.style.display === 'none' || pathDisplay.style.display === '')) {
                await toggleFilePath(file.id);
            }
        }
    } else {
        btn.innerHTML = '<i class="fas fa-map-marked-alt"></i> 전체 경로 보기';
        // 모든 경로 숨기기
        for (const file of currentGroupData.files) {
            const pathDisplay = document.getElementById(`file-path-${file.id}`);
            if (pathDisplay && pathDisplay.style.display !== 'none' && pathDisplay.style.display !== '') {
                await toggleFilePath(file.id);
            }
        }
    }
}

// 메모 편집 토글
function toggleMemoEdit() {
    const display = document.getElementById('memo-display');
    const empty = document.getElementById('memo-empty');
    const edit = document.getElementById('memo-edit');
    const editBtn = document.getElementById('memo-edit-btn');

    if (edit.style.display === 'none') {
        edit.style.display = 'block';
        if (display) display.style.display = 'none';
        if (empty) empty.style.display = 'none';
        if (editBtn) editBtn.style.display = 'none';
        document.getElementById('memo-textarea').focus();
    } else {
        cancelMemoEdit();
    }
}

// 메모 편집 취소
function cancelMemoEdit() {
    const edit = document.getElementById('memo-edit');
    const editBtn = document.getElementById('memo-edit-btn');
    const textarea = document.getElementById('memo-textarea');

    edit.style.display = 'none';
    if (editBtn) editBtn.style.display = '';

    // 원래 값 복원
    const currentMemo = currentGroupData ? (currentGroupData.memo || '') : '';
    textarea.value = currentMemo;

    if (currentMemo) {
        document.getElementById('memo-display').style.display = '';
        document.getElementById('memo-empty').style.display = 'none';
    } else {
        document.getElementById('memo-display').style.display = 'none';
        document.getElementById('memo-empty').style.display = '';
    }
}

// 메모 저장
async function saveMemo() {
    if (!currentGroupId) return;

    const textarea = document.getElementById('memo-textarea');
    const memo = textarea.value.trim();

    try {
        await window.API.Duplicate.updateMemo(currentGroupId, memo);

        // 현재 그룹 데이터 업데이트
        if (currentGroupData) {
            currentGroupData.memo = memo;
        }

        // UI 업데이트
        const display = document.getElementById('memo-display');
        const empty = document.getElementById('memo-empty');
        const edit = document.getElementById('memo-edit');
        const editBtn = document.getElementById('memo-edit-btn');

        edit.style.display = 'none';
        if (editBtn) editBtn.style.display = '';

        if (memo) {
            display.innerHTML = `<div style="background: #fffbeb; border: 1px solid #fcd34d; border-radius: 8px; padding: 12px; color: #92400e; font-size: 0.9rem; white-space: pre-wrap;">${memo}</div>`;
            display.style.display = '';
            empty.style.display = 'none';
        } else {
            display.style.display = 'none';
            empty.style.display = '';
        }

        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage('메모가 저장되었습니다.');
        }

        // 목록 새로고침 (메모 아이콘 반영)
        if (typeof updateRecentDuplicates === 'function') {
            await updateRecentDuplicates(true);
        }
    } catch (error) {
        console.error('메모 저장 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('메모 저장 실패: ' + error.message);
        }
    }
}

// ESC 키로 모달 닫기
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        const groupModal = document.getElementById('group-details-modal');
        if (groupModal && !groupModal.classList.contains('hidden')) {
            closeGroupDetailsModal();
            return;
        }
        
        const folderModal = document.getElementById('folderComparisonModal');
        if (folderModal && !folderModal.classList.contains('hidden')) {
            closeFolderComparisonModal();
            return;
        }

        const singleFolderModal = document.getElementById('singleFolderDuplicateModal');
        if (singleFolderModal && !singleFolderModal.classList.contains('hidden')) {
            closeSingleFolderDuplicateModal();
            return;
        }
    }
});