/**
 * 폴더 비교 관련 JavaScript
 * 폴더 비교, 삭제, 진행 상황 추적 기능들
 */

// 폴더 비교 관련 변수들
let currentComparisonId = null;
let currentComparisonResult = null;  // 현재 비교 결과 저장
let comparisonProgressInterval = null;
let deletionProgressInterval = null;
let comparisonStartTime = null;
let elapsedTimeInterval = null;

// 폴더 비교 모달 관리 함수들
function compareFolders() {
    const modal = document.getElementById('folderComparisonModal');
    if (modal) {
        modal.classList.remove('hidden');
        showStep('folderInputStep');
        resetFolderComparisonModal();
        checkPendingComparisons();
    }
}

function closeFolderComparisonModal() {
    const modal = document.getElementById('folderComparisonModal');
    if (modal) {
        modal.classList.add('hidden');
        clearInterval(comparisonProgressInterval);
        clearInterval(deletionProgressInterval);
        clearInterval(elapsedTimeInterval);
    }
}

function resetFolderComparisonModal() {
    // Clear input values
    const sourceFolderUrl = document.getElementById('sourceFolderUrl');
    const targetFolderUrl = document.getElementById('targetFolderUrl');
    const includeSubfolders = document.getElementById('includeSubfolders');
    const deepComparison = document.getElementById('deepComparison');

    if (sourceFolderUrl) sourceFolderUrl.value = '';
    if (targetFolderUrl) targetFolderUrl.value = '';
    if (includeSubfolders) includeSubfolders.checked = true;
    if (deepComparison) deepComparison.checked = true;

    // Reset folder info cards
    const folderInfoCards = document.getElementById('folderInfoCards');
    if (folderInfoCards) folderInfoCards.style.display = 'none';
    const sourceFolderName = document.getElementById('sourceFolderName');
    const sourceFolderPath = document.getElementById('sourceFolderPath');
    const targetFolderName = document.getElementById('targetFolderName');
    const targetFolderPath = document.getElementById('targetFolderPath');
    if (sourceFolderName) sourceFolderName.textContent = '-';
    if (sourceFolderPath) sourceFolderPath.textContent = '-';
    if (targetFolderName) targetFolderName.textContent = '-';
    if (targetFolderPath) targetFolderPath.textContent = '-';

    // Reset progress
    const comparisonProgressBar = document.getElementById('comparisonProgressBar');
    const comparisonProgressText = document.getElementById('comparisonProgressText');
    const deletionProgressBar = document.getElementById('deletionProgressBar');
    const deletionProgressText = document.getElementById('deletionProgressText');

    if (comparisonProgressBar) comparisonProgressBar.style.width = '0%';
    if (comparisonProgressText) comparisonProgressText.textContent = '비교 작업 준비 중...';
    if (deletionProgressBar) deletionProgressBar.style.width = '0%';
    if (deletionProgressText) deletionProgressText.textContent = '삭제 작업 준비 중...';
    
    // Clear results
    const comparisonSummary = document.getElementById('comparisonSummary');
    const deletionOptions = document.getElementById('deletionOptions');
    const duplicateFilesList = document.getElementById('duplicateFilesList');
    const deletionStatus = document.getElementById('deletionStatus');
    
    if (comparisonSummary) comparisonSummary.innerHTML = '';
    if (deletionOptions) deletionOptions.innerHTML = '';
    if (duplicateFilesList) duplicateFilesList.innerHTML = '';
    if (deletionStatus) deletionStatus.innerHTML = '';
    
    currentComparisonId = null;
}

function showStep(stepId) {
    const steps = ['folderInputStep', 'comparisonProgress', 'comparisonResult', 'deletionProgress'];
    steps.forEach(id => {
        const element = document.getElementById(id);
        if (element) {
            element.style.display = id === stepId ? 'block' : 'none';
        }
    });
}

// 폴더 정보 가져오기
async function getFolderInfo(folderId) {
    try {
        // API 베이스 URL 가져오기 (api.js의 설정 사용)
        const baseUrl = window.API_BASE || 'http://localhost:9090';
        const response = await fetch(`${baseUrl}/api/explorer/file-info?fileId=${encodeURIComponent(folderId)}`);

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        return await response.json();
    } catch (error) {
        console.warn(`폴더 정보 조회 실패 (${folderId}):`, error);
        return null;
    }
}

// 폴더 정보 표시
async function displayFolderInfoCards(sourceFolderId, targetFolderId) {
    const folderInfoCards = document.getElementById('folderInfoCards');
    const sourceFolderName = document.getElementById('sourceFolderName');
    const sourceFolderPath = document.getElementById('sourceFolderPath');
    const targetFolderName = document.getElementById('targetFolderName');
    const targetFolderPath = document.getElementById('targetFolderPath');

    if (!folderInfoCards) return;

    // 카드 표시
    folderInfoCards.style.display = 'flex';

    // 로딩 상태 표시
    if (sourceFolderName) sourceFolderName.innerHTML = '<span class="loading-spinner"><i class="fas fa-spinner fa-spin"></i> 로딩 중...</span>';
    if (targetFolderName) targetFolderName.innerHTML = '<span class="loading-spinner"><i class="fas fa-spinner fa-spin"></i> 로딩 중...</span>';
    if (sourceFolderPath) sourceFolderPath.textContent = '';
    if (targetFolderPath) targetFolderPath.textContent = '';

    // 병렬로 폴더 정보 가져오기
    const [sourceInfo, targetInfo] = await Promise.all([
        getFolderInfo(sourceFolderId),
        getFolderInfo(targetFolderId)
    ]);

    // 기준 폴더 정보 표시
    if (sourceInfo) {
        if (sourceFolderName) sourceFolderName.textContent = sourceInfo.name || '알 수 없음';
        if (sourceFolderPath) sourceFolderPath.textContent = sourceInfo.path || `ID: ${sourceFolderId}`;
    } else {
        if (sourceFolderName) sourceFolderName.textContent = '폴더 정보 없음';
        if (sourceFolderPath) sourceFolderPath.textContent = `ID: ${sourceFolderId}`;
    }

    // 대상 폴더 정보 표시
    if (targetInfo) {
        if (targetFolderName) targetFolderName.textContent = targetInfo.name || '알 수 없음';
        if (targetFolderPath) targetFolderPath.textContent = targetInfo.path || `ID: ${targetFolderId}`;
    } else {
        if (targetFolderName) targetFolderName.textContent = '폴더 정보 없음';
        if (targetFolderPath) targetFolderPath.textContent = `ID: ${targetFolderId}`;
    }
}

// 폴더 비교 시작
async function startFolderComparison() {
    const sourceFolderUrl = document.getElementById('sourceFolderUrl')?.value.trim();
    const targetFolderUrl = document.getElementById('targetFolderUrl')?.value.trim();

    if (!sourceFolderUrl || !targetFolderUrl) {
        alert('기준 폴더와 대상 폴더 URL을 모두 입력해주세요.');
        return;
    }

    try {
        // Extract folder IDs from URLs
        const sourceFolderResponse = await window.API.Comparison.extractFolderIdFromUrl(sourceFolderUrl);
        const targetFolderResponse = await window.API.Comparison.extractFolderIdFromUrl(targetFolderUrl);
        const sourceFolderId = sourceFolderResponse.folderId;
        const targetFolderId = targetFolderResponse.folderId;

        console.log('Starting folder comparison:', { sourceFolderId, targetFolderId });

        showStep('comparisonProgress');

        // 폴더 정보 카드 표시 (비동기로 로드)
        displayFolderInfoCards(sourceFolderId, targetFolderId);

        // 제외할 폴더명 파싱
        const excludeFolderNamesInput = document.getElementById('excludeFolderNames')?.value.trim() || '';
        const excludeFolderNames = excludeFolderNamesInput
            ? excludeFolderNamesInput.split(',').map(name => name.trim()).filter(name => name.length > 0)
            : [];

        const options = {
            includeSubfolders: document.getElementById('includeSubfolders')?.checked || true,
            deepComparison: document.getElementById('deepComparison')?.checked || true,
            forceNewComparison: document.getElementById('forceNewComparison')?.checked || false,
            excludeFolderNames: excludeFolderNames
        };

        if (excludeFolderNames.length > 0) {
            console.log('제외할 폴더명:', excludeFolderNames);
        }

        const response = await window.API.Comparison.compareFolders(sourceFolderId, targetFolderId, options);
        console.log('폴더 비교 API 응답:', response);
        
        if (response && response.progress) {
            // 새로운 비교 작업이 시작된 경우
            console.log('새로운 비교 작업 시작:', response.progress);
            currentComparisonId = response.progress.id;
            startComparisonProgressTracking();
        } else if (response && response.comparisonResult) {
            // 기존 비교 결과가 있는 경우
            displayComparisonResult(response.comparisonResult);
            showStep('comparisonResult');
        } else {
            console.error('예상치 못한 응답 구조:', response);
            throw new Error('비교 작업 시작에 실패했습니다.');
        }

    } catch (error) {
        console.error('Folder comparison error:', error);
        alert('폴더 비교 중 오류가 발생했습니다: ' + error.message);
        showStep('folderInputStep');
    }
}

// 중단된 작업 확인 및 표시
async function checkPendingComparisons() {
    try {
        const pendingComparisons = await window.API.Comparison.getPendingComparisons();
        const pendingSection = document.getElementById('pendingComparisons');
        const pendingList = document.getElementById('pendingComparisonsList');
        
        if (pendingComparisons && pendingComparisons.length > 0) {
            if (pendingSection) pendingSection.style.display = 'block';
            if (pendingList) {
                pendingList.innerHTML = '';
                
                pendingComparisons.forEach(comparison => {
                    const comparisonItem = createPendingComparisonItem(comparison);
                    pendingList.appendChild(comparisonItem);
                });
            }
        } else {
            if (pendingSection) pendingSection.style.display = 'none';
        }
    } catch (error) {
        console.error('중단된 작업 확인 실패:', error);
    }
}

function createPendingComparisonItem(comparison) {
    const item = document.createElement('div');
    item.style.cssText = 'padding: 10px; margin: 5px 0; border: 1px solid #dee2e6; border-radius: 5px; background: white;';
    
    const sourceFolderId = comparison.metadata?.sourceFolderId || '알 수 없음';
    const targetFolderId = comparison.metadata?.targetFolderId || '알 수 없음';
    const currentPhase = comparison.metadata?.currentPhase || '알 수 없음';
    const progressText = `${comparison.processedItems || 0} / ${comparison.totalItems || 0}`;
    
    const statusColor = getStatusColor(comparison.status);
    const phaseText = getPhaseText(currentPhase);
    
    item.innerHTML = `
        <div style="display: flex; justify-content: between; align-items: center;">
            <div style="flex: 1;">
                <div style="font-weight: bold; margin-bottom: 5px;">
                    📂 ${sourceFolderId.substring(0, 8)}... → 📁 ${targetFolderId.substring(0, 8)}...
                </div>
                <div style="font-size: 0.9rem; color: #666;">
                    <span style="color: ${statusColor};">●</span> ${comparison.status} | 
                    ${phaseText} | 진행률: ${progressText}
                </div>
                <div style="font-size: 0.8rem; color: #999; margin-top: 3px;">
                    시작: ${new Date(comparison.startTime).toLocaleString('ko-KR')}
                </div>
            </div>
            <div style="margin-left: 10px;">
                <button class="secondary-btn" onclick="resumeComparison(${comparison.id})" 
                        style="padding: 5px 10px; font-size: 0.8rem;">
                    <i class="fas fa-play"></i> 재개
                </button>
            </div>
        </div>
    `;
    
    return item;
}

// 작업 재개 함수
async function resumeComparison(progressId) {
    try {
        addLogEntry(`🔄 작업 재개 중... (Progress ID: ${progressId})`, 'info');

        const response = await window.API.Comparison.resumeComparison(progressId);

        if (response && response.progress) {
            currentComparisonId = response.progress.id;

            // 진행 상황 화면으로 전환
            showStep('comparisonProgress');

            // 중단된 작업 섹션 숨기기
            const pendingSection = document.getElementById('pendingComparisons');
            if (pendingSection) pendingSection.style.display = 'none';

            // 폴더 정보 카드 표시 (메타데이터에서 폴더 ID 가져오기)
            const metadata = response.progress.metadata;
            if (metadata && metadata.sourceFolderId && metadata.targetFolderId) {
                displayFolderInfoCards(metadata.sourceFolderId, metadata.targetFolderId);
            }

            // 진행 상황 추적 시작
            startComparisonProgressTracking();

            addLogEntry('✅ 작업이 성공적으로 재개되었습니다!', 'success');
        } else {
            throw new Error('작업 재개 응답이 올바르지 않습니다.');
        }
    } catch (error) {
        console.error('작업 재개 실패:', error);
        alert('작업 재개에 실패했습니다: ' + error.message);
        addLogEntry('❌ 작업 재개에 실패했습니다: ' + error.message, 'error');
    }
}

// 작업 취소
async function cancelComparison() {
    if (!currentComparisonId) {
        alert('취소할 작업이 없습니다.');
        return;
    }

    const confirmCancel = confirm('정말로 작업을 중단하시겠습니까?\n현재까지의 진행 상황은 저장되지 않습니다.');
    if (!confirmCancel) {
        return;
    }

    const baseUrl = window.API_BASE || 'http://localhost:9090';

    try {
        // 중단 버튼 비활성화
        const cancelBtn = document.getElementById('cancelComparisonBtn');
        if (cancelBtn) {
            cancelBtn.disabled = true;
            cancelBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 중단 중...';
        }

        const response = await fetch(`${baseUrl}/api/compare/cancel`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ progressId: currentComparisonId })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText || '작업 중단 실패');
        }

        // 진행 상황 추적 중지
        clearInterval(comparisonProgressInterval);
        clearInterval(elapsedTimeInterval);

        // 로그에 기록
        addLogEntry('🛑 작업이 사용자에 의해 중단되었습니다.', 'warning');

        // 알림 표시
        alert('작업이 중단되었습니다.');

        // 모달 닫기
        closeFolderComparisonModal();

    } catch (error) {
        console.error('Cancel comparison error:', error);
        alert('작업 중단 중 오류가 발생했습니다: ' + error.message);

        // 버튼 복구
        const cancelBtn = document.getElementById('cancelComparisonBtn');
        if (cancelBtn) {
            cancelBtn.disabled = false;
            cancelBtn.innerHTML = '<i class="fas fa-stop-circle"></i> 작업 중단';
        }
    }
}

// 진행 상황 추적 시작
function startComparisonProgressTracking() {
    if (!currentComparisonId) return;
    comparisonStartTime = Date.now();
    
    // 로그 초기화
    initializeProgressLog();

    comparisonProgressInterval = setInterval(async () => {
        try {
            const progress = await window.API.Comparison.getComparisonProgress(currentComparisonId);
            
            if (progress) {
                updateDetailedProgress(progress);
                
                const percentage = progress.totalItems > 0 ? Math.round((progress.processedItems / progress.totalItems) * 100) : 0;
                const comparisonProgressBar = document.getElementById('comparisonProgressBar');
                const comparisonProgressText = document.getElementById('comparisonProgressText');
                
                if (comparisonProgressBar) comparisonProgressBar.style.width = percentage + '%';
                if (comparisonProgressText) {
                    comparisonProgressText.textContent = `${progress.currentStep || '비교 진행 중...'} (${percentage}%)`;
                }

                if (progress.status === 'completed') {
                    clearInterval(comparisonProgressInterval);
                    clearInterval(elapsedTimeInterval);
                    addLogEntry('✅ 폴더 비교가 완료되었습니다!', 'success');
                    await loadComparisonResult();
                } else if (progress.status === 'failed') {
                    clearInterval(comparisonProgressInterval);
                    clearInterval(elapsedTimeInterval);
                    addLogEntry('❌ 폴더 비교에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'), 'error');
                    alert('폴더 비교에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'));
                    showStep('folderInputStep');
                } else if (progress.status === 'cancelled') {
                    clearInterval(comparisonProgressInterval);
                    clearInterval(elapsedTimeInterval);
                    addLogEntry('🛑 작업이 취소되었습니다.', 'warning');
                    showStep('folderInputStep');
                }
            }
        } catch (error) {
            console.error('Progress tracking error:', error);
            addLogEntry('⚠️ 진행 상황 확인 중 오류 발생: ' + error.message, 'warning');
        }
    }, 1000);
    
    // 경과 시간 업데이트용 별도 타이머
    elapsedTimeInterval = setInterval(updateElapsedTime, 1000);
}

// 상세 진행 상황 업데이트
function updateDetailedProgress(progress) {
    // 폴더 정보 업데이트 (처음 한 번만)
    if (!updateDetailedProgress.foldersInitialized) {
        updateFolderNames();
        updateDetailedProgress.foldersInitialized = true;
    }
    
    // 현재 작업 상태 업데이트
    const currentTask = document.getElementById('currentTask');
    if (currentTask) {
        currentTask.textContent = progress.currentStep || '비교 진행 중...';
    }
    
    // 처리된 파일 수 업데이트 (스캔 중에는 실시간 카운터 표시)
    const processedFiles = document.getElementById('processedFiles');
    if (processedFiles) {
        const metadata = progress.metadata || {};
        const scannedFileCount = metadata.scannedFileCount || 0;
        const scannedFolderCount = metadata.scannedFolderCount || 0;
        const scanStartTime = metadata.scanStartTime || 0;
        const currentPhase = metadata.currentPhase || '';

        // 스캔 중일 때 실시간 카운터와 속도 표시
        if (currentPhase === 'scanning_source' || currentPhase === 'scanning_target') {
            const speed = calculateScanSpeed(scannedFileCount, scanStartTime);
            const speedText = speed > 0 ? ` (${speed.toFixed(1)} 파일/초)` : '';
            processedFiles.textContent = `📂 ${scannedFolderCount}개 폴더, 📄 ${scannedFileCount}개 파일${speedText}`;
            processedFiles.style.color = '#3b82f6';
        } else {
            // 기존 로직: 해시 계산 등 다른 단계
            const completed = progress.processedItems || 0;
            const total = progress.totalItems || 0;
            processedFiles.textContent = total > 0 ? `${completed} / ${total}개` : `${completed}개`;
            processedFiles.style.color = '';
        }
    }
    
    // 해시 계산 진행 상황 업데이트
    const hashProgress = document.getElementById('hashProgress');
    if (hashProgress) {
        if (progress.currentStep && progress.currentStep.includes('해시')) {
            const percentage = progress.totalItems > 0 ? Math.round((progress.processedItems / progress.totalItems) * 100) : 0;
            hashProgress.textContent = `${percentage}% 완료`;
            hashProgress.style.color = '#3b82f6';
        } else if (progress.status === 'completed') {
            hashProgress.textContent = '완료';
            hashProgress.style.color = '#10b981';
        } else if (progress.currentStep && !progress.currentStep.includes('해시')) {
            hashProgress.textContent = '대기 중';
            hashProgress.style.color = '#6b7280';
        }
    }
    
    // 상세 진행 상황별 처리
    if (progress.currentStep) {
        const sourceProgress = document.getElementById('sourceProgress');
        const targetProgress = document.getElementById('targetProgress');
        const metadata = progress.metadata || {};
        const lastScannedPath = metadata.lastScannedPath || '';
        const truncatedPath = lastScannedPath.length > 30 ? '...' + lastScannedPath.slice(-27) : lastScannedPath;

        if (progress.currentStep.includes('기준 폴더')) {
            if (sourceProgress) {
                // 현재 스캔 중인 경로 표시
                sourceProgress.textContent = truncatedPath ? `📂 ${truncatedPath}` : '파일 조회 중...';
                sourceProgress.style.color = '#3b82f6';
                sourceProgress.title = lastScannedPath; // 전체 경로를 툴팁으로
            }
        } else if (progress.currentStep.includes('대상 폴더')) {
            if (sourceProgress) {
                sourceProgress.textContent = '✅ 완료';
                sourceProgress.style.color = '#10b981';
                sourceProgress.title = '';
            }
            if (targetProgress) {
                targetProgress.textContent = truncatedPath ? `📂 ${truncatedPath}` : '파일 조회 중...';
                targetProgress.style.color = '#3b82f6';
                targetProgress.title = lastScannedPath;
            }
        } else if (progress.currentStep.includes('해시 계산') || progress.currentStep.includes('중복 파일 검색')) {
            if (sourceProgress) {
                sourceProgress.textContent = '✅ 완료';
                sourceProgress.style.color = '#10b981';
                sourceProgress.title = '';
            }
            if (targetProgress) {
                targetProgress.textContent = '✅ 완료';
                targetProgress.style.color = '#10b981';
                targetProgress.title = '';
            }
        }
    }
    
    // 실시간 로그 추가
    if (progress.currentStep && progress.currentStep !== updateDetailedProgress.lastStep) {
        updateDetailedProgress.lastStep = progress.currentStep;
        addLogEntry('🔍 ' + progress.currentStep, 'info');
    }
    
    // 중요한 이정표에서 로그 추가
    if (progress.processedItems && progress.totalItems) {
        const percentage = Math.round((progress.processedItems / progress.totalItems) * 100);
        if (percentage > 0 && percentage % 25 === 0 && percentage !== updateDetailedProgress.lastPercentage) {
            updateDetailedProgress.lastPercentage = percentage;
            addLogEntry(`📊 진행률: ${percentage}% (${progress.processedItems}/${progress.totalItems}개 파일 처리)`, 'info');
        }
    }
}

function updateFolderNames() {
    const sourceFolderUrl = document.getElementById('sourceFolderUrl')?.value || '';
    const targetFolderUrl = document.getElementById('targetFolderUrl')?.value || '';
    
    // 폴더 이름 또는 ID 표시
    const sourceProgress = document.getElementById('sourceProgress');
    const targetProgress = document.getElementById('targetProgress');
    
    if (sourceProgress) {
        const sourceName = extractFolderNameFromUrl(sourceFolderUrl) || '기준 폴더';
        sourceProgress.textContent = sourceName;
    }
    
    if (targetProgress) {
        const targetName = extractFolderNameFromUrl(targetFolderUrl) || '대상 폴더';
        targetProgress.textContent = targetName;
    }
}

function updateElapsedTime() {
    if (!comparisonStartTime) return;

    const elapsedTimeElement = document.getElementById('elapsedTime');
    if (elapsedTimeElement) {
        elapsedTimeElement.textContent = formatElapsedTime(comparisonStartTime);
    }
}

// 스캔 속도 계산 (파일/초)
function calculateScanSpeed(fileCount, scanStartTime) {
    if (!scanStartTime || fileCount <= 0) return 0;
    const elapsedMs = Date.now() - scanStartTime;
    if (elapsedMs <= 0) return 0;
    return (fileCount / elapsedMs) * 1000; // files per second
}

function initializeProgressLog() {
    // 진행 상황 추적 변수 초기화
    updateDetailedProgress.lastStep = null;
    updateDetailedProgress.lastPercentage = null;
    updateDetailedProgress.foldersInitialized = false;
    
    // 로그 컨테이너 초기화
    const logContainer = document.getElementById('progressLog');
    if (logContainer) {
        logContainer.innerHTML = '<div class="log-entry">비교 작업을 시작합니다...</div>';
    }
    
    // 상세 정보 초기화
    const elements = {
        'sourceProgress': '-',
        'targetProgress': '-',
        'currentTask': '시작 중...',
        'processedFiles': '0개',
        'elapsedTime': '0초',
        'hashProgress': '대기 중'
    };
    
    Object.entries(elements).forEach(([id, value]) => {
        const element = document.getElementById(id);
        if (element) {
            element.textContent = value;
            element.style.color = '';
        }
    });
    
    addLogEntry('🚀 폴더 비교 작업을 시작합니다...', 'info');
}

function addLogEntry(message, type = 'default') {
    const logContainer = document.getElementById('progressLog');
    if (!logContainer) return;
    
    const logEntry = document.createElement('div');
    logEntry.className = `log-entry ${type}`;
    
    // 현재 시간을 추가
    const timeStr = formatTime();
    
    logEntry.textContent = `[${timeStr}] ${message}`;
    
    // 새 로그를 맨 위에 추가
    logContainer.insertBefore(logEntry, logContainer.firstChild);
    
    // 로그가 너무 많으면 오래된 것들 삭제 (최대 50개 유지)
    while (logContainer.children.length > 50) {
        logContainer.removeChild(logContainer.lastChild);
    }
}

// 비교 결과 로드
async function loadComparisonResult() {
    try {
        const sourceFolderUrl = document.getElementById('sourceFolderUrl')?.value;
        const targetFolderUrl = document.getElementById('targetFolderUrl')?.value;
        
        if (!sourceFolderUrl || !targetFolderUrl) return;
        
        const sourceFolderResponse = await window.API.Comparison.extractFolderIdFromUrl(sourceFolderUrl);
        const targetFolderResponse = await window.API.Comparison.extractFolderIdFromUrl(targetFolderUrl);
        
        const result = await window.API.Comparison.loadSavedComparison(
            sourceFolderResponse.folderId, 
            targetFolderResponse.folderId
        );
        
        if (result) {
            displayComparisonResult(result);
            showStep('comparisonResult');
        } else {
            throw new Error('비교 결과를 불러올 수 없습니다.');
        }
    } catch (error) {
        console.error('Failed to load comparison result:', error);
        alert('비교 결과를 불러오는 중 오류가 발생했습니다: ' + error.message);
        showStep('folderInputStep');
    }
}

// 비교 결과 표시
function displayComparisonResult(result) {
    // 디버그 로그
    console.log('displayComparisonResult - full result:', result);
    console.log('displayComparisonResult - duplicateCount:', result?.duplicateCount);
    console.log('displayComparisonResult - duplicateFiles:', result?.duplicateFiles);
    console.log('displayComparisonResult - uniqueInTarget:', result?.uniqueInTarget);

    // Set the current comparison ID for deletion operations
    currentComparisonId = result.id;

    // Store the full comparison result for later use
    currentComparisonResult = result;
    
    // Get target folder ID
    const targetId = result.targetFolderId || result.TargetFolderID || result.target_folder_id || result.targetFolderID;
    
    const summaryHtml = `
        <div class="comparison-summary">
            <h4>📊 비교 결과</h4>
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin: 15px 0;">
                <div>
                    <strong>기준 폴더:</strong> ${result.sourceFolderName}<br>
                    <small>파일 수: ${result.sourceFileCount}개, 크기: ${formatFileSize(result.sourceTotalSize)}</small>
                </div>
                <div>
                    <strong>대상 폴더:</strong> ${result.targetFolderName}<br>
                    <small>파일 수: ${result.targetFileCount}개, 크기: ${formatFileSize(result.targetTotalSize)}</small>
                </div>
            </div>
            <div style="text-align: center; margin: 20px 0;">
                <div style="font-size: 1.5rem; font-weight: bold; color: ${result.duplicationPercentage === 100 ? '#ef4444' : '#f59e0b'};">
                    ${result.duplicationPercentage.toFixed(1)}% 중복
                </div>
                <div style="color: #666; margin-top: 5px;">
                    ${result.duplicateCount || 0}개 중복 파일, ${formatFileSize(result.duplicateSize || 0)} 절약 가능
                </div>
            </div>
        </div>
    `;
    
    const comparisonSummary = document.getElementById('comparisonSummary');
    if (comparisonSummary) comparisonSummary.innerHTML = summaryHtml;
    
    // Show deletion options
    displayDeletionOptions(result);
    
    // Show duplicate files list if not 100% duplicated
    if (result.duplicationPercentage < 100 && result.duplicateFiles && result.duplicateFiles.length > 0) {
        displayDuplicateFilesList(result.duplicateFiles);
    }
}

// 삭제 옵션 표시
function displayDeletionOptions(result) {
    let optionsHtml = '';
    
    // Find the target folder ID using multiple possible field names
    const targetId = result.targetFolderId || result.TargetFolderID || result.target_folder_id || result.targetFolderID;
    
    if (result.duplicationPercentage === 100) {
        // 100% 중복 - 폴더 전체 삭제 가능
        if (targetId) {
            optionsHtml = `
                <div class="deletion-options">
                    <button class="deletion-btn delete-folder" onclick="deleteTargetFolder('${targetId}')">
                        <i class="fas fa-folder-minus"></i>
                        대상 폴더 전체 삭제
                        <div style="font-size: 0.8rem; margin-top: 4px;">
                            모든 파일이 기준 폴더에 있으므로 안전하게 삭제 가능
                        </div>
                    </button>
                </div>
            `;
        } else {
            optionsHtml = `
                <div class="deletion-options">
                    <div style="color: #dc2626; padding: 10px; background: #fef2f2; border-radius: 6px;">
                        ⚠️ 대상 폴더 ID를 찾을 수 없어 삭제 버튼을 표시할 수 없습니다.
                        <br>브라우저 개발자 도구(F12) → Console 탭에서 디버그 정보를 확인해주세요.
                        <br><br>
                        <button onclick="location.reload()" style="padding: 4px 8px; background: #dc2626; color: white; border: none; border-radius: 4px;">
                            페이지 새로고침 후 다시 시도
                        </button>
                    </div>
                </div>
            `;
        }
    } else if (result.duplicateCount > 0) {
        // 부분 중복 - 중복 파일만 삭제 가능
        const uniqueInTargetCount = result.uniqueInTarget ? result.uniqueInTarget.length : 0;
        const uniqueFilesSection = uniqueInTargetCount > 0 ? `
            <div style="margin-top: 15px; padding: 15px; background: #f0fdf4; border-radius: 8px; border: 1px solid #22c55e;">
                <h5 style="margin: 0 0 10px 0; color: #15803d;">
                    <i class="fas fa-arrow-right"></i> 비중복 파일 이동 가능 (${uniqueInTargetCount}개)
                </h5>
                <p style="margin: 0 0 10px 0; color: #166534; font-size: 0.9rem;">
                    대상 폴더에만 있는 파일들을 기준 폴더로 이동할 수 있습니다.
                </p>
                <button class="deletion-btn" onclick="showMoveUniqueFilesModal(${result.id})"
                        style="background: #22c55e; color: white; margin-top: 5px;">
                    <i class="fas fa-folder-open"></i>
                    비중복 파일 기준 폴더로 이동
                </button>
            </div>
        ` : '';

        optionsHtml = `
            <div class="deletion-options">
                <button class="deletion-btn delete-files" onclick="deleteDuplicateFiles()">
                    <i class="fas fa-files-medical"></i>
                    중복 파일만 삭제 (${result.duplicateCount}개)
                </button>
            </div>
            <div style="margin-top: 10px;">
                <label>
                    <input type="checkbox" id="deleteEmptyFolders" checked>
                    삭제 후 빈 폴더도 함께 정리
                </label>
            </div>
            ${uniqueFilesSection}
        `;
    } else {
        // 중복이 없을 때도 비중복 파일 이동 옵션 표시
        const uniqueInTargetCount = result.uniqueInTarget ? result.uniqueInTarget.length : 0;
        if (uniqueInTargetCount > 0) {
            optionsHtml = `
                <div style="text-align: center; color: #059669; font-weight: 600; margin-bottom: 15px;">
                    🎉 중복 파일이 없습니다!
                </div>
                <div style="padding: 15px; background: #f0fdf4; border-radius: 8px; border: 1px solid #22c55e;">
                    <h5 style="margin: 0 0 10px 0; color: #15803d;">
                        <i class="fas fa-arrow-right"></i> 비중복 파일 이동 가능 (${uniqueInTargetCount}개)
                    </h5>
                    <p style="margin: 0 0 10px 0; color: #166534; font-size: 0.9rem;">
                        대상 폴더에만 있는 파일들을 기준 폴더로 이동할 수 있습니다.
                    </p>
                    <button class="deletion-btn" onclick="showMoveUniqueFilesModal(${result.id})"
                            style="background: #22c55e; color: white; margin-top: 5px;">
                        <i class="fas fa-folder-open"></i>
                        비중복 파일 기준 폴더로 이동
                    </button>
                </div>
            `;
        } else {
            optionsHtml = `
                <div style="text-align: center; color: #059669; font-weight: 600;">
                    🎉 중복 파일이 없습니다!
                </div>
            `;
        }
    }
    
    const deletionOptions = document.getElementById('deletionOptions');
    if (deletionOptions) deletionOptions.innerHTML = optionsHtml;
}

// 중복 파일 목록 표시 (페이지네이션 포함)
function displayDuplicateFilesList(duplicateFiles) {
    if (!duplicateFiles || duplicateFiles.length === 0) return;
    
    // 페이지네이션 설정
    const itemsPerPage = 10; // 페이지당 표시할 파일 수
    const totalPages = Math.ceil(duplicateFiles.length / itemsPerPage);
    let currentPage = 1;
    
    function renderPage(page) {
        const startIndex = (page - 1) * itemsPerPage;
        const endIndex = startIndex + itemsPerPage;
        const pageFiles = duplicateFiles.slice(startIndex, endIndex);
        
        const listHtml = `
            <h4>📋 중복 파일 목록 (총 ${duplicateFiles.length}개)</h4>
            
            <!-- 페이지네이션 상단 -->
            ${totalPages > 1 ? `
                <div class="pagination-info" style="text-align: center; margin: 10px 0; color: #666; font-size: 0.9rem;">
                    페이지 ${page} / ${totalPages} (${startIndex + 1}-${Math.min(endIndex, duplicateFiles.length)}번째 파일)
                </div>
                <div class="pagination-controls" style="text-align: center; margin: 15px 0;">
                    <button onclick="displayDuplicateFilesPage(${page - 1})" ${page <= 1 ? 'disabled' : ''} 
                            style="padding: 8px 12px; margin: 0 5px; border: 1px solid #ddd; background: ${page <= 1 ? '#f5f5f5' : 'white'}; border-radius: 4px; cursor: ${page <= 1 ? 'not-allowed' : 'pointer'};">
                        <i class="fas fa-chevron-left"></i> 이전
                    </button>
                    <span style="margin: 0 15px; font-weight: 500;">${page} / ${totalPages}</span>
                    <button onclick="displayDuplicateFilesPage(${page + 1})" ${page >= totalPages ? 'disabled' : ''} 
                            style="padding: 8px 12px; margin: 0 5px; border: 1px solid #ddd; background: ${page >= totalPages ? '#f5f5f5' : 'white'}; border-radius: 4px; cursor: ${page >= totalPages ? 'not-allowed' : 'pointer'};">
                        다음 <i class="fas fa-chevron-right"></i>
                    </button>
                </div>
            ` : ''}
            
            <!-- 스크롤 가능한 파일 목록 -->
            <div class="file-list-container" style="max-height: 400px; overflow-y: auto; border: 1px solid #e2e8f0; border-radius: 8px; background: #f8fafc;">
                <div class="file-list">
                    ${pageFiles.map(file => `
                        <div class="file-item-deletion" id="file-${file.id}" style="padding: 12px; border-bottom: 1px solid #e2e8f0; display: flex; justify-content: space-between; align-items: center;">
                            <div style="flex: 1;">
                                <div style="font-weight: 500; margin-bottom: 4px;">${file.name}</div>
                                <div style="font-size: 0.875rem; color: #666;">
                                    ${formatFileSize(file.size)} • ${new Date(file.modifiedTime).toLocaleDateString()}
                                </div>
                            </div>
                            <div class="deletion-status" id="status-${file.id}" style="text-align: right; font-size: 0.85rem;">
                                <i class="fas fa-clock" style="color: #6b7280;"></i>
                                대기 중
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
            
            <!-- 페이지네이션 하단 -->
            ${totalPages > 1 ? `
                <div class="pagination-controls" style="text-align: center; margin: 15px 0;">
                    <button onclick="displayDuplicateFilesPage(${page - 1})" ${page <= 1 ? 'disabled' : ''} 
                            style="padding: 8px 12px; margin: 0 5px; border: 1px solid #ddd; background: ${page <= 1 ? '#f5f5f5' : 'white'}; border-radius: 4px; cursor: ${page <= 1 ? 'not-allowed' : 'pointer'};">
                        <i class="fas fa-chevron-left"></i> 이전
                    </button>
                    <span style="margin: 0 15px; font-weight: 500;">${page} / ${totalPages}</span>
                    <button onclick="displayDuplicateFilesPage(${page + 1})" ${page >= totalPages ? 'disabled' : ''} 
                            style="padding: 8px 12px; margin: 0 5px; border: 1px solid #ddd; background: ${page >= totalPages ? '#f5f5f5' : 'white'}; border-radius: 4px; cursor: ${page >= totalPages ? 'not-allowed' : 'pointer'};">
                        다음 <i class="fas fa-chevron-right"></i>
                    </button>
                </div>
            ` : ''}
        `;
        
        const duplicateFilesList = document.getElementById('duplicateFilesList');
        if (duplicateFilesList) {
            duplicateFilesList.innerHTML = listHtml;
        }
    }
    
    // 전역 함수로 페이지 변경 함수 등록
    window.displayDuplicateFilesPage = function(page) {
        if (page >= 1 && page <= totalPages) {
            currentPage = page;
            renderPage(currentPage);
        }
    };
    
    // 전역 변수로 현재 파일 목록 저장 (삭제 시 필요)
    window.currentDuplicateFiles = duplicateFiles;
    
    // 첫 페이지 렌더링
    renderPage(currentPage);
}

// 대상 폴더 삭제
async function deleteTargetFolder(targetFolderId) {
    const message = `
        <div style="margin-bottom: 16px;">
            <strong style="color: #ef4444;">⚠️ 대상 폴더 전체 삭제</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; line-height: 1.5;">
            대상 폴더를 삭제하시겠습니까?
        </div>
        <div style="margin-top: 12px; font-size: 0.85rem; color: #6b7280;">
            폴더 ID: <code style="background: #f3f4f6; padding: 2px 6px; border-radius: 3px;">${targetFolderId}</code>
        </div>
    `;

    // Google Drive 파일 삭제이므로 휴지통/완전삭제 옵션 표시
    showDeleteConfirmation(message, async (data) => {
        await deleteTargetFolderDirectly(targetFolderId, data?.permanentDelete || false);
    }, null, true);
}

async function deleteTargetFolderDirectly(targetFolderId, permanentDelete = false) {

    showStep('deletionProgress');

    try {
        const response = await window.API.Comparison.deleteTargetFolder(currentComparisonId, targetFolderId, permanentDelete);

        if (response && response.progress) {
            startDeletionProgressTracking(response.progress.id, 'folder');
        } else {
            throw new Error('폴더 삭제 작업 시작에 실패했습니다.');
        }

    } catch (error) {
        console.error('Target folder deletion error:', error);
        alert('폴더 삭제 중 오류가 발생했습니다: ' + error.message);
        showStep('comparisonResult');
    }
}

// 중복 파일 삭제
async function deleteDuplicateFiles() {
    const fileCount = window.currentDuplicateFiles ? window.currentDuplicateFiles.length : 0;

    const message = `
        <div style="margin-bottom: 16px;">
            <strong>중복 파일 삭제</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; line-height: 1.5;">
            ${fileCount}개의 중복 파일을 삭제하시겠습니까?
        </div>
        <div style="margin-top: 8px; font-size: 0.85rem; color: #6b7280;">
            ${document.getElementById('deleteEmptyFolders')?.checked ? '✅ 삭제 후 빈 폴더도 함께 정리됩니다.' : ''}
        </div>
    `;

    // Google Drive 파일 삭제이므로 휴지통/완전삭제 옵션 표시
    showDeleteConfirmation(message, async (data) => {
        await deleteDuplicateFilesDirectly(data?.permanentDelete || false);
    }, null, true);
}

async function deleteDuplicateFilesDirectly(permanentDelete = false) {

    // Get file IDs from the stored duplicate files (전체 목록에서)
    const fileIds = window.currentDuplicateFiles ?
        window.currentDuplicateFiles.map(file => file.id) :
        Array.from(document.querySelectorAll('.file-item-deletion')).map(el => el.id.replace('file-', ''));

    if (fileIds.length === 0) {
        alert('삭제할 파일이 없습니다.');
        return;
    }

    showStep('deletionProgress');

    try {
        const deleteEmptyFolders = document.getElementById('deleteEmptyFolders')?.checked || false;
        const response = await window.API.Comparison.deleteDuplicateFiles(currentComparisonId, fileIds, deleteEmptyFolders, permanentDelete);

        if (response && response.progress) {
            startDeletionProgressTracking(response.progress.id, 'files');
        } else {
            throw new Error('파일 삭제 작업 시작에 실패했습니다.');
        }

    } catch (error) {
        console.error('Duplicate files deletion error:', error);
        alert('파일 삭제 중 오류가 발생했습니다: ' + error.message);
        showStep('comparisonResult');
    }
}

// 삭제 진행 상황 추적
function startDeletionProgressTracking(progressId, type) {
    deletionProgressInterval = setInterval(async () => {
        try {
            const progress = await window.API.Comparison.getComparisonProgress(progressId);
            
            if (progress) {
                console.log('Deletion progress:', progress); // 디버그 로그
                
                // 다양한 필드명 시도 (totalItems/processedItems 또는 total/completed)
                const total = progress.totalItems || progress.total || 0;
                const processed = progress.processedItems || progress.completed || 0;
                const percentage = total > 0 ? Math.round((processed / total) * 100) : 0;
                
                const deletionProgressBar = document.getElementById('deletionProgressBar');
                const deletionProgressText = document.getElementById('deletionProgressText');
                
                if (deletionProgressBar) deletionProgressBar.style.width = percentage + '%';

                // progress-percentage 텍스트도 함께 갱신
                const deletionPercentageEl = deletionProgressBar?.closest('.progress-item')?.querySelector('.progress-percentage');
                if (deletionPercentageEl) deletionPercentageEl.textContent = percentage + '%';

                if (deletionProgressText) {
                    deletionProgressText.textContent = `${progress.currentStep || '삭제 진행 중...'} (${percentage}%)`;
                }

                // 빈 폴더 정리 현황 실시간 표시
                const cleanupFolders = progress.metadata?.cleanupFolders;
                if (cleanupFolders && Array.isArray(cleanupFolders)) {
                    const logContainer = document.getElementById('emptyFolderCleanupLog');
                    const listEl = document.getElementById('cleanupFolderList');
                    const processedEl = document.getElementById('cleanupProcessedCount');
                    const totalEl = document.getElementById('cleanupTotalCount');

                    if (logContainer) logContainer.style.display = 'block';
                    if (processedEl) processedEl.textContent = progress.metadata.cleanupProcessed || 0;
                    if (totalEl) totalEl.textContent = progress.metadata.cleanupTotal || 0;

                    if (listEl) {
                        const wasAtBottom = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 30;
                        listEl.innerHTML = cleanupFolders.map(f => {
                            let icon = '❌';
                            if (f.status === 'deleted') icon = '✅';
                            else if (f.status === 'skipped') icon = '⏭️';
                            return `<div class="cleanup-folder-item"><span>${icon}</span><span>${f.name}</span></div>`;
                        }).join('');
                        if (wasAtBottom) listEl.scrollTop = listEl.scrollHeight;
                    }
                }

                if (progress.status === 'completed') {
                    clearInterval(deletionProgressInterval);
                    showDeletionComplete(type);
                } else if (progress.status === 'failed') {
                    clearInterval(deletionProgressInterval);
                    alert('삭제 작업에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'));
                    showStep('comparisonResult');
                }
            }
        } catch (error) {
            console.error('Deletion progress tracking error:', error);
        }
    }, 1000);
}

// 삭제 완료 표시
function showDeletionComplete(type) {
    const message = type === 'folder'
        ? '✅ 대상 폴더가 성공적으로 삭제되었습니다!'
        : '✅ 중복 파일들이 성공적으로 삭제되었습니다!';

    // 디버그 로그
    console.log('showDeletionComplete - type:', type);
    console.log('showDeletionComplete - currentComparisonResult:', currentComparisonResult);
    console.log('showDeletionComplete - uniqueInTarget:', currentComparisonResult?.uniqueInTarget);

    // 🔄 중복 파일 삭제 완료 시 로컬 데이터 업데이트
    if (type !== 'folder' && currentComparisonResult) {
        currentComparisonResult.duplicateFiles = [];
        currentComparisonResult.duplicateCount = 0;
        console.log('✅ 중복 파일 데이터 초기화 완료');
    }

    // 비중복 파일이 남아있는지 확인 (폴더 전체 삭제가 아닌 경우에만)
    const hasUniqueFiles = type !== 'folder' && currentComparisonResult &&
        currentComparisonResult.uniqueInTarget &&
        currentComparisonResult.uniqueInTarget.length > 0;

    console.log('showDeletionComplete - hasUniqueFiles:', hasUniqueFiles);

    const deletionStatus = document.getElementById('deletionStatus');
    if (deletionStatus) {
        // 비중복 파일이 있으면 이동 옵션 추가
        const nextStepHtml = hasUniqueFiles ? `
            <div style="margin-top: 20px; padding: 15px; background: #f0fdf4; border-radius: 8px; border: 1px solid #22c55e;">
                <h5 style="margin: 0 0 10px 0; color: #15803d;">
                    <i class="fas fa-arrow-right"></i> 다음 단계
                </h5>
                <p style="margin: 0 0 10px 0; color: #166534; font-size: 0.9rem;">
                    대상 폴더에만 있는 ${currentComparisonResult.uniqueInTarget.length}개의 비중복 파일을
                    기준 폴더로 이동하시겠습니까?
                </p>
            </div>
        ` : '';

        const buttonsHtml = hasUniqueFiles ? `
            <div style="text-align: center; margin-top: 20px; display: flex; gap: 10px; justify-content: center; flex-wrap: wrap;">
                <button class="action-btn" style="background: #6b7280;" onclick="closeFolderComparisonModal()">
                    <i class="fas fa-check"></i> 완료
                </button>
                <button class="action-btn" style="background: #22c55e;" onclick="proceedToMoveUniqueFiles();">
                    <i class="fas fa-folder-open"></i> 다음: 비중복 파일 이동
                </button>
            </div>
        ` : `
            <div style="text-align: center; margin-top: 20px;">
                <button class="action-btn" onclick="closeFolderComparisonModal()">
                    <i class="fas fa-check"></i> 완료
                </button>
            </div>
        `;

        deletionStatus.innerHTML = `
            <div style="text-align: center; padding: 40px; color: #059669; font-size: 1.2rem; font-weight: 600;">
                ${message}
            </div>
            ${nextStepHtml}
            ${buttonsHtml}
        `;
    }
}

// 삭제 완료 후 비중복 파일 이동으로 진행
function proceedToMoveUniqueFiles() {
    // 비교 결과 화면으로 돌아간 후 비중복 파일 이동 모달 표시
    showStep('comparisonResult');
    if (currentComparisonId) {
        showMoveUniqueFilesModal(currentComparisonId);
    }
}

// ==================== 비중복 파일 이동 기능 ====================

let moveProgressInterval = null;
let currentMoveProgressId = null;
let currentUniqueFiles = [];

// 비중복 파일 이동 모달 표시
async function showMoveUniqueFilesModal(comparisonId) {
    const baseUrl = window.API_BASE || 'http://localhost:9090';

    try {
        // 비중복 파일 목록 조회
        const response = await fetch(`${baseUrl}/api/compare/unique-files?comparisonId=${comparisonId}`);
        if (!response.ok) {
            throw new Error('비중복 파일 목록 조회 실패');
        }

        const data = await response.json();
        const uniqueFiles = data.uniqueFiles || [];

        if (uniqueFiles.length === 0) {
            alert('이동할 비중복 파일이 없습니다.');
            return;
        }

        // 파일 목록을 모듈 변수에 저장
        currentUniqueFiles = uniqueFiles;

        // 총 크기 계산
        const totalSize = uniqueFiles.reduce((sum, f) => sum + (f.size || 0), 0);

        // 파일 목록 HTML 생성
        const fileListHtml = uniqueFiles.map((file, idx) => `
            <div style="display: flex; justify-content: space-between; align-items: center; padding: 6px 10px; border-bottom: 1px solid #f0f0f0; font-size: 0.85rem;${idx % 2 === 0 ? ' background: #fafafa;' : ''}">
                <div style="flex: 1; min-width: 0;">
                    <div style="font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${file.name}">${file.name}</div>
                    ${file.path ? `<div style="color: #999; font-size: 0.75rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${file.path}">${file.path}</div>` : ''}
                </div>
                <div style="flex-shrink: 0; margin-left: 10px; color: #666; font-size: 0.8rem;">${formatFileSize(file.size || 0)}</div>
            </div>
        `).join('');

        // 모달 HTML 생성
        const modalHtml = `
            <div id="moveUniqueFilesModal" class="modal" style="display: flex;">
                <div class="modal-content" style="max-width: 650px;">
                    <div class="modal-header">
                        <h3><i class="fas fa-folder-open"></i> 비중복 파일 이동</h3>
                        <button class="close-btn" onclick="closeMoveUniqueFilesModal()">
                            <i class="fas fa-times"></i>
                        </button>
                    </div>
                    <div class="modal-body">
                        <div style="margin-bottom: 20px;">
                            <p style="color: #666; margin-bottom: 15px;">
                                대상 폴더에만 있는 <strong>${uniqueFiles.length}개</strong> 파일 (총 <strong>${formatFileSize(totalSize)}</strong>)을 기준 폴더로 이동합니다.
                            </p>

                            <!-- 이동 전 파일 목록 -->
                            <div style="margin-bottom: 15px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
                                <div onclick="document.getElementById('moveFileListBody').style.display = document.getElementById('moveFileListBody').style.display === 'none' ? 'block' : 'none'; document.getElementById('moveFileListToggle').classList.toggle('fa-chevron-down'); document.getElementById('moveFileListToggle').classList.toggle('fa-chevron-up');"
                                     style="cursor: pointer; display: flex; justify-content: space-between; align-items: center; padding: 10px 15px; background: #f0fdf4; border-bottom: 1px solid #e5e7eb;">
                                    <span style="font-weight: 500; color: #166534;">📋 이동할 파일 목록 (${uniqueFiles.length}개, ${formatFileSize(totalSize)})</span>
                                    <i class="fas fa-chevron-down" id="moveFileListToggle" style="color: #666;"></i>
                                </div>
                                <div id="moveFileListBody" style="display: none; max-height: 250px; overflow-y: auto;">
                                    ${fileListHtml}
                                </div>
                            </div>

                            <div style="background: #f8f9fa; padding: 15px; border-radius: 8px; margin-bottom: 15px;">
                                <label style="display: flex; align-items: center; margin-bottom: 10px;">
                                    <input type="checkbox" id="preservePath" checked style="margin-right: 8px;">
                                    <span>폴더 구조 유지 (하위 폴더 구조를 그대로 복사)</span>
                                </label>

                                <div style="margin-top: 10px;">
                                    <label style="display: block; margin-bottom: 5px; font-weight: 500;">파일명 충돌 시 처리:</label>
                                    <select id="onConflict" style="width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px;">
                                        <option value="rename">자동 이름 변경 (파일명_1.확장자)</option>
                                        <option value="skip">건너뛰기</option>
                                    </select>
                                </div>
                            </div>

                            <div id="moveUniqueFilesProgress" style="display: none;">
                                <div class="progress-bar-container" style="background: #e5e7eb; border-radius: 8px; height: 20px; overflow: hidden;">
                                    <div id="moveProgressBar" style="width: 0%; height: 100%; background: linear-gradient(90deg, #22c55e, #16a34a); transition: width 0.3s;"></div>
                                </div>
                                <p id="moveProgressText" style="text-align: center; margin-top: 10px; color: #666;">이동 준비 중...</p>
                                <div style="text-align: center; margin-top: 15px;">
                                    <button class="cancel-btn" onclick="cancelMoveUniqueFiles()" style="background: #ef4444; color: white; padding: 8px 20px; border: none; border-radius: 4px; cursor: pointer;">
                                        <i class="fas fa-stop"></i> 이동 취소
                                    </button>
                                </div>
                            </div>

                            <div id="moveUniqueFilesResult" style="display: none;"></div>
                        </div>
                    </div>
                    <div class="modal-footer" id="moveModalFooter">
                        <button class="cancel-btn" onclick="closeMoveUniqueFilesModal()">취소</button>
                        <button class="action-btn" onclick="startMoveUniqueFiles(${comparisonId})" style="background: #22c55e;">
                            <i class="fas fa-arrow-right"></i> 이동 시작
                        </button>
                    </div>
                </div>
            </div>
        `;

        // 모달 추가
        document.body.insertAdjacentHTML('beforeend', modalHtml);

    } catch (error) {
        console.error('Failed to show move unique files modal:', error);
        alert('비중복 파일 목록을 불러오는 중 오류가 발생했습니다: ' + error.message);
    }
}

// 비중복 파일 이동 모달 닫기
function closeMoveUniqueFilesModal() {
    const modal = document.getElementById('moveUniqueFilesModal');
    if (modal) {
        modal.remove();
    }
    if (moveProgressInterval) {
        clearInterval(moveProgressInterval);
        moveProgressInterval = null;
    }
    currentMoveProgressId = null;
}

// 비중복 파일 이동 취소
async function cancelMoveUniqueFiles() {
    if (!currentMoveProgressId) {
        alert('취소할 이동 작업이 없습니다.');
        return;
    }

    if (!confirm('파일 이동을 취소하시겠습니까?\n이미 이동된 파일은 유지됩니다.')) {
        return;
    }

    const baseUrl = window.API_BASE || 'http://localhost:9090';

    try {
        const response = await fetch(`${baseUrl}/api/compare/move-unique-files/cancel?progressId=${currentMoveProgressId}`, {
            method: 'POST'
        });

        if (response.ok) {
            // 진행 상황 추적 중지
            if (moveProgressInterval) {
                clearInterval(moveProgressInterval);
                moveProgressInterval = null;
            }

            // 취소 메시지 표시
            const progressDiv = document.getElementById('moveUniqueFilesProgress');
            const resultDiv = document.getElementById('moveUniqueFilesResult');

            if (progressDiv) progressDiv.style.display = 'none';
            if (resultDiv) {
                resultDiv.style.display = 'block';
                resultDiv.innerHTML = `
                    <div style="text-align: center; padding: 20px; color: #f59e0b;">
                        <i class="fas fa-exclamation-triangle" style="font-size: 48px; margin-bottom: 15px;"></i>
                        <h4>이동 작업 취소됨</h4>
                        <p style="color: #666; margin-top: 10px;">
                            이미 이동된 파일은 유지됩니다.
                        </p>
                        <button class="action-btn" onclick="closeMoveUniqueFilesModal()" style="margin-top: 15px;">
                            닫기
                        </button>
                    </div>
                `;
            }

            currentMoveProgressId = null;
        } else {
            throw new Error('취소 요청 실패');
        }
    } catch (error) {
        console.error('Failed to cancel move:', error);
        alert('이동 취소 중 오류가 발생했습니다: ' + error.message);
    }
}

// 비중복 파일 이동 시작
async function startMoveUniqueFiles(comparisonId) {
    const baseUrl = window.API_BASE || 'http://localhost:9090';
    const preservePath = document.getElementById('preservePath').checked;
    const onConflict = document.getElementById('onConflict').value;

    // UI 업데이트
    document.getElementById('moveUniqueFilesProgress').style.display = 'block';
    document.getElementById('moveModalFooter').style.display = 'none';

    try {
        const response = await fetch(`${baseUrl}/api/compare/move-unique-files`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                comparisonId: comparisonId,
                preservePath: preservePath,
                onConflict: onConflict
            })
        });

        if (!response.ok) {
            throw new Error('파일 이동 요청 실패');
        }

        const data = await response.json();
        const progressId = data.progress?.id || data.progressId;

        // Store progress ID for cancellation
        currentMoveProgressId = progressId;

        // 진행 상황 추적
        trackMoveProgress(progressId);

    } catch (error) {
        console.error('Failed to start move:', error);
        alert('파일 이동 시작 중 오류가 발생했습니다: ' + error.message);
        closeMoveUniqueFilesModal();
    }
}

// 이동 진행 상황 추적
function trackMoveProgress(progressId) {
    const baseUrl = window.API_BASE || 'http://localhost:9090';

    moveProgressInterval = setInterval(async () => {
        try {
            const response = await fetch(`${baseUrl}/api/compare/move-unique-files/progress?progressId=${progressId}`);
            if (response.ok) {
                const progress = await response.json();

                const percentage = progress.totalItems > 0
                    ? Math.round((progress.processedItems / progress.totalItems) * 100)
                    : 0;

                const progressBar = document.getElementById('moveProgressBar');
                const progressText = document.getElementById('moveProgressText');

                if (progressBar) progressBar.style.width = percentage + '%';
                if (progressText) {
                    progressText.textContent = `${progress.currentStep || '이동 진행 중...'} (${percentage}%)`;
                }

                if (progress.status === 'completed') {
                    clearInterval(moveProgressInterval);
                    moveProgressInterval = null;
                    showMoveComplete(progress);
                } else if (progress.status === 'failed') {
                    clearInterval(moveProgressInterval);
                    moveProgressInterval = null;
                    alert('파일 이동에 실패했습니다: ' + (progress.errorMessage || '알 수 없는 오류'));
                    closeMoveUniqueFilesModal();
                }
            }
        } catch (error) {
            console.error('Move progress tracking error:', error);
        }
    }, 1000);
}

// 이동 완료 표시
function showMoveComplete(progress) {
    const resultDiv = document.getElementById('moveUniqueFilesResult');
    const progressDiv = document.getElementById('moveUniqueFilesProgress');

    if (progressDiv) progressDiv.style.display = 'none';

    // 🔄 비중복 파일 이동 완료 시 로컬 데이터 업데이트
    if (currentComparisonResult) {
        currentComparisonResult.uniqueInTarget = [];
        console.log('✅ 비중복 파일 데이터 초기화 완료');
    }

    // 디버그 로그
    console.log('showMoveComplete - currentComparisonResult:', currentComparisonResult);
    console.log('showMoveComplete - duplicateCount:', currentComparisonResult?.duplicateCount);
    console.log('showMoveComplete - duplicateFiles:', currentComparisonResult?.duplicateFiles);

    // 중복 파일이 남아있는지 확인
    const hasDuplicateFiles = currentComparisonResult &&
        (currentComparisonResult.duplicateCount > 0 ||
         (currentComparisonResult.duplicateFiles && currentComparisonResult.duplicateFiles.length > 0));

    console.log('showMoveComplete - hasDuplicateFiles:', hasDuplicateFiles);

    if (resultDiv) {
        resultDiv.style.display = 'block';

        // 이동 완료 후 파일 목록 HTML 생성
        const movedCount = progress.processedItems || 0;
        const totalFiles = currentUniqueFiles.length;
        const movedTotalSize = currentUniqueFiles.reduce((sum, f) => sum + (f.size || 0), 0);

        const resultFileListHtml = currentUniqueFiles.map((file, idx) => `
            <div style="display: flex; justify-content: space-between; align-items: center; padding: 6px 10px; border-bottom: 1px solid #f0f0f0; font-size: 0.85rem;${idx % 2 === 0 ? ' background: #fafafa;' : ''}">
                <div style="flex: 1; min-width: 0;">
                    <div style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${file.name}">
                        <span style="color: #22c55e; margin-right: 4px;">${idx < movedCount ? '✅' : '❌'}</span>
                        <span style="font-weight: 500;">${file.name}</span>
                    </div>
                    ${file.path ? `<div style="color: #999; font-size: 0.75rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding-left: 22px;" title="${file.path}">${file.path}</div>` : ''}
                </div>
                <div style="flex-shrink: 0; margin-left: 10px; color: #666; font-size: 0.8rem;">${formatFileSize(file.size || 0)}</div>
            </div>
        `).join('');

        // 중복 파일이 있으면 삭제 옵션 추가
        const nextStepHtml = hasDuplicateFiles ? `
            <div style="margin-top: 20px; padding: 15px; background: #fef3c7; border-radius: 8px; border: 1px solid #f59e0b;">
                <h5 style="margin: 0 0 10px 0; color: #b45309;">
                    <i class="fas fa-exclamation-triangle"></i> 다음 단계
                </h5>
                <p style="margin: 0 0 10px 0; color: #92400e; font-size: 0.9rem;">
                    아직 ${currentComparisonResult.duplicateCount || currentComparisonResult.duplicateFiles?.length || 0}개의 중복 파일이 남아있습니다.
                    중복 파일도 삭제하시겠습니까?
                </p>
            </div>
        ` : '';

        const buttonsHtml = hasDuplicateFiles ? `
            <div style="text-align: center; margin-top: 20px; display: flex; gap: 10px; justify-content: center; flex-wrap: wrap;">
                <button class="action-btn" style="background: #22c55e;" onclick="closeMoveUniqueFilesModal(); closeFolderComparisonModal();">
                    <i class="fas fa-check"></i> 완료
                </button>
                <button class="action-btn" style="background: #f59e0b;" onclick="proceedToDuplicateDeletion();">
                    <i class="fas fa-trash-alt"></i> 다음: 중복 파일 삭제
                </button>
            </div>
        ` : `
            <div style="text-align: center; margin-top: 20px;">
                <button class="action-btn" onclick="closeMoveUniqueFilesModal(); closeFolderComparisonModal();">
                    <i class="fas fa-check"></i> 완료
                </button>
            </div>
        `;

        resultDiv.innerHTML = `
            <div style="text-align: center; padding: 20px; color: #059669;">
                <i class="fas fa-check-circle" style="font-size: 48px; margin-bottom: 15px;"></i>
                <h4>파일 이동 완료!</h4>
                <p style="color: #666; margin-top: 10px;">
                    ${movedCount}개 파일이 성공적으로 이동되었습니다.${totalFiles > movedCount ? ` (${totalFiles - movedCount}개 실패/건너뜀)` : ''}
                </p>
            </div>

            <!-- 이동 결과 파일 목록 -->
            <div style="margin-top: 15px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
                <div onclick="document.getElementById('moveResultFileListBody').style.display = document.getElementById('moveResultFileListBody').style.display === 'none' ? 'block' : 'none'; document.getElementById('moveResultFileListToggle').classList.toggle('fa-chevron-down'); document.getElementById('moveResultFileListToggle').classList.toggle('fa-chevron-up');"
                     style="cursor: pointer; display: flex; justify-content: space-between; align-items: center; padding: 10px 15px; background: #f0fdf4; border-bottom: 1px solid #e5e7eb;">
                    <span style="font-weight: 500; color: #166534;">📋 이동된 파일 목록 (${movedCount}개, ${formatFileSize(movedTotalSize)})</span>
                    <i class="fas fa-chevron-down" id="moveResultFileListToggle" style="color: #666;"></i>
                </div>
                <div id="moveResultFileListBody" style="display: none; max-height: 250px; overflow-y: auto;">
                    ${resultFileListHtml}
                </div>
            </div>

            ${nextStepHtml}
            ${buttonsHtml}
        `;
    }
}

// 이동 완료 후 중복 파일 삭제로 진행
function proceedToDuplicateDeletion() {
    closeMoveUniqueFilesModal();

    // window.currentDuplicateFiles가 설정되지 않은 경우 currentComparisonResult에서 설정
    if (!window.currentDuplicateFiles && currentComparisonResult && currentComparisonResult.duplicateFiles) {
        window.currentDuplicateFiles = currentComparisonResult.duplicateFiles;
    }

    // 비교 결과 화면으로 돌아가서 삭제 옵션 표시
    showStep('comparisonResult');
    // 중복 파일 삭제 함수 호출
    deleteDuplicateFiles();
}