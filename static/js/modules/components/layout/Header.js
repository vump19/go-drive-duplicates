import { Component } from '../base/Component.js';
import { getState, watchState } from '../../core/StateManager.js';

/**
 * 헤더 컴포넌트 - 애플리케이션 상단 헤더
 */
export class Header extends Component {
    constructor(element) {
        super(element);
        this.systemStatus = {
            server: 'unknown',
            database: 'unknown',
            storage: 'unknown'
        };
    }

    onInit() {
        this.setupStateWatchers();
        this.render();
    }

    setupStateWatchers() {
        // 시스템 상태 워치
        this.watch('system', (systemState) => {
            if (systemState) {
                this.systemStatus = {
                    server: systemState.server?.status || 'unknown',
                    database: systemState.database?.status || 'unknown',
                    storage: systemState.storage?.status || 'unknown'
                };
                this.updateStatusIndicators();
            }
        });

        // 테마 변경 워치
        this.watch('app.theme', (theme) => {
            this.updateThemeButton(theme);
        });
    }

    template() {
        return `
            <div class="header-content">
                <div class="header-left">
                    <h1 class="header-title">
                        <i class="icon-drive"></i>
                        Google Drive 중복 파일 검사기
                    </h1>
                    <span class="header-subtitle">Clean Architecture v2.0.0</span>
                </div>
                
                <div class="header-right">
                    <div class="system-status">
                        <div class="status-indicator" data-status="server">
                            <i class="icon-server"></i>
                            <span class="status-text">서버</span>
                            <span class="status-dot" title="서버 상태"></span>
                        </div>
                        <div class="status-indicator" data-status="database">
                            <i class="icon-database"></i>
                            <span class="status-text">DB</span>
                            <span class="status-dot" title="데이터베이스 상태"></span>
                        </div>
                        <div class="status-indicator" data-status="storage">
                            <i class="icon-cloud"></i>
                            <span class="status-text">Drive</span>
                            <span class="status-dot" title="Google Drive 상태"></span>
                        </div>
                    </div>
                    
                    <div class="header-actions">
                        <button class="btn btn-icon theme-toggle" title="테마 변경">
                            <i class="icon-theme"></i>
                        </button>
                        <button class="btn btn-icon refresh-btn" title="상태 새로고침">
                            <i class="icon-refresh"></i>
                        </button>
                        <button class="btn btn-icon help-btn" title="도움말">
                            <i class="icon-help"></i>
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    afterRender() {
        super.afterRender();
        this.updateStatusIndicators();
        this.updateThemeButton(getState('app.theme'));
    }

    bindEvents() {
        // 테마 토글 버튼
        const themeToggle = this.$('.theme-toggle');
        if (themeToggle) {
            this.addEventListener(themeToggle, 'click', this.handleThemeToggle);
        }

        // 새로고침 버튼
        const refreshBtn = this.$('.refresh-btn');
        if (refreshBtn) {
            this.addEventListener(refreshBtn, 'click', this.handleRefresh);
        }

        // 도움말 버튼
        const helpBtn = this.$('.help-btn');
        if (helpBtn) {
            this.addEventListener(helpBtn, 'click', this.handleHelp);
        }

        // 상태 인디케이터 클릭
        this.$$('.status-indicator').forEach(indicator => {
            this.addEventListener(indicator, 'click', this.handleStatusClick);
        });
    }

    handleThemeToggle() {
        const currentTheme = getState('app.theme');
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';
        
        this.emit('theme:change', newTheme);
        
        // CSS 클래스 변경
        document.documentElement.setAttribute('data-theme', newTheme);
        
        // localStorage에 테마 저장
        localStorage.setItem('theme', newTheme);
    }

    handleRefresh() {
        this.emit('system:refresh-status');
        
        // 새로고침 애니메이션
        const refreshBtn = this.$('.refresh-btn i');
        if (refreshBtn) {
            refreshBtn.style.transform = 'rotate(360deg)';
            refreshBtn.style.transition = 'transform 0.5s ease';
            
            setTimeout(() => {
                refreshBtn.style.transform = '';
                refreshBtn.style.transition = '';
            }, 500);
        }
    }

    handleHelp() {
        this.emit('ui:show-modal', {
            type: 'help',
            title: '도움말',
            content: this.getHelpContent()
        });
    }

    handleStatusClick(event) {
        const indicator = event.currentTarget;
        const statusType = indicator.getAttribute('data-status');
        const status = this.systemStatus[statusType];
        
        this.emit('ui:show-toast', {
            type: 'info',
            message: `${this.getStatusTypeLabel(statusType)} 상태: ${this.getStatusLabel(status)}`,
            duration: 3000
        });
    }

    updateStatusIndicators() {
        if (!this.systemStatus) return;
        
        Object.entries(this.systemStatus).forEach(([type, status]) => {
            const indicator = this.$(`[data-status="${type}"] .status-dot`);
            if (indicator) {
                // 기존 상태 클래스 제거
                indicator.classList.remove('status-healthy', 'status-unhealthy', 'status-unknown');
                
                // 새로운 상태 클래스 추가
                switch (status) {
                    case 'healthy':
                    case 'ok':
                        indicator.classList.add('status-healthy');
                        break;
                    case 'unhealthy':
                    case 'error':
                        indicator.classList.add('status-unhealthy');
                        break;
                    default:
                        indicator.classList.add('status-unknown');
                        break;
                }

                // 툴팁 업데이트
                indicator.title = `${this.getStatusTypeLabel(type)}: ${this.getStatusLabel(status)}`;
            }
        });
    }

    updateThemeButton(theme) {
        const themeButton = this.$('.theme-toggle i');
        if (themeButton) {
            themeButton.className = theme === 'dark' ? 'icon-sun' : 'icon-moon';
        }
    }

    getStatusTypeLabel(type) {
        const labels = {
            server: '서버',
            database: '데이터베이스',
            storage: 'Google Drive'
        };
        return labels[type] || type;
    }

    getStatusLabel(status) {
        const labels = {
            healthy: '정상',
            ok: '정상',
            unhealthy: '오류',
            error: '오류',
            unknown: '확인 중'
        };
        return labels[status] || status;
    }

    getHelpContent() {
        return `
            <div class="help-content">
                <h3>Google Drive 중복 파일 검사기</h3>
                <p>이 도구는 Google Drive에서 중복된 파일을 찾아 정리할 수 있도록 도와줍니다.</p>
                
                <h4>주요 기능</h4>
                <ul>
                    <li><strong>파일 스캔</strong>: Google Drive의 모든 파일을 스캔하여 메타데이터를 수집합니다</li>
                    <li><strong>중복 검색</strong>: SHA-256 해시를 사용하여 정확한 중복 파일을 찾습니다</li>
                    <li><strong>폴더 비교</strong>: 두 폴더 간의 중복 파일을 비교합니다</li>
                    <li><strong>파일 정리</strong>: 안전하게 중복 파일을 삭제하고 빈 폴더를 정리합니다</li>
                </ul>
                
                <h4>사용법</h4>
                <ol>
                    <li>먼저 '파일 스캔' 탭에서 전체 스캔을 실행하세요</li>
                    <li>스캔이 완료되면 해시 계산을 실행하세요</li>
                    <li>'중복 파일' 탭에서 중복 파일을 검색하고 확인하세요</li>
                    <li>'파일 정리' 탭에서 불필요한 파일을 삭제하세요</li>
                </ol>
                
                <h4>키보드 단축키</h4>
                <ul>
                    <li><kbd>1</kbd> - 대시보드</li>
                    <li><kbd>2</kbd> - 파일 스캔</li>
                    <li><kbd>3</kbd> - 중복 파일</li>
                    <li><kbd>4</kbd> - 파일 정리</li>
                    <li><kbd>5</kbd> - 설정</li>
                    <li><kbd>T</kbd> - 테마 변경</li>
                    <li><kbd>R</kbd> - 새로고침</li>
                </ul>
            </div>
        `;
    }

    /**
     * 시스템 상태 업데이트
     * @param {Object} status - 상태 객체
     */
    updateSystemStatus(status) {
        this.systemStatus = { ...this.systemStatus, ...status };
        this.updateStatusIndicators();
    }

    /**
     * 헤더 표시
     */
    show() {
        super.show();
        this.addClass('header-visible');
    }

    /**
     * 헤더 숨김
     */
    hide() {
        super.hide();
        this.removeClass('header-visible');
    }
}