/**
 * Audio Conversion JavaScript Module
 * 오디오 파일 형식 변환 기능을 위한 JavaScript 모듈
 */

const AUDIO_API_BASE = window.API_BASE || 'http://localhost:9090';

let currentJobId = null;
let pollingInterval = null;

// 페이지 로드 시 초기화
document.addEventListener('DOMContentLoaded', () => {
    // 폴더 입력 필드에 경로 표시 연결
    if (typeof attachFolderResolver === 'function') {
        const sourceInput = document.getElementById('sourceFolderInput');
        const outputInput = document.getElementById('outputFolderInput');
        if (sourceInput) attachFolderResolver(sourceInput);
        if (outputInput) attachFolderResolver(outputInput);
    }

    // 지원 형식 로드
    loadFormats();
});

// video/* MIME으로 분류되어 기본 audio/* 필터에 잡히지 않는 확장자 목록
const NON_AUDIO_MIME_EXTENSIONS = new Set(['3gp', 'mp4', 'webm']);

/**
 * 지원 형식 로드
 */
async function loadFormats() {
    try {
        const data = await fetch(`${AUDIO_API_BASE}/api/audio-convert/formats`).then(r => r.json());
        if (data.formats) {
            const select = document.getElementById('outputFormat');
            select.innerHTML = '';
            data.formats.forEach(fmt => {
                const option = document.createElement('option');
                option.value = fmt.extension;
                option.textContent = fmt.label;
                select.appendChild(option);
            });
        }
        // 소스 형식 체크박스 생성
        if (data.sourceFormats) {
            renderSourceFormatCheckboxes(data.sourceFormats);
        }
    } catch (error) {
        console.warn('형식 목록 로드 실패, 기본값 사용:', error.message);
    }
}

/**
 * 소스 형식 체크박스 동적 생성
 */
function renderSourceFormatCheckboxes(sourceFormats) {
    const container = document.getElementById('sourceFormatCheckboxes');
    if (!container) return;
    container.innerHTML = '';

    sourceFormats.forEach(fmt => {
        const label = document.createElement('label');
        label.className = 'checkbox-label';
        label.style.margin = '0';

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.value = fmt.extension;
        checkbox.className = 'source-format-checkbox';
        // video/* MIME 형식은 기본 체크 해제, audio/* 형식은 기본 체크 해제 (audio/*는 MIME으로 자동 인식)
        checkbox.checked = false;

        const span = document.createElement('span');
        // audio/* MIME이 아닌 형식에 표시 추가
        if (NON_AUDIO_MIME_EXTENSIONS.has(fmt.extension)) {
            span.textContent = `${fmt.label} (${fmt.mimeType})`;
            span.style.color = '#f59e0b';
        } else {
            span.textContent = fmt.label;
        }

        label.appendChild(checkbox);
        label.appendChild(span);
        container.appendChild(label);
    });
}

/**
 * 변환 시작
 */
async function startConversion() {
    const sourceInput = document.getElementById('sourceFolderInput').value.trim();
    if (!sourceInput) {
        alert('소스 폴더 URL 또는 ID를 입력해주세요.');
        return;
    }

    const outputFormat = document.getElementById('outputFormat').value;
    const bitrate = document.getElementById('bitrate').value;
    const sampleRate = document.getElementById('sampleRate').value;
    const includeSubfolders = document.getElementById('includeSubfolders').checked;
    const deleteOriginal = document.getElementById('deleteOriginal').checked;
    const outputFolderInput = document.getElementById('outputFolderInput').value.trim();

    // 폴더 URL/ID 판별
    let folderUrl = '';
    let folderId = '';
    if (sourceInput.includes('drive.google.com') || sourceInput.includes('/folders/')) {
        folderUrl = sourceInput;
    } else {
        folderId = sourceInput;
    }

    // 출력 폴더 URL/ID 판별
    let outputFolderUrl = '';
    let outputFolderId = '';
    if (outputFolderInput) {
        if (outputFolderInput.includes('drive.google.com') || outputFolderInput.includes('/folders/')) {
            outputFolderUrl = outputFolderInput;
        } else {
            outputFolderId = outputFolderInput;
        }
    }

    // 선택된 소스 확장자 수집
    const sourceExtensions = [];
    document.querySelectorAll('.source-format-checkbox:checked').forEach(cb => {
        sourceExtensions.push(cb.value);
    });

    const options = {
        outputFormat: outputFormat,
        bitrate: bitrate,
        sampleRate: sampleRate,
        includeSubfolders: includeSubfolders,
        deleteOriginal: deleteOriginal,
        outputFolderId: outputFolderId,
        outputFolderUrl: outputFolderUrl,
        sourceExtensions: sourceExtensions
    };

    setUIState('running');

    try {
        const response = await fetch(`${AUDIO_API_BASE}/api/audio-convert/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folderUrl: folderUrl,
                folderId: folderId,
                options: options
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText);
        }

        const result = await response.json();
        currentJobId = result.jobId;
        console.log('🎵 오디오 변환 시작:', result);

        startProgressPolling();

    } catch (error) {
        console.error('오디오 변환 시작 실패:', error);
        alert('변환 시작 실패: ' + error.message);
        setUIState('idle');
    }
}

/**
 * 작업 취소
 */
async function cancelConversion() {
    if (!currentJobId) return;

    if (!confirm('정말 작업을 취소하시겠습니까?')) return;

    try {
        await fetch(`${AUDIO_API_BASE}/api/audio-convert/cancel?jobId=${currentJobId}`, {
            method: 'POST'
        });
        console.log('🛑 작업 취소 요청됨');
    } catch (error) {
        console.error('취소 실패:', error);
        alert('취소 요청 실패: ' + error.message);
    }
}

/**
 * 진행 상황 폴링
 */
function startProgressPolling() {
    if (pollingInterval) clearInterval(pollingInterval);
    pollingInterval = setInterval(pollProgress, 500);
    pollProgress();
}

function stopProgressPolling() {
    if (pollingInterval) {
        clearInterval(pollingInterval);
        pollingInterval = null;
    }
}

async function pollProgress() {
    if (!currentJobId) return;

    try {
        const response = await fetch(`${AUDIO_API_BASE}/api/audio-convert/progress?jobId=${currentJobId}`);
        if (!response.ok) {
            if (response.status === 404) {
                stopProgressPolling();
                setUIState('idle');
                return;
            }
            throw new Error('진행 상황 조회 실패');
        }

        const job = await response.json();
        updateProgressUI(job);

        if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
            stopProgressPolling();
            setUIState('completed');
            showResults(job);
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
    const progress = job.totalFiles > 0
        ? Math.round((job.completedFiles + job.failedFiles) / job.totalFiles * 100) : 0;

    // 진행률 바
    const bar = document.getElementById('progressBar');
    bar.style.width = `${progress}%`;
    bar.textContent = `${progress}%`;

    // 통계
    document.getElementById('completedCount').textContent = job.completedFiles;
    document.getElementById('failedCount').textContent = job.failedFiles;
    document.getElementById('remainingCount').textContent =
        Math.max(0, job.totalFiles - job.completedFiles - job.failedFiles);
    document.getElementById('totalCount').textContent = job.totalFiles;

    // 제목
    const title = document.getElementById('progressTitle');
    if (job.status === 'running') {
        title.textContent = `변환 진행 중... (${job.completedFiles + job.failedFiles}/${job.totalFiles})`;
    } else if (job.status === 'completed') {
        if (job.totalFiles === 0) {
            title.innerHTML = '오디오 파일을 찾지 못했습니다. <span style="color:#f59e0b;">소스 형식 필터를 확인해주세요.</span>';
        } else {
            title.textContent = '변환 완료!';
        }
    } else if (job.status === 'cancelled') {
        title.textContent = '변환 취소됨';
    } else if (job.status === 'failed') {
        title.textContent = '변환 실패';
    }

    // 현재 파일 정보
    if (job.currentFile) {
        document.getElementById('currentFileName').textContent = job.currentFile.fileName || '';
        document.getElementById('currentPhaseText').textContent = getPhaseText(job.currentFile.phase, job.currentFile.message);

        // 단계 인디케이터
        updatePhaseIndicator(job.currentFile.phase);
    }
}

function getPhaseText(phase, message) {
    if (message) return message;
    const texts = {
        'scanning': '오디오 파일 스캔 중...',
        'downloading': '다운로드 중...',
        'converting': '변환 중...',
        'uploading': '업로드 중...'
    };
    return texts[phase] || phase;
}

function updatePhaseIndicator(currentPhase) {
    const phases = ['downloading', 'converting', 'uploading'];
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
 * 결과 표시
 */
function showResults(job) {
    const resultsCard = document.getElementById('resultsCard');
    resultsCard.classList.remove('hidden');

    // 요약
    document.getElementById('summaryTotal').textContent = job.totalFiles;
    document.getElementById('summaryCompleted').textContent = job.completedFiles;
    document.getElementById('summaryFailed').textContent = job.failedFiles;

    // 용량 변화 계산
    let totalOriginal = 0;
    let totalConverted = 0;
    if (job.results) {
        job.results.forEach(r => {
            if (r.status === 'completed') {
                totalOriginal += r.originalSize;
                totalConverted += r.convertedSize;
            }
        });
    }
    document.getElementById('summarySizeChange').textContent =
        `${formatFileSize(totalOriginal)} → ${formatFileSize(totalConverted)}`;

    // 결과 목록
    const list = document.getElementById('resultsList');
    list.innerHTML = '';

    if (job.results) {
        job.results.forEach(result => {
            const item = document.createElement('div');

            if (result.status === 'completed') {
                item.className = 'result-item success';
                item.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">
                            <i class="fas fa-check-circle" style="color:#10b981;"></i>
                            ${escapeHtml(result.originalName)} → ${escapeHtml(result.convertedName)}
                        </div>
                        <div class="result-details">
                            ${formatFileSize(result.originalSize)} → ${formatFileSize(result.convertedSize)}
                        </div>
                    </div>
                `;
            } else if (result.status === 'skipped') {
                item.className = 'result-item';
                item.style.borderLeft = '4px solid #f59e0b';
                item.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">
                            <i class="fas fa-forward" style="color:#f59e0b;"></i>
                            ${escapeHtml(result.originalName)}
                        </div>
                        <div class="result-details" style="color:#f59e0b;">
                            ${escapeHtml(result.error || '건너뜀')}
                        </div>
                    </div>
                `;
            } else {
                item.className = 'result-item error';
                item.innerHTML = `
                    <div class="result-info">
                        <div class="result-name">
                            <i class="fas fa-times-circle" style="color:#ef4444;"></i>
                            ${escapeHtml(result.originalName)}
                        </div>
                        <div class="result-details" style="color:#ef4444;">
                            ${escapeHtml(result.error || '알 수 없는 오류')}
                        </div>
                    </div>
                `;
            }

            list.appendChild(item);
        });
    }
}

/**
 * UI 상태 설정
 */
function setUIState(state) {
    const startBtn = document.getElementById('startConvertBtn');
    const cancelBtn = document.getElementById('cancelConvertBtn');
    const inputs = document.querySelectorAll('#inputSection input, #inputSection select');

    if (state === 'running') {
        startBtn.classList.add('hidden');
        cancelBtn.classList.remove('hidden');
        inputs.forEach(el => el.disabled = true);
    } else {
        startBtn.classList.remove('hidden');
        cancelBtn.classList.add('hidden');
        inputs.forEach(el => el.disabled = false);
    }
}

/**
 * 유틸리티 함수
 */
function formatFileSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const k = 1024;
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i];
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
