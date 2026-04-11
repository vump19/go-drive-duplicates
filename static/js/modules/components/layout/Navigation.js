import { Component } from '../base/Component.js';
// import { router } from '../../core/Router.js'; // 전역 router 제거됨, window.router 사용
import { EVENTS } from '../../core/EventBus.js';

/**
 * 네비게이션 컴포넌트 - 탭 기반 네비게이션
 */
export class Navigation extends Component {
    constructor(element) {
        super(element);
        this.currentRoute = 'dashboard';
        this.routes = [
            { path: 'dashboard', name: '대시보드', icon: 'icon-dashboard', key: '1' },
            { path: 'scan', name: '파일 스캔', icon: 'icon-scan', key: '2' },
            { path: 'duplicates', name: '중복 파일', icon: 'icon-duplicate', key: '3' },
            { path: 'cleanup', name: '파일 정리', icon: 'icon-cleanup', key: '4' },
            { path: 'settings', name: '설정', icon: 'icon-settings', key: '5' }
        ];
        
        // 안전장치: routes가 제대로 초기화되지 않은 경우
        if (!this.routes) {
            this.routes = [];
        }
    }

    onInit() {
        this.setupEventListeners();
        this.setupKeyboardShortcuts();
        this.render();
    }

    setupEventListeners() {
        // 라우트 변경 이벤트 리스닝
        this.onEvent(EVENTS.ROUTE_CHANGE, this.handleRouteChange);
    }

    setupKeyboardShortcuts() {
        // 키보드 단축키 설정
        this.addEventListener(document, 'keydown', this.handleKeydown);
    }

    template() {
        if (!this.routes) {
            return '<div class="nav-container"><div class="nav-loading">Loading...</div></div>';
        }
        
        return `
            <div class="nav-container">
                <div class="nav-tabs">
                    ${this.routes.map(route => this.renderTab(route)).join('')}
                </div>
                <div class="nav-indicator"></div>
                ${this.renderProgressBar()}
            </div>
        `;
    }

    renderTab(route) {
        const isActive = this.currentRoute === route.path;
        const activeClass = isActive ? 'active' : '';
        
        return `
            <button 
                class="tab-button ${activeClass}" 
                data-route="${route.path}"
                title="${route.name} (${route.key})"
                aria-label="${route.name}"
                ${isActive ? 'aria-current="page"' : ''}
            >
                <i class="${route.icon}"></i>
                <span class="tab-text">${route.name}</span>
                <span class="tab-key">${route.key}</span>
                <div class="tab-progress" data-route="${route.path}"></div>
            </button>
        `;
    }

    renderProgressBar() {
        return `
            <div class="nav-progress">
                <div class="progress-track">
                    <div class="progress-fill" data-progress="0"></div>
                </div>
                <span class="progress-text">준비됨</span>
            </div>
        `;
    }

    afterRender() {
        super.afterRender();
        this.updateActiveTab();
        this.updateIndicator();
    }

    bindEvents() {
        // 탭 클릭 이벤트
        this.$$('.tab-button').forEach(button => {
            this.addEventListener(button, 'click', this.handleTabClick);
        });

        // 탭 호버 이벤트
        this.$$('.tab-button').forEach(button => {
            this.addEventListener(button, 'mouseenter', this.handleTabHover);
            this.addEventListener(button, 'mouseleave', this.handleTabLeave);
        });
    }

    handleTabClick(event) {
        const button = event.currentTarget;
        const route = button.getAttribute('data-route');
        
        if (route && route !== this.currentRoute) {
            // 클릭 애니메이션
            this.animateTabClick(button);
            
            // 라우트 변경
            window.router.navigate(`/${route}`);
        }
    }

    handleTabHover(event) {
        const button = event.currentTarget;
        const route = button.getAttribute('data-route');
        
        // 호버 효과
        this.emit('nav:tab-hover', { route, element: button });
    }

    handleTabLeave(event) {
        const button = event.currentTarget;
        const route = button.getAttribute('data-route');
        
        // 호버 해제 효과
        this.emit('nav:tab-leave', { route, element: button });
    }

    handleRouteChange(data) {
        const { to } = data;
        if (to && to.name) {
            this.setActiveTab(to.name);
        }
    }

    handleKeydown(event) {
        // 숫자 키 1-5로 탭 전환
        if (event.key >= '1' && event.key <= '5') {
            const routeIndex = parseInt(event.key) - 1;
            const route = this.routes[routeIndex];
            
            if (route && route.path !== this.currentRoute) {
                event.preventDefault();
                window.router.navigate(`/${route.path}`);
            }
        }
        
        // 좌우 화살표로 탭 전환
        if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
            const currentIndex = this.routes.findIndex(r => r.path === this.currentRoute);
            if (currentIndex !== -1) {
                let newIndex;
                if (event.key === 'ArrowLeft') {
                    newIndex = currentIndex > 0 ? currentIndex - 1 : this.routes.length - 1;
                } else {
                    newIndex = currentIndex < this.routes.length - 1 ? currentIndex + 1 : 0;
                }
                
                const newRoute = this.routes[newIndex];
                if (newRoute) {
                    event.preventDefault();
                    window.router.navigate(`/${newRoute.path}`);
                }
            }
        }
    }

    setActiveTab(routeName) {
        // 라우트 이름을 경로로 변환
        const routeMap = {
            'dashboard': 'dashboard',
            'fileScan': 'scan',
            'duplicates': 'duplicates',
            'cleanup': 'cleanup',
            'settings': 'settings'
        };
        
        const routePath = routeMap[routeName] || routeName;
        
        if (routePath !== this.currentRoute) {
            this.currentRoute = routePath;
            this.updateActiveTab();
            this.updateIndicator();
        }
    }

    updateActiveTab() {
        // 모든 탭에서 active 클래스 제거
        this.$$('.tab-button').forEach(button => {
            button.classList.remove('active');
            button.removeAttribute('aria-current');
        });

        // 현재 탭에 active 클래스 추가
        const activeButton = this.$(`[data-route="${this.currentRoute}"]`);
        if (activeButton) {
            activeButton.classList.add('active');
            activeButton.setAttribute('aria-current', 'page');
        }
    }

    updateIndicator() {
        const indicator = this.$('.nav-indicator');
        const activeButton = this.$(`[data-route="${this.currentRoute}"]`);
        
        if (indicator && activeButton) {
            const buttonRect = activeButton.getBoundingClientRect();
            const containerRect = this.element.getBoundingClientRect();
            
            const left = buttonRect.left - containerRect.left;
            const width = buttonRect.width;
            
            indicator.style.transform = `translateX(${left}px)`;
            indicator.style.width = `${width}px`;
        }
    }

    animateTabClick(button) {
        button.classList.add('tab-clicking');
        
        setTimeout(() => {
            button.classList.remove('tab-clicking');
        }, 150);
    }

    /**
     * 탭별 진행률 업데이트
     * @param {string} route - 라우트 경로
     * @param {number} progress - 진행률 (0-100)
     */
    updateTabProgress(route, progress) {
        const progressElement = this.$(`[data-route="${route}"] .tab-progress`);
        if (progressElement) {
            progressElement.style.width = `${Math.max(0, Math.min(100, progress))}%`;
            progressElement.style.opacity = progress > 0 ? '1' : '0';
        }
    }

    /**
     * 전체 진행률 업데이트
     * @param {number} progress - 진행률 (0-100)
     * @param {string} text - 진행률 텍스트
     */
    updateProgress(progress, text = '') {
        const progressFill = this.$('.progress-fill');
        const progressText = this.$('.progress-text');
        
        if (progressFill) {
            progressFill.style.width = `${Math.max(0, Math.min(100, progress))}%`;
            progressFill.setAttribute('data-progress', progress);
        }
        
        if (progressText && text) {
            progressText.textContent = text;
        }
    }

    /**
     * 탭 배지 표시
     * @param {string} route - 라우트 경로
     * @param {string|number} badge - 배지 내용
     */
    showTabBadge(route, badge) {
        const button = this.$(`[data-route="${route}"]`);
        if (button) {
            let badgeElement = button.querySelector('.tab-badge');
            if (!badgeElement) {
                badgeElement = document.createElement('span');
                badgeElement.className = 'tab-badge';
                button.appendChild(badgeElement);
            }
            badgeElement.textContent = badge;
            badgeElement.style.display = 'block';
        }
    }

    /**
     * 탭 배지 숨김
     * @param {string} route - 라우트 경로
     */
    hideTabBadge(route) {
        const button = this.$(`[data-route="${route}"]`);
        const badgeElement = button?.querySelector('.tab-badge');
        if (badgeElement) {
            badgeElement.style.display = 'none';
        }
    }

    /**
     * 탭 비활성화
     * @param {string} route - 라우트 경로
     * @param {boolean} disabled - 비활성화 여부
     */
    setTabDisabled(route, disabled) {
        const button = this.$(`[data-route="${route}"]`);
        if (button) {
            button.disabled = disabled;
            if (disabled) {
                button.classList.add('disabled');
            } else {
                button.classList.remove('disabled');
            }
        }
    }

    /**
     * 네비게이션 숨김/표시
     * @param {boolean} collapsed - 축소 여부
     */
    setCollapsed(collapsed) {
        if (collapsed) {
            this.addClass('nav-collapsed');
        } else {
            this.removeClass('nav-collapsed');
        }
    }

    onDestroy() {
        // 키보드 이벤트 리스너는 자동으로 정리됨 (Component.destroy에서 처리)
        super.onDestroy();
    }
}