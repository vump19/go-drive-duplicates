import { Component } from '../components/base/Component.js';
import { ProgressBar } from '../components/widgets/ProgressBar.js';
import { EVENTS } from '../core/EventBus.js';
import { getState, setState } from '../core/StateManager.js';

/**
 * 파일 스캔 페이지 컴포넌트
 */
export class FileScan extends Component {
    constructor(element) {
        super(element);
        this.progressBar = null;
        this.hashProgressBar = null;
        this.isScanning = false;
        this.isCalculatingHashes = false;
        this.progressUpdateInterval = null;
    }

    /**
     * Render method for App.js compatibility
     */
    render() {
        const html = this.template();
        if (this.element) {
            this.element.innerHTML = html;
            this.afterRender();
            this.bindEvents();
        }
        return html;
    }

    onInit() {
        this.setupStateWatchers();
        this.setupEventListeners();
        this.render();
    }

    setupStateWatchers() {
        // 파일 스캔 상태 워치
        this.watch('fileScan', (scanState) => {
            this.isScanning = scanState.isRunning || false;
            this.updateScanStatus();
            this.updateProgress(scanState.progress);
        });

        // 해시 계산 상태 워치 (별도 상태로 관리될 예정)
        this.watch('hashCalculation', (hashState) => {
            this.isCalculatingHashes = hashState?.isRunning || false;
            this.updateHashProgress(hashState?.progress);
        });
    }

    setupEventListeners() {
        // 파일 스캔 관련 이벤트
        this.onEvent(EVENTS.FILE_SCAN_START, this.handleScanStart);
        this.onEvent(EVENTS.FILE_SCAN_PROGRESS, this.handleScanProgress);
        this.onEvent(EVENTS.FILE_SCAN_COMPLETE, this.handleScanComplete);
    }

    template() {
        return `
            <div class="file-scan-container">
                <!-- 스캔 컨트롤 카드 -->
                <div class="scan-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-scan"></i>
                            파일 스캔
                        </h3>
                        <div class="scan-status">
                            <span class="status-indicator" data-status="${this.isScanning ? 'running' : 'idle'}">
                                ${this.isScanning ? '진행 중' : '대기'}
                            </span>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="scan-description">
                            <p>Google Drive의 모든 파일을 스캔하여 메타데이터를 수집합니다. 
                            이 작업은 파일 수에 따라 시간이 소요될 수 있습니다.</p>
                        </div>
                        
                        <div class="scan-controls">
                            <button class="btn btn-primary scan-btn" 
                                    ${this.isScanning ? 'disabled' : ''}>
                                <i class="icon-play"></i>
                                ${this.isScanning ? '스캔 중...' : '전체 스캔 시작'}
                            </button>
                            
                            <button class="btn btn-secondary refresh-btn">
                                <i class="icon-refresh"></i>
                                진행 상황 새로고침
                            </button>
                            
                            ${this.isScanning ? `
                                <button class="btn btn-danger stop-btn">
                                    <i class="icon-stop"></i>
                                    스캔 중단
                                </button>
                            ` : ''}
                        </div>
                    </div>
                </div>

                <!-- 진행 상황 카드 -->
                <div class="progress-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-activity"></i>
                            스캔 진행 상황
                        </h3>
                    </div>
                    <div class="card-body">
                        <div class="progress-section" id="scan-progress-section">
                            ${this.renderProgressSection()}
                        </div>
                    </div>
                </div>

                <!-- 폴더별 스캔 카드 -->
                <div class="folder-scan-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-folder"></i>
                            폴더별 스캔
                        </h3>
                    </div>
                    <div class="card-body">
                        <div class="folder-scan-description">
                            <p>특정 폴더만 스캔하려면 Google Drive 폴더 ID를 입력하세요.</p>
                        </div>
                        
                        <div class="folder-scan-form">
                            <div class="input-group">
                                <input type="text" 
                                       class="form-input folder-id-input" 
                                       placeholder="폴더 ID (예: 1A2B3C4D5E6F7G8H9I0J)"
                                       ${this.isScanning ? 'disabled' : ''}>
                                <button class="btn btn-primary folder-scan-btn"
                                        ${this.isScanning ? 'disabled' : ''}>
                                    <i class="icon-search"></i>
                                    폴더 스캔
                                </button>
                            </div>
                            
                            <div class="folder-options">
                                <label class="checkbox-label">
                                    <input type="checkbox" class="recursive-checkbox" checked>
                                    <span class="checkmark"></span>
                                    하위 폴더 포함
                                </label>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- 해시 계산 카드 -->
                <div class="hash-calculation-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-hash"></i>
                            해시 계산
                        </h3>
                        <div class="hash-status">
                            <span class="status-indicator" data-status="${this.isCalculatingHashes ? 'running' : 'idle'}">
                                ${this.isCalculatingHashes ? '계산 중' : '대기'}
                            </span>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="hash-description">
                            <p>스캔된 파일들의 SHA-256 해시를 계산합니다. 
                            해시 값은 정확한 중복 파일 검출에 사용됩니다.</p>
                        </div>
                        
                        <div class="hash-controls">
                            <button class="btn btn-primary hash-btn"
                                    ${this.isCalculatingHashes ? 'disabled' : ''}>
                                <i class="icon-calculate"></i>
                                ${this.isCalculatingHashes ? '계산 중...' : '해시 계산 시작'}
                            </button>
                            
                            <div class="hash-options">
                                <select class="form-select hash-algorithm-select">
                                    <option value="sha256">SHA-256 (권장)</option>
                                    <option value="sha1">SHA-1</option>
                                    <option value="md5">MD5</option>
                                </select>
                                
                                <label class="checkbox-label">
                                    <input type="checkbox" class="skip-large-files-checkbox">
                                    <span class="checkmark"></span>
                                    대용량 파일 제외 (100MB 이상)
                                </label>
                            </div>
                        </div>
                        
                        <div class="hash-progress-section" id="hash-progress-section">
                            ${this.renderHashProgressSection()}
                        </div>
                    </div>
                </div>

                <!-- 스캔 결과 카드 -->
                <div class="scan-results-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-list"></i>
                            스캔 결과
                        </h3>
                    </div>
                    <div class="card-body">
                        <div class="scan-summary" id="scan-summary">
                            ${this.renderScanSummary()}
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    renderProgressSection() {
        const progress = getState('fileScan.progress');
        
        return `
            <div class="progress-container">
                <div class="progress-bar-wrapper" id="scan-progress-bar"></div>
                <div class="progress-details">
                    <div class="progress-stats">
                        <span class="stat-item">
                            <strong>처리된 파일:</strong> 
                            <span id="processed-files">${progress?.processedItems || 0}</span>
                        </span>
                        <span class="stat-item">
                            <strong>전체 파일:</strong> 
                            <span id="total-files">${progress?.totalItems || 0}</span>
                        </span>
                        <span class="stat-item">
                            <strong>현재 단계:</strong> 
                            <span id="current-step">${progress?.currentStep || '대기 중'}</span>
                        </span>
                    </div>
                    
                    <div class="progress-time">
                        <span class="time-item">
                            <strong>경과 시간:</strong> 
                            <span id="elapsed-time">${this.formatDuration(progress?.elapsedTime || 0)}</span>
                        </span>
                        <span class="time-item">
                            <strong>예상 완료:</strong> 
                            <span id="estimated-time">${this.formatETA(progress?.estimatedTime)}</span>
                        </span>
                    </div>
                </div>
            </div>
        `;
    }

    renderHashProgressSection() {
        const progress = getState('hashCalculation.progress');
        
        return `
            <div class="hash-progress-container">
                <div class="progress-bar-wrapper" id="hash-progress-bar"></div>
                <div class="hash-details">
                    <div class="hash-stats">
                        <span class="stat-item">
                            <strong>계산 완료:</strong> 
                            <span id="hash-completed">${progress?.completed || 0}</span>
                        </span>
                        <span class="stat-item">
                            <strong>대기 중:</strong> 
                            <span id="hash-pending">${progress?.pending || 0}</span>
                        </span>
                        <span class="stat-item">
                            <strong>현재 파일:</strong> 
                            <span id="current-file">${progress?.currentFile || '없음'}</span>
                        </span>
                    </div>
                </div>
            </div>
        `;
    }

    renderScanSummary() {
        const lastScan = getState('fileScan.lastScan');
        
        if (!lastScan) {
            return `
                <div class="empty-state">
                    <i class="icon-info"></i>
                    <h4>스캔 결과가 없습니다</h4>
                    <p>파일 스캔을 시작하여 Google Drive의 파일 정보를 수집하세요.</p>
                </div>
            `;
        }

        return `
            <div class="summary-grid">
                <div class="summary-item">
                    <div class="summary-icon">
                        <i class="icon-file"></i>
                    </div>
                    <div class="summary-content">
                        <div class="summary-value">${this.formatNumber(lastScan.totalFiles)}</div>
                        <div class="summary-label">스캔된 파일</div>
                    </div>
                </div>
                
                <div class="summary-item">
                    <div class="summary-icon">
                        <i class="icon-folder"></i>
                    </div>
                    <div class="summary-content">
                        <div class="summary-value">${this.formatNumber(lastScan.totalFolders)}</div>
                        <div class="summary-label">폴더</div>
                    </div>
                </div>
                
                <div class="summary-item">
                    <div class="summary-icon">
                        <i class="icon-database"></i>
                    </div>
                    <div class="summary-content">
                        <div class="summary-value">${this.formatFileSize(lastScan.totalSize)}</div>
                        <div class="summary-label">전체 크기</div>
                    </div>
                </div>
                
                <div class="summary-item">
                    <div class="summary-icon">
                        <i class="icon-clock"></i>
                    </div>
                    <div class="summary-content">
                        <div class="summary-value">${this.formatDuration(lastScan.duration)}</div>
                        <div class="summary-label">소요 시간</div>
                    </div>
                </div>
            </div>
            
            <div class="scan-actions">
                <button class="btn btn-primary" onclick="window.location.hash = '/duplicates'">
                    <i class="icon-search"></i>
                    중복 파일 검색하기
                </button>
                <button class="btn btn-secondary export-results-btn">
                    <i class="icon-download"></i>
                    결과 내보내기
                </button>
            </div>
        `;
    }

    afterRender() {
        super.afterRender();
        this.initializeProgressBars();
        this.updateScanStatus();
    }

    bindEvents() {
        // 전체 스캔 버튼
        const scanBtn = this.$('.scan-btn');
        if (scanBtn) {
            this.addEventListener(scanBtn, 'click', this.handleStartScan);
        }

        // 새로고침 버튼
        const refreshBtn = this.$('.refresh-btn');
        if (refreshBtn) {
            this.addEventListener(refreshBtn, 'click', this.handleRefreshProgress);
        }

        // 스캔 중단 버튼
        const stopBtn = this.$('.stop-btn');
        if (stopBtn) {
            this.addEventListener(stopBtn, 'click', this.handleStopScan);
        }

        // 폴더 스캔 버튼
        const folderScanBtn = this.$('.folder-scan-btn');
        if (folderScanBtn) {
            this.addEventListener(folderScanBtn, 'click', this.handleFolderScan);
        }

        // 해시 계산 버튼
        const hashBtn = this.$('.hash-btn');
        if (hashBtn) {
            this.addEventListener(hashBtn, 'click', this.handleStartHashCalculation);
        }

        // 결과 내보내기 버튼
        const exportBtn = this.$('.export-results-btn');
        if (exportBtn) {
            this.addEventListener(exportBtn, 'click', this.handleExportResults);
        }

        // 폴더 ID 입력 시 엔터키 처리
        const folderInput = this.$('.folder-id-input');
        if (folderInput) {
            this.addEventListener(folderInput, 'keypress', (event) => {
                if (event.key === 'Enter') {
                    this.handleFolderScan();
                }
            });
        }
    }

    initializeProgressBars() {
        // 스캔 진행률 바
        const scanContainer = this.$('#scan-progress-bar');
        if (scanContainer) {
            this.progressBar = new ProgressBar(scanContainer, {
                label: '파일 스캔 진행률',
                showText: true,
                showPercentage: true,
                color: 'primary',
                height: 'normal'
            });
        }

        // 해시 계산 진행률 바
        const hashContainer = this.$('#hash-progress-bar');
        if (hashContainer) {
            this.hashProgressBar = new ProgressBar(hashContainer, {
                label: '해시 계산 진행률',
                showText: true,
                showPercentage: true,
                color: 'info',
                height: 'normal'
            });
        }
    }

    async handleStartScan() {
        if (this.isScanning) return;

        try {
            this.emit(EVENTS.SHOW_LOADING, '파일 스캔을 시작하는 중...');
            
            // API 호출 (실제 구현에서는 API 서비스 사용)
            const response = await this.startFileScan();
            
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '파일 스캔이 시작되었습니다.'
            });

            // 스캔 상태 업데이트
            setState('fileScan.isRunning', true);
            
            // 진행 상황 모니터링 시작
            this.startProgressMonitoring();

        } catch (error) {
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `스캔 시작 실패: ${error.message}`
            });
        }
    }

    async handleFolderScan() {
        const folderInput = this.$('.folder-id-input');
        const folderId = folderInput?.value.trim();

        if (!folderId) {
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'warning',
                message: '폴더 ID를 입력하세요.'
            });
            return;
        }

        const recursive = this.$('.recursive-checkbox')?.checked || false;

        try {
            this.emit(EVENTS.SHOW_LOADING, '폴더 스캔을 시작하는 중...');
            
            const response = await this.startFolderScan(folderId, recursive);
            
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '폴더 스캔이 시작되었습니다.'
            });

            // 입력 필드 초기화
            folderInput.value = '';
            
            // 스캔 상태 업데이트
            setState('fileScan.isRunning', true);
            this.startProgressMonitoring();

        } catch (error) {
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `폴더 스캔 실패: ${error.message}`
            });
        }
    }

    async handleStartHashCalculation() {
        if (this.isCalculatingHashes) return;

        const algorithm = this.$('.hash-algorithm-select')?.value || 'sha256';
        const skipLargeFiles = this.$('.skip-large-files-checkbox')?.checked || false;

        try {
            this.emit(EVENTS.SHOW_LOADING, '해시 계산을 시작하는 중...');
            
            const response = await this.startHashCalculation(algorithm, skipLargeFiles);
            
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '해시 계산이 시작되었습니다.'
            });

            // 해시 계산 상태 업데이트
            setState('hashCalculation.isRunning', true);

        } catch (error) {
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `해시 계산 실패: ${error.message}`
            });
        }
    }

    async handleStopScan() {
        try {
            const response = await this.stopFileScan();
            
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'info',
                message: '스캔이 중단되었습니다.'
            });

            setState('fileScan.isRunning', false);
            this.stopProgressMonitoring();

        } catch (error) {
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `스캔 중단 실패: ${error.message}`
            });
        }
    }

    handleRefreshProgress() {
        this.refreshProgressStatus();
    }

    async handleExportResults() {
        try {
            const results = await this.exportScanResults();
            
            // CSV 파일 다운로드
            const blob = new Blob([results], { type: 'text/csv' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `scan-results-${new Date().toISOString().slice(0, 10)}.csv`;
            a.click();
            
            URL.revokeObjectURL(url);
            
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '스캔 결과가 내보내졌습니다.'
            });

        } catch (error) {
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `내보내기 실패: ${error.message}`
            });
        }
    }

    startProgressMonitoring() {
        if (this.progressUpdateInterval) {
            clearInterval(this.progressUpdateInterval);
        }

        this.progressUpdateInterval = setInterval(() => {
            this.refreshProgressStatus();
        }, 2000); // 2초마다 업데이트
    }

    stopProgressMonitoring() {
        if (this.progressUpdateInterval) {
            clearInterval(this.progressUpdateInterval);
            this.progressUpdateInterval = null;
        }
    }

    async refreshProgressStatus() {
        try {
            // 실제로는 API에서 진행 상황을 가져옴
            const progress = await this.fetchScanProgress();
            setState('fileScan.progress', progress);
            
        } catch (error) {
            console.error('진행 상황 업데이트 실패:', error);
        }
    }

    updateScanStatus() {
        // 스캔 상태 인디케이터 업데이트
        const statusIndicator = this.$('.scan-card .status-indicator');
        if (statusIndicator) {
            statusIndicator.setAttribute('data-status', this.isScanning ? 'running' : 'idle');
            statusIndicator.textContent = this.isScanning ? '진행 중' : '대기';
        }

        // 버튼 상태 업데이트
        const scanBtn = this.$('.scan-btn');
        if (scanBtn) {
            scanBtn.disabled = this.isScanning;
            scanBtn.innerHTML = this.isScanning ? 
                '<i class="icon-spinner"></i> 스캔 중...' : 
                '<i class="icon-play"></i> 전체 스캔 시작';
        }

        // 폴더 입력 및 버튼 상태 업데이트
        const folderInput = this.$('.folder-id-input');
        const folderBtn = this.$('.folder-scan-btn');
        if (folderInput) folderInput.disabled = this.isScanning;
        if (folderBtn) folderBtn.disabled = this.isScanning;
    }

    updateProgress(progress) {
        if (!progress || !this.progressBar) return;

        const percentage = progress.totalItems > 0 ? 
            (progress.processedItems / progress.totalItems) * 100 : 0;
        
        this.progressBar.setValue(percentage);
        this.progressBar.setStatusText(progress.currentStep || '');
        
        if (progress.estimatedTime) {
            this.progressBar.setETA(progress.estimatedTime);
        }

        // 진행 상황 텍스트 업데이트
        this.updateProgressText(progress);
    }

    updateHashProgress(progress) {
        if (!progress || !this.hashProgressBar) return;

        const percentage = progress.total > 0 ? 
            (progress.completed / progress.total) * 100 : 0;
        
        this.hashProgressBar.setValue(percentage);
        this.hashProgressBar.setStatusText(progress.currentFile || '');
    }

    updateProgressText(progress) {
        const elements = {
            'processed-files': progress.processedItems || 0,
            'total-files': progress.totalItems || 0,
            'current-step': progress.currentStep || '대기 중',
            'elapsed-time': this.formatDuration(progress.elapsedTime || 0),
            'estimated-time': this.formatETA(progress.estimatedTime)
        };

        Object.entries(elements).forEach(([id, value]) => {
            const element = this.$(`#${id}`);
            if (element) {
                element.textContent = value;
            }
        });
    }

    handleScanStart(data) {
        this.isScanning = true;
        this.updateScanStatus();
        this.startProgressMonitoring();
    }

    handleScanProgress(data) {
        this.updateProgress(data.progress);
    }

    handleScanComplete(data) {
        this.isScanning = false;
        this.updateScanStatus();
        this.stopProgressMonitoring();
        
        // 스캔 완료 알림
        this.emit(EVENTS.SHOW_TOAST, {
            type: 'success',
            message: `파일 스캔이 완료되었습니다. ${data.totalFiles}개의 파일을 스캔했습니다.`
        });

        // 결과 요약 업데이트
        const summaryContainer = this.$('#scan-summary');
        if (summaryContainer) {
            summaryContainer.innerHTML = this.renderScanSummary();
        }
    }

    // API 호출 메서드들 (실제로는 별도 서비스에서 처리)
    async startFileScan() {
        // 모의 구현
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async startFolderScan(folderId, recursive) {
        // 모의 구현
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async startHashCalculation(algorithm, skipLargeFiles) {
        // 모의 구현
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async stopFileScan() {
        // 모의 구현
        return new Promise(resolve => setTimeout(resolve, 500));
    }

    async fetchScanProgress() {
        // 모의 구현
        return {
            processedItems: Math.floor(Math.random() * 1000),
            totalItems: 1000,
            currentStep: '파일 메타데이터 수집 중...',
            elapsedTime: Date.now() - (Date.now() - 60000),
            estimatedTime: 120
        };
    }

    async exportScanResults() {
        // 모의 구현
        return 'File Name,Size,Modified Date,Path\ntest.txt,1024,2024-01-01,/test.txt';
    }

    // 유틸리티 메서드들
    formatNumber(num) {
        return num.toLocaleString('ko-KR');
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatDuration(ms) {
        const seconds = Math.floor(ms / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        
        if (hours > 0) {
            return `${hours}시간 ${minutes % 60}분`;
        } else if (minutes > 0) {
            return `${minutes}분 ${seconds % 60}초`;
        } else {
            return `${seconds}초`;
        }
    }

    formatETA(seconds) {
        if (!seconds || seconds <= 0) return '계산 중...';
        
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        
        if (hours > 0) {
            return `약 ${hours}시간 ${minutes % 60}분 남음`;
        } else if (minutes > 0) {
            return `약 ${minutes}분 남음`;
        } else {
            return `약 ${seconds}초 남음`;
        }
    }

    onDestroy() {
        this.stopProgressMonitoring();
        
        if (this.progressBar) {
            this.progressBar.destroy();
        }
        
        if (this.hashProgressBar) {
            this.hashProgressBar.destroy();
        }
        
        super.onDestroy();
    }
}