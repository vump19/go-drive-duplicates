// Document Consolidation JavaScript

let currentJobId = null;
let progressInterval = null;

// 폴더 URL/ID에서 폴더 ID 추출
function extractFolderIdFromInput(input) {
    input = input.trim();
    // URL에서 folders/ 뒤의 ID 추출
    const match = input.match(/folders\/([a-zA-Z0-9_-]+)/);
    if (match) return match[1];
    // 순수 ID인 경우
    if (input.length > 10 && !input.includes('/')) return input;
    return null;
}

// 통합 시작
async function startConsolidation() {
    const folderInput = document.getElementById('folderInput').value.trim();
    if (!folderInput) {
        showStatus('폴더 URL 또는 ID를 입력하세요.', 'error');
        return;
    }

    const folderId = extractFolderIdFromInput(folderInput);
    if (!folderId) {
        showStatus('유효하지 않은 폴더 URL 또는 ID입니다.', 'error');
        return;
    }

    const includeSubfolders = document.getElementById('includeSubfolders').checked;
    const maxFileSizeMB = parseInt(document.getElementById('maxFileSizeMB').value) || 10;

    // 확장자 필터 수집
    const extensions = [];
    document.querySelectorAll('.ext-checkbox:checked').forEach(cb => {
        extensions.push(cb.value);
    });
    const customExt = document.getElementById('customExtensions').value.trim();
    if (customExt) {
        customExt.split(',').forEach(ext => {
            ext = ext.trim();
            if (ext) extensions.push(ext);
        });
    }

    // UI 상태 전환
    document.getElementById('inputSection').style.display = 'none';
    document.getElementById('progressSection').style.display = 'block';
    document.getElementById('resultSection').style.display = 'none';
    resetProgress();

    try {
        const response = await apiCall('/api/consolidate/start', {
            method: 'POST',
            body: JSON.stringify({
                folderId: folderId,
                includeSubfolders: includeSubfolders,
                maxFileSizeMB: maxFileSizeMB,
                fileExtensions: extensions
            })
        });

        currentJobId = response.jobId;
        showStatus('문서 통합 작업이 시작되었습니다...', 'info');
        startProgressMonitoring();
    } catch (error) {
        showStatus('통합 시작 실패: ' + error.message, 'error');
        document.getElementById('inputSection').style.display = 'block';
        document.getElementById('progressSection').style.display = 'none';
    }
}

// 진행 상황 모니터링
function startProgressMonitoring() {
    if (progressInterval) clearInterval(progressInterval);
    progressInterval = setInterval(async () => {
        if (!currentJobId) {
            clearInterval(progressInterval);
            return;
        }

        try {
            const job = await apiCall(`/api/consolidate/progress?jobId=${currentJobId}`, { silent: true });
            updateProgressUI(job);

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(progressInterval);
                progressInterval = null;

                if (job.status === 'completed') {
                    showResult(job);
                } else if (job.status === 'failed') {
                    showStatus('작업 실패: ' + (job.error || '알 수 없는 오류'), 'error');
                    document.getElementById('inputSection').style.display = 'block';
                } else {
                    showStatus('작업이 취소되었습니다.', 'warning');
                    document.getElementById('inputSection').style.display = 'block';
                }
            }
        } catch (error) {
            // Silent retry on network errors
        }
    }, 1000);
}

// 진행 상황 UI 업데이트
function updateProgressUI(job) {
    // 단계 표시
    const phases = ['scanning', 'downloading', 'deduplicating', 'merging', 'uploading', 'completed'];
    const phaseNames = {
        scanning: '파일 탐색',
        downloading: '파일 다운로드',
        deduplicating: '중복 제거',
        merging: 'Markdown 생성',
        uploading: '업로드',
        completed: '완료'
    };

    phases.forEach(phase => {
        const el = document.getElementById(`phase-${phase}`);
        if (!el) return;

        el.classList.remove('active', 'completed');
        const currentIdx = phases.indexOf(job.phase);
        const phaseIdx = phases.indexOf(phase);

        if (phaseIdx < currentIdx) {
            el.classList.add('completed');
        } else if (phaseIdx === currentIdx) {
            el.classList.add('active');
        }
    });

    // 메시지 업데이트
    const messageEl = document.getElementById('progressMessage');
    if (messageEl) {
        messageEl.textContent = job.message || '';
    }

    // 진행 통계 업데이트
    const progress = job.progress;
    if (progress) {
        updateStat('totalFiles', progress.totalFiles);
        updateStat('textFilesFound', progress.textFilesFound);
        updateStat('downloadedFiles', progress.downloadedFiles);
        updateStat('duplicatesFound', progress.duplicatesFound);
        updateStat('totalInputSize', formatFileSize(progress.totalInputSize || 0));

        // 프로그레스 바
        if (progress.textFilesFound > 0 && job.phase === 'downloading') {
            const pct = Math.round((progress.downloadedFiles / progress.textFilesFound) * 100);
            updateProgressBar(pct);
        } else if (job.phase === 'completed') {
            updateProgressBar(100);
        }
    }
}

function updateStat(id, value) {
    const el = document.getElementById(id);
    if (el) el.textContent = typeof value === 'number' ? value.toLocaleString() : value;
}

function updateProgressBar(percent) {
    const bar = document.getElementById('progressBar');
    const text = document.getElementById('progressPercent');
    if (bar) bar.style.width = percent + '%';
    if (text) text.textContent = percent + '%';
}

function resetProgress() {
    updateProgressBar(0);
    updateStat('totalFiles', '0');
    updateStat('textFilesFound', '0');
    updateStat('downloadedFiles', '0');
    updateStat('duplicatesFound', '0');
    updateStat('totalInputSize', '0 B');
}

// 결과 표시
function showResult(job) {
    document.getElementById('progressSection').style.display = 'none';
    document.getElementById('resultSection').style.display = 'block';

    const result = job.result;
    if (!result) return;

    // 요약 정보
    document.getElementById('resultSummary').innerHTML = `
        <div class="result-stat">
            <span class="result-stat-label">전체 파일 수</span>
            <span class="result-stat-value">${result.totalFilesScanned.toLocaleString()}</span>
        </div>
        <div class="result-stat">
            <span class="result-stat-label">텍스트 파일</span>
            <span class="result-stat-value">${result.textFilesFound.toLocaleString()}</span>
        </div>
        <div class="result-stat">
            <span class="result-stat-label">중복 제거</span>
            <span class="result-stat-value">${result.duplicatesRemoved.toLocaleString()}</span>
        </div>
        <div class="result-stat">
            <span class="result-stat-label">고유 문서</span>
            <span class="result-stat-value">${result.uniqueDocuments.toLocaleString()}</span>
        </div>
        <div class="result-stat">
            <span class="result-stat-label">입력 크기</span>
            <span class="result-stat-value">${formatFileSize(result.totalInputSize)}</span>
        </div>
        <div class="result-stat">
            <span class="result-stat-label">출력 크기</span>
            <span class="result-stat-value">${formatFileSize(result.totalOutputSize)}</span>
        </div>
    `;

    // 출력 파일 목록
    const fileListEl = document.getElementById('outputFileList');
    if (result.outputFiles && result.outputFiles.length > 0) {
        fileListEl.innerHTML = result.outputFiles.map(file => `
            <div class="output-file-item">
                <div class="output-file-info">
                    <i class="fas fa-file-alt"></i>
                    <div>
                        <strong>${file.fileName}</strong>
                        <small>${formatFileSize(file.fileSize)} · ${file.documentCount}개 문서</small>
                    </div>
                </div>
                ${file.webViewLink ? `<a href="${file.webViewLink}" target="_blank" class="btn btn-sm btn-primary">
                    <i class="fas fa-external-link-alt"></i> 열기
                </a>` : `<span class="text-muted">ID: ${file.fileId}</span>`}
            </div>
        `).join('');
    } else {
        fileListEl.innerHTML = '<p class="text-muted">출력 파일이 없습니다.</p>';
    }
}

// 취소
async function cancelConsolidation() {
    if (!currentJobId) return;

    try {
        await apiCall(`/api/consolidate/cancel?jobId=${currentJobId}`, {
            method: 'POST'
        });
        showStatus('취소 요청됨...', 'warning');
    } catch (error) {
        showStatus('취소 실패: ' + error.message, 'error');
    }
}

// 새 작업
function startNewConsolidation() {
    currentJobId = null;
    document.getElementById('inputSection').style.display = 'block';
    document.getElementById('progressSection').style.display = 'none';
    document.getElementById('resultSection').style.display = 'none';
}

// 상태 메시지 표시
function showStatus(message, type) {
    const el = document.getElementById('statusMessage');
    if (!el) return;
    el.textContent = message;
    el.className = 'status-message status-' + type;
    el.style.display = 'block';

    if (type !== 'error') {
        setTimeout(() => { el.style.display = 'none'; }, 5000);
    }
}

// 전체 선택/해제
function toggleAllExtensions(checked) {
    document.querySelectorAll('.ext-checkbox').forEach(cb => {
        cb.checked = checked;
    });
}
