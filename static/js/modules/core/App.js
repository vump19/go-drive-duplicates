import { eventBus, EVENTS } from './EventBus.js';
import { stateManager, setState, getState } from './StateManager.js';
// import { router } from './Router.js'; // 전역 router 제거됨, window.router 사용

// 컴포넌트 imports
import { Header } from '../components/layout/Header.js';
import { Navigation } from '../components/layout/Navigation.js';

// 페이지 컴포넌트들 (동적 import로 변경 예정)
let Dashboard, FileScan, Duplicates, Cleanup, Settings;

/**
 * 메인 애플리케이션 클래스
 */
export class App {
    constructor() {
        this.components = new Map();
        this.currentPage = null;
        this.isInitialized = false;
        this.debug = (typeof process !== 'undefined' && process?.env?.NODE_ENV === 'development') || 
                     (typeof window !== 'undefined' && window.isDevelopment);
        
        // 애플리케이션 상태 초기화
        this.initializeState();
    }

    /**
     * 애플리케이션 상태 초기화
     */
    initializeState() {
        // 테마 복원
        const savedTheme = localStorage.getItem('theme') || 'light';
        setState('app.theme', savedTheme);
        document.documentElement.setAttribute('data-theme', savedTheme);

        // 설정 복원
        this.restoreSettings();
    }

    /**
     * 애플리케이션 초기화
     */
    async init() {
        if (this.isInitialized) {
            console.warn('[App] 이미 초기화된 애플리케이션입니다.');
            return;
        }

        try {
            if (this.debug) {
                console.log('[App] 애플리케이션 초기화 시작');
            }

            // 로딩 표시
            this.showInitialLoading();

            // 핵심 시스템 초기화
            await this.initializeCore();

            // 레이아웃 컴포넌트 초기화
            await this.initializeLayout();

            // 라우팅 시스템 초기화
            await this.initializeRouting();

            // 이벤트 리스너 설정
            this.setupEventListeners();

            // 서비스 초기화
            await this.initializeServices();

            // 키보드 단축키 설정
            this.setupKeyboardShortcuts();

            // 에러 핸들링 설정
            this.setupErrorHandling();

            // 초기화 완료
            this.isInitialized = true;
            setState('app.initialized', true);

            // 로딩 숨김
            this.hideInitialLoading();

            // 시스템 상태 확인
            this.checkSystemStatus();

            if (this.debug) {
                console.log('[App] 애플리케이션 초기화 완료');
            }

            // 초기화 완료 이벤트
            eventBus.emit('app:initialized');

        } catch (error) {
            console.error('[App] 애플리케이션 초기화 실패:', error);
            this.handleInitializationError(error);
        }
    }

    /**
     * 핵심 시스템 초기화
     */
    async initializeCore() {
        // 상태 관리자 미들웨어 추가
        stateManager.addMiddleware((key, value, oldValue) => {
            if (this.debug && key.startsWith('debug.')) {
                console.log(`[StateManager] Debug: ${key}`, { oldValue, newValue: value });
            }
            return value;
        });

        // 라우터 미들웨어 추가
        window.router.use(async (to, from) => {
            // 페이지 전환 시 로딩 표시
            setState('app.loading', true);
            return true;
        });
    }

    /**
     * 레이아웃 컴포넌트 초기화
     */
    async initializeLayout() {
        // 헤더 초기화
        const headerElement = document.querySelector('.header');
        if (headerElement) {
            const header = new Header(headerElement);
            this.components.set('header', header);
        }

        // 네비게이션 초기화
        const navElement = document.querySelector('.nav-tabs');
        if (navElement) {
            const navigation = new Navigation(navElement.parentElement);
            this.components.set('navigation', navigation);
        }
    }

    /**
     * 라우팅 시스템 초기화
     */
    async initializeRouting() {
        // 라우트 변경 이벤트 리스닝
        eventBus.on(EVENTS.ROUTE_CHANGE, this.handleRouteChange.bind(this));

        // 라우트별 가드 설정
        window.router.use(this.routeGuard.bind(this));

        // 현재 라우트로 초기 페이지 로드
        await this.loadCurrentPage();
    }

    /**
     * 라우트 가드
     * @param {Object} to - 이동할 라우트
     * @param {Object} from - 현재 라우트
     */
    async routeGuard(to, from) {
        // API 연결 상태 확인 (설정 페이지 제외)
        if (to.name !== 'settings') {
            const serverStatus = getState('system.server.status');
            if (serverStatus === 'unhealthy') {
                // 서버 연결 불가 시 경고 표시
                eventBus.emit(EVENTS.SHOW_TOAST, {
                    type: 'warning',
                    message: '서버에 연결할 수 없습니다. 일부 기능이 제한될 수 있습니다.'
                });
            }
        }

        return true;
    }

    /**
     * 라우트 변경 처리
     * @param {Object} data - 라우트 변경 데이터
     */
    async handleRouteChange(data) {
        const { to, from } = data;
        
        try {
            // 이전 페이지 정리
            if (this.currentPage) {
                await this.unloadPage(this.currentPage);
            }

            // 새 페이지 로드
            await this.loadPage(to.name);

            // 로딩 상태 해제
            setState('app.loading', false);

        } catch (error) {
            console.error('[App] 페이지 로드 실패:', error);
            setState('app.loading', false);
            
            eventBus.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: '페이지를 로드할 수 없습니다.'
            });
        }
    }

    /**
     * 현재 페이지 로드
     */
    async loadCurrentPage() {
        const currentRoute = window.router.getCurrentRoute();
        if (currentRoute.route) {
            await this.loadPage(currentRoute.route.name);
        }
    }

    /**
     * 페이지 로드
     * @param {string} pageName - 페이지 이름
     */
    async loadPage(pageName) {
        const pageContainer = document.getElementById(this.getPageContainerId(pageName));
        if (!pageContainer) {
            console.error(`[App] 페이지 컨테이너를 찾을 수 없음: ${pageName}`);
            return;
        }

        // 모든 페이지 숨김
        this.hideAllPages();

        // 동적 페이지 컴포넌트 로드
        const PageComponent = await this.loadPageComponent(pageName);
        if (PageComponent) {
            const pageInstance = new PageComponent(pageContainer);
            this.currentPage = {
                name: pageName,
                component: pageInstance,
                container: pageContainer
            };
        }

        // 페이지 표시
        pageContainer.classList.add('active');

        if (this.debug) {
            console.log(`[App] 페이지 로드됨: ${pageName}`);
        }
    }

    /**
     * 페이지 컴포넌트 동적 로드
     * @param {string} pageName - 페이지 이름
     */
    async loadPageComponent(pageName) {
        try {
            switch (pageName) {
                case 'dashboard':
                    if (!Dashboard) {
                        const module = await import('../pages/Dashboard.js');
                        Dashboard = module.Dashboard;
                    }
                    return Dashboard;

                case 'fileScan':
                    if (!FileScan) {
                        const module = await import('../pages/FileScan.js');
                        FileScan = module.FileScan;
                    }
                    return FileScan;

                case 'duplicates':
                    if (!Duplicates) {
                        const module = await import('../pages/Duplicates.js');
                        Duplicates = module.Duplicates;
                    }
                    return Duplicates;

                case 'cleanup':
                    if (!Cleanup) {
                        const module = await import('../pages/Cleanup.js');
                        Cleanup = module.Cleanup;
                    }
                    return Cleanup;

                case 'settings':
                    if (!Settings) {
                        const module = await import('../pages/Settings.js');
                        Settings = module.Settings;
                    }
                    return Settings;

                default:
                    console.warn(`[App] 알 수 없는 페이지: ${pageName}`);
                    return null;
            }
        } catch (error) {
            console.error(`[App] 페이지 컴포넌트 로드 실패 (${pageName}):`, error);
            return null;
        }
    }

    /**
     * 페이지 언로드
     * @param {Object} page - 페이지 객체
     */
    async unloadPage(page) {
        if (page && page.component) {
            // 컴포넌트 정리
            if (typeof page.component.destroy === 'function') {
                page.component.destroy();
            }
            
            // 컨테이너 숨김
            if (page.container) {
                page.container.classList.remove('active');
            }
        }
    }

    /**
     * 모든 페이지 숨김
     */
    hideAllPages() {
        document.querySelectorAll('.tab-content').forEach(page => {
            page.classList.remove('active');
        });
    }

    /**
     * 페이지 컨테이너 ID 반환
     * @param {string} pageName - 페이지 이름
     */
    getPageContainerId(pageName) {
        const containerMap = {
            'dashboard': 'dashboard',
            'fileScan': 'scan',
            'duplicates': 'duplicates',
            'cleanup': 'cleanup',
            'settings': 'settings'
        };
        return containerMap[pageName] || pageName;
    }

    /**
     * 이벤트 리스너 설정
     */
    setupEventListeners() {
        // 테마 변경
        eventBus.on('theme:change', (theme) => {
            setState('app.theme', theme);
            document.documentElement.setAttribute('data-theme', theme);
            localStorage.setItem('theme', theme);
        });

        // 시스템 상태 새로고침
        eventBus.on('system:refresh-status', () => {
            this.checkSystemStatus();
        });

        // 전역 에러 처리
        eventBus.on(EVENTS.SYSTEM_ERROR, this.handleSystemError.bind(this));
    }

    /**
     * 서비스 초기화
     */
    async initializeServices() {
        // API 서비스 초기화는 별도 모듈에서 처리
        // 여기서는 상태만 설정
        setState('app.services.api', 'initializing');
        
        // 잠시 후 초기화 완료로 설정 (실제로는 API 서비스가 설정)
        setTimeout(() => {
            setState('app.services.api', 'ready');
        }, 1000);
    }

    /**
     * 키보드 단축키 설정
     */
    setupKeyboardShortcuts() {
        document.addEventListener('keydown', (event) => {
            // Ctrl/Cmd + 조합 키들
            if (event.ctrlKey || event.metaKey) {
                switch (event.key) {
                    case 'r':
                        event.preventDefault();
                        this.checkSystemStatus();
                        break;
                    case 'h':
                        event.preventDefault();
                        eventBus.emit(EVENTS.SHOW_MODAL, {
                            type: 'help',
                            title: '키보드 단축키',
                            content: this.getKeyboardShortcuts()
                        });
                        break;
                }
            }

            // 단일 키들
            switch (event.key) {
                case 't':
                case 'T':
                    if (!event.ctrlKey && !event.metaKey && !event.altKey) {
                        event.preventDefault();
                        eventBus.emit('theme:change', 
                            getState('app.theme') === 'light' ? 'dark' : 'light'
                        );
                    }
                    break;
            }
        });
    }

    /**
     * 에러 핸들링 설정
     */
    setupErrorHandling() {
        // 전역 JavaScript 에러 캐치
        window.addEventListener('error', (event) => {
            this.handleGlobalError(event.error, 'JavaScript Error');
        });

        // Promise rejection 에러 캐치
        window.addEventListener('unhandledrejection', (event) => {
            this.handleGlobalError(event.reason, 'Unhandled Promise Rejection');
        });
    }

    /**
     * 시스템 상태 확인
     */
    async checkSystemStatus() {
        // 실제 구현에서는 API 서비스에서 처리
        // 여기서는 모의 상태 설정
        setState('system.server.status', 'healthy');
        setState('system.database.status', 'healthy');
        setState('system.storage.status', 'healthy');
    }

    /**
     * 설정 복원
     */
    restoreSettings() {
        try {
            const savedSettings = localStorage.getItem('app-settings');
            if (savedSettings) {
                const settings = JSON.parse(savedSettings);
                setState('settings', settings);
            }
        } catch (error) {
            console.error('[App] 설정 복원 실패:', error);
        }
    }

    /**
     * 초기 로딩 표시
     */
    showInitialLoading() {
        const loadingOverlay = document.getElementById('loading-overlay');
        if (loadingOverlay) {
            loadingOverlay.classList.remove('hidden');
            
            const loadingMessage = document.getElementById('loading-message');
            if (loadingMessage) {
                loadingMessage.textContent = '애플리케이션을 초기화하는 중...';
            }
        }
    }

    /**
     * 초기 로딩 숨김
     */
    hideInitialLoading() {
        const loadingOverlay = document.getElementById('loading-overlay');
        if (loadingOverlay) {
            loadingOverlay.classList.add('hidden');
        }
    }

    /**
     * 초기화 에러 처리
     * @param {Error} error - 에러 객체
     */
    handleInitializationError(error) {
        const loadingMessage = document.getElementById('loading-message');
        if (loadingMessage) {
            loadingMessage.textContent = '초기화 중 오류가 발생했습니다.';
        }

        // 에러 메시지 표시
        setTimeout(() => {
            alert('애플리케이션 초기화에 실패했습니다. 페이지를 새로고침해 주세요.');
        }, 1000);
    }

    /**
     * 전역 에러 처리
     * @param {Error} error - 에러 객체
     * @param {string} type - 에러 타입
     */
    handleGlobalError(error, type) {
        console.error(`[App] ${type}:`, error);
        
        eventBus.emit(EVENTS.SYSTEM_ERROR, {
            error,
            type,
            timestamp: new Date()
        });
    }

    /**
     * 시스템 에러 처리
     * @param {Object} data - 에러 데이터
     */
    handleSystemError(data) {
        const { error, type } = data;
        
        // 중요한 에러는 모달로 표시
        if (type === 'API Error' || type === 'JavaScript Error') {
            eventBus.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `시스템 오류가 발생했습니다: ${error.message}`,
                duration: 0 // 수동으로 닫기
            });
        }
    }

    /**
     * 키보드 단축키 목록 반환
     */
    getKeyboardShortcuts() {
        return `
            <div class="keyboard-shortcuts">
                <h4>네비게이션</h4>
                <ul>
                    <li><kbd>1</kbd> - 대시보드</li>
                    <li><kbd>2</kbd> - 파일 스캔</li>
                    <li><kbd>3</kbd> - 중복 파일</li>
                    <li><kbd>4</kbd> - 파일 정리</li>
                    <li><kbd>5</kbd> - 설정</li>
                    <li><kbd>←</kbd><kbd>→</kbd> - 탭 이동</li>
                </ul>
                
                <h4>기능</h4>
                <ul>
                    <li><kbd>T</kbd> - 테마 변경</li>
                    <li><kbd>Ctrl</kbd>+<kbd>R</kbd> - 상태 새로고침</li>
                    <li><kbd>Ctrl</kbd>+<kbd>H</kbd> - 도움말 표시</li>
                    <li><kbd>Esc</kbd> - 모달 닫기</li>
                </ul>
            </div>
        `;
    }

    /**
     * 애플리케이션 종료
     */
    destroy() {
        // 모든 컴포넌트 정리
        this.components.forEach(component => component.destroy());
        this.components.clear();

        // 현재 페이지 정리
        if (this.currentPage) {
            this.unloadPage(this.currentPage);
        }

        // 라우터 정리
        window.router.destroy();

        // 상태 저장
        stateManager.persist(['settings', 'app.theme']);

        this.isInitialized = false;
        
        if (this.debug) {
            console.log('[App] 애플리케이션 종료됨');
        }
    }
}

// 전역 앱 인스턴스
export const app = new App();