import { eventBus, EVENTS } from './EventBus.js';
import { setState, getState } from './StateManager.js';

/**
 * SPA 라우터 - Hash 기반 라우팅 시스템
 */
export class Router {
    constructor() {
        this.routes = new Map();
        this.middlewares = [];
        this.currentRoute = null;
        this.params = {};
        this.query = {};
        this.debug = (typeof process !== 'undefined' && process?.env?.NODE_ENV === 'development') || 
                     (typeof window !== 'undefined' && window.isDevelopment);
        
        this.init();
    }

    /**
     * 라우터 초기화
     */
    init() {
        // 기본 라우트는 App에서 등록하므로 여기서는 제외
        // this.registerDefaultRoutes();
        
        // hashchange 이벤트 리스너 등록
        window.addEventListener('hashchange', () => this.handleRouteChange());
        window.addEventListener('load', () => this.handleRouteChange());
        
        if (this.debug) {
            console.log('[Router] 라우터 초기화 완료');
        }
    }

    /**
     * 라우터 시작 (초기 라우트 처리)
     */
    start() {
        this.handleRouteChange();
        
        if (this.debug) {
            console.log('[Router] 라우터 시작됨');
        }
    }

    /**
     * 기본 라우트들 등록
     */
    registerDefaultRoutes() {
        this.register('', { 
            name: 'dashboard', 
            redirect: '#/dashboard' 
        });
        
        this.register('dashboard', { 
            name: 'dashboard',
            title: '대시보드'
        });
        
        this.register('scan', { 
            name: 'fileScan',
            title: '파일 스캔'
        });
        
        this.register('duplicates', { 
            name: 'duplicates',
            title: '중복 파일'
        });
        
        this.register('cleanup', { 
            name: 'cleanup',
            title: '파일 정리'
        });
        
        this.register('settings', { 
            name: 'settings',
            title: '설정'
        });
    }

    /**
     * 라우트 등록
     * @param {string} path - 라우트 경로
     * @param {Object} config - 라우트 설정
     */
    register(path, config) {
        const route = {
            path: path,
            name: config.name,
            title: config.title || config.name,
            component: config.component || null,
            beforeEnter: config.beforeEnter || null,
            redirect: config.redirect || null,
            meta: config.meta || {}
        };

        this.routes.set(path, route);

        if (this.debug) {
            console.log(`[Router] 라우트 등록: ${path}`, route);
        }
    }

    /**
     * 미들웨어 추가
     * @param {Function} middleware - 미들웨어 함수
     */
    use(middleware) {
        this.middlewares.push(middleware);
    }

    /**
     * 라우트 변경 처리
     */
    async handleRouteChange() {
        const hash = window.location.hash.slice(1) || '/';
        const url = new URL(hash.startsWith('/') ? hash.slice(1) : hash, 'http://localhost');
        const path = url.pathname;
        
        // 쿼리 파라미터 파싱
        this.query = {};
        url.searchParams.forEach((value, key) => {
            this.query[key] = value;
        });

        // 라우트 매칭
        const route = this.matchRoute(path);
        if (!route) {
            console.error(`[Router] 라우트를 찾을 수 없음: ${path}`);
            console.log('[Router] 등록된 라우트:', Array.from(this.routes.keys()));
            
            // 무한 루프 방지: 이미 /dashboard로 리다이렉트 중이면 중지
            if (path === '/dashboard') {
                console.error('[Router] /dashboard 라우트가 등록되지 않음, 라우트 등록을 확인하세요');
                return;
            }
            
            this.navigate('/dashboard');
            return;
        }

        // 리다이렉트 처리
        if (route.redirect) {
            window.location.hash = route.redirect;
            return;
        }

        const previousRoute = this.currentRoute;

        // beforeEnter 가드 실행
        if (route.beforeEnter) {
            const result = await route.beforeEnter(route, previousRoute);
            if (result === false) {
                if (this.debug) {
                    console.log(`[Router] 라우트 변경 취소: ${path}`);
                }
                return;
            }
        }

        // 미들웨어 실행
        for (const middleware of this.middlewares) {
            const result = await middleware(route, previousRoute);
            if (result === false) {
                if (this.debug) {
                    console.log(`[Router] 미들웨어에 의해 라우트 변경 취소: ${path}`);
                }
                return;
            }
        }

        // before-change 이벤트 발생
        eventBus.emit(EVENTS.ROUTE_BEFORE_CHANGE, {
            from: previousRoute,
            to: route,
            path: path
        });

        // 현재 라우트 업데이트
        this.currentRoute = route;

        // 상태 업데이트
        setState('app.currentRoute', route.name);

        // 페이지 타이틀 변경
        document.title = route.title ? 
            `${route.title} - Google Drive 중복 파일 검사기` : 
            'Google Drive 중복 파일 검사기';

        // route-change 이벤트 발생
        eventBus.emit(EVENTS.ROUTE_CHANGE, {
            from: previousRoute,
            to: route,
            path: path,
            params: this.params,
            query: this.query
        });

        if (this.debug) {
            console.log(`[Router] 라우트 변경: ${previousRoute?.name || 'none'} -> ${route.name}`, {
                path,
                params: this.params,
                query: this.query
            });
        }
    }

    /**
     * 라우트 매칭
     * @param {string} path - 현재 경로
     * @returns {Object|null} 매칭된 라우트
     */
    matchRoute(path) {
        // 정확한 매치 시도
        if (this.routes.has(path)) {
            return this.routes.get(path);
        }

        // 파라미터가 있는 라우트 매칭 (미래 확장용)
        for (const [routePath, route] of this.routes) {
            if (routePath.includes(':')) {
                const match = this.matchParameterizedRoute(path, routePath);
                if (match) {
                    this.params = match.params;
                    return route;
                }
            }
        }

        return null;
    }

    /**
     * 파라미터가 있는 라우트 매칭
     * @param {string} path - 현재 경로
     * @param {string} routePath - 라우트 패턴
     * @returns {Object|null} 매칭 결과
     */
    matchParameterizedRoute(path, routePath) {
        const pathParts = path.split('/').filter(Boolean);
        const routeParts = routePath.split('/').filter(Boolean);

        if (pathParts.length !== routeParts.length) {
            return null;
        }

        const params = {};
        
        for (let i = 0; i < routeParts.length; i++) {
            const routePart = routeParts[i];
            const pathPart = pathParts[i];

            if (routePart.startsWith(':')) {
                // 파라미터
                const paramName = routePart.slice(1);
                params[paramName] = decodeURIComponent(pathPart);
            } else if (routePart !== pathPart) {
                // 정확히 매치되지 않음
                return null;
            }
        }

        return { params };
    }

    /**
     * 프로그래밍 방식으로 라우트 이동
     * @param {string} path - 이동할 경로
     * @param {Object} options - 옵션
     */
    navigate(path, options = {}) {
        if (options.replace) {
            window.location.replace(`#${path}`);
        } else {
            window.location.hash = path;
        }
    }

    /**
     * 이전 페이지로 이동
     */
    back() {
        window.history.back();
    }

    /**
     * 다음 페이지로 이동
     */
    forward() {
        window.history.forward();
    }

    /**
     * 현재 라우트 정보 반환
     */
    getCurrentRoute() {
        return {
            route: this.currentRoute,
            params: this.params,
            query: this.query,
            path: window.location.hash.slice(1)
        };
    }

    /**
     * 쿼리 파라미터 생성
     * @param {Object} params - 파라미터 객체
     * @returns {string} 쿼리 문자열
     */
    buildQuery(params) {
        const searchParams = new URLSearchParams();
        Object.entries(params).forEach(([key, value]) => {
            if (value !== null && value !== undefined) {
                searchParams.append(key, value);
            }
        });
        return searchParams.toString();
    }

    /**
     * 라우트 존재 확인
     * @param {string} path - 확인할 경로
     * @returns {boolean}
     */
    hasRoute(path) {
        return this.routes.has(path);
    }

    /**
     * 등록된 모든 라우트 반환
     */
    getRoutes() {
        return Array.from(this.routes.values());
    }

    /**
     * 라우트 제거
     * @param {string} path - 제거할 라우트 경로
     */
    unregister(path) {
        this.routes.delete(path);
        
        if (this.debug) {
            console.log(`[Router] 라우트 제거: ${path}`);
        }
    }

    /**
     * 라우터 정리
     */
    destroy() {
        window.removeEventListener('hashchange', this.handleRouteChange);
        window.removeEventListener('load', this.handleRouteChange);
        this.routes.clear();
        this.middlewares = [];
        
        if (this.debug) {
            console.log('[Router] 라우터 정리됨');
        }
    }
}

// 전역 라우터 인스턴스는 App.js에서 생성하므로 제거
// export const router = new Router();