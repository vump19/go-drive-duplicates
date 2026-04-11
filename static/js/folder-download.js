/**
 * Folder Download Module
 * Google Drive 폴더를 ZIP 파일로 다운로드하는 기능
 * 서버 사이드 다운로드 방식: 서버에 파일을 먼저 다운로드 후 ZIP으로 제공
 */

// 전역 상태
let currentDownload = {
    folderId: null,
    folderName: null,
    totalFiles: 0,
    totalSize: 0,
    prepareId: null,
    downloadId: null,
    eventSource: null,
    resultReceived: false
};

// API Base URL 헬퍼 함수
function getDownloadApiBaseUrl() {
    try {
        if (typeof window.API_BASE !== 'undefined') return window.API_BASE;
        if (typeof API_BASE !== 'undefined') return API_BASE;
    } catch (e) {
        // ReferenceError 무시
    }
    return 'http://localhost:9090';
}

/**
 * 폴더 다운로드 준비 (폴링 방식으로 진행 상황 표시)
 */
async function prepareDownload() {
    const urlInput = document.getElementById('downloadFolderUrl');
    const url = urlInput ? urlInput.value.trim() : '';

    if (!url) {
        showDownloadError('Google Drive 폴더 URL을 입력해주세요.');
        return;
    }

    // 옵션 읽기
    const includeSubfolders = document.getElementById('downloadIncludeSubfolders')?.checked ?? true;
    const skipGoogleDocs = document.getElementById('downloadSkipGoogleDocs')?.checked ?? true;

    // 상태 초기화
    currentDownload = {
        folderId: null,
        folderName: null,
        totalFiles: 0,
        totalSize: 0,
        prepareId: null,
        downloadId: null,
        eventSource: null,
        resultReceived: false
    };

    showPrepareProgress({
        status: 'scanning',
        phase: 'starting',
        message: '다운로드 준비 시작...'
    });

    const baseUrl = getDownloadApiBaseUrl();

    try {
        // 1. 다운로드 준비 시작 요청
        const startResponse = await fetch(`${baseUrl}/api/folder/download/prepare/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                url: url,
                includeSubfolders: includeSubfolders,
                skipGoogleDocs: skipGoogleDocs
            })
        });

        if (!startResponse.ok) {
            const errorText = await startResponse.text();
            throw new Error(errorText || `HTTP ${startResponse.status}`);
        }

        const startResult = await startResponse.json();
        currentDownload.prepareId = startResult.prepareId;
        console.log('다운로드 준비 시작:', startResult);

        // 2. 폴링으로 진행 상황 확인
        await pollPrepareProgress(startResult.prepareId);

    } catch (error) {
        console.error('다운로드 준비 오류:', error);
        showDownloadError(`다운로드 준비 실패: ${error.message}`);
    }
}

/**
 * 폴링으로 준비 진행 상황 확인
 */
async function pollPrepareProgress(prepareId) {
    const baseUrl = getDownloadApiBaseUrl();
    const pollInterval = 1000;

    console.log('폴링 시작, prepareId:', prepareId);

    while (true) {
        try {
            const progressUrl = `${baseUrl}/api/folder/download/prepare/progress?prepareId=${encodeURIComponent(prepareId)}`;
            console.log('📡 폴링 요청:', progressUrl);
            const response = await fetch(progressUrl);

            console.log('📡 폴링 응답 상태:', response.status);

            if (!response.ok) {
                const errorText = await response.text();
                console.error('📡 폴링 오류 응답:', errorText);
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }

            const progress = await response.json();
            console.log('📡 폴링 데이터:', progress);

            if (progress.status === 'completed') {
                currentDownload.resultReceived = true;
                currentDownload.folderId = progress.result.folderId;
                currentDownload.folderName = progress.result.folderName;
                currentDownload.totalFiles = progress.result.totalFiles;
                currentDownload.totalSize = progress.result.totalSize;
                showPrepareResult(progress.result);
                return;
            } else if (progress.status === 'failed') {
                showDownloadError(`다운로드 준비 실패: ${progress.error}`);
                return;
            } else {
                if (progress.progress) {
                    showPrepareProgress(progress.progress);
                }
            }

            await new Promise(resolve => setTimeout(resolve, pollInterval));

        } catch (error) {
            console.error('진행 상황 조회 오류:', error);
            showDownloadError(`진행 상황 조회 실패: ${error.message}`);
            return;
        }
    }
}

/**
 * 준비 진행 상황 UI 표시
 */
function showPrepareProgress(progress) {
    const statusDiv = document.getElementById('downloadStatus');
    if (!statusDiv) return;

    if (progress.status === 'completed' || progress.phase === 'completed') {
        return;
    }

    let phaseText = '';
    switch (progress.phase) {
        case 'folder_info':
            phaseText = '폴더 정보 조회';
            break;
        case 'listing_files':
            phaseText = '파일 목록 스캔';
            break;
        case 'processing_files':
            phaseText = '파일 처리';
            break;
        case 'completed':
            phaseText = '완료';
            break;
        default:
            phaseText = '준비 중';
    }

    const html = `
        <div class="download-prepare-progress">
            <h4><i class="fas fa-spinner fa-spin"></i> 다운로드 준비 중...</h4>
            <div class="prepare-phase">
                <span class="phase-label">단계:</span>
                <span class="phase-value">${phaseText}</span>
            </div>
            ${progress.folderName ? `
                <div class="prepare-info">
                    <span class="info-label">폴더:</span>
                    <span class="info-value">${escapeHtml(progress.folderName)}</span>
                </div>
            ` : ''}
            ${progress.currentFolder ? `
                <div class="prepare-info">
                    <span class="info-label">현재 위치:</span>
                    <span class="info-value">${escapeHtml(progress.currentFolder)}</span>
                </div>
            ` : ''}
            <div class="prepare-stats">
                <div class="stat">
                    <i class="fas fa-folder"></i>
                    <span>${progress.scannedFolders || 0}개 폴더</span>
                </div>
                <div class="stat">
                    <i class="fas fa-file"></i>
                    <span>${progress.scannedFiles || 0}개 파일</span>
                </div>
                ${progress.totalSize ? `
                    <div class="stat">
                        <i class="fas fa-database"></i>
                        <span>${formatSize(progress.totalSize)}</span>
                    </div>
                ` : ''}
            </div>
            <div class="prepare-message">${escapeHtml(progress.message || '')}</div>
        </div>
    `;

    statusDiv.innerHTML = html;
    statusDiv.style.display = 'block';
}

/**
 * 다운로드 준비 결과 표시
 */
function showPrepareResult(result) {
    const statusDiv = document.getElementById('downloadStatus');
    if (!statusDiv) return;

    const sizeFormatted = formatSize(result.totalSize);

    let html = `
        <div class="download-prepare-result">
            <h4><i class="fas fa-check-circle" style="color: green;"></i> 다운로드 준비 완료</h4>
            <div class="info-grid">
                <div class="info-item">
                    <span class="label">폴더명:</span>
                    <span class="value">${escapeHtml(result.folderName)}</span>
                </div>
                <div class="info-item">
                    <span class="label">경로:</span>
                    <span class="value">${escapeHtml(result.folderPath)}</span>
                </div>
                <div class="info-item">
                    <span class="label">파일 수:</span>
                    <span class="value">${result.totalFiles.toLocaleString()}개</span>
                </div>
                <div class="info-item">
                    <span class="label">총 크기:</span>
                    <span class="value">${sizeFormatted}</span>
                </div>
            </div>
    `;

    if (result.skippedInfo) {
        html += `
            <div class="skipped-info">
                <h5>제외된 파일</h5>
                <p>Google Docs: ${result.skippedInfo.googleDocsCount}개 (${formatSize(result.skippedInfo.googleDocsSize)})</p>
            </div>
        `;
    }

    html += `
            <div class="download-note" style="margin: 15px 0; padding: 10px; background: #e3f2fd; border-radius: 4px;">
                <i class="fas fa-info-circle"></i>
                <strong>서버 다운로드 방식:</strong> 파일들을 서버에 먼저 다운로드한 후 ZIP 파일로 제공합니다.
                대용량 폴더도 안정적으로 다운로드할 수 있습니다.
            </div>
            <div class="download-actions">
                <button onclick="startServerDownload()" class="btn btn-primary">
                    <i class="fas fa-server"></i> 서버 다운로드 시작
                </button>
                <button onclick="cancelPrepare()" class="btn btn-secondary">취소</button>
            </div>
        </div>
    `;

    statusDiv.innerHTML = html;
    statusDiv.style.display = 'block';
}

/**
 * 서버 다운로드 시작 (서버에 파일을 먼저 다운로드)
 */
async function startServerDownload() {
    console.log('서버 다운로드 시작, prepareId:', currentDownload.prepareId);

    if (!currentDownload.prepareId) {
        showDownloadError('다운로드를 먼저 준비해주세요.');
        return;
    }

    const baseUrl = getDownloadApiBaseUrl();

    try {
        // 서버 다운로드 시작 요청
        const response = await fetch(`${baseUrl}/api/folder/download/server/start`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                prepareId: currentDownload.prepareId
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText || `HTTP ${response.status}`);
        }

        const result = await response.json();
        currentDownload.downloadId = result.downloadId;
        console.log('서버 다운로드 시작됨:', result);

        // 진행 상황 표시 시작
        showServerDownloadProgress({
            status: 'downloading',
            phase: 'download',
            message: '서버에 파일 다운로드 중...',
            downloadedFiles: 0,
            totalFiles: currentDownload.totalFiles,
            downloadedSize: 0,
            totalSize: currentDownload.totalSize
        });

        // 폴링으로 서버 다운로드 진행 상황 확인
        await pollServerDownloadProgress(result.downloadId);

    } catch (error) {
        console.error('서버 다운로드 시작 오류:', error);
        showDownloadError(`서버 다운로드 시작 실패: ${error.message}`);
    }
}

/**
 * 서버 다운로드 진행 상황 폴링
 */
async function pollServerDownloadProgress(downloadId) {
    const baseUrl = getDownloadApiBaseUrl();
    const pollInterval = 1000;

    console.log('서버 다운로드 폴링 시작, downloadId:', downloadId);

    while (true) {
        try {
            const response = await fetch(`${baseUrl}/api/folder/download/server/progress?downloadId=${encodeURIComponent(downloadId)}`);

            if (!response.ok) {
                if (response.status === 404) {
                    showDownloadError('다운로드 정보를 찾을 수 없습니다.');
                    return;
                }
                throw new Error(`HTTP ${response.status}`);
            }

            const state = await response.json();
            console.log('서버 다운로드 상태:', state);
            console.log('Progress 객체:', state.progress);
            if (state.progress) {
                console.log('다운로드된 파일:', state.progress.downloadedFiles, '/', state.progress.totalFiles);
            }

            if (state.status === 'completed') {
                // 다운로드 완료 - ZIP 파일 다운로드 가능
                showServerDownloadComplete(state.result);
                return;
            } else if (state.status === 'failed') {
                showDownloadError(`서버 다운로드 실패: ${state.error}`);
                return;
            } else if (state.status === 'cancelled') {
                showDownloadStatus('cancelled', '다운로드가 취소되었습니다.');
                return;
            } else {
                // 진행 중
                if (state.progress) {
                    showServerDownloadProgress(state.progress);
                }
            }

            await new Promise(resolve => setTimeout(resolve, pollInterval));

        } catch (error) {
            console.error('서버 다운로드 진행 상황 조회 오류:', error);
            showDownloadError(`진행 상황 조회 실패: ${error.message}`);
            return;
        }
    }
}

/**
 * 서버 다운로드 진행 상황 UI 표시
 */
function showServerDownloadProgress(progress) {
    const statusDiv = document.getElementById('downloadStatus');
    if (!statusDiv) return;

    console.log('showServerDownloadProgress 호출됨:', progress);

    // 명시적으로 숫자로 변환
    const downloadedFiles = Number(progress.downloadedFiles) || 0;
    const totalFiles = Number(progress.totalFiles) || 1;
    const filePercent = Math.round((downloadedFiles / totalFiles) * 100);

    const downloadedSize = Number(progress.downloadedSize) || 0;
    const totalSize = Number(progress.totalSize) || 1;
    const sizePercent = Math.round((downloadedSize / totalSize) * 100);

    console.log(`파일: ${downloadedFiles}/${totalFiles} (${filePercent}%), 크기: ${downloadedSize}/${totalSize}`);

    let phaseText = '';
    let phaseIcon = 'fa-spinner fa-spin';
    switch (progress.phase) {
        case 'download':
            phaseText = '파일 다운로드 중';
            break;
        case 'zip':
            phaseText = 'ZIP 파일 생성 중';
            phaseIcon = 'fa-file-archive';
            break;
        case 'completed':
            phaseText = '완료';
            phaseIcon = 'fa-check-circle';
            break;
        default:
            phaseText = '처리 중';
    }

    const html = `
        <div class="download-progress">
            <h4><i class="fas ${phaseIcon}"></i> ${phaseText}</h4>
            <div class="progress-info">
                <div class="folder-name">
                    <strong>폴더:</strong> ${escapeHtml(progress.folderName || currentDownload.folderName)}
                </div>
                ${progress.currentFile ? `
                    <div class="current-file">
                        <strong>현재 파일:</strong> ${escapeHtml(progress.currentFile)}
                    </div>
                ` : ''}
                <div class="progress-stats">
                    <span><i class="fas fa-file"></i> 파일: ${downloadedFiles.toLocaleString()} / ${totalFiles.toLocaleString()} (${filePercent}%)</span>
                    <span><i class="fas fa-database"></i> 크기: ${formatSize(downloadedSize)} / ${formatSize(totalSize)} (${sizePercent}%)</span>
                    ${progress.failedFiles > 0 ? `
                        <span style="color: orange;"><i class="fas fa-exclamation-triangle"></i> 실패: ${progress.failedFiles}개</span>
                    ` : ''}
                </div>
            </div>
            <div class="progress-bar-container" style="background: #e0e0e0; border-radius: 4px; height: 20px; margin: 10px 0;">
                <div class="progress-bar" style="background: #4CAF50; height: 100%; border-radius: 4px; width: ${filePercent}%; transition: width 0.3s;"></div>
            </div>
            <div class="progress-message" style="color: #666; font-size: 0.9em;">
                ${escapeHtml(progress.message || '')}
            </div>
            <div class="download-actions" style="margin-top: 15px;">
                <button onclick="cancelServerDownload()" class="btn btn-danger">
                    <i class="fas fa-times"></i> 취소
                </button>
            </div>
        </div>
    `;

    statusDiv.innerHTML = html;
    statusDiv.style.display = 'block';
}

/**
 * 서버 다운로드 완료 UI 표시
 */
function showServerDownloadComplete(result) {
    const statusDiv = document.getElementById('downloadStatus');
    if (!statusDiv) return;

    const html = `
        <div class="download-complete">
            <h4><i class="fas fa-check-circle" style="color: green;"></i> 다운로드 준비 완료!</h4>
            <div class="info-grid" style="margin: 15px 0;">
                <div class="info-item">
                    <span class="label">폴더명:</span>
                    <span class="value">${escapeHtml(result.folderName)}</span>
                </div>
                <div class="info-item">
                    <span class="label">ZIP 파일 크기:</span>
                    <span class="value">${formatSize(result.zipSize)}</span>
                </div>
                <div class="info-item">
                    <span class="label">다운로드된 파일:</span>
                    <span class="value">${result.downloadedFiles.toLocaleString()} / ${result.totalFiles.toLocaleString()}개</span>
                </div>
                <div class="info-item">
                    <span class="label">소요 시간:</span>
                    <span class="value">${result.duration}</span>
                </div>
                ${result.failedFiles > 0 ? `
                    <div class="info-item" style="color: orange;">
                        <span class="label">실패한 파일:</span>
                        <span class="value">${result.failedFiles}개</span>
                    </div>
                ` : ''}
            </div>
            <div class="download-actions">
                <button onclick="downloadZipFile('${result.id}')" class="btn btn-primary btn-lg">
                    <i class="fas fa-download"></i> ZIP 파일 다운로드
                </button>
                <button onclick="cleanupAndClose('${result.id}')" class="btn btn-secondary">
                    닫기
                </button>
            </div>
            <div class="download-note" style="margin-top: 15px; padding: 10px; background: #fff3cd; border-radius: 4px; font-size: 0.9em;">
                <i class="fas fa-info-circle"></i>
                ZIP 파일은 30분 후 자동으로 삭제됩니다. 바로 다운로드해주세요.
            </div>
        </div>
    `;

    statusDiv.innerHTML = html;
    statusDiv.style.display = 'block';
}

/**
 * ZIP 파일 다운로드
 */
function downloadZipFile(downloadId) {
    const baseUrl = getDownloadApiBaseUrl();
    const downloadUrl = `${baseUrl}/api/folder/download/server/download?downloadId=${encodeURIComponent(downloadId)}`;

    console.log('ZIP 파일 다운로드:', downloadUrl);

    // 새 탭에서 다운로드
    window.open(downloadUrl, '_blank');
}

/**
 * 정리 및 닫기
 */
async function cleanupAndClose(downloadId) {
    const baseUrl = getDownloadApiBaseUrl();

    try {
        await fetch(`${baseUrl}/api/folder/download/server/cleanup?downloadId=${encodeURIComponent(downloadId)}`, {
            method: 'POST'
        });
    } catch (error) {
        console.error('정리 오류:', error);
    }

    cancelPrepare();
}

/**
 * 서버 다운로드 취소
 */
async function cancelServerDownload() {
    if (!currentDownload.downloadId) return;

    const baseUrl = getDownloadApiBaseUrl();

    try {
        await fetch(`${baseUrl}/api/folder/download/cancel`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                downloadId: currentDownload.downloadId
            })
        });

        showDownloadStatus('cancelled', '다운로드가 취소되었습니다.');
    } catch (error) {
        console.error('취소 오류:', error);
    }
}

/**
 * 준비 취소
 */
function cancelPrepare() {
    currentDownload = {
        folderId: null,
        folderName: null,
        totalFiles: 0,
        totalSize: 0,
        prepareId: null,
        downloadId: null,
        eventSource: null,
        resultReceived: false
    };

    const statusDiv = document.getElementById('downloadStatus');
    if (statusDiv) {
        statusDiv.style.display = 'none';
        statusDiv.innerHTML = '';
    }
}

/**
 * 상태 메시지 표시
 */
function showDownloadStatus(status, message) {
    const statusDiv = document.getElementById('downloadStatus');
    if (!statusDiv) return;

    let iconClass = '';
    let statusClass = '';

    switch (status) {
        case 'preparing':
            iconClass = 'fa-spinner fa-spin';
            statusClass = 'status-preparing';
            break;
        case 'downloading':
            iconClass = 'fa-spinner fa-spin';
            statusClass = 'status-downloading';
            break;
        case 'completed':
            iconClass = 'fa-check-circle';
            statusClass = 'status-completed';
            break;
        case 'failed':
            iconClass = 'fa-times-circle';
            statusClass = 'status-failed';
            break;
        case 'cancelled':
            iconClass = 'fa-ban';
            statusClass = 'status-cancelled';
            break;
        default:
            iconClass = 'fa-info-circle';
    }

    statusDiv.innerHTML = `
        <div class="download-status ${statusClass}">
            <i class="fas ${iconClass}"></i>
            <span>${escapeHtml(message)}</span>
            <div style="margin-top: 15px;">
                <button onclick="cancelPrepare()" class="btn btn-secondary">닫기</button>
            </div>
        </div>
    `;
    statusDiv.style.display = 'block';
}

/**
 * 오류 메시지 표시
 */
function showDownloadError(message) {
    showDownloadStatus('failed', message);
}

/**
 * 파일 크기 포맷팅
 */
function formatSize(bytes) {
    if (bytes === 0) return '0 B';

    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const k = 1024;
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i];
}

/**
 * HTML 이스케이프
 */
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * 페이지 로드 시 초기화
 */
document.addEventListener('DOMContentLoaded', function() {
    const downloadSection = document.getElementById('downloadSection');
    if (!downloadSection) return;

    const downloadForm = document.getElementById('downloadForm');
    if (downloadForm) {
        downloadForm.addEventListener('submit', function(e) {
            e.preventDefault();
            prepareDownload();
        });
    }

    const urlInput = document.getElementById('downloadFolderUrl');
    if (urlInput) {
        urlInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                e.preventDefault();
                prepareDownload();
            }
        });
    }
});
