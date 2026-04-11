/**
 * 폴더 분석 기능
 * Google Drive 폴더의 구조 분석, 크기 계산, 빈 폴더 관리
 */

// 현재 분석 중인 폴더 정보
let currentAnalysisResult = null;
let currentAnalysisFolderId = null; // 현재 분석 중인 폴더 ID
let isAnalysisCancelled = false; // 취소 플래그
let currentExcludePatterns = []; // 분석 시 사용된 제외 패턴 (삭제 시 전달용)

/**
 * 폴더 구조 분석 시작
 */
async function analyzeFolderStructure() {
    const folderUrl = document.getElementById('folderAnalysisUrl').value.trim();

    if (!folderUrl) {
        alert('폴더 URL 또는 ID를 입력하세요.');
        return;
    }

    // 제외 폴더 패턴 읽기
    const excludePatternsInput = document.getElementById('excludeFolderPatterns');
    const excludePatterns = excludePatternsInput ?
        excludePatternsInput.value.split(',').map(p => p.trim()).filter(p => p.length > 0) : [];

    // 제외 패턴 저장 (삭제 시 사용)
    currentExcludePatterns = excludePatterns;

    // 취소 플래그 초기화
    isAnalysisCancelled = false;

    // 모달 표시
    const modal = document.getElementById('folderAnalysisModal');
    if (modal) {
        modal.classList.remove('hidden');

        // 진행 상황 표시 (제외 패턴 정보 포함)
        showAnalysisProgress(excludePatterns);
    }

    try {
        // 폴더 ID 추출
        const folderId = await extractFolderIdFromUrl(folderUrl);
        currentAnalysisFolderId = folderId; // 폴더 ID 저장

        // 진행 상황 모니터링 시작
        monitorAnalysisProgress();

        // 제외 패턴 로깅
        if (excludePatterns.length > 0) {
            console.log('제외할 폴더 패턴:', excludePatterns);
        }

        // 폴더 분석 시작 (백그라운드 실행)
        const response = await fetch(`${API_BASE}/api/folder/analyze`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                folderId: folderId,
                url: folderUrl,
                excludeFolderNames: excludePatterns
            })
        });

        // 진행 상황 모니터링 중단
        if (window.currentAnalysisProgressInterval) {
            clearInterval(window.currentAnalysisProgressInterval);
        }

        // 취소된 경우 처리
        if (isAnalysisCancelled) {
            console.log('폴더 분석이 취소되었습니다.');
            return;
        }

        // 취소 응답 처리
        if (response.status === 408) {
            const result = await response.json();
            if (result.cancelled) {
                showAnalysisCancelled();
                return;
            }
        }

        if (!response.ok) {
            throw new Error('폴더 분석 요청이 실패했습니다.');
        }

        const result = await response.json();
        currentAnalysisResult = result;

        // 분석 결과에서 제외 패턴 복원 (서버에서 반환된 값 사용)
        if (result.excludeFolderNames && result.excludeFolderNames.length > 0) {
            currentExcludePatterns = result.excludeFolderNames;
            console.log('📋 분석 결과에서 제외 패턴 복원:', currentExcludePatterns);
        }

        // 최종 진행률 100%로 설정
        const progressBar = document.getElementById('analysisProgressBar');
        const progressPercentage = document.getElementById('analysisProgressPercentage');
        const progressText = document.getElementById('analysisProgressText');

        progressBar.style.width = '100%';
        progressPercentage.textContent = '100%';
        progressText.textContent = '분석 완료!';

        // 1초 후 결과 표시
        setTimeout(() => {
            showAnalysisResult(result);
        }, 1000);

    } catch (error) {
        console.error('폴더 분석 오류:', error);

        // 진행 상황 모니터링 중단
        if (window.currentAnalysisProgressInterval) {
            clearInterval(window.currentAnalysisProgressInterval);
        }

        // 취소된 경우 에러 메시지 표시 안 함
        if (isAnalysisCancelled) {
            return;
        }

        alert(`폴더 분석 중 오류가 발생했습니다: ${error.message}`);
        closeFolderAnalysisModal();
    } finally {
        currentAnalysisFolderId = null; // 폴더 ID 초기화
    }
}

/**
 * URL에서 폴더 ID 추출
 */
async function extractFolderIdFromUrl(url) {
    try {
        const response = await fetch(`${API_BASE}/api/utils/extract-folder-id`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ url: url })
        });
        
        if (!response.ok) {
            throw new Error('폴더 ID 추출에 실패했습니다.');
        }
        
        const result = await response.json();
        return result.folderId;
    } catch (error) {
        // URL에서 직접 추출 시도
        const patterns = [
            /\/folders\/([a-zA-Z0-9_-]+)/,
            /[?&]id=([a-zA-Z0-9_-]+)/,
            /\/drive\/u\/\d+\/folders\/([a-zA-Z0-9_-]+)/
        ];
        
        for (const pattern of patterns) {
            const match = url.match(pattern);
            if (match) {
                return match[1];
            }
        }
        
        // ID만 입력한 경우
        if (/^[a-zA-Z0-9_-]+$/.test(url)) {
            return url;
        }
        
        throw new Error('유효한 Google Drive 폴더 URL 또는 ID가 아닙니다.');
    }
}

/**
 * 분석 진행 상황 표시 - 실제 백엔드 연동
 * @param {Array} excludePatterns - 제외할 폴더 패턴 목록 (선택사항)
 */
function showAnalysisProgress(excludePatterns = []) {
    // 제외 패턴 표시 HTML 생성
    const excludeInfoHtml = excludePatterns.length > 0 ? `
        <div class="exclude-patterns-info" style="background: #fef3c7; border: 1px solid #f59e0b; border-radius: 8px; padding: 10px 15px; margin-bottom: 15px;">
            <div style="display: flex; align-items: center; gap: 8px; color: #92400e;">
                <i class="fas fa-filter"></i>
                <span><strong>제외 패턴:</strong> ${excludePatterns.join(', ')}</span>
            </div>
        </div>
    ` : '';

    // 진행 상황 섹션의 원래 HTML 복원
    const progressSection = document.getElementById('folderAnalysisProgress');
    if (progressSection) {
        progressSection.innerHTML = `
            <div class="step-header">
                <i class="fas fa-cog fa-spin"></i> 폴더 분석 진행 중...
            </div>

            ${excludeInfoHtml}

            <div class="progress-item">
                <div class="progress-header">
                    <div class="progress-title">분석 진행률</div>
                    <div class="progress-percentage" id="analysisProgressPercentage">0%</div>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" id="analysisProgressBar" style="width: 0%"></div>
                </div>
                <div class="progress-details" id="analysisProgressText">폴더 분석 시작 중...</div>
            </div>

            <!-- 취소 버튼 -->
            <div style="text-align: center; margin-top: 20px;">
                <button class="btn btn-danger" id="cancelAnalysisBtn" onclick="cancelFolderAnalysis()">
                    <i class="fas fa-stop-circle"></i> 분석 취소
                </button>
            </div>
        `;
        progressSection.style.display = 'block';
    }
    document.getElementById('folderAnalysisResult').style.display = 'none';

    // 초기 상태 설정
    const progressBar = document.getElementById('analysisProgressBar');
    const progressPercentage = document.getElementById('analysisProgressPercentage');
    const progressText = document.getElementById('analysisProgressText');

    if (progressBar) progressBar.style.width = '0%';
    if (progressPercentage) progressPercentage.textContent = '0%';
    if (progressText) progressText.textContent = '분석 준비 중...';
}

/**
 * 실시간 진행 상황 모니터링
 */
function monitorAnalysisProgress() {
    let attempts = 0;
    const maxAttempts = 180; // 3분 최대 대기 (1초 간격)
    
    const progressInterval = setInterval(async () => {
        attempts++;
        
        try {
            // 백엔드에서 진행 상황 조회 (일단 폴더 분석 완료 확인)
            // 실제로는 진행 상황을 추적할 수 있는 API가 필요하지만, 
            // 현재는 완료 여부만 확인
            
            const progressBar = document.getElementById('analysisProgressBar');
            const progressPercentage = document.getElementById('analysisProgressPercentage');
            const progressText = document.getElementById('analysisProgressText');
            
            // 가상의 진행률 업데이트 (실제 백엔드 응답이 올 때까지)
            const currentProgress = Math.min(5 + (attempts * 2), 95);
            progressBar.style.width = `${currentProgress}%`;
            progressPercentage.textContent = `${currentProgress}%`;
            
            // 진행 단계별 메시지
            if (currentProgress < 30) {
                progressText.textContent = '폴더 구조 스캔 중...';
            } else if (currentProgress < 60) {
                progressText.textContent = '하위 폴더 분석 중...';
            } else if (currentProgress < 90) {
                progressText.textContent = '파일 크기 계산 중...';
            } else {
                progressText.textContent = '분석 마무리 중...';
            }
            
            // 최대 시도 횟수 도달 시 타임아웃
            if (attempts >= maxAttempts) {
                clearInterval(progressInterval);
                progressText.textContent = '분석이 오래 걸리고 있습니다. 잠시만 기다려주세요...';
            }
            
        } catch (error) {
            console.error('진행 상황 조회 오류:', error);
            
            // 에러 발생 시에도 계속 시도
            if (attempts >= maxAttempts) {
                clearInterval(progressInterval);
                alert('분석 진행 상황을 확인할 수 없습니다. 페이지를 새로고침해 보세요.');
            }
        }
    }, 1000); // 1초마다 확인
    
    // 진행 상황 모니터링 정보를 저장하여 나중에 중단할 수 있도록 함
    window.currentAnalysisProgressInterval = progressInterval;
}

/**
 * 분석 결과 표시
 */
function showAnalysisResult(result) {
    // 진행 상황 숨기고 결과 표시
    document.getElementById('folderAnalysisProgress').style.display = 'none';
    document.getElementById('folderAnalysisResult').style.display = 'block';
    
    // 전체 통계 표시
    const summaryHtml = `
        <div class="analysis-summary">
            <div class="summary-header">
                <h3><i class="fas fa-chart-bar"></i> ${result.rootFolder.name}</h3>
                <div class="summary-stats">
                    <div class="stat-item">
                        <span class="stat-value">${formatNumber(result.totalFiles)}</span>
                        <span class="stat-label">총 파일</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${formatFileSize(result.totalSize)}</span>
                        <span class="stat-label">총 크기</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${result.emptyFolders.length}</span>
                        <span class="stat-label">빈 폴더</span>
                    </div>
                </div>
            </div>
            <div style="display: flex; align-items: center; justify-content: space-between; margin-top: 10px;">
                <div class="analysis-time">
                    <small><i class="fas fa-clock"></i> 분석 시간: ${result.analysisTime}</small>
                </div>
                <button class="btn btn-primary btn-sm" onclick="exportFolderToSpreadsheet()" id="exportSpreadsheetBtn">
                    <i class="fas fa-file-export"></i> 스프레드시트로 내보내기
                </button>
            </div>
        </div>
    `;
    
    document.getElementById('folderAnalysisSummary').innerHTML = summaryHtml;
    
    // 폴더 구조 트리 표시
    const treeHtml = buildFolderTree(result.rootFolder);
    document.getElementById('folderStructureTree').innerHTML = `
        <h3><i class="fas fa-sitemap"></i> 폴더 구조</h3>
        <div class="folder-tree">
            ${treeHtml}
        </div>
    `;
    
    // 빈 폴더 목록 표시
    if (result.emptyFolders.length > 0) {
        // 제외 패턴 안내 문구 (제외 패턴이 있을 경우에만 표시)
        const excludeNoticeHtml = (result.excludeFolderNames && result.excludeFolderNames.length > 0) ? `
            <div class="exclude-notice" style="background: #fef3c7; border: 1px solid #f59e0b; border-radius: 8px; padding: 12px 15px; margin-bottom: 15px;">
                <div style="display: flex; align-items: flex-start; gap: 10px; color: #92400e;">
                    <i class="fas fa-info-circle" style="margin-top: 2px;"></i>
                    <div>
                        <div style="font-weight: 600; margin-bottom: 4px;">제외 패턴: ${result.excludeFolderNames.join(', ')}</div>
                        <div style="font-size: 0.9rem;">빈 폴더 휴지통 이동 시, 제외 패턴에 해당하는 하위 폴더도 함께 이동됩니다.</div>
                    </div>
                </div>
            </div>
        ` : '';

        // 체크박스 컨트롤 헤더
        const controlsHtml = `
            ${excludeNoticeHtml}
            <div class="empty-folders-controls">
                <div class="checkbox-controls">
                    <button class="btn btn-sm btn-outline" onclick="selectAllEmptyFolders()">
                        <i class="fas fa-check-square"></i> 전체 선택
                    </button>
                    <button class="btn btn-sm btn-outline" onclick="deselectAllEmptyFolders()">
                        <i class="fas fa-square"></i> 전체 해제
                    </button>
                    <span class="selected-count" id="selectedEmptyFoldersCount">0개 선택됨</span>
                </div>
                <button class="btn btn-danger btn-sm" onclick="deleteSelectedEmptyFolders()" id="deleteSelectedEmptyFoldersBtn" disabled>
                    <i class="fas fa-trash"></i> 선택 휴지통 이동
                </button>
            </div>
        `;

        // 빈 폴더 목록 (체크박스 포함)
        const emptyFoldersHtml = result.emptyFolders.map(folder => `
            <div class="empty-folder-item">
                <label class="empty-folder-checkbox">
                    <input type="checkbox" class="empty-folder-check" data-folder-id="${folder.id}" onchange="updateEmptyFolderSelection()">
                    <span class="checkmark"></span>
                </label>
                <div class="empty-folder-info">
                    <i class="fas fa-folder-open"></i>
                    <span class="empty-folder-name">${folder.name}</span>
                    <small class="empty-folder-path">${folder.path}</small>
                </div>
                <button class="btn btn-danger btn-sm" onclick="deleteSingleEmptyFolder('${folder.id}')">
                    <i class="fas fa-trash"></i> 휴지통
                </button>
            </div>
        `).join('');

        document.getElementById('emptyFoldersList').innerHTML = controlsHtml + emptyFoldersHtml;
        document.getElementById('emptyFoldersSection').style.display = 'block';
    } else {
        document.getElementById('emptyFoldersSection').style.display = 'none';
    }
}

/**
 * 폴더 구조 트리 빌드
 */
function buildFolderTree(folder, level = 0) {
    const indent = '  '.repeat(level);
    const isEmpty = folder.isEmpty ? ' empty-folder' : '';
    const icon = folder.isEmpty ? 'fas fa-folder-open' : 'fas fa-folder';
    
    let html = `
        <div class="folder-node${isEmpty}" style="margin-left: ${level * 20}px;">
            <div class="folder-info">
                <i class="${icon}"></i>
                <span class="folder-name">${folder.name}</span>
                <span class="folder-stats">
                    (${formatNumber(folder.fileCount)}개 파일, ${formatFileSize(folder.totalSize)})
                </span>
                ${folder.isEmpty ? '<span class="empty-badge">빈 폴더</span>' : ''}
            </div>
        </div>
    `;
    
    // 하위 폴더들 추가 (크기 순으로 정렬됨)
    if (folder.subFolders && folder.subFolders.length > 0) {
        for (const subFolder of folder.subFolders) {
            html += buildFolderTree(subFolder, level + 1);
        }
    }
    
    return html;
}

/**
 * 단일 빈 폴더 삭제
 */
async function deleteSingleEmptyFolder(folderId) {
    // 폴더 이름 찾기
    let folderName = folderId;
    if (currentAnalysisResult && currentAnalysisResult.emptyFolders) {
        const folder = currentAnalysisResult.emptyFolders.find(f => f.id === folderId);
        if (folder) {
            folderName = folder.name;
        }
    }

    const message = `
        <div style="margin-bottom: 16px;">
            <strong>이 빈 폴더를 휴지통으로 이동하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.95rem; color: #6b7280;">
            폴더명: <code style="background: #f3f4f6; padding: 2px 6px; border-radius: 3px;">${folderName}</code>
        </div>
    `;

    // 항상 휴지통으로 이동
    showDeleteConfirmation(message, async (data) => {
        await executeSingleEmptyFolderDelete(folderId, false);
    }, { folderId }, false);
}

/**
 * 단일 빈 폴더 삭제 실행
 */
async function executeSingleEmptyFolderDelete(folderId, permanentDelete) {
    // 제외 패턴 결정: currentExcludePatterns가 비어있으면 분석 결과에서 가져옴
    let excludePatterns = currentExcludePatterns;
    if ((!excludePatterns || excludePatterns.length === 0) && currentAnalysisResult && currentAnalysisResult.excludeFolderNames) {
        excludePatterns = currentAnalysisResult.excludeFolderNames;
        console.log('📋 분석 결과에서 제외 패턴 사용:', excludePatterns);
    }

    try {
        // 제외 패턴 및 삭제 방식을 포함하여 삭제 요청
        const response = await fetch(`${API_BASE}/api/folder/empty/delete`, {
            method: 'DELETE',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                folderId: folderId,
                excludeFolderNames: excludePatterns,
                permanentDelete: permanentDelete
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText || '빈 폴더 휴지통 이동에 실패했습니다.');
        }

        const result = await response.json();

        if (result.success) {
            showSuccessMessage(`빈 폴더가 휴지통으로 이동되었습니다.`);

            // 삭제된 폴더를 목록에서 제거
            if (currentAnalysisResult) {
                currentAnalysisResult.emptyFolders = currentAnalysisResult.emptyFolders.filter(
                    folder => folder.id !== folderId
                );
                showAnalysisResult(currentAnalysisResult);
            }
        }

    } catch (error) {
        console.error('빈 폴더 휴지통 이동 오류:', error);
        showErrorMessage(`빈 폴더 휴지통 이동 중 오류가 발생했습니다: ${error.message}`);
        throw error;
    }
}

/**
 * 빈 폴더 전체 선택
 */
function selectAllEmptyFolders() {
    const checkboxes = document.querySelectorAll('.empty-folder-check');
    checkboxes.forEach(checkbox => {
        checkbox.checked = true;
    });
    updateEmptyFolderSelection();
}

/**
 * 빈 폴더 전체 해제
 */
function deselectAllEmptyFolders() {
    const checkboxes = document.querySelectorAll('.empty-folder-check');
    checkboxes.forEach(checkbox => {
        checkbox.checked = false;
    });
    updateEmptyFolderSelection();
}

/**
 * 빈 폴더 선택 상태 업데이트
 */
function updateEmptyFolderSelection() {
    const checkboxes = document.querySelectorAll('.empty-folder-check');
    const checkedCount = document.querySelectorAll('.empty-folder-check:checked').length;

    // 선택된 개수 표시
    const countSpan = document.getElementById('selectedEmptyFoldersCount');
    if (countSpan) {
        countSpan.textContent = `${checkedCount}개 선택됨`;
    }

    // 선택 삭제 버튼 활성화/비활성화
    const deleteBtn = document.getElementById('deleteSelectedEmptyFoldersBtn');
    if (deleteBtn) {
        deleteBtn.disabled = checkedCount === 0;
    }
}

/**
 * 선택된 빈 폴더 삭제 (진행 상황 표시)
 */
async function deleteSelectedEmptyFolders() {
    const checkedBoxes = document.querySelectorAll('.empty-folder-check:checked');
    const selectedIds = Array.from(checkedBoxes).map(cb => cb.dataset.folderId);

    if (selectedIds.length === 0) {
        alert('휴지통으로 이동할 빈 폴더를 선택하세요.');
        return;
    }

    // 선택된 폴더 정보 가져오기
    const selectedFolders = currentAnalysisResult.emptyFolders.filter(
        folder => selectedIds.includes(folder.id)
    );

    const message = `
        <div style="margin-bottom: 16px;">
            <strong>선택된 ${selectedIds.length}개의 빈 폴더를 휴지통으로 이동하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280; max-height: 120px; overflow-y: auto; background: #f8f9fa; padding: 8px; border-radius: 4px;">
            ${selectedFolders.slice(0, 5).map(f => `• ${f.name}`).join('<br>')}
            ${selectedFolders.length > 5 ? `<br>... 외 ${selectedFolders.length - 5}개` : ''}
        </div>
    `;

    // 항상 휴지통으로 이동
    showDeleteConfirmation(message, async (data) => {
        await deleteEmptyFoldersWithProgress(selectedFolders, false);
    }, { folderIds: selectedIds }, false);
}

/**
 * 모든 빈 폴더 삭제 (진행 상황 표시)
 */
async function deleteAllEmptyFolders() {
    if (!currentAnalysisResult || !currentAnalysisResult.emptyFolders.length) {
        alert('휴지통으로 이동할 빈 폴더가 없습니다.');
        return;
    }

    const folderCount = currentAnalysisResult.emptyFolders.length;
    const message = `
        <div style="margin-bottom: 16px;">
            <strong style="color: #ef4444;">⚠️ 모든 빈 폴더 ${folderCount}개를 휴지통으로 이동하시겠습니까?</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280;">
            이 작업은 분석된 모든 빈 폴더를 휴지통으로 이동합니다.
        </div>
    `;

    // 항상 휴지통으로 이동
    showDeleteConfirmation(message, async (data) => {
        await deleteEmptyFoldersWithProgress([...currentAnalysisResult.emptyFolders], false);
    }, { folderCount }, false);
}

/**
 * 빈 폴더 삭제 (진행 상황 표시)
 * @param {Array} foldersToDelete - 삭제할 폴더 목록 [{id, name, path}, ...]
 * @param {boolean} permanentDelete - true: 완전 삭제, false: 휴지통 이동
 */
async function deleteEmptyFoldersWithProgress(foldersToDelete, permanentDelete = false) {
    if (!foldersToDelete || foldersToDelete.length === 0) {
        return;
    }

    const totalCount = foldersToDelete.length;
    const folderIds = foldersToDelete.map(f => f.id);

    // 제외 패턴 결정: currentExcludePatterns가 비어있으면 분석 결과에서 가져옴
    let excludePatterns = currentExcludePatterns;
    if ((!excludePatterns || excludePatterns.length === 0) && currentAnalysisResult && currentAnalysisResult.excludeFolderNames) {
        excludePatterns = currentAnalysisResult.excludeFolderNames;
        console.log('📋 분석 결과에서 제외 패턴 사용:', excludePatterns);
    }

    // 삭제 방식 로깅
    const deleteMethodText = permanentDelete ? '완전 삭제' : '휴지통 이동';
    console.log(`🗑️ 빈 폴더 삭제 시작: ${totalCount}개 폴더, 삭제 방식: ${deleteMethodText}`);

    // 삭제 진행 상황 모달 표시
    showEmptyFolderDeletionProgress(totalCount, permanentDelete);

    // 마지막으로 받은 진행 상황 (SSE 실패 시 사용)
    let lastProgress = { completed: 0, total: totalCount, success: 0, failed: 0 };

    try {
        // SSE를 사용한 실시간 진행 상황 수신
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 30 * 60 * 1000); // 30분 타임아웃

        const response = await fetch(`${API_BASE}/api/folder/empty/delete-all`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'text/event-stream',
            },
            body: JSON.stringify({
                folderIds: folderIds,
                excludeFolderNames: excludePatterns,
                permanentDelete: permanentDelete
            }),
            signal: controller.signal
        });

        clearTimeout(timeoutId);

        if (!response.ok) {
            throw new Error('빈 폴더 삭제 요청이 실패했습니다.');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        let finalResult = null;
        let lastEventTime = Date.now();

        // 연결 상태 모니터링 (30초 동안 이벤트 없으면 경고)
        const connectionCheckInterval = setInterval(() => {
            const elapsed = Date.now() - lastEventTime;
            if (elapsed > 30000) {
                console.log('⏳ SSE 이벤트 대기 중... (마지막 이벤트:', Math.round(elapsed / 1000), '초 전)');
            }
        }, 10000);

        try {
            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                lastEventTime = Date.now();
                buffer += decoder.decode(value, { stream: true });

                // SSE 이벤트 파싱
                const lines = buffer.split('\n');
                buffer = lines.pop() || ''; // 마지막 불완전한 라인 유지

                let eventType = null;
                let eventData = null;

                for (const line of lines) {
                    // heartbeat 주석은 무시 (연결 유지용)
                    if (line.startsWith(':')) {
                        console.log('💓 SSE heartbeat 수신');
                        continue;
                    }

                    if (line.startsWith('event: ')) {
                        eventType = line.slice(7).trim();
                    } else if (line.startsWith('data: ')) {
                        try {
                            eventData = JSON.parse(line.slice(6));
                        } catch (e) {
                            console.error('SSE 데이터 파싱 오류:', e);
                        }
                    } else if (line === '' && eventType && eventData) {
                        // 이벤트 처리
                        if (eventType === 'progress') {
                            lastProgress = eventData;
                            updateEmptyFolderDeletionProgressSSE(eventData);
                        } else if (eventType === 'complete') {
                            finalResult = eventData;
                        }
                        eventType = null;
                        eventData = null;
                    }
                }
            }
        } finally {
            clearInterval(connectionCheckInterval);
        }

        // 결과 표시
        if (finalResult) {
            const failedFolders = [];
            if (finalResult.failedCount > 0) {
                failedFolders.push({ name: `${finalResult.failedCount}개 폴더`, error: '삭제 실패' });
            }
            // 연쇄 삭제 카운트 포함
            showEmptyFolderDeletionResult(
                finalResult.successCount,
                finalResult.failedCount,
                finalResult.totalCount,
                failedFolders,
                finalResult.cascadeDeletedCount || 0
            );

            // 삭제된 폴더를 목록에서 제거
            if (currentAnalysisResult && finalResult.successCount > 0) {
                currentAnalysisResult.emptyFolders = currentAnalysisResult.emptyFolders.filter(
                    folder => !folderIds.includes(folder.id)
                );
            }
        } else if (lastProgress.completed > 0) {
            // SSE가 완료 이벤트 없이 끝난 경우, 마지막 진행 상황으로 결과 표시
            console.warn('⚠️ SSE 완료 이벤트 없이 종료, 마지막 진행 상황 사용:', lastProgress);
            showEmptyFolderDeletionResult(
                lastProgress.success || lastProgress.completed,
                lastProgress.failed || 0,
                lastProgress.total,
                []
            );

            // 삭제된 폴더를 목록에서 제거
            if (currentAnalysisResult && lastProgress.completed > 0) {
                currentAnalysisResult.emptyFolders = currentAnalysisResult.emptyFolders.filter(
                    folder => !folderIds.includes(folder.id)
                );
            }
        }

    } catch (error) {
        console.error('빈 폴더 삭제 오류:', error);

        // 네트워크 오류이지만 일부 삭제가 진행된 경우
        if (lastProgress.completed > 0) {
            console.warn('⚠️ 네트워크 오류 발생, 마지막 진행 상황:', lastProgress);

            // 서버에서 삭제가 계속 진행 중일 수 있으므로 안내 메시지 표시
            showEmptyFolderDeletionResult(
                lastProgress.success || lastProgress.completed,
                lastProgress.failed || 0,
                lastProgress.total,
                [{ name: '연결 오류', error: '연결이 끊어졌습니다. 서버에서 삭제가 계속 진행 중일 수 있습니다.' }]
            );

            // 삭제된 것으로 확인된 폴더를 목록에서 제거
            if (currentAnalysisResult && lastProgress.completed > 0) {
                currentAnalysisResult.emptyFolders = currentAnalysisResult.emptyFolders.filter(
                    folder => !folderIds.includes(folder.id)
                );
            }
        } else {
            showEmptyFolderDeletionResult(0, totalCount, totalCount, [{ name: '전체', error: error.message }]);
        }
    }
}

/**
 * SSE 진행 상황 업데이트
 */
function updateEmptyFolderDeletionProgressSSE(data) {
    const modal = document.getElementById('emptyFolderDeletionModal');
    if (!modal) return;

    const percentage = Math.round((data.completed / data.total) * 100);

    modal.querySelector('.modal-body').innerHTML = `
        <div style="text-align: center; padding: 20px;">
            <div style="margin-bottom: 20px;">
                <i class="fas fa-trash-alt" style="font-size: 48px; color: #3b82f6;"></i>
            </div>
            <div style="font-size: 1.5rem; font-weight: bold; color: #3b82f6; margin-bottom: 10px;">
                ${percentage}%
            </div>
            <div class="progress-bar-container" style="margin: 15px 0;">
                <div style="height: 12px; background: #e2e8f0; border-radius: 6px; overflow: hidden;">
                    <div style="width: ${percentage}%; height: 100%; background: linear-gradient(90deg, #3b82f6, #60a5fa); transition: width 0.3s ease;"></div>
                </div>
            </div>
            <div style="font-size: 1.1rem; color: #374151; margin-bottom: 8px;">
                <strong>${data.completed}</strong> / ${data.total} 폴더 완료
            </div>
            <div style="display: flex; justify-content: center; gap: 20px; font-size: 0.9rem; color: #6b7280;">
                <span style="color: #10b981;">✓ 성공: ${data.success || 0}</span>
                <span style="color: #ef4444;">✗ 실패: ${data.failed || 0}</span>
            </div>
        </div>
    `;
}

/**
 * 빈 폴더 삭제 진행 상황 모달 표시
 * @param {number} totalCount - 삭제할 폴더 수
 * @param {boolean} permanentDelete - true: 완전 삭제, false: 휴지통 이동
 */
function showEmptyFolderDeletionProgress(totalCount, permanentDelete = false) {
    // 기존 모달이 있으면 제거
    let modal = document.getElementById('emptyFolderDeletionModal');
    if (modal) {
        modal.remove();
    }

    const actionText = permanentDelete ? '완전 삭제' : '휴지통으로 이동';
    const iconColor = permanentDelete ? '#ef4444' : '#3b82f6';

    // 새 모달 생성
    modal = document.createElement('div');
    modal.id = 'emptyFolderDeletionModal';
    modal.className = 'modal';
    modal.innerHTML = `
        <div class="modal-content" style="max-width: 400px;">
            <div class="modal-header">
                <h2 class="modal-title">
                    <i class="fas fa-trash-alt"></i> 빈 폴더 ${actionText} 중
                </h2>
            </div>
            <div class="modal-body" style="text-align: center; padding: 30px;">
                <div style="margin-bottom: 20px;">
                    <i class="fas fa-spinner fa-spin" style="font-size: 48px; color: ${iconColor};"></i>
                </div>
                <div style="font-size: 1.2rem; color: #374151; margin-bottom: 10px;">
                    총 <strong>${totalCount}개</strong> 폴더 ${actionText} 중...
                </div>
                <div style="color: #6b7280; font-size: 0.9rem;">
                    잠시만 기다려주세요
                </div>
                <div style="margin-top: 10px; padding: 6px 12px; background: ${permanentDelete ? '#fef2f2' : '#eff6ff'}; border-radius: 6px; display: inline-block;">
                    <span style="color: ${permanentDelete ? '#dc2626' : '#2563eb'}; font-size: 0.85rem;">
                        <i class="fas ${permanentDelete ? 'fa-exclamation-triangle' : 'fa-recycle'}"></i>
                        ${permanentDelete ? '완전 삭제 (복구 불가)' : '휴지통으로 이동 (복구 가능)'}
                    </span>
                </div>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
}

/**
 * 빈 폴더 삭제 진행 상황 업데이트
 * @param {number} completed - 완료된 폴더 수
 * @param {number} total - 전체 폴더 수
 * @param {string} folderName - 현재 처리 중인 폴더 이름
 * @param {boolean} isCompleted - 현재 폴더 삭제가 완료되었는지 여부
 */
function updateEmptyFolderDeletionProgress(completed, total, folderName, isCompleted = false) {
    const progressBar = document.getElementById('deletionProgressBar');
    const progressText = document.getElementById('deletionProgressText');
    const progressPercentage = document.getElementById('deletionProgressPercentage');
    const currentFolder = document.getElementById('deletionCurrentFolder');

    if (!progressBar) return;

    const percentage = Math.round((completed / total) * 100);

    progressBar.style.width = `${percentage}%`;
    progressText.textContent = `${completed} / ${total} 폴더`;
    progressPercentage.textContent = `${percentage}%`;

    if (completed < total) {
        if (isCompleted) {
            // 현재 폴더 삭제 완료, 다음 폴더 대기 중
            currentFolder.innerHTML = `<i class="fas fa-check" style="color: #10b981;"></i> <span>완료: ${folderName}</span>`;
        } else {
            // 현재 폴더 삭제 중
            currentFolder.innerHTML = `<i class="fas fa-spinner fa-spin" style="color: #3b82f6;"></i> <span>삭제 중: ${folderName}</span>`;
        }
    } else {
        currentFolder.innerHTML = `<i class="fas fa-check-circle" style="color: #10b981;"></i> <span>모든 폴더 삭제 완료!</span>`;
    }
}

/**
 * 빈 폴더 삭제 결과 표시
 * @param {number} successCount - 성공한 삭제 수
 * @param {number} failedCount - 실패한 삭제 수
 * @param {number} totalCount - 총 요청된 삭제 수
 * @param {Array} failedFolders - 실패한 폴더 목록
 * @param {number} cascadeDeletedCount - 연쇄 삭제된 부모 폴더 수 (선택)
 */
function showEmptyFolderDeletionResult(successCount, failedCount, totalCount, failedFolders, cascadeDeletedCount = 0) {
    const modal = document.getElementById('emptyFolderDeletionModal');
    if (!modal) return;

    const modalContent = modal.querySelector('.modal-content');

    // 연쇄 삭제 정보 표시
    const cascadeInfo = cascadeDeletedCount > 0
        ? `<div class="stat-item">
               <div style="font-size: 2rem; font-weight: bold; color: #8b5cf6;">${cascadeDeletedCount}</div>
               <div style="color: #6b7280;">연쇄 삭제</div>
           </div>`
        : '';

    const cascadeNote = cascadeDeletedCount > 0
        ? `<div style="color: #8b5cf6; font-size: 0.9rem; margin-top: 8px;">
               <i class="fas fa-link"></i> 부모 폴더 ${cascadeDeletedCount}개가 비어서 함께 삭제되었습니다.
           </div>`
        : '';

    let resultHtml = `
        <div class="modal-header">
            <h2 class="modal-title">
                <i class="fas fa-check-circle" style="color: #10b981;"></i> 삭제 완료
            </h2>
        </div>
        <div class="modal-body">
            <div class="deletion-result-summary" style="text-align: center; padding: 20px;">
                <div style="display: flex; justify-content: center; gap: 40px; margin-bottom: 20px;">
                    <div class="stat-item">
                        <div style="font-size: 2rem; font-weight: bold; color: #10b981;">${successCount}</div>
                        <div style="color: #6b7280;">삭제 성공</div>
                    </div>
                    ${cascadeInfo}
                    <div class="stat-item">
                        <div style="font-size: 2rem; font-weight: bold; color: #ef4444;">${failedCount}</div>
                        <div style="color: #6b7280;">삭제 실패</div>
                    </div>
                </div>
                <div style="color: #6b7280; margin-bottom: 20px;">
                    총 ${totalCount}개 폴더 중 ${successCount}개 삭제됨
                    ${cascadeNote}
                </div>
    `;

    if (failedFolders.length > 0) {
        resultHtml += `
                <div style="text-align: left; margin-top: 20px; padding: 15px; background: #fef2f2; border-radius: 8px; max-height: 150px; overflow-y: auto;">
                    <div style="font-weight: 600; color: #dc2626; margin-bottom: 10px;">
                        <i class="fas fa-exclamation-triangle"></i> 실패한 폴더:
                    </div>
                    ${failedFolders.map(f => `
                        <div style="font-size: 0.9rem; color: #6b7280; padding: 4px 0;">
                            • ${f.name} <small style="color: #dc2626;">(${f.error})</small>
                        </div>
                    `).join('')}
                </div>
        `;
    }

    resultHtml += `
            </div>
        </div>
        <div class="modal-footer" style="display: flex; justify-content: center; padding: 15px 20px; border-top: 1px solid #e2e8f0;">
            <button class="btn btn-primary" onclick="closeEmptyFolderDeletionModal()">
                <i class="fas fa-check"></i> 확인
            </button>
        </div>
    `;

    modalContent.innerHTML = resultHtml;
}

/**
 * 빈 폴더 삭제 모달 닫기
 */
function closeEmptyFolderDeletionModal() {
    const modal = document.getElementById('emptyFolderDeletionModal');
    if (modal) {
        modal.remove();
    }

    // 분석 결과 새로고침
    if (currentAnalysisResult) {
        showAnalysisResult(currentAnalysisResult);
    }
}

/**
 * 폴더 분석 모달 닫기
 */
function closeFolderAnalysisModal() {
    // 진행 상황 모니터링 중단
    if (window.currentAnalysisProgressInterval) {
        clearInterval(window.currentAnalysisProgressInterval);
        window.currentAnalysisProgressInterval = null;
    }

    const modal = document.getElementById('folderAnalysisModal');
    if (modal) {
        modal.classList.add('hidden');

        // 상태 초기화
        document.getElementById('folderAnalysisProgress').style.display = 'none';
        document.getElementById('folderAnalysisResult').style.display = 'none';
        document.getElementById('folderAnalysisUrl').value = '';

        currentAnalysisResult = null;
        currentAnalysisFolderId = null;
        isAnalysisCancelled = false;
        currentExcludePatterns = []; // 제외 패턴 초기화
    }
}

/**
 * 폴더 분석 취소
 */
async function cancelFolderAnalysis() {
    if (!currentAnalysisFolderId) {
        console.log('취소할 분석이 없습니다.');
        closeFolderAnalysisModal();
        return;
    }

    // 취소 플래그 설정
    isAnalysisCancelled = true;

    // 진행 상황 모니터링 중단
    if (window.currentAnalysisProgressInterval) {
        clearInterval(window.currentAnalysisProgressInterval);
        window.currentAnalysisProgressInterval = null;
    }

    // 취소 버튼 비활성화 및 상태 표시
    const cancelBtn = document.getElementById('cancelAnalysisBtn');
    if (cancelBtn) {
        cancelBtn.disabled = true;
        cancelBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 취소 중...';
    }

    // 진행 상황 텍스트 업데이트
    const progressText = document.getElementById('analysisProgressText');
    if (progressText) {
        progressText.textContent = '분석 취소 중...';
    }

    try {
        const response = await fetch(`${API_BASE}/api/folder/analyze/cancel`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                folderId: currentAnalysisFolderId
            })
        });

        const result = await response.json();
        console.log('분석 취소 결과:', result);

        // 취소 완료 UI 표시
        showAnalysisCancelled();

    } catch (error) {
        console.error('분석 취소 오류:', error);
        // 오류가 발생해도 모달 닫기
        closeFolderAnalysisModal();
    }
}

/**
 * 분석 취소 완료 UI 표시
 */
function showAnalysisCancelled() {
    // 진행 상황 섹션 업데이트
    const progressSection = document.getElementById('folderAnalysisProgress');
    if (progressSection) {
        progressSection.innerHTML = `
            <div class="step-header" style="color: #f59e0b;">
                <i class="fas fa-ban"></i> 폴더 분석 취소됨
            </div>

            <div style="text-align: center; padding: 30px;">
                <i class="fas fa-exclamation-circle" style="font-size: 48px; color: #f59e0b; margin-bottom: 15px;"></i>
                <p style="color: #6b7280; margin-bottom: 20px;">폴더 분석이 사용자에 의해 취소되었습니다.</p>
                <button class="btn btn-primary" onclick="closeFolderAnalysisModal()">
                    <i class="fas fa-times"></i> 닫기
                </button>
            </div>
        `;
        progressSection.style.display = 'block';
    }

    // 결과 섹션 숨기기
    document.getElementById('folderAnalysisResult').style.display = 'none';
}

// 모달 외부 클릭 시 닫기
document.getElementById('folderAnalysisModal')?.addEventListener('click', function(e) {
    if (e.target === this) {
        closeFolderAnalysisModal();
    }
});

// ESC 키로 모달 닫기
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        closeFolderAnalysisModal();
        closeCleanupResultModal();
    }
});

/**
 * 빈 하위폴더 정리 기능
 */
async function cleanupEmptySubfolders() {
    const folderUrl = document.getElementById('folderAnalysisUrl').value.trim();
    
    if (!folderUrl) {
        alert('폴더 URL 또는 ID를 입력하세요.');
        return;
    }
    
    // 확인 다이얼로그
    if (!confirm('이 폴더의 모든 빈 하위폴더를 삭제하시겠습니까?\n\n주의: 삭제된 폴더는 복구할 수 없습니다.')) {
        return;
    }
    
    // 모달 표시
    const modal = document.getElementById('cleanupResultModal');
    if (modal) {
        modal.classList.remove('hidden');
        showCleanupProgress();
    }
    
    try {
        // 폴더 ID 추출
        const folderId = await extractFolderIdFromUrl(folderUrl);
        
        // 진행 상황 모니터링 시작
        monitorCleanupProgress();
        
        // 빈 하위폴더 정리 시작
        const response = await fetch(`${API_BASE}/api/folder/cleanup-empty-subfolders`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                parentFolderId: folderId,
                url: folderUrl
            })
        });
        
        // 진행 상황 모니터링 중단
        if (window.currentCleanupProgressInterval) {
            clearInterval(window.currentCleanupProgressInterval);
        }
        
        if (!response.ok) {
            throw new Error('빈 하위폴더 정리 요청이 실패했습니다.');
        }
        
        const result = await response.json();
        
        // 최종 진행률 100%로 설정
        const progressBar = document.getElementById('cleanupProgressBar');
        const progressPercentage = document.getElementById('cleanupProgressPercentage');
        const progressText = document.getElementById('cleanupProgressText');
        
        progressBar.style.width = '100%';
        progressPercentage.textContent = '100%';
        progressText.textContent = '정리 완료!';
        
        // 1초 후 결과 표시
        setTimeout(() => {
            showCleanupResult(result);
        }, 1000);
        
    } catch (error) {
        console.error('빈 하위폴더 정리 오류:', error);
        
        // 진행 상황 모니터링 중단
        if (window.currentCleanupProgressInterval) {
            clearInterval(window.currentCleanupProgressInterval);
        }
        
        alert(`빈 하위폴더 정리 중 오류가 발생했습니다: ${error.message}`);
        closeCleanupResultModal();
    }
}

/**
 * 정리 진행 상황 표시
 */
function showCleanupProgress() {
    document.getElementById('cleanupProgress').style.display = 'block';
    document.getElementById('cleanupResult').style.display = 'none';
    
    // 초기 상태 설정
    const progressBar = document.getElementById('cleanupProgressBar');
    const progressPercentage = document.getElementById('cleanupProgressPercentage');
    const progressText = document.getElementById('cleanupProgressText');
    
    progressBar.style.width = '0%';
    progressPercentage.textContent = '0%';
    progressText.textContent = '빈 폴더 검색 준비 중...';
}

/**
 * 정리 진행 상황 모니터링
 */
function monitorCleanupProgress() {
    let attempts = 0;
    const maxAttempts = 120; // 2분 최대 대기
    
    const progressInterval = setInterval(async () => {
        attempts++;
        
        try {
            const progressBar = document.getElementById('cleanupProgressBar');
            const progressPercentage = document.getElementById('cleanupProgressPercentage');
            const progressText = document.getElementById('cleanupProgressText');
            
            // 가상의 진행률 업데이트
            const currentProgress = Math.min(10 + (attempts * 3), 95);
            progressBar.style.width = `${currentProgress}%`;
            progressPercentage.textContent = `${currentProgress}%`;
            
            // 진행 단계별 메시지
            if (currentProgress < 30) {
                progressText.textContent = '빈 폴더 검색 중...';
            } else if (currentProgress < 60) {
                progressText.textContent = '하위 폴더 스캔 중...';
            } else if (currentProgress < 90) {
                progressText.textContent = '빈 폴더 삭제 중...';
            } else {
                progressText.textContent = '정리 마무리 중...';
            }
            
            // 최대 시도 횟수 도달 시 타임아웃
            if (attempts >= maxAttempts) {
                clearInterval(progressInterval);
                progressText.textContent = '정리가 오래 걸리고 있습니다...';
            }
            
        } catch (error) {
            console.error('정리 진행 상황 조회 오류:', error);
            
            if (attempts >= maxAttempts) {
                clearInterval(progressInterval);
                alert('정리 진행 상황을 확인할 수 없습니다.');
            }
        }
    }, 1000);
    
    window.currentCleanupProgressInterval = progressInterval;
}

/**
 * 정리 결과 표시
 */
function showCleanupResult(result) {
    document.getElementById('cleanupProgress').style.display = 'none';
    document.getElementById('cleanupResult').style.display = 'block';
    
    // 정리 요약 표시
    const summaryHtml = `
        <div class="cleanup-summary">
            <div class="summary-header">
                <h3><i class="fas fa-folder"></i> ${result.parentFolderName}</h3>
                <div class="summary-stats">
                    <div class="stat-item">
                        <span class="stat-value">${result.totalScanned}</span>
                        <span class="stat-label">스캔된 폴더</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" style="color: #ef4444;">${result.deletedCount}</span>
                        <span class="stat-label">삭제됨</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" style="color: #f59e0b;">${result.skippedCount}</span>
                        <span class="stat-label">건너뜀</span>
                    </div>
                </div>
            </div>
        </div>
    `;
    
    document.getElementById('cleanupSummary').innerHTML = summaryHtml;
    
    // 삭제된 폴더 목록 표시
    if (result.deletedCount > 0) {
        const deletedHtml = result.deletedFolders.map(folderName => `
            <div class="cleanup-folder-item">
                <i class="fas fa-check-circle" style="color: #10b981;"></i>
                <span>${folderName}</span>
            </div>
        `).join('');
        
        document.getElementById('deletedFoldersContent').innerHTML = deletedHtml;
        document.getElementById('deletedFoldersList').style.display = 'block';
    }
    
    // 건너뛴 폴더 목록 표시
    if (result.skippedCount > 0) {
        const skippedHtml = result.skippedFolders.map(folderName => `
            <div class="cleanup-folder-item">
                <i class="fas fa-exclamation-triangle" style="color: #f59e0b;"></i>
                <span>${folderName}</span>
            </div>
        `).join('');
        
        document.getElementById('skippedFoldersContent').innerHTML = skippedHtml;
        document.getElementById('skippedFoldersList').style.display = 'block';
    }
}

/**
 * 정리 결과 모달 닫기
 */
function closeCleanupResultModal() {
    // 진행 상황 모니터링 중단
    if (window.currentCleanupProgressInterval) {
        clearInterval(window.currentCleanupProgressInterval);
        window.currentCleanupProgressInterval = null;
    }
    
    const modal = document.getElementById('cleanupResultModal');
    if (modal) {
        modal.classList.add('hidden');
        
        // 상태 초기화
        document.getElementById('cleanupProgress').style.display = 'none';
        document.getElementById('cleanupResult').style.display = 'none';
        document.getElementById('deletedFoldersList').style.display = 'none';
        document.getElementById('skippedFoldersList').style.display = 'none';
    }
}

// 정리 결과 모달 외부 클릭 시 닫기
document.getElementById('cleanupResultModal')?.addEventListener('click', function(e) {
    if (e.target === this) {
        closeCleanupResultModal();
    }
});

// ==========================================
// 특정 폴더명 검색 및 삭제 기능
// ==========================================

// 검색된 폴더 결과 저장
let searchedFoldersResult = null;

/**
 * 특정 폴더명으로 폴더 검색
 */
async function searchFoldersByName() {
    const folderUrl = document.getElementById('searchRootFolderUrl').value.trim();
    const folderName = document.getElementById('deleteFolderName').value.trim();
    const skipStats = document.getElementById('skipFolderStats')?.checked || false;

    if (!folderUrl) {
        alert('검색 대상 폴더 URL 또는 ID를 입력하세요.');
        return;
    }

    if (!folderName) {
        alert('검색할 폴더명을 입력하세요.');
        return;
    }

    // 모달 표시 및 진행 상황 표시
    const modal = document.getElementById('folderNameSearchModal');
    modal.classList.remove('hidden');

    document.getElementById('folderNameSearchProgress').style.display = 'block';
    document.getElementById('folderNameSearchResult').style.display = 'none';
    document.getElementById('folderNameDeleteProgress').style.display = 'none';
    document.getElementById('folderNameDeleteComplete').style.display = 'none';

    const searchModeText = skipStats ? ' (빠른 검색)' : '';
    document.getElementById('folderNameSearchProgressText').textContent = `'${folderName}' 폴더 검색 중...${searchModeText}`;

    try {
        // 폴더 ID 추출
        const folderId = await extractFolderIdFromUrl(folderUrl);

        // 폴더 검색 API 호출 (SSE 사용)
        const response = await fetch(`${API_BASE}/api/folder/find-by-name`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                folderId: folderId,
                folderName: folderName,
                skipStats: skipStats
            })
        });

        if (!response.ok) {
            throw new Error('폴더 검색 요청이 실패했습니다.');
        }

        // SSE 스트림 읽기
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });

            // SSE 이벤트 파싱
            const lines = buffer.split('\n\n');
            buffer = lines.pop() || ''; // 마지막 불완전한 이벤트는 버퍼에 유지

            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    try {
                        const data = JSON.parse(line.substring(6));
                        handleFolderSearchSSE(data, folderName);
                    } catch (e) {
                        console.error('SSE 데이터 파싱 오류:', e);
                    }
                }
            }
        }

    } catch (error) {
        console.error('폴더 검색 오류:', error);
        alert(`폴더 검색 중 오류가 발생했습니다: ${error.message}`);
        closeFolderNameSearchModal();
    }
}

/**
 * 폴더 검색 SSE 이벤트 처리
 */
function handleFolderSearchSSE(data, folderName) {
    const progressText = document.getElementById('folderNameSearchProgressText');

    switch (data.type) {
        case 'progress':
            progressText.textContent = `'${folderName}' 검색 중... (${data.scannedFolders}개 폴더 확인, ${data.matchedCount}개 발견)`;
            break;

        case 'found':
            progressText.textContent = `'${folderName}' 검색 중... (${data.scannedFolders}개 폴더 확인, ${data.matchedCount}개 발견)`;
            console.log(`폴더 발견: ${data.matchedFolder.path}`);
            break;

        case 'stats':
            progressText.textContent = `검색 완료! ${data.matchedCount}개 폴더 발견. 통계 계산 중...`;
            break;

        case 'complete':
            searchedFoldersResult = data.result;
            document.getElementById('folderNameSearchProgress').style.display = 'none';
            showFolderSearchResult(data.result);
            break;

        case 'error':
            alert(`폴더 검색 오류: ${data.error}`);
            closeFolderNameSearchModal();
            break;

        default:
            console.log('알 수 없는 SSE 이벤트:', data);
    }
}

/**
 * 폴더 검색 결과 표시
 */
function showFolderSearchResult(result) {
    document.getElementById('folderNameSearchResult').style.display = 'block';

    // 통계 정보가 있는지 확인 (빠른 검색 시 0)
    const hasStats = result.totalFiles > 0 || result.totalSize > 0;

    // 요약 정보 표시
    let summaryHtml;
    if (hasStats) {
        summaryHtml = `
            <div style="display: flex; justify-content: space-around; text-align: center;">
                <div>
                    <div style="font-size: 1.5rem; font-weight: bold; color: #3b82f6;">${result.totalCount}</div>
                    <div style="color: #6b7280; font-size: 0.9rem;">발견된 폴더</div>
                </div>
                <div>
                    <div style="font-size: 1.5rem; font-weight: bold; color: #10b981;">${result.totalFiles}</div>
                    <div style="color: #6b7280; font-size: 0.9rem;">총 파일 수</div>
                </div>
                <div>
                    <div style="font-size: 1.5rem; font-weight: bold; color: #f59e0b;">${formatFileSize(result.totalSize)}</div>
                    <div style="color: #6b7280; font-size: 0.9rem;">총 크기</div>
                </div>
            </div>
            <div style="text-align: center; margin-top: 15px; padding-top: 15px; border-top: 1px solid #e2e8f0;">
                <span style="background: #dbeafe; color: #1d4ed8; padding: 4px 12px; border-radius: 20px; font-size: 0.9rem;">
                    <i class="fas fa-search"></i> 검색어: <strong>${result.searchPattern}</strong>
                </span>
            </div>
        `;
    } else {
        // 빠른 검색 (통계 생략)
        summaryHtml = `
            <div style="display: flex; justify-content: center; text-align: center;">
                <div>
                    <div style="font-size: 1.5rem; font-weight: bold; color: #3b82f6;">${result.totalCount}</div>
                    <div style="color: #6b7280; font-size: 0.9rem;">발견된 폴더</div>
                </div>
            </div>
            <div style="text-align: center; margin-top: 15px; padding-top: 15px; border-top: 1px solid #e2e8f0;">
                <span style="background: #dbeafe; color: #1d4ed8; padding: 4px 12px; border-radius: 20px; font-size: 0.9rem;">
                    <i class="fas fa-search"></i> 검색어: <strong>${result.searchPattern}</strong>
                </span>
                <span style="background: #fef3c7; color: #92400e; padding: 4px 12px; border-radius: 20px; font-size: 0.9rem; margin-left: 8px;">
                    <i class="fas fa-bolt"></i> 빠른 검색 (통계 생략)
                </span>
            </div>
        `;
    }
    document.getElementById('folderNameSearchSummary').innerHTML = summaryHtml;

    // 폴더 목록 표시
    if (result.matchedFolders && result.matchedFolders.length > 0) {
        const listHtml = result.matchedFolders.map(folder => {
            // 통계 정보가 있는지 확인
            const folderHasStats = folder.fileCount > 0 || folder.size > 0;
            const statsHtml = folderHasStats
                ? `<div style="text-align: right; flex-shrink: 0;">
                       <div style="font-weight: 500; color: #374151;">${formatFileSize(folder.size)}</div>
                       <div style="font-size: 0.8rem; color: #6b7280;">${folder.fileCount}개 파일</div>
                   </div>`
                : `<div style="text-align: right; flex-shrink: 0;">
                       <div style="font-size: 0.8rem; color: #9ca3af;"><i class="fas fa-bolt"></i> 빠른 검색</div>
                   </div>`;
            return `
                <div class="searched-folder-item" style="display: flex; align-items: center; padding: 12px 15px; border-bottom: 1px solid #e2e8f0; gap: 12px;">
                    <label style="display: flex; align-items: center; cursor: pointer;">
                        <input type="checkbox" class="searched-folder-check" data-folder-id="${folder.id}" onchange="updateSearchedFolderSelection()">
                    </label>
                    <div style="flex: 1; min-width: 0;">
                        <div style="display: flex; align-items: center; gap: 8px;">
                            <i class="fas fa-folder" style="color: #f59e0b;"></i>
                            <span style="font-weight: 500; color: #1e293b;">${folder.name}</span>
                        </div>
                        <div style="font-size: 0.85rem; color: #6b7280; margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                            ${folder.path}
                        </div>
                    </div>
                    ${statsHtml}
                </div>
            `;
        }).join('');
        document.getElementById('folderNameSearchList').innerHTML = listHtml;
    } else {
        document.getElementById('folderNameSearchList').innerHTML = `
            <div style="text-align: center; padding: 40px; color: #6b7280;">
                <i class="fas fa-folder-open" style="font-size: 48px; margin-bottom: 15px; color: #d1d5db;"></i>
                <div>검색 결과가 없습니다.</div>
            </div>
        `;
    }

    // 선택 상태 초기화
    updateSearchedFolderSelection();
}

/**
 * 검색된 폴더 전체 선택
 */
function selectAllSearchedFolders() {
    const checkboxes = document.querySelectorAll('.searched-folder-check');
    checkboxes.forEach(checkbox => {
        checkbox.checked = true;
    });
    updateSearchedFolderSelection();
}

/**
 * 검색된 폴더 전체 해제
 */
function deselectAllSearchedFolders() {
    const checkboxes = document.querySelectorAll('.searched-folder-check');
    checkboxes.forEach(checkbox => {
        checkbox.checked = false;
    });
    updateSearchedFolderSelection();
}

/**
 * 검색된 폴더 선택 상태 업데이트
 */
function updateSearchedFolderSelection() {
    const checkedCount = document.querySelectorAll('.searched-folder-check:checked').length;

    const countSpan = document.getElementById('selectedSearchedFoldersCount');
    if (countSpan) {
        countSpan.textContent = `${checkedCount}개 선택됨`;
    }

    const deleteBtn = document.getElementById('deleteSelectedSearchedFoldersBtn');
    if (deleteBtn) {
        deleteBtn.disabled = checkedCount === 0;
    }
}

/**
 * 선택된 폴더 삭제
 */
async function deleteSelectedSearchedFolders() {
    const checkedBoxes = document.querySelectorAll('.searched-folder-check:checked');
    const selectedIds = Array.from(checkedBoxes).map(cb => cb.dataset.folderId);

    if (selectedIds.length === 0) {
        alert('삭제할 폴더를 선택하세요.');
        return;
    }

    // 선택된 폴더 정보 가져오기
    const selectedFolders = searchedFoldersResult.matchedFolders.filter(
        folder => selectedIds.includes(folder.id)
    );

    // 총 크기 계산
    const totalSize = selectedFolders.reduce((sum, f) => sum + f.size, 0);
    const totalFiles = selectedFolders.reduce((sum, f) => sum + f.fileCount, 0);
    const hasStats = totalFiles > 0 || totalSize > 0;

    // 통계 정보 HTML (빠른 검색 시 숨김)
    const statsHtml = hasStats
        ? `<div style="font-size: 0.9rem; color: #6b7280; margin-bottom: 12px;">
               <div>• 총 파일: <strong>${totalFiles}개</strong></div>
               <div>• 총 크기: <strong>${formatFileSize(totalSize)}</strong></div>
           </div>`
        : `<div style="font-size: 0.9rem; color: #9ca3af; margin-bottom: 12px;">
               <i class="fas fa-bolt"></i> 빠른 검색으로 인해 통계 정보가 없습니다
           </div>`;

    const message = `
        <div style="margin-bottom: 16px;">
            <strong>선택된 ${selectedIds.length}개의 폴더를 삭제하시겠습니까?</strong>
        </div>
        ${statsHtml}
        <div style="font-size: 0.9rem; color: #6b7280; max-height: 100px; overflow-y: auto; background: #f8f9fa; padding: 8px; border-radius: 4px;">
            ${selectedFolders.slice(0, 5).map(f => `• ${f.name} (${f.path})`).join('<br>')}
            ${selectedFolders.length > 5 ? `<br>... 외 ${selectedFolders.length - 5}개` : ''}
        </div>
    `;

    // Google Drive 폴더 삭제이므로 휴지통/완전삭제 옵션 표시
    showDeleteConfirmation(message, async (data) => {
        await executeDeleteSearchedFolders(selectedIds, data.permanentDelete);
    }, { folderIds: selectedIds }, true);
}

/**
 * 폴더 삭제 실행
 */
async function executeDeleteSearchedFolders(folderIds, permanentDelete = false) {
    // 검색 결과 숨기고 삭제 진행 상황 표시
    document.getElementById('folderNameSearchResult').style.display = 'none';
    document.getElementById('folderNameDeleteProgress').style.display = 'block';

    const deleteMethodText = permanentDelete ? '완전 삭제' : '휴지통 이동';
    document.getElementById('folderNameDeleteText').textContent = `폴더 ${deleteMethodText} 중...`;

    try {
        // SSE를 사용한 실시간 진행 상황 수신
        const response = await fetch(`${API_BASE}/api/folder/delete-by-name`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'text/event-stream',
            },
            body: JSON.stringify({
                folderIds: folderIds,
                permanentDelete: permanentDelete
            })
        });

        if (!response.ok) {
            throw new Error('폴더 삭제 요청이 실패했습니다.');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        let finalResult = null;

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });

            // SSE 이벤트 파싱
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            let eventType = null;
            let eventData = null;

            for (const line of lines) {
                if (line.startsWith(':')) continue; // heartbeat

                if (line.startsWith('event: ')) {
                    eventType = line.slice(7).trim();
                } else if (line.startsWith('data: ')) {
                    try {
                        eventData = JSON.parse(line.slice(6));
                    } catch (e) {
                        console.error('SSE 데이터 파싱 오류:', e);
                    }
                } else if (line === '' && eventType && eventData) {
                    if (eventType === 'progress') {
                        updateFolderNameDeleteProgress(eventData);
                    } else if (eventType === 'complete') {
                        finalResult = eventData;
                    }
                    eventType = null;
                    eventData = null;
                }
            }
        }

        // 결과 표시
        if (finalResult) {
            showFolderNameDeleteComplete(finalResult);
        }

    } catch (error) {
        console.error('폴더 삭제 오류:', error);
        alert(`폴더 삭제 중 오류가 발생했습니다: ${error.message}`);
        closeFolderNameSearchModal();
    }
}

/**
 * 삭제 진행 상황 업데이트
 */
function updateFolderNameDeleteProgress(data) {
    const percentage = Math.round((data.completed / data.total) * 100);

    document.getElementById('folderNameDeletePercentage').textContent = `${percentage}%`;
    document.getElementById('folderNameDeleteBar').style.width = `${percentage}%`;
    document.getElementById('folderNameDeleteText').textContent =
        `${data.completed} / ${data.total} 폴더 삭제 완료 (성공: ${data.success || 0}, 실패: ${data.failed || 0})`;
}

/**
 * 삭제 완료 결과 표시
 */
function showFolderNameDeleteComplete(result) {
    document.getElementById('folderNameDeleteProgress').style.display = 'none';
    document.getElementById('folderNameDeleteComplete').style.display = 'block';

    document.getElementById('folderNameDeleteResult').innerHTML = `
        <div style="display: flex; justify-content: center; gap: 40px; margin-bottom: 15px;">
            <div>
                <span style="font-size: 1.5rem; font-weight: bold; color: #10b981;">${result.successCount}</span>
                <span style="color: #6b7280;"> 성공</span>
            </div>
            <div>
                <span style="font-size: 1.5rem; font-weight: bold; color: #ef4444;">${result.failedCount}</span>
                <span style="color: #6b7280;"> 실패</span>
            </div>
        </div>
        <div style="color: #6b7280;">
            총 ${result.totalCount}개 폴더 중 ${result.successCount}개 삭제됨
        </div>
    `;
}

/**
 * 폴더명 검색 모달 닫기
 */
function closeFolderNameSearchModal() {
    const modal = document.getElementById('folderNameSearchModal');
    if (modal) {
        modal.classList.add('hidden');
    }
    searchedFoldersResult = null;
}

// ==========================================
// 폴더 파일 목록 스프레드시트 내보내기
// ==========================================

// 현재 내보내기 취소 컨트롤러
let currentExportController = null;

/**
 * 폴더 파일 목록을 Google 스프레드시트로 내보내기
 */
function exportFolderToSpreadsheet() {
    // 현재 분석 결과에서 폴더 ID 추출
    const folderUrl = document.getElementById('folderAnalysisUrl').value.trim();
    if (!folderUrl && !currentAnalysisResult) {
        alert('먼저 폴더를 분석하세요.');
        return;
    }

    // 버튼 비활성화
    const exportBtn = document.getElementById('exportSpreadsheetBtn');
    if (exportBtn) {
        exportBtn.disabled = true;
        exportBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 내보내기 중...';
    }

    // 내보내기 진행 상황 모달 표시
    showExportProgressModal();

    // 폴더 ID 추출
    extractFolderIdFromUrl(folderUrl).then(folderId => {
        currentExportController = window.API.Export.exportFolderToSpreadsheet(
            folderId,
            { includeSubfolders: true },
            // onProgress
            (event) => {
                updateExportProgress(event);
            },
            // onComplete
            (result) => {
                showExportComplete(result);
                resetExportButton();
            },
            // onError
            (errorMessage) => {
                showExportError(errorMessage);
                resetExportButton();
            }
        );
    }).catch(error => {
        showExportError(error.message);
        resetExportButton();
    });
}

/**
 * 내보내기 버튼 상태 초기화
 */
function resetExportButton() {
    const exportBtn = document.getElementById('exportSpreadsheetBtn');
    if (exportBtn) {
        exportBtn.disabled = false;
        exportBtn.innerHTML = '<i class="fas fa-file-export"></i> 스프레드시트로 내보내기';
    }
}

/**
 * 내보내기 진행 상황 모달 표시
 */
function showExportProgressModal() {
    let modal = document.getElementById('exportProgressModal');
    if (modal) modal.remove();

    modal = document.createElement('div');
    modal.id = 'exportProgressModal';
    modal.className = 'modal';
    modal.innerHTML = `
        <div class="modal-content" style="max-width: 450px;">
            <div class="modal-header">
                <h2 class="modal-title">
                    <i class="fas fa-file-export"></i> 스프레드시트 내보내기
                </h2>
            </div>
            <div class="modal-body" id="exportProgressBody" style="text-align: center; padding: 30px;">
                <div style="margin-bottom: 20px;">
                    <i class="fas fa-spinner fa-spin" style="font-size: 48px; color: #3b82f6;"></i>
                </div>
                <div style="font-size: 1.1rem; color: #374151; margin-bottom: 8px;">
                    파일 목록 수집 준비 중...
                </div>
                <div style="font-size: 0.9rem; color: #6b7280;" id="exportProgressDetail">
                </div>
            </div>
        </div>
    `;
    modal.addEventListener('click', function(e) {
        if (e.target === this) {
            cancelExport();
        }
    });
    document.body.appendChild(modal);
}

/**
 * 내보내기 진행 상황 업데이트
 */
function updateExportProgress(event) {
    const body = document.getElementById('exportProgressBody');
    if (!body) return;

    let icon = 'fa-spinner fa-spin';
    let color = '#3b82f6';
    let message = event.message || '처리 중...';
    let detail = '';

    switch (event.type) {
        case 'scanning':
            icon = 'fa-search';
            if (event.scannedFiles > 0 || event.scannedFolders > 0) {
                detail = `파일: ${event.scannedFiles || 0}개, 폴더: ${event.scannedFolders || 0}개`;
            }
            break;
        case 'generating':
            icon = 'fa-file-csv';
            color = '#f59e0b';
            detail = `파일: ${event.totalFiles}개, 폴더: ${event.totalFolders}개`;
            break;
        case 'uploading':
            icon = 'fa-cloud-upload-alt';
            color = '#8b5cf6';
            detail = `파일: ${event.totalFiles}개, 폴더: ${event.totalFolders}개`;
            break;
    }

    body.innerHTML = `
        <div style="margin-bottom: 20px;">
            <i class="fas ${icon}" style="font-size: 48px; color: ${color};"></i>
        </div>
        <div style="font-size: 1.1rem; color: #374151; margin-bottom: 8px;">
            ${message}
        </div>
        <div style="font-size: 0.9rem; color: #6b7280;">
            ${detail}
        </div>
    `;
}

/**
 * 내보내기 완료 표시
 */
function showExportComplete(result) {
    const modal = document.getElementById('exportProgressModal');
    if (!modal) return;

    const content = modal.querySelector('.modal-content');
    content.innerHTML = `
        <div class="modal-header">
            <h2 class="modal-title">
                <i class="fas fa-check-circle" style="color: #10b981;"></i> 내보내기 완료
            </h2>
        </div>
        <div class="modal-body" style="text-align: center; padding: 30px;">
            <div style="margin-bottom: 20px;">
                <i class="fas fa-file-spreadsheet" style="font-size: 48px; color: #10b981;"></i>
            </div>
            <div style="font-size: 1.1rem; color: #374151; margin-bottom: 15px;">
                <strong>${result.folderName}</strong> 폴더의 파일 목록이<br>
                Google 스프레드시트로 내보내졌습니다.
            </div>
            <div style="display: flex; justify-content: center; gap: 30px; margin-bottom: 20px;">
                <div>
                    <div style="font-size: 1.3rem; font-weight: bold; color: #3b82f6;">${result.totalFiles}</div>
                    <div style="font-size: 0.85rem; color: #6b7280;">파일</div>
                </div>
                <div>
                    <div style="font-size: 1.3rem; font-weight: bold; color: #f59e0b;">${result.totalFolders}</div>
                    <div style="font-size: 0.85rem; color: #6b7280;">폴더</div>
                </div>
            </div>
            <a href="${result.spreadsheetUrl}" target="_blank"
               style="display: inline-block; padding: 10px 24px; background: #10b981; color: white; border-radius: 8px; text-decoration: none; font-weight: 500;">
                <i class="fas fa-external-link-alt"></i> 스프레드시트 열기
            </a>
        </div>
        <div class="modal-footer" style="display: flex; justify-content: center; padding: 15px 20px; border-top: 1px solid #e2e8f0;">
            <button class="btn btn-primary" onclick="closeExportModal()">
                <i class="fas fa-check"></i> 확인
            </button>
        </div>
    `;
}

/**
 * 내보내기 오류 표시
 */
function showExportError(errorMessage) {
    const modal = document.getElementById('exportProgressModal');
    if (!modal) return;

    const content = modal.querySelector('.modal-content');
    content.innerHTML = `
        <div class="modal-header">
            <h2 class="modal-title">
                <i class="fas fa-exclamation-circle" style="color: #ef4444;"></i> 내보내기 실패
            </h2>
        </div>
        <div class="modal-body" style="text-align: center; padding: 30px;">
            <div style="margin-bottom: 20px;">
                <i class="fas fa-times-circle" style="font-size: 48px; color: #ef4444;"></i>
            </div>
            <div style="font-size: 1rem; color: #374151; margin-bottom: 10px;">
                스프레드시트 내보내기 중 오류가 발생했습니다.
            </div>
            <div style="font-size: 0.9rem; color: #6b7280; background: #fef2f2; padding: 10px; border-radius: 6px;">
                ${errorMessage}
            </div>
        </div>
        <div class="modal-footer" style="display: flex; justify-content: center; padding: 15px 20px; border-top: 1px solid #e2e8f0;">
            <button class="btn btn-primary" onclick="closeExportModal()">
                <i class="fas fa-times"></i> 닫기
            </button>
        </div>
    `;
}

/**
 * 내보내기 취소
 */
function cancelExport() {
    if (currentExportController) {
        currentExportController.abort();
        currentExportController = null;
    }
    closeExportModal();
    resetExportButton();
}

/**
 * 내보내기 모달 닫기
 */
function closeExportModal() {
    const modal = document.getElementById('exportProgressModal');
    if (modal) modal.remove();
    currentExportController = null;
}

// 모달 외부 클릭 시 닫기
document.getElementById('folderNameSearchModal')?.addEventListener('click', function(e) {
    if (e.target === this) {
        closeFolderNameSearchModal();
    }
});