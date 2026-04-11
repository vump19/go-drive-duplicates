/**
 * Batch Folder Archive JavaScript Module
 * 폴더 일괄 아카이브 기능을 위한 JavaScript 모듈
 */

// API 기본 URL
const API_BASE_URL = window.API_BASE || 'http://localhost:9090';

// 전역 상태
let folderCount = 0;
let currentJobId = null;
let pollingInterval = null;

// 페이지 로드 시 초기화
document.addEventListener('DOMContentLoaded', () => {
    // 기본 폴더 항목 3개 추가
    for (let i = 0; i < 3; i++) {
        addFolderItem();
    }

    // 고정 폴더 입력 필드에 경로 표시 연결
    if (typeof attachFolderResolver === 'function') {
        const parentInput = document.getElementById('parentFolderInput');
        const uploadInput = document.getElementById('uploadFolderUrl');
        if (parentInput) attachFolderResolver(parentInput);
        if (uploadInput) attachFolderResolver(uploadInput);
    }
});

/**
 * 폴더 항목 추가
 */
function addFolderItem() {
    folderCount++;
    const container = document.getElementById('folderListContainer');

    const item = document.createElement('div');
    item.className = 'folder-item';
    item.id = `folder-item-${folderCount}`;
    item.dataset.index = folderCount;

    item.innerHTML = `
        <div class="folder-item-number">${folderCount}</div>
        <input type="text" class="folder-item-input" placeholder="Google Drive 폴더 URL 또는 ID 입력" data-index="${folderCount}">
        <div class="folder-item-status"></div>
        <button class="folder-item-delete" onclick="removeFolderItem(${folderCount})" title="삭제">
            <i class="fas fa-times"></i>
        </button>
    `;

    container.appendChild(item);
    updateFolderNumbers();

    // 새 입력 필드에 경로 표시 연결
    if (typeof attachFolderResolver === 'function') {
        const input = item.querySelector('.folder-item-input');
        if (input) attachFolderResolver(input);
    }
}

/**
 * 폴더 항목 제거
 */
function removeFolderItem(index) {
    const item = document.getElementById(`folder-item-${index}`);
    if (item) {
        item.remove();
        updateFolderNumbers();
    }
}

/**
 * 폴더 번호 업데이트
 */
function updateFolderNumbers() {
    const items = document.querySelectorAll('.folder-item');
    items.forEach((item, idx) => {
        const number = item.querySelector('.folder-item-number');
        if (number) {
            number.textContent = idx + 1;
        }
    });
}

/**
 * 모든 폴더 URL/ID 수집
 */
function collectFolderInputs() {
    const inputs = document.querySelectorAll('#folderListContainer .folder-item-input');
    const folders = [];

    inputs.forEach(input => {
        const value = input.value.trim();
        if (value) {
            // URL인지 ID인지 판별
            if (value.includes('drive.google.com') || value.includes('/folders/')) {
                folders.push({ url: value });
            } else {
                folders.push({ folderId: value });
            }
        }
    });

    return folders;
}

/**
 * 업로드 폴더 입력 필드 토글
 */
function toggleUploadFolderInput() {
    const checkbox = document.getElementById('useCustomUploadFolder');
    const input = document.getElementById('uploadFolderUrl');
    const hint = document.getElementById('uploadFolderHint');

    if (checkbox.checked) {
        input.style.display = 'block';
        hint.textContent = 'ZIP 파일이 지정된 폴더에 업로드됩니다';
    } else {
        input.style.display = 'none';
        input.value = '';
        hint.textContent = '기본: 각 폴더의 상위 폴더에 ZIP 파일 생성';
    }
}

/**
 * 부모 폴더에서 하위 폴더 불러오기
 */
async function loadSubfolders() {
    const input = document.getElementById('parentFolderInput');
    const value = input.value.trim();

    if (!value) {
        alert('부모 폴더 URL 또는 ID를 입력해주세요.');
        return;
    }

    const loadingArea = document.getElementById('subfoldersLoadingArea');
    const resultArea = document.getElementById('subfoldersResultArea');
    const btn = document.getElementById('loadSubfoldersBtn');

    // UI 상태: 로딩
    loadingArea.classList.remove('hidden');
    resultArea.classList.add('hidden');
    btn.disabled = true;

    // 요청 데이터 구성
    const requestBody = {};
    if (value.includes('drive.google.com') || value.includes('/folders/')) {
        requestBody.parentFolderUrl = value;
    } else {
        requestBody.parentFolderId = value;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/api/batch-archive/list-subfolders`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(requestBody)
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText);
        }

        const data = await response.json();

        // 로딩 숨기기
        loadingArea.classList.add('hidden');

        if (data.subfolders && data.subfolders.length > 0) {
            // 기존 빈 입력 필드 제거
            const existingInputs = document.querySelectorAll('#folderListContainer .folder-item-input');
            existingInputs.forEach(inp => {
                if (!inp.value.trim()) {
                    const item = inp.closest('.folder-item');
                    if (item) item.remove();
                }
            });

            // 하위 폴더를 목록에 추가
            data.subfolders.forEach(subfolder => {
                addFolderItemWithValue(subfolder.folderId, subfolder.name);
            });

            updateFolderNumbers();

            // 결과 표시
            resultArea.classList.remove('hidden');
            document.getElementById('subfoldersResultText').innerHTML =
                `<i class="fas fa-check-circle" style="color: #10b981;"></i> ` +
                `<strong>${data.parentFolderName}</strong> 폴더에서 <strong>${data.subfolders.length}</strong>개의 하위 폴더를 불러왔습니다.`;
        } else {
            resultArea.classList.remove('hidden');
            resultArea.style.background = '#fefce8';
            resultArea.style.borderColor = '#fde68a';
            document.getElementById('subfoldersResultText').innerHTML =
                `<i class="fas fa-info-circle" style="color: #f59e0b;"></i> ` +
                `<strong>${data.parentFolderName || '해당 폴더'}</strong>에 하위 폴더가 없습니다.`;
        }

    } catch (error) {
        console.error('하위 폴더 불러오기 실패:', error);
        loadingArea.classList.add('hidden');
        resultArea.classList.remove('hidden');
        resultArea.style.background = '#fef2f2';
        resultArea.style.borderColor = '#fecaca';
        document.getElementById('subfoldersResultText').innerHTML =
            `<i class="fas fa-exclamation-triangle" style="color: #ef4444;"></i> 불러오기 실패: ${error.message}`;
    } finally {
        btn.disabled = false;
    }
}

/**
 * 폴더 항목 추가 (값 포함)
 */
function addFolderItemWithValue(folderId, folderName) {
    folderCount++;
    const container = document.getElementById('folderListContainer');

    const item = document.createElement('div');
    item.className = 'folder-item';
    item.id = `folder-item-${folderCount}`;
    item.dataset.index = folderCount;

    item.innerHTML = `
        <div class="folder-item-number">${folderCount}</div>
        <input type="text" class="folder-item-input" value="${folderId}" data-index="${folderCount}">
        <div class="folder-item-name" style="color: #64748b; font-size: 0.85rem; min-width: 150px;">${folderName}</div>
        <div class="folder-item-status"></div>
        <button class="folder-item-delete" onclick="removeFolderItem(${folderCount})" title="삭제">
            <i class="fas fa-times"></i>
        </button>
    `;

    container.appendChild(item);
}

/**
 * 일괄 아카이브 시작
 */
async function startBatchArchive() {
    const folders = collectFolderInputs();

    if (folders.length === 0) {
        alert('아카이브할 폴더를 하나 이상 입력해주세요.');
        return;
    }

    // 옵션 수집
    const uploadFolderUrl = document.getElementById('uploadFolderUrl').value.trim();
    const options = {
        includeSubfolders: document.getElementById('includeSubfolders').checked,
        skipGoogleDocs: document.getElementById('skipGoogleDocs').checked,
        zipNameFormat: document.getElementById('zipNameFormat').value,
        deleteMethod: document.getElementById('deleteMethod').value,
        uploadFolderUrl: uploadFolderUrl || ''
    };

    // UI 상태 변경
    setUIState('running');

    try {
        const response = await fetch(`${API_BASE_URL}/api/batch-archive/start`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                folders: folders,
                options: options
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText);
        }

        const result = await response.json();
        currentJobId = result.jobId;

        console.log('📦 아카이브 작업 시작:', result);

        // 진행 상황 폴링 시작
        startProgressPolling();

    } catch (error) {
        console.error('아카이브 시작 실패:', error);
        alert('아카이브 시작 실패: ' + error.message);
        setUIState('idle');
    }
}

/**
 * 작업 취소
 */
async function cancelBatchArchive() {
    if (!currentJobId) return;

    if (!confirm('정말 작업을 취소하시겠습니까?\n진행 중인 폴더 처리가 완료된 후 중단됩니다.')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE_URL}/api/batch-archive/cancel?jobId=${currentJobId}`, {
            method: 'POST'
        });

        if (!response.ok) {
            throw new Error('취소 요청 실패');
        }

        console.log('🛑 작업 취소 요청됨');

    } catch (error) {
        console.error('취소 실패:', error);
        alert('취소 요청 실패: ' + error.message);
    }
}

/**
 * 진행 상황 폴링 시작
 */
function startProgressPolling() {
    // 기존 폴링 중지
    if (pollingInterval) {
        clearInterval(pollingInterval);
    }

    // 500ms 간격으로 폴링
    pollingInterval = setInterval(pollProgress, 500);
    pollProgress(); // 즉시 한 번 실행
}

/**
 * 진행 상황 폴링 중지
 */
function stopProgressPolling() {
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = null;
    }
}

/**
 * 진행 상황 조회
 */
async function pollProgress() {
    if (!currentJobId) return;

    try {
        const response = await fetch(`${API_BASE_URL}/api/batch-archive/progress?jobId=${currentJobId}`);

        if (!response.ok) {
            if (response.status === 404) {
                console.log('작업을 찾을 수 없음');
                stopProgressPolling();
                setUIState('idle');
                return;
            }
            throw new Error('진행 상황 조회 실패');
        }

        const job = await response.json();
        updateProgressUI(job);

        // 작업 완료 확인
        if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
            stopProgressPolling();
            setUIState('completed');
            showResults(job);

            // 스피너를 완료 아이콘으로 교체
            const spinnerEl = document.querySelector('#currentFolderInfo .current-folder-phase i');
            if (spinnerEl) {
                spinnerEl.className = job.status === 'completed'
                    ? 'fas fa-check-circle' : 'fas fa-times-circle';
                spinnerEl.style.color = job.status === 'completed' ? '#10b981' : '#ef4444';
            }
        }

    } catch (error) {
        console.error('진행 상황 조회 실패:', error);
    }
}

/**
 * 진행 상황 UI 업데이트
 */
function updateProgressUI(job) {
    const progressCard = document.getElementById('progressCard');
    progressCard.classList.remove('hidden');

    // 진행률 계산
    const progress = job.totalFolders > 0 ? Math.round((job.completedFolders / job.totalFolders) * 100) : 0;

    // 진행률 바 업데이트
    const progressBar = document.getElementById('progressBar');
    progressBar.style.width = `${progress}%`;
    progressBar.textContent = `${progress}%`;

    // 통계 업데이트
    document.getElementById('completedCount').textContent = job.completedFolders;
    document.getElementById('errorCount').textContent = job.errors ? job.errors.length : 0;
    document.getElementById('remainingCount').textContent = job.totalFolders - job.completedFolders - (job.errors ? job.errors.length : 0);

    // 제목 업데이트
    const progressTitle = document.getElementById('progressTitle');
    if (job.status === 'running') {
        progressTitle.textContent = `아카이브 진행 중... (${job.currentFolderIndex + 1}/${job.totalFolders})`;
    } else if (job.status === 'completed') {
        progressTitle.textContent = '아카이브 완료!';
    } else if (job.status === 'cancelled') {
        progressTitle.textContent = '아카이브 취소됨';
    } else if (job.status === 'failed') {
        progressTitle.textContent = '아카이브 실패';
    }

    // 현재 폴더 정보 업데이트
    if (job.currentFolder) {
        document.getElementById('currentFolderName').textContent = job.currentFolder.folderName || job.currentFolder.folderId;
        document.getElementById('currentPhaseText').textContent = getPhaseText(job.currentFolder.phase, job.currentFolder.message);

        // 단계 인디케이터 업데이트
        updatePhaseIndicator(job.currentFolder.phase);

        // 파일 다운로드 진행 상황 표시
        if (job.currentFolder.phase === 'downloading' && job.currentFolder.totalFiles > 0) {
            const downloadProgress = `${job.currentFolder.downloadedFiles}/${job.currentFolder.totalFiles} 파일`;
            document.getElementById('currentPhaseText').textContent = `다운로드 중... (${downloadProgress})`;
        }
    }

    // 폴더 목록 항목 상태 업데이트
    updateFolderItemsStatus(job);
}

/**
 * 단계 텍스트 반환
 */
function getPhaseText(phase, message) {
    if (message) return message;

    const phaseTexts = {
        'downloading': '파일 다운로드 중...',
        'zipping': 'ZIP 파일 생성 중...',
        'uploading': 'ZIP 파일 업로드 중...',
        'trashing': '원본 폴더 정리 중...',
        'completed': '완료',
        'failed': '실패'
    };

    return phaseTexts[phase] || phase;
}

/**
 * 단계 인디케이터 업데이트
 */
function updatePhaseIndicator(currentPhase) {
    const phases = ['downloading', 'zipping', 'uploading', 'trashing'];
    const steps = document.querySelectorAll('.phase-step');
    let foundCurrent = false;

    steps.forEach(step => {
        const stepPhase = step.dataset.phase;
        step.classList.remove('active', 'completed');

        if (stepPhase === currentPhase) {
            step.classList.add('active');
            foundCurrent = true;
        } else if (!foundCurrent && phases.indexOf(stepPhase) < phases.indexOf(currentPhase)) {
            step.classList.add('completed');
        }
    });
}

/**
 * 폴더 목록 항목 상태 업데이트
 */
function updateFolderItemsStatus(job) {
    const items = document.querySelectorAll('.folder-item');

    items.forEach((item, index) => {
        item.classList.remove('processing', 'completed', 'failed');
        const statusEl = item.querySelector('.folder-item-status');

        // 결과에서 해당 인덱스의 상태 확인
        if (job.results && job.results[index]) {
            const result = job.results[index];
            if (result.status === 'skipped') {
                item.classList.add('completed');
                statusEl.textContent = '건너뜀';
                statusEl.className = 'folder-item-status completed';
            } else if (result.status === 'completed') {
                item.classList.add('completed');
                statusEl.textContent = '완료';
                statusEl.className = 'folder-item-status completed';
            } else if (result.status === 'failed') {
                item.classList.add('failed');
                statusEl.textContent = '실패';
                statusEl.className = 'folder-item-status failed';
            }
        } else if (index === job.currentFolderIndex && job.status === 'running') {
            item.classList.add('processing');
            statusEl.textContent = '처리 중...';
            statusEl.className = 'folder-item-status processing';
        }
    });
}

/**
 * 결과 표시
 */
function showResults(job) {
    const resultsCard = document.getElementById('resultsCard');
    resultsCard.classList.remove('hidden');

    // 요약 업데이트
    document.getElementById('summaryTotal').textContent = job.totalFolders;
    document.getElementById('summaryCompleted').textContent = job.completedFolders;
    document.getElementById('summaryFailed').textContent = job.errors ? job.errors.length : 0;

    // 절약된 용량 계산
    let totalSaved = 0;
    if (job.results) {
        job.results.forEach(result => {
            if (result.savedSpace) {
                totalSaved += result.savedSpace;
            }
        });
    }
    document.getElementById('summarySaved').textContent = formatFileSize(totalSaved);

    // 결과 목록 생성
    const resultsList = document.getElementById('resultsList');
    resultsList.innerHTML = '';

    if (job.results) {
        job.results.forEach(result => {
            const resultItem = document.createElement('div');
            resultItem.className = `result-item ${result.status === 'completed' ? 'success' : 'error'}`;

            if (result.status === 'skipped') {
                resultItem.className = 'result-item';
                resultItem.style.borderLeft = '4px solid #f59e0b';
                resultItem.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">${result.folderName || result.folderId}</div>
                        <div class="result-details" style="color: #f59e0b;">
                            <i class="fas fa-forward"></i> 건너뜀 (다운로드할 파일 없음)
                        </div>
                    </div>
                `;
            } else if (result.status === 'completed') {
                resultItem.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">${result.folderName}</div>
                        <div class="result-details">
                            <i class="fas fa-file-archive"></i> ${result.zipFileName}
                        </div>
                    </div>
                    <div class="result-savings">
                        <div class="saved">-${formatFileSize(result.savedSpace)}</div>
                        <div class="original">원본: ${formatFileSize(result.originalSize)}</div>
                    </div>
                `;
            } else {
                resultItem.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">${result.folderName || result.folderId}</div>
                        <div class="result-details" style="color: #ef4444;">
                            <i class="fas fa-exclamation-triangle"></i> ${result.error || '알 수 없는 오류'}
                        </div>
                    </div>
                `;
            }

            resultsList.appendChild(resultItem);
        });
    }

    // 에러 목록도 표시
    if (job.errors && job.errors.length > 0) {
        job.errors.forEach(error => {
            const resultItem = document.createElement('div');
            resultItem.className = 'result-item error';
            resultItem.innerHTML = `
                <div class="result-info">
                    <div class="result-name">${error.folderName || error.folderId}</div>
                    <div class="result-details" style="color: #ef4444;">
                        <i class="fas fa-exclamation-triangle"></i> [${error.phase}] ${error.error}
                    </div>
                </div>
            `;
            resultsList.appendChild(resultItem);
        });
    }
}

/**
 * UI 상태 설정
 */
function setUIState(state) {
    const startBtn = document.getElementById('startArchiveBtn');
    const cancelBtn = document.getElementById('cancelArchiveBtn');
    const inputs = document.querySelectorAll('#folderListContainer .folder-item-input');
    const deleteButtons = document.querySelectorAll('#folderListContainer .folder-item-delete');
    const addBtn = document.querySelector('.add-folder-btn');

    if (state === 'running') {
        startBtn.classList.add('hidden');
        cancelBtn.classList.remove('hidden');
        inputs.forEach(input => input.disabled = true);
        deleteButtons.forEach(btn => btn.disabled = true);
        addBtn.style.display = 'none';
    } else {
        startBtn.classList.remove('hidden');
        cancelBtn.classList.add('hidden');
        inputs.forEach(input => input.disabled = false);
        deleteButtons.forEach(btn => btn.disabled = false);
        addBtn.style.display = 'block';
    }
}

/**
 * 파일 크기 포맷팅
 */
function formatFileSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';

    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const k = 1024;
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i];
}
