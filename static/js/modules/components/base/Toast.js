import { Component } from './Component.js';
import { EVENTS } from '../../core/EventBus.js';

/**
 * 토스트 알림 컴포넌트
 */
export class Toast extends Component {
    constructor(options = {}) {
        super(null);
        
        this.options = {
            message: '',
            type: 'info', // success, error, warning, info
            duration: 5000, // 0이면 자동으로 사라지지 않음
            position: 'top-right', // top-right, top-left, bottom-right, bottom-left, top-center, bottom-center
            closable: true,
            persistent: false, // 새로고침 후에도 유지
            icon: null, // 커스텀 아이콘
            action: null, // 액션 버튼 { text: '버튼텍스트', handler: function }
            ...options
        };
        
        this.isVisible = false;
        this.timeoutId = null;
        this.createToastElement();
    }

    /**
     * 토스트 엘리먼트 생성
     */
    createToastElement() {
        this.element = document.createElement('div');
        this.element.className = `toast toast-${this.options.type}`;
        this.element.setAttribute('role', 'alert');
        this.element.setAttribute('aria-live', 'polite');
        
        // 토스트 컨테이너 확인 및 생성
        let container = document.querySelector('.toast-container');
        if (!container) {
            container = document.createElement('div');
            container.className = 'toast-container';
            document.body.appendChild(container);
        }
        
        // 위치별 컨테이너 확인
        const positionClass = `toast-${this.options.position}`;
        let positionContainer = container.querySelector(`.${positionClass}`);
        if (!positionContainer) {
            positionContainer = document.createElement('div');
            positionContainer.className = positionClass;
            container.appendChild(positionContainer);
        }
        
        positionContainer.appendChild(this.element);
        this.init();
    }

    template() {
        const icon = this.getIcon();
        
        return `
            <div class="toast-content">
                ${icon ? `<div class="toast-icon">${icon}</div>` : ''}
                <div class="toast-body">
                    <div class="toast-message">${this.options.message}</div>
                    ${this.options.action ? this.renderAction() : ''}
                </div>
                ${this.options.closable ? '<button class="toast-close" aria-label="닫기"><i class="icon-close"></i></button>' : ''}
            </div>
            ${this.options.duration > 0 ? '<div class="toast-progress"></div>' : ''}
        `;
    }

    renderAction() {
        return `
            <button class="toast-action" data-action="custom">
                ${this.options.action.text}
            </button>
        `;
    }

    getIcon() {
        if (this.options.icon) {
            return `<i class="${this.options.icon}"></i>`;
        }

        const icons = {
            success: '<i class="icon-check-circle"></i>',
            error: '<i class="icon-error-circle"></i>',
            warning: '<i class="icon-warning-circle"></i>',
            info: '<i class="icon-info-circle"></i>'
        };

        return icons[this.options.type] || '';
    }

    bindEvents() {
        // 닫기 버튼
        const closeButton = this.$('.toast-close');
        if (closeButton) {
            this.addEventListener(closeButton, 'click', this.handleClose);
        }

        // 액션 버튼
        const actionButton = this.$('.toast-action');
        if (actionButton) {
            this.addEventListener(actionButton, 'click', this.handleAction);
        }

        // 호버 시 자동 닫힘 일시정지
        if (this.options.duration > 0) {
            this.addEventListener(this.element, 'mouseenter', this.pauseTimer);
            this.addEventListener(this.element, 'mouseleave', this.resumeTimer);
        }

        // 클릭으로 닫기 (액션 버튼이 없는 경우)
        if (!this.options.action && this.options.closable) {
            this.addEventListener(this.element, 'click', this.handleClose);
        }
    }

    /**
     * 토스트 표시
     */
    show() {
        if (this.isVisible) return;

        this.render();
        this.isVisible = true;

        // 입장 애니메이션
        requestAnimationFrame(() => {
            this.element.classList.add('toast-entering');
            
            setTimeout(() => {
                this.element.classList.remove('toast-entering');
                this.element.classList.add('toast-visible');
                
                // 표시 이벤트 발생
                this.emit('toast:show', this);
                
                // 자동 닫힘 타이머 시작
                if (this.options.duration > 0) {
                    this.startTimer();
                }
            }, 10);
        });

        // 전역 토스트 이벤트 발생
        this.emit(EVENTS.SHOW_TOAST, this);
    }

    /**
     * 토스트 숨김
     */
    hide() {
        if (!this.isVisible) return;

        this.clearTimer();
        
        // 퇴장 애니메이션
        this.element.classList.add('toast-leaving');
        this.element.classList.remove('toast-visible');

        setTimeout(() => {
            this.isVisible = false;
            
            // 숨김 이벤트 발생
            this.emit('toast:hide', this);
            
            // 토스트 제거
            this.destroy();
        }, 300); // 애니메이션 시간과 맞춤
    }

    /**
     * 자동 닫힘 타이머 시작
     */
    startTimer() {
        if (this.options.duration <= 0) return;

        this.startTime = Date.now();
        this.remainingTime = this.options.duration;
        
        this.timeoutId = setTimeout(() => {
            this.hide();
        }, this.remainingTime);

        // 진행률 바 애니메이션
        this.updateProgress();
    }

    /**
     * 타이머 일시정지
     */
    pauseTimer() {
        if (this.timeoutId) {
            clearTimeout(this.timeoutId);
            this.remainingTime -= Date.now() - this.startTime;
            
            // 진행률 바 일시정지
            const progressBar = this.$('.toast-progress');
            if (progressBar) {
                progressBar.style.animationPlayState = 'paused';
            }
        }
    }

    /**
     * 타이머 재개
     */
    resumeTimer() {
        if (this.remainingTime > 0) {
            this.startTime = Date.now();
            this.timeoutId = setTimeout(() => {
                this.hide();
            }, this.remainingTime);
            
            // 진행률 바 재개
            const progressBar = this.$('.toast-progress');
            if (progressBar) {
                progressBar.style.animationPlayState = 'running';
            }
        }
    }

    /**
     * 타이머 정리
     */
    clearTimer() {
        if (this.timeoutId) {
            clearTimeout(this.timeoutId);
            this.timeoutId = null;
        }
    }

    /**
     * 진행률 바 업데이트
     */
    updateProgress() {
        const progressBar = this.$('.toast-progress');
        if (progressBar) {
            progressBar.style.animation = `toast-progress ${this.options.duration}ms linear`;
        }
    }

    handleClose() {
        this.hide();
    }

    handleAction() {
        if (this.options.action && typeof this.options.action.handler === 'function') {
            this.options.action.handler(this);
        }
        
        this.emit('toast:action', this);
        
        // 액션 후 자동으로 닫기
        this.hide();
    }

    /**
     * 메시지 업데이트
     * @param {string} message - 새로운 메시지
     */
    updateMessage(message) {
        this.options.message = message;
        const messageElement = this.$('.toast-message');
        if (messageElement) {
            messageElement.innerHTML = message;
        }
    }

    /**
     * 타입 변경
     * @param {string} type - 새로운 타입
     */
    updateType(type) {
        // 기존 타입 클래스 제거
        this.element.classList.remove(`toast-${this.options.type}`);
        
        // 새로운 타입 적용
        this.options.type = type;
        this.element.classList.add(`toast-${type}`);
        
        // 아이콘 업데이트
        const iconElement = this.$('.toast-icon');
        if (iconElement) {
            iconElement.innerHTML = this.getIcon();
        }
    }

    /**
     * 지속적인 토스트로 변경
     */
    makePersistent() {
        this.options.duration = 0;
        this.clearTimer();
        
        const progressBar = this.$('.toast-progress');
        if (progressBar) {
            progressBar.style.display = 'none';
        }
    }

    destroy() {
        this.clearTimer();
        super.destroy();
    }
}

/**
 * 토스트 관리자
 */
export class ToastManager {
    constructor() {
        this.toasts = new Map();
        this.maxToasts = 5; // 최대 동시 표시 개수
        this.defaultOptions = {
            duration: 5000,
            position: 'top-right',
            closable: true
        };
    }

    /**
     * 토스트 표시
     * @param {Object} options - 토스트 옵션
     * @returns {Toast} 생성된 토스트 인스턴스
     */
    show(options) {
        const toastOptions = { ...this.defaultOptions, ...options };
        const toast = new Toast(toastOptions);
        
        // 최대 개수 초과 시 가장 오래된 토스트 제거
        if (this.toasts.size >= this.maxToasts) {
            const oldestToast = this.toasts.values().next().value;
            oldestToast.hide();
        }
        
        this.toasts.set(toast.id, toast);
        
        // 토스트 제거 시 맵에서도 제거
        toast.onEvent('toast:hide', () => {
            this.toasts.delete(toast.id);
        });
        
        toast.show();
        return toast;
    }

    /**
     * 성공 토스트
     * @param {string} message - 메시지
     * @param {Object} options - 추가 옵션
     */
    success(message, options = {}) {
        return this.show({
            message,
            type: 'success',
            ...options
        });
    }

    /**
     * 오류 토스트
     * @param {string} message - 메시지
     * @param {Object} options - 추가 옵션
     */
    error(message, options = {}) {
        return this.show({
            message,
            type: 'error',
            duration: 0, // 오류는 수동으로 닫기
            ...options
        });
    }

    /**
     * 경고 토스트
     * @param {string} message - 메시지
     * @param {Object} options - 추가 옵션
     */
    warning(message, options = {}) {
        return this.show({
            message,
            type: 'warning',
            duration: 7000, // 경고는 조금 더 오래 표시
            ...options
        });
    }

    /**
     * 정보 토스트
     * @param {string} message - 메시지
     * @param {Object} options - 추가 옵션
     */
    info(message, options = {}) {
        return this.show({
            message,
            type: 'info',
            ...options
        });
    }

    /**
     * 모든 토스트 닫기
     */
    clear() {
        this.toasts.forEach(toast => toast.hide());
        this.toasts.clear();
    }

    /**
     * 특정 타입의 토스트들 닫기
     * @param {string} type - 토스트 타입
     */
    clearByType(type) {
        this.toasts.forEach(toast => {
            if (toast.options.type === type) {
                toast.hide();
            }
        });
    }
}

// 전역 토스트 관리자 인스턴스
export const toastManager = new ToastManager();