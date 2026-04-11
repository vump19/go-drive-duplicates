/**
 * 검증된 중복 관리 JavaScript
 * 검증된 중복 그룹 마킹, 조회, 무시 기능들
 */

let currentGroupData = null; // 현재 그룹의 전체 데이터 저장

// 현재 그룹을 검증됨으로 마킹
async function markCurrentGroupAsVerified() {
    if (!currentGroupId || !currentGroupData) {
        showErrorMessage('그룹 정보를 찾을 수 없습니다.');
        return;
    }

    await markGroupAsVerified(currentGroupData, '사용자가 검증된 중복으로 표시');
}

// 현재 그룹을 무시로 마킹
async function markCurrentGroupAsIgnored() {
    if (!currentGroupId || !currentGroupData) {
        showErrorMessage('그룹 정보를 찾을 수 없습니다.');
        return;
    }

    await markGroupAsIgnored(currentGroupData, '사용자가 무시로 표시');
}

// 그룹을 검증됨으로 마킹
async function markGroupAsVerified(groupData, description = '') {
    try {
        console.log('🔍 그룹을 검증됨으로 마킹:', groupData);

        const request = {
            hash: groupData.hash,
            file_count: groupData.count || groupData.files?.length || 0,
            total_size: groupData.totalSize || (groupData.files?.[0]?.size * groupData.count) || 0,
            status: 'verified',
            description: description
        };

        const result = await window.API.VerifiedDuplicate.markAsVerified(request);
        console.log('✅ 검증 마킹 성공:', result);

        // 중복 그룹 목록에서 제거
        if (currentGroupId) {
            try {
                await window.API.Duplicate.deleteDuplicateGroup(currentGroupId);
                console.log('✅ 중복 그룹 목록에서 제거 완료:', currentGroupId);
            } catch (deleteErr) {
                console.warn('⚠️ 중복 그룹 목록 제거 실패 (이미 삭제됨):', deleteErr);
            }
        }

        // 성공 메시지 표시
        showSuccessMessage(`✅ 중복 그룹이 검증됨으로 표시되고 목록에서 제거되었습니다.`);

        // 그룹 상세 모달 닫기
        closeGroupDetailsModal();

        // 목록 새로고침
        await refreshCurrentView();

        return result;

    } catch (error) {
        console.error('검증 마킹 실패:', error);
        showErrorMessage('검증 마킹 실패: ' + error.message);
        throw error;
    }
}

// 그룹을 무시로 마킹
async function markGroupAsIgnored(groupData, description = '') {
    try {
        console.log('🔍 그룹을 무시로 마킹:', groupData);

        const request = {
            hash: groupData.hash,
            file_count: groupData.count || groupData.files?.length || 0,
            total_size: groupData.totalSize || (groupData.files?.[0]?.size * groupData.count) || 0,
            status: 'ignored',
            description: description
        };

        const result = await window.API.VerifiedDuplicate.markAsVerified(request);
        console.log('✅ 무시 마킹 성공:', result);

        // 중복 그룹 목록에서 제거
        if (currentGroupId) {
            try {
                await window.API.Duplicate.deleteDuplicateGroup(currentGroupId);
                console.log('✅ 중복 그룹 목록에서 제거 완료:', currentGroupId);
            } catch (deleteErr) {
                console.warn('⚠️ 중복 그룹 목록 제거 실패 (이미 삭제됨):', deleteErr);
            }
        }

        // 성공 메시지 표시
        showSuccessMessage(`⚠️ 중복 그룹이 무시로 표시되고 목록에서 제거되었습니다.`);

        // 그룹 상세 모달 닫기
        closeGroupDetailsModal();

        // 목록 새로고침
        await refreshCurrentView();

        return result;

    } catch (error) {
        console.error('무시 마킹 실패:', error);
        showErrorMessage('무시 마킹 실패: ' + error.message);
        throw error;
    }
}

// 검증된 중복 목록 조회
async function getVerifiedDuplicates(status = null) {
    try {
        const result = await window.API.VerifiedDuplicate.getVerifiedDuplicates(status);
        console.log('✅ 검증된 중복 조회 성공:', result);

        return result.verified_duplicates || [];

    } catch (error) {
        console.error('검증된 중복 조회 실패:', error);
        throw error;
    }
}

// 검증 상태 업데이트
async function updateVerificationStatus(id, status, description = '') {
    try {
        const result = await window.API.VerifiedDuplicate.updateStatus(id, status, description);
        console.log('✅ 검증 상태 업데이트 성공:', result);

        return result;

    } catch (error) {
        console.error('검증 상태 업데이트 실패:', error);
        throw error;
    }
}

// 검증 기록 삭제
async function removeVerification(id) {
    try {
        const result = await window.API.VerifiedDuplicate.remove(id);
        console.log('✅ 검증 기록 삭제 성공:', result);

        return result;

    } catch (error) {
        console.error('검증 기록 삭제 실패:', error);
        throw error;
    }
}

// 해시로 검증 상세 정보 조회
async function getVerificationDetails(hash) {
    try {
        const result = await window.API.VerifiedDuplicate.getByHash(hash);

        // API layer now returns null for 404 (not verified), so no special handling needed
        if (result) {
            console.log('✅ 검증 정보 조회 성공:', result);
        }

        return result; // Will be null if not verified, or verification object if verified

    } catch (error) {
        console.error('검증 정보 조회 실패:', error);
        throw error;
    }
}

// 대량 검증 마킹
async function bulkMarkAsVerified(groupDataList, status = 'verified') {
    try {
        console.log('🔍 대량 검증 마킹 시작:', groupDataList.length, '개 그룹');

        const requests = groupDataList.map(groupData => ({
            hash: groupData.hash,
            file_count: groupData.count || groupData.files?.length || 0,
            total_size: groupData.totalSize || (groupData.files?.[0]?.size * groupData.count) || 0,
            status: status,
            description: `대량 ${status === 'verified' ? '검증' : '무시'} 처리`
        }));

        const result = await window.API.VerifiedDuplicate.bulkMark(requests);
        console.log('✅ 대량 검증 마킹 성공:', result);

        // 성공 메시지 표시
        const successCount = result.success_count || 0;
        const failedCount = result.failed_count || 0;
        
        let message = `✅ ${successCount}개의 중복 그룹이 ${status === 'verified' ? '검증됨' : '무시'}으로 표시되었습니다.`;
        if (failedCount > 0) {
            message += `\n⚠️ ${failedCount}개의 그룹 처리에 실패했습니다.`;
        }

        showSuccessMessage(message);

        return result;

    } catch (error) {
        console.error('대량 검증 마킹 실패:', error);
        showErrorMessage('대량 검증 마킹 실패: ' + error.message);
        throw error;
    }
}

// 현재 뷰 새로고침 (호출하는 페이지에 따라 다름)
async function refreshCurrentView() {
    try {
        // 대시보드 업데이트
        if (typeof updateRecentDuplicates === 'function') {
            await updateRecentDuplicates();
        }
        if (typeof updateStatusCards === 'function') {
            await updateStatusCards();
        }
        
        // 중복 관리 페이지 디바운스 업데이트 (검증 마킹은 즉시 반영할 필요 없음)
        if (typeof debouncedLoadBulkGroups === 'function') {
            debouncedLoadBulkGroups(2000); // 2초 후 새로고침
        }
        
        // 기타 페이지별 새로고침 함수
        if (typeof refreshDuplicates === 'function') {
            await refreshDuplicates();
        }

    } catch (error) {
        console.error('뷰 새로고침 실패:', error);
    }
}

// 검증된 중복 관리 UI 표시
async function showVerifiedDuplicatesManagement() {
    const modal = document.getElementById('verified-duplicates-modal');
    if (!modal) {
        console.error('검증된 중복 모달을 찾을 수 없습니다');
        return;
    }

    // 모달 표시
    modal.classList.remove('hidden');

    // 검증됨 탭 활성화 및 로드
    switchVerificationTab('verified');
}

// 검증된 중복 모달 닫기
function closeVerifiedDuplicatesModal() {
    const modal = document.getElementById('verified-duplicates-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

// 탭 전환
async function switchVerificationTab(tabType) {
    // 탭 버튼 활성화 상태 변경
    const verifiedTab = document.getElementById('verified-tab');
    const ignoredTab = document.getElementById('ignored-tab');
    const verifiedContainer = document.getElementById('verified-list-container');
    const ignoredContainer = document.getElementById('ignored-list-container');

    if (tabType === 'verified') {
        verifiedTab.classList.add('active');
        ignoredTab.classList.remove('active');
        verifiedContainer.style.display = 'block';
        ignoredContainer.style.display = 'none';

        // 검증됨 목록 로드
        await loadVerificationList('verified');
    } else {
        ignoredTab.classList.add('active');
        verifiedTab.classList.remove('active');
        ignoredContainer.style.display = 'block';
        verifiedContainer.style.display = 'none';

        // 무시됨 목록 로드
        await loadVerificationList('ignored');
    }
}

// 검증 목록 로드
async function loadVerificationList(status) {
    const container = status === 'verified'
        ? document.getElementById('verified-list-container')
        : document.getElementById('ignored-list-container');

    const countElement = status === 'verified'
        ? document.getElementById('verified-count')
        : document.getElementById('ignored-count');

    if (!container) return;

    try {
        // 로딩 표시
        container.innerHTML = '<div class="loading"><i class="fas fa-spinner fa-spin"></i> 로딩 중...</div>';

        // API 호출
        const response = await window.API.VerifiedDuplicate.getVerifiedDuplicates(status);
        const verifiedList = response.verified_duplicates || [];

        // 카운트 업데이트
        if (countElement) {
            countElement.textContent = verifiedList.length;
        }

        if (verifiedList.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-${status === 'verified' ? 'check-circle' : 'eye-slash'}" style="font-size: 3rem; color: #cbd5e0; margin-bottom: 16px;"></i>
                    <div style="color: #718096; font-size: 1.1rem;">${status === 'verified' ? '검증된' : '무시된'} 중복 그룹이 없습니다</div>
                </div>
            `;
            return;
        }

        // 목록 렌더링
        let html = '<div class="verified-groups-list">';

        verifiedList.forEach((item, index) => {
            const createdDate = new Date(item.created_at).toLocaleDateString('ko-KR', {
                year: 'numeric',
                month: 'long',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });

            const statusIcon = status === 'verified'
                ? '<i class="fas fa-check-circle" style="color: #10b981;"></i>'
                : '<i class="fas fa-eye-slash" style="color: #f59e0b;"></i>';

            const wastedSpace = item.file_count > 1 ? (item.file_count - 1) * (item.total_size / item.file_count) : 0;

            html += `
                <div class="verified-group-item" id="verified-item-${item.id}">
                    <div class="verified-group-header" onclick="toggleVerifiedGroupDetails(${item.id})">
                        <div class="verified-group-status">
                            ${statusIcon}
                            <div>
                                <div class="verified-group-title">${status === 'verified' ? '검증됨' : '무시됨'} #${item.id}</div>
                                <div style="font-size: 0.875rem; color: #6b7280; margin-top: 4px;">
                                    ${item.file_count || 0}개 파일 • ${formatFileSize(item.total_size || 0)} •
                                    절약 가능: ${formatFileSize(wastedSpace)}
                                </div>
                            </div>
                        </div>
                        <div class="verified-expand-icon" id="expand-icon-${item.id}">
                            <i class="fas fa-chevron-down"></i>
                        </div>
                    </div>

                    <!-- 상세 정보 (초기에는 숨김) -->
                    <div class="verified-group-details" id="verified-details-${item.id}" style="display: none;">
                        <div class="verified-group-info">
                            <div class="info-row">
                                <span class="info-label">파일 수:</span>
                                <span class="info-value">${item.file_count || 0}개</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">총 크기:</span>
                                <span class="info-value">${formatFileSize(item.total_size || 0)}</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">절약 가능:</span>
                                <span class="info-value" style="color: #ef4444;">${formatFileSize(wastedSpace)}</span>
                            </div>
                            <div class="info-row full-width">
                                <span class="info-label">파일 해시:</span>
                                <span class="info-value" style="font-family: monospace; font-size: 0.85rem; word-break: break-all;">${item.hash || 'N/A'}</span>
                            </div>
                            ${item.description ? `
                            <div class="info-row full-width">
                                <span class="info-label">설명:</span>
                                <span class="info-value">${item.description}</span>
                            </div>
                            ` : ''}
                            <div class="info-row full-width">
                                <span class="info-label">등록일:</span>
                                <span class="info-value">${createdDate}</span>
                            </div>
                        </div>

                        <!-- 중복 파일 목록 로딩 영역 -->
                        <div class="verified-files-section" id="verified-files-${item.id}">
                            <button class="btn btn-primary btn-block" onclick="loadVerifiedGroupFiles('${item.hash}', ${item.id})">
                                <i class="fas fa-list"></i> 중복 파일 목록 보기
                            </button>
                        </div>

                        <!-- 액션 버튼 -->
                        <div class="verified-group-actions-expanded">
                            <button class="btn btn-secondary" onclick="event.stopPropagation(); changeVerificationStatus(${item.id}, '${status === 'verified' ? 'ignored' : 'verified'}')">
                                <i class="fas fa-${status === 'verified' ? 'eye-slash' : 'check'}"></i>
                                ${status === 'verified' ? '무시로 변경' : '검증됨으로 변경'}
                            </button>
                            <button class="btn btn-danger" onclick="event.stopPropagation(); removeVerificationRecord(${item.id})">
                                <i class="fas fa-trash"></i> 기록 삭제
                            </button>
                        </div>
                    </div>
                </div>
            `;
        });

        html += '</div>';
        container.innerHTML = html;

    } catch (error) {
        console.error(`${status} 목록 로드 실패:`, error);
        container.innerHTML = `
            <div class="error-state">
                <i class="fas fa-exclamation-circle" style="font-size: 3rem; color: #ef4444; margin-bottom: 16px;"></i>
                <div style="color: #dc2626; font-size: 1.1rem;">목록을 불러올 수 없습니다</div>
                <div style="color: #6b7280; margin-top: 8px;">${error.message}</div>
            </div>
        `;
    }
}

// 검증 목록 새로고침
async function refreshVerifiedLists() {
    const verifiedTab = document.getElementById('verified-tab');
    const ignoredTab = document.getElementById('ignored-tab');

    if (verifiedTab && verifiedTab.classList.contains('active')) {
        await loadVerificationList('verified');
    } else if (ignoredTab && ignoredTab.classList.contains('active')) {
        await loadVerificationList('ignored');
    }

    if (typeof showSuccessMessage === 'function') {
        showSuccessMessage('목록이 새로고침되었습니다.');
    }
}

// 검증 상태 변경
async function changeVerificationStatus(id, newStatus) {
    const statusLabel = newStatus === 'verified' ? '검증됨' : '무시됨';

    if (!confirm(`이 그룹을 '${statusLabel}'으로 변경하시겠습니까?`)) {
        return;
    }

    try {
        await window.API.VerifiedDuplicate.updateStatus(id, newStatus, `상태를 ${statusLabel}으로 변경`);

        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage(`✅ 상태가 '${statusLabel}'으로 변경되었습니다.`);
        }

        // 양쪽 목록 모두 새로고침
        await loadVerificationList('verified');
        await loadVerificationList('ignored');

    } catch (error) {
        console.error('상태 변경 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('상태 변경에 실패했습니다: ' + error.message);
        } else {
            alert('❌ 상태 변경에 실패했습니다: ' + error.message);
        }
    }
}

// 검증 기록 삭제
async function removeVerificationRecord(id) {
    if (!confirm('이 검증 기록을 삭제하시겠습니까?\n\n다음 중복 검색 시 이 그룹이 다시 나타날 수 있습니다.')) {
        return;
    }

    try {
        await window.API.VerifiedDuplicate.remove(id);

        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage('✅ 검증 기록이 삭제되었습니다.');
        }

        // 양쪽 목록 모두 새로고침
        await loadVerificationList('verified');
        await loadVerificationList('ignored');

    } catch (error) {
        console.error('기록 삭제 실패:', error);
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('기록 삭제에 실패했습니다: ' + error.message);
        } else {
            alert('❌ 기록 삭제에 실패했습니다: ' + error.message);
        }
    }
}

// 검증된 그룹 상세 정보 토글
function toggleVerifiedGroupDetails(itemId) {
    const detailsElement = document.getElementById(`verified-details-${itemId}`);
    const expandIcon = document.getElementById(`expand-icon-${itemId}`);

    if (detailsElement && expandIcon) {
        if (detailsElement.style.display === 'none') {
            detailsElement.style.display = 'block';
            expandIcon.innerHTML = '<i class="fas fa-chevron-up"></i>';
        } else {
            detailsElement.style.display = 'none';
            expandIcon.innerHTML = '<i class="fas fa-chevron-down"></i>';
        }
    }
}

// 검증된 그룹의 파일 목록 로드
async function loadVerifiedGroupFiles(hash, itemId) {
    const filesSection = document.getElementById(`verified-files-${itemId}`);
    if (!filesSection) return;

    try {
        // 로딩 표시
        filesSection.innerHTML = '<div class="loading"><i class="fas fa-spinner fa-spin"></i> 파일 목록 로딩 중...</div>';

        // 새로운 API를 사용하여 해시로 직접 중복 그룹 조회 (검증 상태와 무관하게 조회)
        const matchingGroup = await window.API.Duplicate.getDuplicateGroupByHash(hash);

        if (!matchingGroup) {
            filesSection.innerHTML = `
                <div class="warning-message">
                    <i class="fas fa-exclamation-triangle"></i>
                    <div>이 그룹의 파일들이 현재 중복 목록에 없습니다.</div>
                    <div style="font-size: 0.875rem; margin-top: 8px; color: #6b7280;">
                        파일이 삭제되었거나 중복 검사를 다시 실행해야 할 수 있습니다.
                    </div>
                </div>
            `;
            return;
        }

        // 파일 목록 표시
        if (!matchingGroup.files || matchingGroup.files.length === 0) {
            filesSection.innerHTML = '<div class="info-message">파일 정보를 불러올 수 없습니다.</div>';
            return;
        }

        let html = '<div class="verified-files-list">';
        html += `<h4 style="margin-bottom: 12px; color: #1f2937;"><i class="fas fa-list"></i> 중복 파일 목록 (${matchingGroup.files.length}개)</h4>`;

        matchingGroup.files.forEach((file, index) => {
            const fileTypeIcon = getFileTypeIcon(file.mimeType || 'application/octet-stream');
            const modifiedDate = file.modifiedTime ? new Date(file.modifiedTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';

            html += `
                <div class="verified-file-item">
                    <div class="file-info-compact">
                        <span class="file-type-icon">${fileTypeIcon}</span>
                        <div class="file-details-compact">
                            <div class="file-name-compact" title="${file.name || '알 수 없는 파일'}">${file.name || '알 수 없는 파일'}</div>
                            <div class="file-meta-compact">
                                <span><i class="fas fa-calendar"></i> ${modifiedDate}</span>
                                <span><i class="fas fa-hdd"></i> ${formatFileSize(file.size || 0)}</span>
                            </div>
                        </div>
                    </div>
                    <div class="file-actions-compact">
                        ${file.parents && file.parents.length > 0 ? `
                            <a href="https://drive.google.com/drive/folders/${file.parents[0]}" target="_blank" class="btn-sm btn-info" title="폴더 열기">
                                <i class="fas fa-folder-open"></i>
                            </a>
                        ` : ''}
                        ${file.webViewLink ? `
                            <a href="${file.webViewLink}" target="_blank" class="btn-sm btn-primary" title="파일 열기">
                                <i class="fas fa-file"></i>
                            </a>
                        ` : ''}
                    </div>
                </div>
            `;
        });

        html += '</div>';
        filesSection.innerHTML = html;

    } catch (error) {
        console.error('파일 목록 로드 실패:', error);
        filesSection.innerHTML = `
            <div class="error-message">
                <i class="fas fa-exclamation-circle"></i>
                파일 목록을 불러오는데 실패했습니다: ${error.message}
            </div>
        `;
    }
}

// 그룹 상세보기에서 검증 상태 표시 업데이트
async function updateGroupVerificationStatus(groupData) {
    if (!groupData || !groupData.hash) return;

    // 버튼 상태를 기본값으로 초기화
    const verifyBtn = document.getElementById('verify-group-btn');
    const ignoreBtn = document.getElementById('ignore-group-btn');
    if (verifyBtn) {
        verifyBtn.disabled = false;
        verifyBtn.className = 'btn btn-success';
        verifyBtn.innerHTML = '<i class="fas fa-check"></i> 검증됨으로 변경';
    }
    if (ignoreBtn) {
        ignoreBtn.disabled = false;
        ignoreBtn.className = 'btn btn-warning';
        ignoreBtn.innerHTML = '<i class="fas fa-eye-slash"></i> 무시됨';
    }

    try {
        const verification = await getVerificationDetails(groupData.hash);

        if (verification) {
            if (verification.status === 'verified') {
                if (verifyBtn) {
                    verifyBtn.innerHTML = '<i class="fas fa-check-circle"></i> 검증됨';
                    verifyBtn.disabled = true;
                    verifyBtn.className = 'btn btn-success disabled';
                }
                if (ignoreBtn) {
                    ignoreBtn.disabled = false;
                    ignoreBtn.innerHTML = '<i class="fas fa-eye-slash"></i> 무시로 변경';
                }
            } else if (verification.status === 'ignored') {
                if (ignoreBtn) {
                    ignoreBtn.innerHTML = '<i class="fas fa-eye-slash"></i> 무시됨';
                    ignoreBtn.disabled = true;
                    ignoreBtn.className = 'btn btn-warning disabled';
                }
                if (verifyBtn) {
                    verifyBtn.disabled = false;
                    verifyBtn.innerHTML = '<i class="fas fa-check"></i> 검증됨으로 변경';
                }
            }
            
            // 검증 정보 표시
            const content = document.getElementById('group-details-content');
            if (content && verification.description) {
                const verificationInfo = document.createElement('div');
                verificationInfo.className = 'verification-info';
                verificationInfo.innerHTML = `
                    <div style="background: #f8f9fa; padding: 12px; border-radius: 6px; margin-bottom: 16px; border-left: 4px solid ${verification.status === 'verified' ? '#28a745' : '#ffc107'};">
                        <strong><i class="fas fa-${verification.status === 'verified' ? 'check-circle' : 'eye-slash'}"></i> ${verification.status === 'verified' ? '검증됨' : '무시됨'}</strong>
                        ${verification.description ? `<div style="margin-top: 4px; font-size: 0.9rem; color: #6c757d;">${verification.description}</div>` : ''}
                        <div style="margin-top: 4px; font-size: 0.8rem; color: #6c757d;">
                            ${new Date(verification.created_at).toLocaleString('ko-KR')}
                        </div>
                    </div>
                `;
                content.insertBefore(verificationInfo, content.firstChild);
            }
        }

    } catch (error) {
        console.error('검증 상태 조회 실패:', error);
    }
}