import { Component } from '../components/base/Component.js';
// import { ProgressBar } from '../components/widgets/ProgressBar.js'; // 임시로 주석 처리
import { EVENTS } from '../core/EventBus.js';
import { getState, setState } from '../core/StateManager.js';

/**
 * 대시보드 페이지 컴포넌트
 */
export class Dashboard extends Component {
    constructor(element) {
        super(element);
        this.progressChart = null;
        this.statusCheckInterval = null;
        this.progressBars = new Map();
        this.progressInterval = null;
        
        // 기본 설정 값들
        this.duplicateWorkers = 3;
        this.hashWorkers = 5;
    }

    onInit() {
        this.setupStateWatchers();
        this.setupEventListeners();
        this.render();
        this.startPeriodicUpdates();
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

    setupStateWatchers() {
        // 시스템 상태 워치
        this.watch('system', () => {
            this.updateSystemStatus();
        });

        // 통계 워치
        this.watch('stats', () => {
            this.updateStatistics();
        });

        // 파일 스캔 진행률 워치
        this.watch('fileScan.progress', (progress) => {
            this.updateScanProgress(progress);
        });

        // 중복 검색 진행률 워치
        this.watch('duplicates', (duplicateState) => {
            this.updateDuplicateProgress(duplicateState);
        });
    }

    setupEventListeners() {
        // 시스템 상태 새로고침 이벤트
        this.onEvent('system:refresh-status', this.refreshSystemStatus);

        // API 요청 관련 이벤트
        this.onEvent(EVENTS.API_REQUEST_START, this.handleApiStart);
        this.onEvent(EVENTS.API_REQUEST_END, this.handleApiEnd);
        this.onEvent(EVENTS.API_REQUEST_ERROR, this.handleApiError);
    }

    template() {
        return `
            <div class="dashboard-container">
                <!-- 헤더 영역 -->
                <header class="dashboard-header">
                    <h1><i class="fas fa-search-plus"></i> Google Drive 중복 파일 관리</h1>
                    <p class="subtitle">구글 드라이브의 중복 파일을 찾고 정리하여 저장공간을 확보하세요</p>
                </header>

                <!-- 시스템 상태 카드 영역 -->
                <section class="status-cards" id="status-cards">
                    ${this.renderStatusCards()}
                </section>

                <!-- 빠른 작업 버튼 영역 -->
                <section class="quick-actions">
                    <h2><i class="fas fa-bolt"></i> 빠른 작업</h2>
                    <div class="action-grid">
                        ${this.renderQuickActions()}
                    </div>
                </section>

                <!-- 진행 중인 작업 현황 -->
                <section class="current-operations" id="current-operations">
                    <h2><i class="fas fa-tasks"></i> 진행 중인 작업</h2>
                    <div class="operations-content" id="operations-content">
                        ${this.renderCurrentOperations()}
                    </div>
                </section>

                <!-- 최근 결과 요약 -->
                <section class="recent-results">
                    <div class="section-header">
                        <h2><i class="fas fa-history"></i> 최근 결과</h2>
                        <button class="btn-link" onclick="window.app.router.navigate('/duplicates')">
                            전체 보기 <i class="fas fa-arrow-right"></i>
                        </button>
                    </div>
                    <div class="recent-duplicates" id="recent-duplicates">
                        ${this.renderRecentDuplicates()}
                    </div>
                </section>

                <!-- 고급 기능 패널 (접힌 상태) -->
                <section class="advanced-panel">
                    <div class="panel-toggle" onclick="window.dashboardInstance.toggleAdvancedPanel()">
                        <h3><i class="fas fa-cogs"></i> 고급 기능 및 설정</h3>
                        <i class="fas fa-chevron-down toggle-icon" id="advanced-toggle-icon"></i>
                    </div>
                    <div class="panel-content" id="advanced-panel-content" style="display: none;">
                        ${this.renderAdvancedFeatures()}
                    </div>
                </section>
            </div>
        `;
    }

    renderStatusCards() {
        const stats = getState('stats') || {};
        const system = getState('system') || {};
        
        return `
            <div class="status-card">
                <div class="card-icon system-status">
                    <i class="fas fa-server"></i>
                </div>
                <div class="card-content">
                    <div class="card-title">시스템 상태</div>
                    <div class="card-value status-${system.status || 'unknown'}">
                        ${this.getStatusText(system.status || 'unknown')}
                    </div>
                    <div class="card-subtitle">
                        Google Drive 연결: ${system.storage?.status ? this.getStatusText(system.storage.status) : '확인 중'}
                    </div>
                </div>
            </div>
            
            <div class="status-card">
                <div class="card-icon files-scanned">
                    <i class="fas fa-file-alt"></i>
                </div>
                <div class="card-content">
                    <div class="card-title">스캔된 파일</div>
                    <div class="card-value">${this.formatNumber(stats.totalFiles || 0)}</div>
                    <div class="card-subtitle">
                        총 크기: ${this.formatFileSize(stats.totalSize || 0)}
                    </div>
                </div>
            </div>
            
            <div class="status-card">
                <div class="card-icon duplicates-found">
                    <i class="fas fa-copy"></i>
                </div>
                <div class="card-content">
                    <div class="card-title">중복 그룹</div>
                    <div class="card-value">${this.formatNumber(stats.duplicateGroups || 0)}</div>
                    <div class="card-subtitle">
                        중복 파일: ${this.formatNumber(stats.duplicateFiles || 0)}개
                    </div>
                </div>
            </div>
            
            <div class="status-card">
                <div class="card-icon space-savings">
                    <i class="fas fa-hdd"></i>
                </div>
                <div class="card-content">
                    <div class="card-title">절약 가능 용량</div>
                    <div class="card-value savings">${this.formatFileSize(stats.potentialSavings || 0)}</div>
                    <div class="card-subtitle">
                        중복 파일 삭제 시
                    </div>
                </div>
            </div>
        `;
    }

    renderQuickActions() {
        return `
            <div class="action-card primary" onclick="window.dashboardInstance.scanDuplicates()" id="scanBtn">
                <div class="action-icon">
                    <i class="fas fa-search-plus"></i>
                </div>
                <div class="action-content">
                    <div class="action-title">중복 검사</div>
                    <div class="action-description">전체 드라이브 스캔</div>
                </div>
            </div>
            
            <div class="action-card secondary" onclick="window.app.router.navigate('/folder-comparison')">
                <div class="action-icon">
                    <i class="fas fa-folder-open"></i>
                </div>
                <div class="action-content">
                    <div class="action-title">폴더 비교</div>
                    <div class="action-description">두 폴더 간 중복 찾기</div>
                </div>
            </div>
            
            <div class="action-card tertiary" onclick="window.app.router.navigate('/cleanup')">
                <div class="action-icon">
                    <i class="fas fa-broom"></i>
                </div>
                <div class="action-content">
                    <div class="action-title">정리 작업</div>
                    <div class="action-description">븈 폴더 및 사용하지 않는 파일</div>
                </div>
            </div>
            
            <div class="action-card info" onclick="window.app.router.navigate('/statistics')">
                <div class="action-icon">
                    <i class="fas fa-chart-bar"></i>
                </div>
                <div class="action-content">
                    <div class="action-title">통계 보기</div>
                    <div class="action-description">파일 분석 및 리포트</div>
                </div>
            </div>
        `;
    }

    renderCurrentOperations() {
        const progress = getState('fileScan.progress');
        const duplicateState = getState('duplicates');
        
        if (!progress && !duplicateState?.inProgress) {
            return `
                <div class="no-operations">
                    <i class="fas fa-check-circle"></i>
                    <span>진행 중인 작업이 없습니다</span>
                </div>
            `;
        }

        let operations = [];

        if (progress && progress.status === 'running') {
            operations.push({
                type: 'scan',
                title: '파일 스캔',
                status: 'running',
                progress: Math.round((progress.processedFiles / progress.totalFiles) * 100) || 0,
                details: `${progress.processedFiles || 0} / ${progress.totalFiles || 0} 파일`
            });
        }

        if (duplicateState?.inProgress) {
            operations.push({
                type: 'duplicate',
                title: '중복 검사',
                status: 'running',
                progress: duplicateState.progress || 0,
                details: `${duplicateState.currentStep || '처리 중'}`
            });
        }

        return operations.map(op => `
            <div class="operation-item">
                <div class="operation-info">
                    <div class="operation-header">
                        <span class="operation-title">
                            <i class="fas fa-${op.type === 'scan' ? 'search' : 'copy'}"></i>
                            ${op.title}
                        </span>
                        <span class="operation-progress">${op.progress}%</span>
                    </div>
                    <div class="operation-details">${op.details}</div>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${op.progress}%"></div>
                    </div>
                </div>
                <div class="operation-actions">
                    <button class="btn-icon" onclick="window.dashboardInstance.pauseOperation('${op.type}')" title="일시정지">
                        <i class="fas fa-pause"></i>
                    </button>
                    <button class="btn-icon danger" onclick="window.dashboardInstance.stopOperation('${op.type}')" title="중지">
                        <i class="fas fa-stop"></i>
                    </button>
                </div>
            </div>
        `).join('');
    }
    }

    renderRecentDuplicates() {
        const duplicates = getState('duplicates.recent') || [];
        
        if (duplicates.length === 0) {
            return `
                <div class="empty-results">
                    <i class="fas fa-inbox"></i>
                    <span>아직 중복 파일을 검사하지 않았습니다</span>
                    <button class="btn-primary" onclick="window.dashboardInstance.scanDuplicates()">
                        지금 검사하기
                    </button>
                </div>
            `;
        }

        return duplicates.slice(0, 5).map((group, index) => `
            <div class="duplicate-preview" onclick="window.app.router.navigate('/duplicates?group=${group.id}')">
                <div class="preview-header">
                    <div class="duplicate-info">
                        <span class="file-name">${group.fileName || '이름 없음'}</span>
                        <span class="file-count">${group.fileCount}개 파일</span>
                    </div>
                    <div class="duplicate-size">${this.formatFileSize(group.totalSize || 0)}</div>
                </div>
                <div class="preview-actions">
                    <button class="btn-sm danger" onclick="event.stopPropagation(); window.dashboardInstance.quickDelete('${group.id}')">
                        빠른 삭제
                    </button>
                    <button class="btn-sm secondary" onclick="event.stopPropagation(); window.dashboardInstance.viewGroup('${group.id}')">
                        상세보기
                    </button>
                </div>
            </div>
        `).join('');
    }

    renderAdvancedFeatures() {
        return `
            <div class="advanced-grid">
                <!-- 성능 설정 -->
                <div class="advanced-section">
                    <h4><i class="fas fa-tachometer-alt"></i> 성능 설정</h4>
                    ${this.renderPerformanceSettings()}
                </div>
                
                <!-- 추가 도구 -->
                <div class="advanced-section">
                    <h4><i class="fas fa-tools"></i> 추가 도구</h4>
                    <div class="tool-buttons">
                        <button class="btn-tool" onclick="window.dashboardInstance.updateParents()">
                            <i class="fas fa-route"></i>
                            경로 정보 업데이트
                        </button>
                        <button class="btn-tool" onclick="window.dashboardInstance.cleanupDeletedFiles()">
                            <i class="fas fa-trash-restore"></i>
                            삭제된 파일 정리
                        </button>
                        <button class="btn-tool" onclick="window.dashboardInstance.cleanupEmptyFolders()">
                            <i class="fas fa-folder-minus"></i>
                            빈 폴더 정리
                        </button>
                        <button class="btn-tool" onclick="window.app.router.navigate('/file-explorer')">
                            <i class="fas fa-search"></i>
                            파일 탐색기
                        </button>
                    </div>
                </div>
                
                <!-- 데이터 관리 -->
                <div class="advanced-section">
                    <h4><i class="fas fa-database"></i> 데이터 관리</h4>
                    <div class="data-actions">
                        <button class="btn-secondary" onclick="window.dashboardInstance.manualRefresh()" id="refreshBtn">
                            <i class="fas fa-sync-alt"></i>
                            데이터 새로고침
                        </button>
                        <button class="btn-secondary" onclick="window.dashboardInstance.resumeScan()" id="resumeBtn" style="display: none;">
                            <i class="fas fa-play"></i>
                            저장된 작업 재개
                        </button>
                        <button class="btn-danger" onclick="window.dashboardInstance.resetData()" id="resetBtn" style="display: none;">
                            <i class="fas fa-trash-alt"></i>
                            모든 데이터 삭제
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    renderPerformanceSettings() {
        return `
            <div class="settings-grid">
                <div class="setting-item">
                    <label class="setting-label">스캔 워커:</label>
                    <div class="setting-controls">
                        <input type="range" id="duplicateWorkerSlider" min="1" max="20" value="${this.duplicateWorkers || 3}" class="slider">
                        <span class="setting-value" id="duplicateWorkerValue">${this.duplicateWorkers || 3}</span>
                        <button class="btn-apply" onclick="window.dashboardInstance.applyDuplicateWorkerSettings()">적용</button>
                    </div>
                </div>
                
                <div class="setting-item">
                    <label class="setting-label">해시 워커:</label>
                    <div class="setting-controls">
                        <input type="range" id="workerSlider" min="1" max="20" value="${this.hashWorkers || 5}" class="slider">
                        <span class="setting-value" id="workerValue">${this.hashWorkers || 5}</span>
                        <button class="btn-apply" onclick="window.dashboardInstance.applyWorkerSettings()">적용</button>
                    </div>
                </div>
            </div>
            
            <div class="setting-info">
                <i class="fas fa-info-circle"></i>
                값이 클수록 빠르지만 시스템 자원을 더 많이 사용합니다.
            </div>
        `;
    }

    // 기존 섹션들은 이제 별도 페이지에서 처리




    afterRender() {
        // 전역 인스턴스 등록
        window.dashboardInstance = this;
        
        // 초기 데이터 로드
        this.loadInitialData();
        
        // 이벤트 리스너 설정
        this.setupSliderEvents();
        this.setupAdvancedPanel();
    }

    bindEvents() {
        // 기본 이벤트 리스너는 이미 온클릭 합수로 처리
    }

    setupSliderEvents() {
        // 중복 워커 슬라이더
        const duplicateSlider = document.getElementById('duplicateWorkerSlider');
        if (duplicateSlider) {
            duplicateSlider.oninput = (e) => {
                const value = parseInt(e.target.value);
                const valueElement = document.getElementById('duplicateWorkerValue');
                if (valueElement) valueElement.textContent = value;
                this.duplicateWorkers = value;
            };
        }
        
        // 해시 워커 슬라이더
        const hashSlider = document.getElementById('workerSlider');
        if (hashSlider) {
            hashSlider.oninput = (e) => {
                const value = parseInt(e.target.value);
                const valueElement = document.getElementById('workerValue');
                if (valueElement) valueElement.textContent = value;
                this.hashWorkers = value;
            };
        }
    }

    setupAdvancedPanel() {
        // 고급 패널 토글 기능은 이미 toggleAdvancedPanel 메서드로 처리
    }

    async loadInitialData() {
        try {
            console.log('🔄 대시보드 초기 데이터 로드 시작');
            
            // API 서비스 확인
            if (!window.apiService) {
                console.warn('⚠️ API 서비스가 아직 초기화되지 않음');
                // 모의 데이터로 먼저 표시
                this.loadMockData();
                return;
            }
            
            // 시스템 상태 로드
            await this.refreshSystemStatus();
            
            // 통계 데이터 로드
            await this.loadStatistics();
            
            // 최근 중복 파일 로드
            await this.loadRecentDuplicates();
            
            // 진행 중인 작업 확인
            await this.checkRunningOperations();
            
            console.log('✅ 대시보드 초기 데이터 로드 완료');
            
        } catch (error) {
            console.error('❌ 대시보드 초기 데이터 로드 실패:', error);
            // 오류 시 모의 데이터 표시
            this.loadMockData();
        }
    }
    
    loadMockData() {
        console.log('📊 모의 데이터로 대시보드 표시');
        
        // 모의 시스템 상태
        setState('system', {
            status: 'ok',
            storage: { status: 'healthy' }
        });
        
        // 모의 통계 데이터
        setState('stats', {
            totalFiles: 12345,
            totalSize: 48318382080, // ~45GB
            duplicateGroups: 234,
            duplicateFiles: 567,
            potentialSavings: 9326927872 // ~8.7GB
        });
        
        // 모의 최근 중복 파일
        setState('duplicates.recent', [
            {
                id: '1',
                fileName: 'IMG_1234.jpg',
                fileCount: 5,
                totalSize: 25690112 // ~24.5MB
            },
            {
                id: '2',
                fileName: 'document.pdf',
                fileCount: 3,
                totalSize: 15941632 // ~15.2MB
            }
        ]);
        
        // UI 업데이트
        this.updateStatusCards();
        this.updateRecentDuplicates();
        this.updateCurrentOperations();
    }

    async refreshSystemStatus() {
        try {
            if (!window.apiService) {
                throw new Error('API 서비스가 초기화되지 않음');
            }
            
            const response = await window.apiService.get('/health');
            console.log('✅ 시스템 상태 로드:', response);
            setState('system', response);
            this.updateStatusCards();
        } catch (error) {
            console.error('❌ 시스템 상태 로드 실패:', error);
            setState('system', { status: 'error', error: error.message });
            this.updateStatusCards();
        }
    }

    async loadStatistics() {
        try {
            if (!window.apiService) {
                throw new Error('API 서비스가 초기화되지 않음');
            }
            
            const response = await window.apiService.get('/api/statistics');
            console.log('✅ 통계 데이터 로드:', response);
            setState('stats', response);
            this.updateStatusCards();
        } catch (error) {
            console.error('❌ 통계 데이터 로드 실패:', error);
        }
    }

    async loadRecentDuplicates() {
        try {
            if (!window.apiService) {
                throw new Error('API 서비스가 초기화되지 않음');
            }
            
            const response = await window.apiService.get('/api/duplicates?limit=5');
            console.log('✅ 최근 중복 파일 로드:', response);
            setState('duplicates.recent', response.duplicates || []);
            this.updateRecentDuplicates();
        } catch (error) {
            console.error('❌ 최근 중복 파일 로드 실패:', error);
        }
    }

    async checkRunningOperations() {
        try {
            if (!window.apiService) {
                throw new Error('API 서비스가 초기화되지 않음');
            }
            
            const response = await window.apiService.get('/api/progress');
            console.log('✅ 진행 상황 확인:', response);
            if (response.progress) {
                setState('fileScan.progress', response.progress);
            }
            this.updateCurrentOperations();
        } catch (error) {
            console.error('❌ 진행 상황 확인 실패:', error);
        }
    }

    // === 주요 기능 메서드들 ===
    
    async scanDuplicates() {
        console.log('🔍 중복 파일 검사 시작');
        
        try {
            const response = await window.apiService.post('/api/files/scan');
            console.log('스캔 시작:', response);
            
            // 성공 알림
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '중복 파일 검사를 시작했습니다.'
            });
            
            // 진행 상황 모니터링 시작
            this.startProgressMonitoring();
            
            // UI 업데이트
            this.updateCurrentOperations();
            
        } catch (error) {
            console.error('스캔 시작 오류:', error);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: '스캔 시작 실패: ' + error.message
            });
        }
    }

    async resumeScan() {
        console.log('🔄 저장된 작업 재개');
        
        try {
            const response = await window.apiService.post('/api/files/scan/resume');
            console.log('작업 재개:', response);
            
            this.startProgressMonitoring();
            document.getElementById('resumeBtn').style.display = 'none';
            
        } catch (error) {
            console.error('작업 재개 오류:', error);
            alert('작업 재개에 실패했습니다: ' + error.message);
        }
    }

    async resetData() {
        if (!confirm('모든 데이터를 삭제하시겠습니까? 이 작업은 취소할 수 없습니다.')) {
            return;
        }
        
        try {
            const response = await window.apiService.delete('/api/files/all');
            console.log('데이터 리셋:', response);
            
            // UI 리셋
            document.getElementById('results').innerHTML = '';
            document.getElementById('progress').style.display = 'none';
            document.getElementById('resumeBtn').style.display = 'none';
            document.getElementById('resetBtn').style.display = 'none';
            
            alert('모든 데이터가 삭제되었습니다.');
            
        } catch (error) {
            console.error('데이터 리셋 오류:', error);
            alert('데이터 삭제에 실패했습니다: ' + error.message);
        }
    }

    async manualRefresh() {
        console.log('🔄 중복 파일 수동 새로고침');
        
        try {
            const response = await window.apiService.post('/api/duplicates/refresh');
            console.log('수동 새로고침:', response);
            
            await this.loadLiveDuplicates();
            alert('중복 파일 데이터가 새로고침되었습니다.');
            
        } catch (error) {
            console.error('수동 새로고침 오류:', error);
            alert('데이터 새로고침에 실패했습니다: ' + error.message);
        }
    }

    async updateParents() {
        console.log('📁 경로 정보 업데이트');
        
        try {
            const response = await window.apiService.post('/api/files/update-parents');
            console.log('경로 업데이트:', response);
            
            alert('경로 정보가 업데이트되었습니다.');
            
        } catch (error) {
            console.error('경로 업데이트 오류:', error);
            alert('경로 정보 업데이트에 실패했습니다: ' + error.message);
        }
    }

    async cleanupDeletedFiles() {
        console.log('🧹 삭제된 파일 정리');
        
        try {
            const response = await window.apiService.post('/api/cleanup/deleted-files');
            console.log('삭제된 파일 정리:', response);
            
            alert(`삭제된 파일 ${response.cleaned || 0}개가 정리되었습니다.`);
            
        } catch (error) {
            console.error('삭제된 파일 정리 오류:', error);
            alert('삭제된 파일 정리에 실패했습니다: ' + error.message);
        }
    }

    async cleanupEmptyFolders() {
        console.log('📂 빈 폴더 정리');
        
        try {
            const response = await window.apiService.post('/api/cleanup/empty-folders');
            console.log('빈 폴더 정리:', response);
            
            alert(`빈 폴더 ${response.cleaned || 0}개가 정리되었습니다.`);
            
        } catch (error) {
            console.error('빈 폴더 정리 오류:', error);
            alert('빈 폴더 정리에 실패했습니다: ' + error.message);
        }
    }

    async loadRecentOperations() {
        const container = this.$('#recent-operations-list');
        if (!container) return;

        try {
            // 실제로는 API에서 데이터를 가져옴
            // 여기서는 모의 데이터 사용
            const operations = await this.fetchRecentOperations();
            
            if (operations.length === 0) {
                container.innerHTML = `
                    <div class="empty-state">
                        <i class="icon-info"></i>
                        <span>최근 작업이 없습니다</span>
                    </div>
                `;
            } else {
                container.innerHTML = operations.map(op => `
                    <div class="operation-item">
                        <div class="operation-icon">
                            <i class="icon-${op.type}"></i>
                        </div>
                        <div class="operation-content">
                            <div class="operation-title">${op.title}</div>
                            <div class="operation-time">${this.formatDate(op.timestamp)}</div>
                        </div>
                        <div class="operation-status status-${op.status}">
                            ${op.status}
                        </div>
                    </div>
                `).join('');
            }
        } catch (error) {
            container.innerHTML = `
                <div class="error-state">
                    <i class="icon-alert"></i>
                    <span>작업 내역을 불러올 수 없습니다</span>
                </div>
            `;
        }
    }

    async fetchRecentOperations() {
        // 모의 데이터 - 실제로는 API 호출
        return new Promise(resolve => {
            setTimeout(() => {
                resolve([
                    { type: 'scan', title: '전체 파일 스캔', timestamp: new Date(Date.now() - 3600000), status: 'completed' },
                    { type: 'duplicate', title: '중복 파일 검색', timestamp: new Date(Date.now() - 7200000), status: 'completed' },
                    { type: 'cleanup', title: '빈 폴더 정리', timestamp: new Date(Date.now() - 10800000), status: 'completed' }
                ]);
            }, 1000);
        });
    }

    startPeriodicUpdates() {
        // 30초마다 상태 확인
        this.statusCheckInterval = setInterval(() => {
            this.emit('system:refresh-status');
        }, 30000);
    }

    handleApiStart(data) {
        setState('app.loading', true);
    }

    handleApiEnd(data) {
        setState('app.loading', false);
    }

    handleApiError(data) {
        setState('app.loading', false);
        this.emit(EVENTS.SHOW_TOAST, {
            type: 'error',
            message: `API 오류: ${data.error?.message || '알 수 없는 오류'}`
        });
    }

    // === 기존 메서드들 업데이트 ===
    
    async applyDuplicateWorkerSettings() {
        try {
            const response = await window.apiService.post('/api/settings/duplicate-workers', {
                workers: this.duplicateWorkers
            });
            console.log('중복 워커 설정 적용:', response);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '설정이 적용되었습니다.'
            });
        } catch (error) {
            console.error('설정 적용 실패:', error);
        }
    }
    
    async applyWorkerSettings() {
        try {
            const response = await window.apiService.post('/api/settings/hash-workers', {
                workers: this.hashWorkers
            });
            console.log('해시 워커 설정 적용:', response);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '설정이 적용되었습니다.'
            });
        } catch (error) {
            console.error('설정 적용 실패:', error);
        }
    }

    // 유틸리티 메서드들
    getStatusText(status) {
        const statusMap = {
            'healthy': '정상',
            'ok': '정상',
            'unhealthy': '오류',
            'error': '오류',
            'unknown': '확인 중'
        };
        return statusMap[status] || '알 수 없음';
    }

    getServiceLabel(service) {
        const labelMap = {
            'server': '서버',
            'database': '데이터베이스',
            'storage': 'Google Drive'
        };
        return labelMap[service] || service;
    }

    getStatusDetails(service) {
        const systemStatus = getState('system');
        const serviceStatus = systemStatus?.[service];
        
        return `
            <div class="status-details">
                <p><strong>서비스:</strong> ${this.getServiceLabel(service)}</p>
                <p><strong>상태:</strong> ${this.getStatusText(serviceStatus?.status)}</p>
                <p><strong>마지막 확인:</strong> ${this.formatLastCheck(serviceStatus?.lastCheck)}</p>
                ${serviceStatus?.error ? `<p><strong>오류:</strong> ${serviceStatus.error}</p>` : ''}
            </div>
        `;
    }

    formatNumber(num) {
        // 전체 숫자를 쉼표로 구분하여 표시
        return num.toLocaleString();
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatDate(date) {
        if (!date) return '정보 없음';
        return new Date(date).toLocaleString('ko-KR');
    }

    formatLastCheck(timestamp) {
        if (!timestamp) return '확인된 적 없음';
        const now = new Date();
        const checkTime = new Date(timestamp);
        const diffMs = now - checkTime;
        const diffMins = Math.floor(diffMs / 60000);
        
        if (diffMins < 1) return '방금 전';
        if (diffMins < 60) return `${diffMins}분 전`;
        const diffHours = Math.floor(diffMins / 60);
        if (diffHours < 24) return `${diffHours}시간 전`;
        return checkTime.toLocaleDateString('ko-KR');
    }

    // === 새로운 대시보드 메서드들 ===

    updateStatusCards() {
        const statusCardsElement = document.getElementById('status-cards');
        if (statusCardsElement) {
            statusCardsElement.innerHTML = this.renderStatusCards();
        }
    }

    updateCurrentOperations() {
        const operationsElement = document.getElementById('operations-content');
        if (operationsElement) {
            operationsElement.innerHTML = this.renderCurrentOperations();
        }
    }

    updateRecentDuplicates() {
        const recentElement = document.getElementById('recent-duplicates');
        if (recentElement) {
            recentElement.innerHTML = this.renderRecentDuplicates();
        }
    }

    toggleAdvancedPanel() {
        const content = document.getElementById('advanced-panel-content');
        const icon = document.getElementById('advanced-toggle-icon');
        
        if (content && icon) {
            const isVisible = content.style.display !== 'none';
            content.style.display = isVisible ? 'none' : 'block';
            icon.classList.toggle('fa-chevron-down', isVisible);
            icon.classList.toggle('fa-chevron-up', !isVisible);
        }
    }

    async pauseOperation(type) {
        try {
            const response = await window.apiService.post(`/api/${type}/pause`);
            console.log(`${type} 작업 일시정지:`, response);
            this.updateCurrentOperations();
        } catch (error) {
            console.error(`${type} 작업 일시정지 실패:`, error);
        }
    }

    async stopOperation(type) {
        if (!confirm('작업을 중지하시겠습니까? 진행 상황이 저장될 수 있습니다.')) {
            return;
        }
        
        try {
            const response = await window.apiService.post(`/api/${type}/stop`);
            console.log(`${type} 작업 중지:`, response);
            this.updateCurrentOperations();
        } catch (error) {
            console.error(`${type} 작업 중지 실패:`, error);
        }
    }

    async quickDelete(groupId) {
        if (!confirm('이 중복 그룹의 파일들을 빠르게 정리하시겠습니까?')) {
            return;
        }
        
        try {
            const response = await window.apiService.delete(`/api/duplicates/group/${groupId}/quick-clean`);
            console.log('빠른 삭제 완료:', response);
            
            // 성공 알림
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: `${response.deletedCount || 0}개 파일이 삭제되었습니다.`
            });
            
            // 데이터 새로고침
            await this.loadRecentDuplicates();
            await this.loadStatistics();
            
        } catch (error) {
            console.error('빠른 삭제 실패:', error);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: '빠른 삭제에 실패했습니다: ' + error.message
            });
        }
    }

    viewGroup(groupId) {
        window.app.router.navigate(`/duplicates?group=${groupId}`);
    }

    startProgressMonitoring() {
        // 진행 상황 모니터링 시작
        if (this.progressInterval) {
            clearInterval(this.progressInterval);
        }
        
        this.progressInterval = setInterval(async () => {
            await this.checkRunningOperations();
        }, 2000); // 2초마다 체크
    }

    onDestroy() {
        // 인터벌 정리
        if (this.statusCheckInterval) {
            clearInterval(this.statusCheckInterval);
        }
        
        if (this.progressInterval) {
            clearInterval(this.progressInterval);
        }

        // 진행률 바 정리
        this.progressBars.forEach(progressBar => progressBar.destroy());
        this.progressBars.clear();

        super.onDestroy();
    }
}