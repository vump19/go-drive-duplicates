/**
 * 메인 애플리케이션 클래스
 * 전체 애플리케이션의 진입점이자 최상위 컨트롤러
 */

import { EventBus } from './core/EventBus.js';
import { StateManager } from './core/StateManager.js';
import { Router } from './core/Router.js';
import { Header } from './components/layout/Header.js';
import { Navigation } from './components/layout/Navigation.js';
import { Toast } from './components/base/Toast.js';

export class App {
    constructor() {
        // 핵심 서비스 초기화
        this.eventBus = new EventBus();
        this.stateManager = new StateManager();
        this.router = new Router();
        
        // 전역 접근을 위해 window에 등록
        window.eventBus = this.eventBus;
        window.stateManager = this.stateManager;
        window.router = this.router;
        
        // 컴포넌트 컨테이너
        this.components = new Map();
        
        // 애플리케이션 상태
        this.isInitialized = false;
        this.debugMode = false;
        
        // 이벤트 리스너 설정
        this.setupEventListeners();
        
        // 라우트 설정
        this.setupRoutes();
    }

    async init() {
        console.log('🚀 애플리케이션 초기화 시작');
        
        try {
            // 앱 컨테이너 확인
            this.appContainer = document.getElementById('app');
            if (!this.appContainer) {
                throw new Error('앱 컨테이너를 찾을 수 없습니다');
            }

            // API 서비스 초기화
            await this.initializeApiService();
            
            // 설정 로드
            await this.loadSettings();
            
            // 헤더 렌더링
            await this.renderHeader();
            
            // 네비게이션 렌더링
            await this.renderNavigation();
            
            // 메인 콘텐츠 컨테이너 생성
            this.mainContainer = document.createElement('main');
            this.mainContainer.className = 'app-main';
            this.appContainer.appendChild(this.mainContainer);
            
            // 라우터 시작
            this.router.start();
            
            // 주기적 업데이트 시작
            this.startPeriodicUpdates();
            
            // 시스템 상태 초기 확인
            await this.checkSystemHealth();
            
            this.isInitialized = true;
            this.eventBus.emit('app:initialized');
            
            console.log('✅ 애플리케이션 초기화 완료');
            
        } catch (error) {
            console.error('❌ 애플리케이션 초기화 실패:', error);
            this.showError('애플리케이션을 초기화할 수 없습니다', error.message);
            throw error;
        }
    }

    async initializeApiService() {
        // ConfigManager 초기화 시도
        let configManager = null;
        try {
            const { ConfigManager } = await import('./core/ConfigManager.js');
            configManager = new ConfigManager();
            await configManager.loadConfig();
        } catch (error) {
            console.warn('[App] ConfigManager 초기화 실패:', error.message);
        }
        
        // ApiService를 전역적으로 사용할 수 있도록 설정
        const { ApiService } = await import('./services/ApiService.js');
        this.apiService = new ApiService(configManager);
        window.apiService = this.apiService; // 전역 접근용
        
        console.log('[App] ApiService 초기화 완료:', this.apiService.baseURL);
    }

    async loadSettings() {
        try {
            const savedSettings = localStorage.getItem('appSettings');
            if (savedSettings) {
                const settings = JSON.parse(savedSettings);
                this.stateManager.set('settings', settings);
                
                // 테마 적용
                this.applyTheme(settings.theme || 'auto');
            }
        } catch (error) {
            console.warn('설정을 불러오는데 실패했습니다:', error);
        }
    }

    applyTheme(theme) {
        let actualTheme = theme;
        
        if (theme === 'auto') {
            actualTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }
        
        document.documentElement.setAttribute('data-theme', actualTheme);
        this.stateManager.set('currentTheme', actualTheme);
    }

    startPeriodicUpdates() {
        // 30초마다 시스템 상태 업데이트
        setInterval(() => {
            this.checkSystemHealth();
        }, 30000);

        // 진행 중인 작업이 있을 때 더 자주 업데이트
        setInterval(() => {
            this.updateActiveOperations();
        }, 5000);
    }

    async checkSystemHealth() {
        try {
            const [serverHealth, dbHealth, storageHealth] = await Promise.allSettled([
                this.apiService.healthCheck(),
                this.apiService.checkDatabase(),
                this.apiService.checkStorage()
            ]);

            const healthStatus = {
                server: serverHealth.status === 'fulfilled',
                database: dbHealth.status === 'fulfilled',
                storage: storageHealth.status === 'fulfilled',
                timestamp: new Date().toISOString()
            };

            this.stateManager.set('systemHealth', healthStatus);
            this.eventBus.emit('system:health-updated', healthStatus);

        } catch (error) {
            console.warn('시스템 상태 확인 실패:', error);
        }
    }

    async updateActiveOperations() {
        try {
            const progress = await this.apiService.getScanProgress();
            if (progress && progress.isRunning) {
                this.stateManager.set('scanProgress', progress);
                this.eventBus.emit('scan:progress-updated', progress);
            }
        } catch (error) {
            // 진행 중인 작업이 없는 경우는 정상
        }
    }

    setupEventListeners() {
        // 글로벌 이벤트 리스너
        this.eventBus.on('app:theme-changed', (theme) => {
            this.applyTheme(theme);
        });

        this.eventBus.on('app:show-notification', (data) => {
            Toast.show(data.message, data.type, data.duration);
        });

        this.eventBus.on('app:error', (error) => {
            console.error('애플리케이션 에러:', error);
            Toast.show('오류가 발생했습니다: ' + error.message, 'error');
        });

        // 시스템 테마 변경 감지
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            const settings = this.stateManager.get('settings') || {};
            if (settings.theme === 'auto') {
                this.applyTheme('auto');
            }
        });

        // 브라우저 이벤트
        window.addEventListener('beforeunload', (e) => {
            this.handleBeforeUnload(e);
        });

        window.addEventListener('online', () => {
            Toast.show('네트워크가 연결되었습니다', 'success');
            this.checkSystemHealth();
        });

        window.addEventListener('offline', () => {
            Toast.show('네트워크 연결이 끊어졌습니다', 'warning');
        });

        // 라우트 변경 이벤트 리스너 추가
        this.eventBus.on('route:change', async (data) => {
            console.log('[App] 라우트 변경됨:', data);
            await this.handleRouteChange(data);
        });
    }

    setupRoutes() {
        // 라우트 정의
        this.router.register('/', {
            name: 'dashboard',
            component: async () => {
                const { Dashboard } = await import('./pages/Dashboard.js');
                return new Dashboard();
            }
        });
        
        this.router.register('/dashboard', {
            name: 'dashboard',
            component: async () => {
                const { Dashboard } = await import('./pages/Dashboard.js');
                return new Dashboard();
            }
        });

        this.router.register('/scan', {
            name: 'scan',
            component: async () => {
                const { FileScan } = await import('./pages/FileScan.js');
                return new FileScan();
            }
        });

        this.router.register('/duplicates', {
            name: 'duplicates',
            component: async () => {
                const { Duplicates } = await import('./pages/Duplicates.js');
                return new Duplicates();
            }
        });

        this.router.register('/compare', {
            name: 'compare',
            component: async () => {
                const { FolderComparison } = await import('./pages/FolderComparison.js');
                return new FolderComparison();
            }
        });

        this.router.register('/cleanup', {
            name: 'cleanup',
            component: async () => {
                const { Cleanup } = await import('./pages/Cleanup.js');
                return new Cleanup();
            }
        });

        this.router.register('/settings', {
            name: 'settings',
            component: async () => {
                const { Settings } = await import('./pages/Settings.js');
                return new Settings();
            }
        });

        // 404 페이지
        this.router.register('*', {
            name: 'notfound',
            component: async () => {
            return {
                render: () => `
                    <div class="error-page">
                        <div class="error-icon">
                            <i class="fas fa-search"></i>
                        </div>
                        <h1 class="error-title">페이지를 찾을 수 없습니다</h1>
                        <p class="error-message">
                            요청하신 페이지가 존재하지 않습니다.
                        </p>
                        <button class="btn btn-primary" onclick="router.navigate('/')">
                            <i class="fas fa-home"></i>
                            홈으로 돌아가기
                        </button>
                    </div>
                `
            };
            }
        });
        
        console.log('[App] 라우트 등록 완료:', Array.from(this.router.routes.keys()));
    }

    async renderHeader() {
        // 헤더 DOM 요소 생성
        const headerElement = document.createElement('header');
        headerElement.className = 'app-header';
        
        const header = new Header(headerElement);
        header.render();
        
        this.appContainer.appendChild(headerElement);
        this.components.set('header', header);
    }

    async renderNavigation() {
        // 네비게이션 DOM 요소 생성
        const navElement = document.createElement('nav');
        navElement.className = 'app-navigation';
        
        const nav = new Navigation(navElement);
        nav.render();
        
        this.appContainer.appendChild(navElement);
        this.components.set('navigation', nav);
    }

    showError(title, message) {
        const errorHtml = `
            <div class="error-page">
                <div class="error-icon">
                    <i class="fas fa-exclamation-triangle"></i>
                </div>
                <h1 class="error-title">${title}</h1>
                <p class="error-message">${message}</p>
                <div class="error-actions">
                    <button class="btn btn-primary" onclick="window.location.reload()">
                        <i class="fas fa-sync-alt"></i>
                        페이지 새로고침
                    </button>
                    <button class="btn btn-secondary" onclick="router.navigate('/')">
                        <i class="fas fa-home"></i>
                        홈으로 돌아가기
                    </button>
                </div>
            </div>
        `;
        
        if (this.mainContainer) {
            this.mainContainer.innerHTML = errorHtml;
        } else {
            this.appContainer.innerHTML = errorHtml;
        }
    }

    handleBeforeUnload(event) {
        // 진행 중인 작업이 있는지 확인
        const scanProgress = this.stateManager.get('scanProgress');
        if (scanProgress && scanProgress.isRunning) {
            event.preventDefault();
            event.returnValue = '파일 스캔이 진행 중입니다. 페이지를 닫으시겠습니까?';
            return event.returnValue;
        }

        // 저장되지 않은 설정이 있는지 확인
        const hasUnsavedSettings = this.stateManager.get('hasUnsavedSettings');
        if (hasUnsavedSettings) {
            event.preventDefault();
            event.returnValue = '저장되지 않은 설정이 있습니다. 페이지를 닫으시겠습니까?';
            return event.returnValue;
        }
    }

    // 개발자 도구용 메서드들
    toggleDebugMode() {
        this.debugMode = !this.debugMode;
        
        if (this.debugMode) {
            console.log('🐛 디버그 모드 활성화');
            this.eventBus.on('*', (event, data) => {
                console.log(`[DEBUG] Event: ${event}`, data);
            });
        } else {
            console.log('🐛 디버그 모드 비활성화');
        }
        
        this.stateManager.set('debugMode', this.debugMode);
        Toast.show(`디버그 모드 ${this.debugMode ? '활성화' : '비활성화'}`, 'info');
    }

    getDebugInfo() {
        return {
            version: '2.0.0',
            isInitialized: this.isInitialized,
            debugMode: this.debugMode,
            currentRoute: window.location.hash,
            stateKeys: Object.keys(this.stateManager.state),
            componentCount: this.components.size,
            eventListeners: this.eventBus.listeners.size
        };
    }

    exportState() {
        const state = {
            settings: this.stateManager.get('settings'),
            currentTheme: this.stateManager.get('currentTheme'),
            timestamp: new Date().toISOString()
        };
        
        const blob = new Blob([JSON.stringify(state, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'app-state.json';
        a.click();
        URL.revokeObjectURL(url);
        
        Toast.show('애플리케이션 상태가 내보내기되었습니다', 'success');
    }

    /**
     * 라우트 변경 처리
     */
    async handleRouteChange(data) {
        try {
            console.log('[App] 라우트 변경 처리 시작:', data);
            
            const { to } = data;
            if (!to || !to.component) {
                console.error('[App] 라우트 컴포넌트가 없습니다:', to);
                return;
            }

            // 이전 컴포넌트 정리
            const appContainer = document.getElementById('app');
            const mainContent = appContainer.querySelector('.main-content');
            if (mainContent) {
                mainContent.innerHTML = '';
            } else {
                // 메인 콘텐츠 컨테이너가 없으면 생성
                const container = document.createElement('div');
                container.className = 'main-content app-main';
                appContainer.appendChild(container);
            }

            // 새 컴포넌트 로드 및 렌더링
            console.log('[App] 컴포넌트 로드 중...', to.name);
            const component = await to.component();
            
            if (component && component.render) {
                const content = component.render();
                document.querySelector('.main-content').innerHTML = content;
                
                // 컴포넌트 초기화
                if (component.init) {
                    await component.init();
                }
                
                console.log('[App] 컴포넌트 렌더링 완료:', to.name);
            } else {
                console.error('[App] 컴포넌트에 render 메서드가 없습니다:', component);
            }
            
        } catch (error) {
            console.error('[App] 라우트 변경 처리 실패:', error);
            this.showError('페이지 로드 실패', error.message);
        }
    }

    // 정리 메서드
    destroy() {
        // 컴포넌트 정리
        this.components.forEach(component => {
            if (component.destroy) {
                component.destroy();
            }
        });
        this.components.clear();

        // 이벤트 리스너 정리
        this.eventBus.removeAllListeners();

        // 라우터 정리
        this.router.stop();

        console.log('애플리케이션이 정리되었습니다');
    }
}