/**
 * 대시보드 관련 JavaScript
 * 메인 대시보드의 상태 카드 업데이트 및 기본 기능들
 */

// 상태 카드 업데이트 디바운스 변수
let updateStatusCardsTimeout = null;
let isUpdatingStatusCards = false;

// 상태 카드 업데이트
async function updateStatusCards() {
    // 이미 업데이트 중이면 중복 호출 방지
    if (isUpdatingStatusCards) {
        console.log('🔄 상태 카드 업데이트 중이므로 중복 호출 무시');
        return;
    }
    
    isUpdatingStatusCards = true;
    
    try {
        // 시스템 상태 확인
        try {
            const health = await window.API.Health.checkServer();
            document.getElementById('system-status').textContent = '정상';
            document.getElementById('system-status').style.color = '#10b981';
            document.getElementById('system-subtitle').textContent = 'Google Drive 연결: 정상';
        } catch (error) {
            document.getElementById('system-status').textContent = '오류';
            document.getElementById('system-status').style.color = '#ef4444';
            document.getElementById('system-subtitle').textContent = 'Google Drive 연결: 오류';
        }

        // 파일 통계 업데이트 (실제 데이터 사용)
        try {
            // 파일 통계 조회 (전체 파일 수 및 해시 계산 진행률)
            try {
                const stats = await window.API.File.getStatistics();

                // 전체 파일 수 및 크기 표시
                document.getElementById('total-files').textContent = formatNumber(stats.totalFiles || 0);
                document.getElementById('total-size').textContent = `총 크기: ${formatFileSize(stats.totalSize || 0)}`;

                // 해시 계산 진행률 표시
                const hashedFiles = stats.hashedFiles || 0;
                const hashedPercentage = stats.hashedPercentage || 0;

                document.getElementById('hashed-files').textContent = formatNumber(hashedFiles);

                // 해시 계산 비율에 따른 색상 및 경고 표시
                const percentageElement = document.getElementById('hashed-percentage');
                if (hashedPercentage < 10) {
                    percentageElement.textContent = `해시 계산: ${hashedPercentage.toFixed(1)}% ⚠️`;
                    percentageElement.style.color = '#ef4444'; // 빨간색
                } else if (hashedPercentage < 50) {
                    percentageElement.textContent = `해시 계산: ${hashedPercentage.toFixed(1)}% ⚠️`;
                    percentageElement.style.color = '#f59e0b'; // 주황색
                } else {
                    percentageElement.textContent = `해시 계산: ${hashedPercentage.toFixed(1)}%`;
                    percentageElement.style.color = '#10b981'; // 초록색
                }
            } catch (error) {
                console.log('파일 통계 조회 실패:', error);
                // 통계 조회 실패 시 기본값 유지
            }

            const duplicatesResponse = await window.API.Duplicate.getDuplicateGroups(1, 100);
            
            // 새로운 페이지네이션 응답 형식 처리
            let duplicates = [];
            let totalGroupsCount = 0;
            
            if (duplicatesResponse && duplicatesResponse.groups && Array.isArray(duplicatesResponse.groups)) {
                duplicates = duplicatesResponse.groups;
                totalGroupsCount = duplicatesResponse.totalGroups || duplicates.length;
            } else if (Array.isArray(duplicatesResponse)) {
                // 이전 API 형식과의 호환성
                duplicates = duplicatesResponse;
                totalGroupsCount = duplicates.length;
            }
            
            if (duplicates.length > 0) {
                let totalDuplicateFiles = 0;
                let totalWastedSpace = 0;
                let totalSize = 0;
                
                duplicates.forEach(group => {
                    totalDuplicateFiles += group.count || 0;
                    totalSize += group.totalSize || 0;
                    const individualFileSize = group.files && group.files.length > 0 ? 
                        group.files[0].size : (group.totalSize || 0) / (group.count || 1);
                    totalWastedSpace += (group.count - 1) * individualFileSize;
                });
                
                document.getElementById('total-files').textContent = formatNumber(totalDuplicateFiles);
                document.getElementById('total-size').textContent = `총 크기: ${formatFileSize(totalSize)}`;
                document.getElementById('duplicate-groups').textContent = formatNumber(totalGroupsCount);
                document.getElementById('duplicate-files').textContent = `중복 파일: ${formatNumber(totalDuplicateFiles)}개`;
                document.getElementById('potential-savings').textContent = formatFileSize(totalWastedSpace);
            } else {
                // 중복 파일이 없는 경우
                document.getElementById('total-files').textContent = '0';
                document.getElementById('total-size').textContent = '총 크기: 0 B';
                document.getElementById('duplicate-groups').textContent = '0';
                document.getElementById('duplicate-files').textContent = '중복 파일: 0개';
                document.getElementById('potential-savings').textContent = '0 B';
            }
        } catch (error) {
            console.log('통계 데이터 로드 실패, 기본값 유지:', error);
            document.getElementById('total-files').textContent = '-';
            document.getElementById('total-size').textContent = '총 크기: -';
            document.getElementById('duplicate-groups').textContent = '-';
            document.getElementById('duplicate-files').textContent = '중복 파일: -개';
            document.getElementById('potential-savings').textContent = '-';
        }
    } catch (error) {
        console.error('상태 카드 업데이트 실패:', error);
    } finally {
        // 업데이트 상태 해제
        isUpdatingStatusCards = false;
    }
}

// 진행 상황 업데이트 (스마트 폴링)
let progressPollingActive = true;
let progressErrorCount = 0;
let lastFoundDuplicateGroups = 0; // 마지막으로 확인한 중복 그룹 수

// 이전 진행 상황 상태 캐시 (깜빡임 방지)
let lastProgressState = {
    scanProgress: null,
    dupProgress: null
};

async function updateProgress() {
    if (!progressPollingActive) return;

    try {
        const container = document.getElementById('progress-container');

        // 두 API를 병렬로 호출하여 더 빠른 업데이트
        const [scanResult, dupResult] = await Promise.allSettled([
            window.API.File.getScanProgress(),
            window.API.Duplicate.getDuplicateProgress()
        ]);

        // 파일 스캔 진행 상황 처리 - API 응답이 성공하면 상태 업데이트
        if (scanResult.status === 'fulfilled') {
            const scanProgress = scanResult.value;
            if (scanProgress && (scanProgress.status === 'running' || scanProgress.status === 'pending')) {
                lastProgressState.scanProgress = scanProgress;
            } else {
                // 명시적으로 완료/실패 상태이거나 응답이 없으면 null로 설정
                lastProgressState.scanProgress = null;
            }
        } else {
            // API 실패 (404 등) = 활성 작업 없음 → 이전 상태 초기화
            lastProgressState.scanProgress = null;
        }

        // 중복 검색 진행 상황 처리 - API 응답이 성공하면 상태 업데이트
        if (dupResult.status === 'fulfilled') {
            const dupProgress = dupResult.value;
            if (dupProgress && (dupProgress.status === 'running' || dupProgress.status === 'pending')) {
                lastProgressState.dupProgress = dupProgress;
            } else {
                // 명시적으로 완료/실패 상태이거나 응답이 없으면 null로 설정
                lastProgressState.dupProgress = null;
            }
        } else {
            // API 실패 (404 등) = 활성 작업 없음 → 이전 상태 초기화
            lastProgressState.dupProgress = null;
        }

        // 성공 시 에러 카운트 초기화
        progressErrorCount = 0;

        // 캐시된 상태 기준으로 활성 작업 목록 생성 (순서 고정)
        const activeProgresses = [];
        if (lastProgressState.scanProgress) {
            activeProgresses.push({ progress: lastProgressState.scanProgress, order: 1 });
        }
        if (lastProgressState.dupProgress) {
            activeProgresses.push({ progress: lastProgressState.dupProgress, order: 2 });
        }

        // 순서대로 정렬 (파일 스캔 먼저, 중복 검색 나중에)
        activeProgresses.sort((a, b) => a.order - b.order);

        if (activeProgresses.length > 0) {
            // 모든 활성 작업을 표시
            let html = '';
            for (const { progress } of activeProgresses) {
                html += renderProgressItem(progress);
            }

            // 항상 전체 업데이트 (순서가 고정되어 있으므로 깜빡임 없음)
            container.innerHTML = html;
        } else {
            container.innerHTML = `
                <div class="loading">
                    <i class="fas fa-check-circle" style="font-size: 2rem; color: #10b981; margin-bottom: 16px;"></i>
                    <div>진행 중인 작업이 없습니다</div>
                </div>
            `;
            lastFoundDuplicateGroups = 0;
        }
    } catch (error) {
        // 예상치 못한 에러만 처리 - 이전 상태 유지
        console.error('진행 상황 업데이트 중 예상치 못한 에러:', error);
        progressErrorCount++;

        if (progressErrorCount >= 5) {
            console.log('📴 진행 상황 폴링 중지 (연속 에러)');
            progressPollingActive = false;
        }
    }
}

// 개별 진행 상황 항목 렌더링
function renderProgressItem(progress) {
    if (!progress || (progress.status !== 'running' && progress.status !== 'pending')) {
        return '';
    }

    // 작업 타입에 따른 제목과 아이콘 설정
    let title = '작업 진행 중';
    let icon = 'fas fa-cog';

    switch (progress.operationType) {
        case 'duplicate_search':
            title = '중복 파일 검색';
            icon = 'fas fa-search';
            break;
        case 'validate_duplicates':
            title = '중복 그룹 검증';
            icon = 'fas fa-sync';
            break;
        case 'file_scan':
            title = '파일 스캔';
            icon = 'fas fa-folder-open';
            break;
        case 'hash_calculation':
            title = '해시 계산';
            icon = 'fas fa-calculator';
            break;
        case 'folder_comparison':
            title = '폴더 비교';
            icon = 'fas fa-exchange-alt';
            break;
        default:
            title = '작업 진행 중';
            icon = 'fas fa-cog';
    }

    // 전체 항목 수를 알 수 있는 경우와 없는 경우 처리
    const hasTotal = progress.totalItems > 0;
    const percent = hasTotal ? Math.round((progress.processedItems / progress.totalItems) * 100) : 0;

    // 파일 스캔의 경우 스캔한 폴더 수 표시
    let scannedFolders = 0;
    if (progress.metadata && progress.metadata.scannedFolders) {
        scannedFolders = Array.isArray(progress.metadata.scannedFolders) ? progress.metadata.scannedFolders.length : 0;
    }

    // 🔔 발견된 중복 그룹 수 확인 및 실시간 새로고침
    const foundGroups = progress.metadata && progress.metadata.foundDuplicateGroups ? progress.metadata.foundDuplicateGroups : 0;
    if (foundGroups > 0 && foundGroups !== lastFoundDuplicateGroups) {
        console.log(`🆕 새로운 중복 그룹 발견: ${lastFoundDuplicateGroups} → ${foundGroups}개, 목록 자동 새로고침`);
        lastFoundDuplicateGroups = foundGroups;

        // 최근 중복 파일 목록 자동 새로고침 (강제)
        if (typeof updateRecentDuplicates === 'function') {
            updateRecentDuplicates(true);
        }
    }

    // 진행 상황 텍스트 생성
    let progressText = '';
    if (hasTotal) {
        progressText = `${formatNumber(progress.processedItems || 0)} / ${formatNumber(progress.totalItems)} 항목 처리됨`;
    } else {
        progressText = `${formatNumber(progress.processedItems || 0)}개 항목 처리됨`;
    }

    // 진행 바 스타일 (total이 없으면 무한 애니메이션)
    let progressBarStyle = hasTotal
        ? `width: ${percent}%`
        : 'width: 100%; animation: progress-indeterminate 5s ease-in-out infinite';

    return `
        <div class="progress-item" style="margin-bottom: 12px;">
            <div class="progress-header">
                <div class="progress-title">
                    <i class="${icon}"></i> ${title}
                </div>
                <div class="progress-percentage">${hasTotal ? percent + '%' : '진행 중...'}</div>
            </div>
            <div class="progress-bar">
                <div class="progress-fill" style="${progressBarStyle}"></div>
            </div>
            <div class="progress-details">
                ${progress.operationType === 'file_scan'
                    ? `<strong style="font-size: 1.1em; color: #3b82f6;">📊 전체 스캔: ${formatNumber(progress.processedItems || 0)}개 파일</strong>`
                    : `${progress.currentStep || '검색 중...'} - ${progressText}`
                }
                ${scannedFolders > 0 ? `<br><strong>스캔 완료 폴더:</strong> ${formatNumber(scannedFolders)}개` : ''}
                ${progress.operationType === 'file_scan' && progress.currentStep ? `<br><span style="color: #6b7280;">${progress.currentStep}</span>` : ''}
                ${progress.metadata && progress.metadata.lastScannedFolderPath ? `<br><strong>현재 폴더:</strong> ${progress.metadata.lastScannedFolderPath}` : ''}
                ${progress.metadata && progress.metadata.currentBatch ? `<br><strong>배치:</strong> ${progress.metadata.currentBatch}번째 (누적: 성공 ${formatNumber(progress.metadata.totalSuccessful || 0)}개, 실패 ${formatNumber(progress.metadata.totalFailed || 0)}개)` : ''}
                ${foundGroups > 0 ? `<br><strong style="color: #10b981;">🎯 발견된 중복 그룹:</strong> ${formatNumber(foundGroups)}개` : ''}
            </div>
        </div>
    `;
}

// 최근 중복 파일 업데이트 디바운스 변수
let updateRecentDuplicatesTimeout = null;
let isUpdatingRecentDuplicates = false;

// 최근 중복 파일 선택 관리
let selectedRecentGroups = new Set();

// 최근 중복 파일 페이지네이션 관리
let currentRecentPage = 1;
const recentItemsPerPage = 10;

// 최근 중복 파일 업데이트
async function updateRecentDuplicates(force = false) {
    // 강제 업데이트가 아니고 이미 업데이트 중이면 중복 호출 방지
    if (!force && isUpdatingRecentDuplicates) {
        console.log('🔄 최근 중복 파일 업데이트 중이므로 중복 호출 무시');
        return;
    }
    
    if (force) {
        console.log('🚀 강제 업데이트 모드: 최근 중복 파일 목록 새로고침');
    }
    
    isUpdatingRecentDuplicates = true;
    
    try {
        const duplicatesResponse = await window.API.Duplicate.getDuplicateGroups(currentRecentPage, recentItemsPerPage);
        const container = document.getElementById('recent-duplicates');
        
        // 새로운 페이지네이션 응답 형식 처리
        let duplicates = [];
        
        if (duplicatesResponse && duplicatesResponse.groups && Array.isArray(duplicatesResponse.groups)) {
            duplicates = duplicatesResponse.groups;
        } else if (Array.isArray(duplicatesResponse)) {
            // 이전 API 형식과의 호환성
            duplicates = duplicatesResponse;
        }
        
        if (duplicates.length === 0) {
            container.innerHTML = `
                <div class="loading">
                    <i class="fas fa-inbox" style="font-size: 2rem; color: #6b7280; margin-bottom: 16px;"></i>
                    <div>중복 파일을 찾지 못했습니다</div>
                </div>
            `;
        } else {
            // 페이지네이션 정보 처리
            const totalGroups = duplicatesResponse.totalGroups || duplicates.length;
            const totalPages = duplicatesResponse.totalPages || Math.ceil(totalGroups / recentItemsPerPage);
            const hasNext = duplicatesResponse.hasNext || currentRecentPage < totalPages;
            const hasPrev = duplicatesResponse.hasPrev || currentRecentPage > 1;
            
            // 상단 일괄 선택 및 삭제 버튼, 페이지네이션
            const headerControls = `
                <div class="recent-duplicates-header" style="margin-bottom: 16px;">
                    <!-- 선택 및 삭제 컨트롤 -->
                    <div style="display: flex; justify-content: space-between; align-items: center; padding: 12px; background: #f8f9fa; border-radius: 6px; margin-bottom: 12px;">
                        <div style="display: flex; align-items: center; gap: 12px;">
                            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer;">
                                <input type="checkbox" id="selectAllRecentGroups" onchange="toggleSelectAllRecentGroups()" style="cursor: pointer;">
                                <span style="font-size: 0.9rem; color: #374151;">전체 선택</span>
                            </label>
                            <span id="selectedRecentCount" style="font-size: 0.85rem; color: #6b7280;">0개 선택됨</span>
                        </div>
                        <div style="display: flex; gap: 8px;">
                            <button class="btn-sm btn-primary" onclick="validateDuplicateGroups()">
                                <i class="fas fa-sync"></i> 재스캔
                            </button>
                            <button class="btn-sm btn-danger" onclick="deleteSelectedRecentGroups()" id="deleteSelectedRecentBtn" disabled>
                                <i class="fas fa-trash"></i> 선택된 항목 삭제
                            </button>
                        </div>
                    </div>
                    
                    <!-- 페이지네이션 -->
                    <div style="display: flex; justify-content: space-between; align-items: center; padding: 8px 12px; background: #f9fafb; border-radius: 6px; font-size: 0.9rem;">
                        <div style="color: #6b7280;">
                            전체 ${totalGroups}개 중 ${Math.min((currentRecentPage - 1) * recentItemsPerPage + 1, totalGroups)}-${Math.min(currentRecentPage * recentItemsPerPage, totalGroups)}개 표시
                        </div>
                        <div style="display: flex; gap: 8px; align-items: center;">
                            <button class="btn-sm btn-secondary" onclick="changeRecentPage(${currentRecentPage - 1})" ${!hasPrev ? 'disabled' : ''}>
                                <i class="fas fa-chevron-left"></i> 이전
                            </button>
                            <span style="padding: 4px 8px; background: white; border-radius: 4px; border: 1px solid #d1d5db;">
                                ${currentRecentPage} / ${totalPages}
                            </span>
                            <button class="btn-sm btn-secondary" onclick="changeRecentPage(${currentRecentPage + 1})" ${!hasNext ? 'disabled' : ''}>
                                다음 <i class="fas fa-chevron-right"></i>
                            </button>
                        </div>
                    </div>
                </div>
            `;
            
            const itemsHtml = duplicates.map(group => {
                const fileName = group.files && group.files.length > 0 ? group.files[0].name : '이름 없음';
                const wastedSpace = group.files && group.files.length > 0 ? 
                    (group.count - 1) * group.files[0].size : 0;
                
                // 검증 상태 마크 추가
                const verificationMark = group.verificationStatus === 'verified' ?
                    '<span style="background: #10b981; color: white; font-size: 0.7rem; padding: 2px 6px; border-radius: 12px; margin-left: 8px;">✅ 검증됨</span>' : '';

                // 메모 마크 추가
                const memoMark = group.memo ?
                    `<span style="background: #fef3c7; color: #92400e; font-size: 0.7rem; padding: 2px 6px; border-radius: 12px; margin-left: 4px; cursor: pointer;" title="${group.memo.replace(/"/g, '&quot;')}"><i class="fas fa-sticky-note"></i> 메모</span>` : '';
                
                const isSelected = selectedRecentGroups.has(group.id);
                
                return `
                <div class="result-item ${isSelected ? 'selected' : ''}" id="recent-item-${group.id}">
                    <div style="display: flex; align-items: center; gap: 12px;">
                        <input type="checkbox" class="recent-group-checkbox" data-group-id="${group.id}" onchange="toggleRecentGroupSelection('${group.id}')" ${isSelected ? 'checked' : ''} style="cursor: pointer;">
                        <div class="result-info" style="flex: 1;">
                            <div class="result-name">${fileName}${verificationMark}${memoMark}</div>
                            <div class="result-count">${group.count || 0}개 중복 파일 (${formatFileSize(wastedSpace)} 절약 가능)</div>
                        </div>
                        <div style="display: flex; align-items: center; gap: 8px;">
                            <div class="result-size">${formatFileSize(group.totalSize || 0)}</div>
                            <div class="result-actions">
                                <button class="btn-sm btn-danger" onclick="quickDelete('${group.id}')" title="이 중복 그룹 삭제">
                                    <i class="fas fa-trash"></i> 삭제
                                </button>
                                <button class="btn-sm btn-secondary" onclick="viewGroup('${group.id}')" title="상세보기">
                                    <i class="fas fa-eye"></i> 상세
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
                `;
            }).join('');
            
            container.innerHTML = headerControls + itemsHtml;
            
            // 목록이 새로 로드된 후 선택 상태 정리 및 UI 업데이트
            cleanupSelectedGroups(duplicates);
            updateRecentSelectionUI();
        }
    } catch (error) {
        console.error('최근 중복 파일 업데이트 실패:', error);
        // 임시로 샘플 데이터 표시
        document.getElementById('recent-duplicates').innerHTML = `
            <div style="text-align: center; padding: 20px; color: #6b7280;">
                <i class="fas fa-info-circle" style="font-size: 2rem; color: #3b82f6; margin-bottom: 16px;"></i>
                <div>아직 중복 파일 검사를 하지 않았습니다</div>
                <button class="action-btn" onclick="startScan()" style="margin-top: 16px; padding: 12px 24px; font-size: 0.9rem;">
                    <i class="fas fa-search-plus"></i>
                    중복 검사 시작하기
                </button>
            </div>
        `;
    } finally {
        // 업데이트 상태 해제
        isUpdatingRecentDuplicates = false;
    }
}

// 최근 중복 파일 페이지 변경
function changeRecentPage(newPage) {
    console.log(`📄 최근 중복 파일 페이지 변경: ${currentRecentPage} → ${newPage}`);
    currentRecentPage = newPage;
    updateRecentDuplicates(true); // 강제 새로고침
}

// 중복 그룹 유효성 검증 및 정리
async function validateDuplicateGroups() {
    if (!confirm('현재 중복 그룹 목록의 유효성을 검증하고, 더 이상 중복이 아닌 그룹을 자동으로 삭제하시겠습니까?\n\n이 작업은 시간이 걸릴 수 있습니다.')) {
        return;
    }

    try {
        console.log('🔍 중복 그룹 유효성 검증 시작');
        
        // 검증 시작
        const response = await window.API.Duplicate.validateAndCleanupDuplicateGroups();
        
        if (response && response.progress) {
            // 성공 메시지 표시
            if (typeof showSuccessMessage === 'function') {
                showSuccessMessage(`🔍 중복 그룹 검증이 시작되었습니다.\n\n📊 진행 상황은 상단의 "현재 진행 중인 작업" 섹션에서 실시간으로 확인할 수 있습니다.\n\n⏱️ 작업이 완료되면 목록이 자동으로 갱신됩니다.`);
            } else {
                alert('🔍 중복 그룹 검증이 시작되었습니다.\n\n📊 진행 상황은 상단의 "현재 진행 중인 작업" 섹션에서 실시간으로 확인할 수 있습니다.\n\n⏱️ 작업이 완료되면 목록이 자동으로 갱신됩니다.');
            }
            
            // 진행 상황 즉시 업데이트
            await updateProgress();
            
            // 5초 후 목록 새로고침 (검증 결과 반영을 위해)
            setTimeout(() => {
                updateRecentDuplicates(true);
            }, 5000);
            
        } else {
            throw new Error('검증 응답이 올바르지 않습니다.');
        }
        
    } catch (error) {
        console.error('중복 그룹 검증 실패:', error);
        
        if (typeof showErrorMessage === 'function') {
            showErrorMessage('중복 그룹 검증 시작 실패: ' + error.message);
        } else {
            alert('❌ 중복 그룹 검증 시작 실패: ' + error.message);
        }
    }
}

// 액션 함수들
async function startFileScan() {
    try {
        if (!confirm('Google Drive에서 파일 메타데이터를 수집하시겠습니까?\n\n이 작업은 시간이 오래 걸릴 수 있습니다.')) {
            return;
        }

        const result = await window.API.File.startScan();
        console.log('파일 스캔 시작:', result);

        alert('✅ 파일 스캔을 시작했습니다!\n\n진행 상황은 대시보드에서 확인하세요.');

        // 폴링 재활성화하여 진행 상황 모니터링
        progressPollingActive = true;
        progressErrorCount = 0;
        updateProgress();

        // 파일 통계도 주기적으로 업데이트 (5초마다)
        const statsUpdateInterval = setInterval(() => {
            if (typeof updateStatusCards === 'function') {
                updateStatusCards();
            }
        }, 5000);

    } catch (error) {
        console.error('파일 스캔 시작 실패:', error);
        alert('❌ 파일 스캔 시작 실패: ' + error.message);
    }
}

async function startScan() {
    // 먼저 재개 가능한 작업이 있는지 확인
    try {
        const resumeCheck = await window.API.Duplicate.checkResumableTasks();

        if (resumeCheck && resumeCheck.hasResumableTask) {
            // 재개 가능한 작업이 있으면 재개/재시작 선택 모달 표시
            showResumeResetModal(resumeCheck.task);
        } else {
            // 재개 가능한 작업이 없으면 바로 옵션 선택 모달 표시
            showDuplicateScanModal();
        }
    } catch (error) {
        console.error('재개 작업 확인 실패:', error);
        // 오류 발생 시에도 스캔 옵션 모달 표시
        showDuplicateScanModal();
    }
}

async function startDuplicateScanWithOptions(calculateHashes = false) {
    try {
        const options = {
            calculateHashes: calculateHashes,
            forceRecalculate: false,
            minFileSize: 1024,
            maxResults: 1000
        };

        await window.API.Duplicate.findDuplicates(options);

        if (calculateHashes) {
            alert('✅ 중복 파일 검사를 시작했습니다!\n\n⚠️ 해시 계산이 포함되어 있어 시간이 오래 걸릴 수 있습니다.\n진행 상황은 대시보드에서 확인하세요.');
        } else {
            alert('✅ 중복 파일 검사를 시작했습니다!\n\n⚠️ 주의: 이전에 계산된 해시만 사용합니다.\n전체 파일을 검사하려면 "해시 계산 포함" 옵션을 선택하세요.');
        }

        // 폴링 재활성화
        progressPollingActive = true;
        progressErrorCount = 0;

        updateProgress();
        updateRecentDuplicates();
    } catch (error) {
        alert('❌ 검사 시작 실패: ' + error.message);
    }
}

function quickDelete(groupId) {
    // 커스텀 확인 모달 사용
    if (typeof confirmQuickDelete === 'function') {
        confirmQuickDelete(groupId);
    } else {
        // 폴백: 기존 confirm 사용
        quickDeleteFallback(groupId);
    }
}

async function quickDeleteFallback(groupId) {
    if (!confirm('이 중복 그룹의 파일들을 삭제하시겠습니까?\n\n⚠️ 이 작업은 되돌릴 수 없습니다.')) return;
    
    try {
        // 더 자세한 사용자 피드백 제공
        const deleteResult = await window.API.Duplicate.deleteDuplicateGroup(groupId);
        
        // 성공 메시지와 함께 UI 업데이트
        if (typeof showSuccessMessage === 'function') {
            showSuccessMessage('중복 그룹이 성공적으로 삭제되었습니다!');
            const modal = document.getElementById('result-notification-modal');
            if (modal) {
                modal.dataset.refreshDashboard = 'true';
            }
        } else {
            alert('✅ 중복 그룹이 성공적으로 삭제되었습니다!');
        }
        
        // 선택 상태에서 제거
        selectedRecentGroups.delete(groupId);
        updateRecentSelectionUI();
        
        // 즉시 대시보드 업데이트 (강제)
        await updateRecentDuplicates(true);
        await updateStatusCards();
        
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
            
            // 선택 상태에서 제거
            selectedRecentGroups.delete(groupId);
            updateRecentSelectionUI();
            
            // 목록 새로고침 (강제)
            await updateRecentDuplicates(true);
            await updateStatusCards();
        } else {
            // 실제 오류인 경우
            if (typeof showErrorMessage === 'function') {
                showErrorMessage('삭제 실패: ' + error.message);
            } else {
                alert('❌ 삭제 실패: ' + error.message);
            }
        }
    }
}

// cleanupFiles 함수는 제거됨 - 이제 showBulkDuplicateManagement()로 대체

function showStats() {
    alert('통계 페이지는 곧 추가됩니다!');
}

// 초기화 및 주기적 업데이트
async function initDashboard() {
    console.log('🚀 대시보드 초기화 시작');
    
    // 페이지네이션 초기화
    currentRecentPage = 1;
    
    await updateStatusCards();
    await updateProgress();
    await updateRecentDuplicates();
    
    // 5초마다 진행 상황 업데이트
    setInterval(updateProgress, 5000);
    
    // 60초마다 상태 카드 업데이트 (간격 늘림)
    setInterval(updateStatusCards, 60000);
    
    // 5분마다 최근 중복 목록 업데이트 (간격 대폭 늘림)
    setInterval(() => {
        // 사용자가 페이지를 보고 있을 때만 업데이트
        if (!document.hidden) {
            updateRecentDuplicates();
        }
    }, 300000); // 5분 = 300초
    
    // 사용자가 탭으로 돌아왔을 때 한 번 새로고침
    document.addEventListener('visibilitychange', () => {
        if (!document.hidden) {
            console.log('👁️ 사용자가 탭으로 돌아옴 - 최근 중복 목록 새로고침');
            updateRecentDuplicates();
        }
    });
    
    console.log('✅ 대시보드 초기화 완료');
}

// DOM 로드 완료 시 초기화
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initDashboard);
} else {
    initDashboard();
}

// 선택된 그룹 중 실제로 존재하지 않는 것들 정리
function cleanupSelectedGroups(currentDuplicates) {
    const currentGroupIds = new Set(currentDuplicates.map(group => group.id));
    const toRemove = [];
    
    selectedRecentGroups.forEach(groupId => {
        if (!currentGroupIds.has(groupId)) {
            toRemove.push(groupId);
        }
    });
    
    toRemove.forEach(groupId => {
        selectedRecentGroups.delete(groupId);
        console.log(`선택 목록에서 제거된 그룹: ${groupId}`);
    });
}

// 최근 중복 파일 선택 관리 함수들
function toggleRecentGroupSelection(groupId) {
    if (selectedRecentGroups.has(groupId)) {
        selectedRecentGroups.delete(groupId);
    } else {
        selectedRecentGroups.add(groupId);
    }
    
    updateRecentSelectionUI();
}

function toggleSelectAllRecentGroups() {
    const selectAllCheckbox = document.getElementById('selectAllRecentGroups');
    const checkboxes = document.querySelectorAll('.recent-group-checkbox');
    
    if (selectAllCheckbox.checked) {
        // 전체 선택
        checkboxes.forEach(checkbox => {
            const groupId = checkbox.dataset.groupId;
            selectedRecentGroups.add(groupId);
            checkbox.checked = true;
            
            // 항목에 선택 스타일 추가
            const item = document.getElementById(`recent-item-${groupId}`);
            if (item) {
                item.classList.add('selected');
            }
        });
    } else {
        // 전체 해제
        checkboxes.forEach(checkbox => {
            const groupId = checkbox.dataset.groupId;
            selectedRecentGroups.delete(groupId);
            checkbox.checked = false;
            
            // 항목에서 선택 스타일 제거
            const item = document.getElementById(`recent-item-${groupId}`);
            if (item) {
                item.classList.remove('selected');
            }
        });
    }
    
    updateRecentSelectionUI();
}

function updateRecentSelectionUI() {
    const selectedCount = selectedRecentGroups.size;
    const selectedCountElement = document.getElementById('selectedRecentCount');
    const deleteButton = document.getElementById('deleteSelectedRecentBtn');
    const selectAllCheckbox = document.getElementById('selectAllRecentGroups');
    const allCheckboxes = document.querySelectorAll('.recent-group-checkbox');
    
    // 선택 개수 업데이트
    if (selectedCountElement) {
        selectedCountElement.textContent = `${selectedCount}개 선택됨`;
    }
    
    // 삭제 버튼 활성화/비활성화
    if (deleteButton) {
        deleteButton.disabled = selectedCount === 0;
    }
    
    // 전체 선택 체크박스 상태 업데이트
    if (selectAllCheckbox && allCheckboxes.length > 0) {
        const checkedCount = document.querySelectorAll('.recent-group-checkbox:checked').length;
        selectAllCheckbox.checked = checkedCount === allCheckboxes.length;
        selectAllCheckbox.indeterminate = checkedCount > 0 && checkedCount < allCheckboxes.length;
    }
    
    // 선택된 항목들에 스타일 적용
    selectedRecentGroups.forEach(groupId => {
        const item = document.getElementById(`recent-item-${groupId}`);
        if (item) {
            item.classList.add('selected');
        }
    });
    
    // 선택되지 않은 항목들에서 스타일 제거
    allCheckboxes.forEach(checkbox => {
        const groupId = checkbox.dataset.groupId;
        if (!selectedRecentGroups.has(groupId)) {
            const item = document.getElementById(`recent-item-${groupId}`);
            if (item) {
                item.classList.remove('selected');
            }
        }
    });
}

// 선택된 최근 중복 그룹들 삭제
async function deleteSelectedRecentGroups() {
    if (selectedRecentGroups.size === 0) {
        alert('삭제할 중복 그룹을 선택해주세요.');
        return;
    }
    
    const groupIds = Array.from(selectedRecentGroups);
    const message = `
        <div style="margin-bottom: 16px;">
            <strong>선택된 ${groupIds.length}개의 중복 그룹을 삭제하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; max-height: 100px; overflow-y: auto; background: #f8f9fa; padding: 8px; border-radius: 4px;">
            ${groupIds.map(id => `• 그룹 ID: ${id}`).join('<br>')}
        </div>
    `;
    
    if (typeof showDeleteConfirmation === 'function') {
        showDeleteConfirmation(message, async () => {
            await deleteSelectedRecentGroupsDirectly();
        });
    } else {
        // 폴백: 기존 confirm 사용
        if (confirm(`선택된 ${groupIds.length}개의 중복 그룹을 삭제하시겠습니까?\n\n⚠️ 이 작업은 되돌릴 수 없습니다.`)) {
            await deleteSelectedRecentGroupsDirectly();
        }
    }
}

async function deleteSelectedRecentGroupsDirectly() {
    if (selectedRecentGroups.size === 0) return;
    
    const groupIds = Array.from(selectedRecentGroups);
    let deletedCount = 0;
    let failedCount = 0;
    
    for (const groupId of groupIds) {
        try {
            await window.API.Duplicate.deleteDuplicateGroup(groupId);
            deletedCount++;
            selectedRecentGroups.delete(groupId);
            
            // DOM에서 즉시 제거 (시각적 피드백)
            const item = document.getElementById(`recent-item-${groupId}`);
            if (item) {
                item.style.transition = 'opacity 0.5s ease, transform 0.5s ease';
                item.style.opacity = '0';
                item.style.transform = 'translateX(-20px)';
                
                setTimeout(() => {
                    if (item.parentNode) {
                        item.parentNode.removeChild(item);
                    }
                }, 500);
            }
            
        } catch (error) {
            console.error(`그룹 ${groupId} 삭제 실패:`, error);
            
            // 404나 500 오류인 경우 이미 삭제된 것으로 간주하고 목록에서 제거
            if (error.message && (error.message.includes('404') || error.message.includes('500') || error.message.includes('찾을 수 없습니다'))) {
                console.log(`그룹 ${groupId}는 이미 삭제된 것으로 간주하여 목록에서 제거`);
                selectedRecentGroups.delete(groupId);
                
                // DOM에서도 제거
                const item = document.getElementById(`recent-item-${groupId}`);
                if (item) {
                    item.style.transition = 'opacity 0.3s ease';
                    item.style.opacity = '0';
                    
                    setTimeout(() => {
                        if (item.parentNode) {
                            item.parentNode.removeChild(item);
                        }
                    }, 300);
                }
                
                // 성공한 것으로 카운트 (이미 삭제됨)
                deletedCount++;
            } else {
                failedCount++;
            }
        }
    }
    
    // 결과 메시지
    let message = `✅ ${deletedCount}개의 중복 그룹이 삭제되었습니다.`;
    if (failedCount > 0) {
        message += `\n❌ ${failedCount}개의 그룹 삭제에 실패했습니다.`;
    }
    
    if (typeof showSuccessMessage === 'function') {
        showSuccessMessage(message);
        const modal = document.getElementById('result-notification-modal');
        if (modal) {
            modal.dataset.refreshDashboard = 'true';
        }
    } else {
        alert(message);
    }
    
    // UI 업데이트
    updateRecentSelectionUI();
    
    // 즉시 목록 새로고침 (삭제 성공 시, 강제)
    if (deletedCount > 0) {
        console.log(`🔄 ${deletedCount}개 그룹 삭제 후 목록 강제 새로고침 시작`);
        await updateRecentDuplicates(true);
        console.log('✅ 목록 강제 새로고침 완료');
    }
    
    // 상태 카드 업데이트 (지연)
    setTimeout(() => {
        if (typeof updateStatusCards === 'function') {
            updateStatusCards();
        }
    }, 1000);
}

// 중복 그룹 검증 및 정리
async function validateDuplicateGroups() {
    // 확인 메시지
    const confirmed = confirm(
        '중복 그룹 검증을 시작하시겠습니까?\n\n' +
        '이 작업은 다음을 수행합니다:\n' +
        '• Google Drive에서 삭제된 파일 확인\n' +
        '• 더 이상 유효하지 않은 중복 그룹 자동 제거\n' +
        '• 2개 미만의 파일만 남은 그룹 정리\n\n' +
        '⚠️ 이 작업은 시간이 걸릴 수 있습니다.'
    );

    if (!confirmed) {
        return;
    }

    try {
        console.log('🔍 중복 그룹 검증 시작');

        // API 호출
        const response = await window.API.Duplicate.validateAndCleanupDuplicateGroups({
            workerCount: 4
        });

        if (response && response.progress) {
            console.log('✅ 검증 작업이 시작되었습니다:', response.progress);

            // 진행 상황 모니터링 시작
            monitorValidationProgress(response.progress.id);

            // 사용자에게 알림
            if (typeof showSuccessMessage === 'function') {
                showSuccessMessage('중복 그룹 검증이 시작되었습니다.\n진행 상황을 확인하세요.');
            } else {
                alert('✅ 중복 그룹 검증이 시작되었습니다.\n진행 상황을 확인하세요.');
            }
        }
    } catch (error) {
        console.error('❌ 중복 그룹 검증 시작 실패:', error);
        alert('중복 그룹 검증을 시작할 수 없습니다:\n' + error.message);
    }
}

// 검증 진행 상황 모니터링 (완료/실패 알림만 처리, UI 업데이트는 전역 updateProgress()에서 담당)
async function monitorValidationProgress(progressId) {
    let isCompleted = false;

    // 진행 상황 폴링 (완료/실패 감지용)
    const pollInterval = setInterval(async () => {
        try {
            const progress = await window.API.Duplicate.getDuplicateProgress();

            if (!progress) {
                clearInterval(pollInterval);
                return;
            }

            // 완료 확인
            if (progress.status === 'completed') {
                clearInterval(pollInterval);
                isCompleted = true;

                // 결과 표시
                if (typeof showSuccessMessage === 'function') {
                    showSuccessMessage(
                        '중복 그룹 검증이 완료되었습니다!\n\n' +
                        (progress.currentStep || '유효하지 않은 중복 그룹이 정리되었습니다.')
                    );
                } else {
                    alert('✅ 중복 그룹 검증이 완료되었습니다!\n\n' + (progress.currentStep || ''));
                }

                // 대시보드 업데이트
                setTimeout(() => {
                    updateStatusCards();
                    updateRecentDuplicates(true);
                }, 1000);

            } else if (progress.status === 'failed') {
                clearInterval(pollInterval);
                alert('❌ 중복 그룹 검증에 실패했습니다:\n' + (progress.errorMessage || '알 수 없는 오류'));
            }

        } catch (error) {
            console.error('진행 상황 조회 중 오류:', error);
            clearInterval(pollInterval);
        }
    }, 2000); // 2초마다 확인

    // 5분 후 자동 종료 (타임아웃)
    setTimeout(() => {
        if (!isCompleted) {
            clearInterval(pollInterval);
            console.warn('⚠️ 검증 진행 상황 모니터링 타임아웃');
        }
    }, 300000);
}

// 중복 검색 옵션 모달
function showDuplicateScanModal() {
    const modal = document.getElementById('duplicateScanModal');
    if (!modal) {
        console.error('중복 검색 모달을 찾을 수 없습니다.');
        // 폴백: 기본 confirm 대화상자
        if (confirm('중복 파일 검사를 시작하시겠습니까?\n\n⚠️ 이전에 계산된 해시만 사용합니다.')) {
            startDuplicateScanWithOptions(false);
        }
        return;
    }
    modal.classList.remove('hidden');
}

function closeDuplicateScanModal() {
    const modal = document.getElementById('duplicateScanModal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

function startDuplicateScanQuick() {
    closeDuplicateScanModal();
    startDuplicateScanWithOptions(false);
}

function startDuplicateScanFull() {
    closeDuplicateScanModal();
    startDuplicateScanWithOptions(true);
}

// 재개/재시작 모달 관련 함수들
function showResumeResetModal(taskInfo) {
    const modal = document.getElementById('resumeResetModal');
    if (!modal) {
        console.error('재개/재시작 모달을 찾을 수 없습니다.');
        // 폴백: 바로 스캔 옵션 모달 표시
        showDuplicateScanModal();
        return;
    }

    // 작업 정보 표시
    if (taskInfo) {
        document.getElementById('resumeBatchNumber').textContent = taskInfo.currentBatch || 0;
        document.getElementById('resumeSuccessCount').textContent = formatNumber(taskInfo.totalSuccessful || 0);
        document.getElementById('resumeFailCount').textContent = formatNumber(taskInfo.totalFailed || 0);

        // 마지막 업데이트 시간 표시
        if (taskInfo.lastUpdated) {
            const lastUpdated = new Date(taskInfo.lastUpdated);
            document.getElementById('resumeLastUpdated').textContent = formatRelativeTime(lastUpdated);
        } else {
            document.getElementById('resumeLastUpdated').textContent = '알 수 없음';
        }
    }

    modal.classList.remove('hidden');
}

function closeResumeResetModal() {
    const modal = document.getElementById('resumeResetModal');
    if (modal) {
        modal.classList.add('hidden');
    }
}

async function resumeDuplicateScan() {
    closeResumeResetModal();

    // 바로 전체 검사 시작 (이어서 계속)
    await startDuplicateScanWithOptions(true);

    alert('✅ 중복 검사를 이어서 진행합니다!\n\n⚠️ 해시 계산이 이전 지점부터 계속됩니다.\n진행 상황은 대시보드에서 확인하세요.');
}

async function resetAndStartNewScan() {
    closeResumeResetModal();

    if (!confirm('⚠️ 정말로 기존 진행 상황을 모두 삭제하고 처음부터 시작하시겠습니까?\n\n이 작업은 되돌릴 수 없습니다.')) {
        // 취소하면 다시 모달 표시
        return;
    }

    try {
        console.log('🗑️ 기존 진행 상황 삭제 중...');

        // 진행 상황 삭제
        const resetResult = await window.API.Duplicate.resetProgress();
        console.log('✅ 진행 상황 삭제 완료:', resetResult);

        // 잠시 대기 후 스캔 옵션 모달 표시
        setTimeout(() => {
            showDuplicateScanModal();
        }, 500);

    } catch (error) {
        console.error('❌ 진행 상황 삭제 실패:', error);
        alert('진행 상황 삭제에 실패했습니다: ' + error.message);
    }
}

// 상대 시간 포맷 함수
function formatRelativeTime(date) {
    const now = new Date();
    const diffMs = now - date;
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);

    if (diffSec < 60) return `${diffSec}초 전`;
    if (diffMin < 60) return `${diffMin}분 전`;
    if (diffHour < 24) return `${diffHour}시간 전`;
    if (diffDay < 30) return `${diffDay}일 전`;

    // 30일 이상이면 날짜 표시
    return date.toLocaleDateString('ko-KR', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}