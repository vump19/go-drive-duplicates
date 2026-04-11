import { Component } from './Component.js';
import { EVENTS } from '../../core/EventBus.js';

/**
 * 모달 다이얼로그 컴포넌트
 */
export class Modal extends Component {
    constructor(options = {}) {
        // 모달은 동적으로 생성되므로 element를 null로 초기화
        super(null);
        
        this.options = {
            title: '',
            content: '',
            type: 'default', // default, confirm, alert, custom
            size: 'medium', // small, medium, large, fullscreen
            closable: true,
            backdrop: true, // 배경 클릭으로 닫기
            keyboard: true, // ESC 키로 닫기
            animation: 'fade', // fade, slide, zoom
            buttons: [], // 커스텀 버튼들
            ...options
        };
        
        this.isOpen = false;
        this.focusedElementBeforeOpen = null;
        this.createModalElement();
    }

    /**
     * 모달 엘리먼트 생성
     */
    createModalElement() {
        this.element = document.createElement('div');
        this.element.className = `modal modal-${this.options.animation}`;
        this.element.setAttribute('role', 'dialog');
        this.element.setAttribute('aria-modal', 'true');
        this.element.setAttribute('tabindex', '-1');
        
        if (this.options.title) {
            this.element.setAttribute('aria-labelledby', `modal-title-${this.id}`);
        }
        
        document.body.appendChild(this.element);
        this.init();
    }

    template() {
        const sizeClass = `modal-${this.options.size}`;
        const typeClass = `modal-${this.options.type}`;
        
        return `
            <div class="modal-backdrop"></div>
            <div class="modal-container ${sizeClass} ${typeClass}">
                <div class="modal-content">
                    ${this.renderHeader()}
                    ${this.renderBody()}
                    ${this.renderFooter()}
                </div>
            </div>
        `;
    }

    renderHeader() {
        if (!this.options.title && !this.options.closable) {
            return '';
        }

        return `
            <div class="modal-header">
                ${this.options.title ? `<h3 class="modal-title" id="modal-title-${this.id}">${this.options.title}</h3>` : ''}
                ${this.options.closable ? '<button class="modal-close" aria-label="닫기"><i class="icon-close"></i></button>' : ''}
            </div>
        `;
    }

    renderBody() {
        return `
            <div class="modal-body">
                ${this.options.content}
            </div>
        `;
    }

    renderFooter() {
        const buttons = this.getButtons();
        
        if (buttons.length === 0) {
            return '';
        }

        return `
            <div class="modal-footer">
                ${buttons.map(button => this.renderButton(button)).join('')}
            </div>
        `;
    }

    renderButton(button) {
        const classes = ['btn', `btn-${button.type || 'secondary'}`];
        if (button.primary) classes.push('btn-primary');
        
        return `
            <button 
                class="${classes.join(' ')}" 
                data-action="${button.action}"
                ${button.disabled ? 'disabled' : ''}
            >
                ${button.icon ? `<i class="${button.icon}"></i>` : ''}
                ${button.text}
            </button>
        `;
    }

    getButtons() {
        // 커스텀 버튼이 있으면 사용
        if (this.options.buttons.length > 0) {
            return this.options.buttons;
        }

        // 타입별 기본 버튼
        switch (this.options.type) {
            case 'confirm':
                return [
                    { text: '취소', action: 'cancel', type: 'secondary' },
                    { text: '확인', action: 'confirm', type: 'primary', primary: true }
                ];
            case 'alert':
                return [
                    { text: '확인', action: 'ok', type: 'primary', primary: true }
                ];
            default:
                return [];
        }
    }

    bindEvents() {
        // 배경 클릭으로 닫기
        if (this.options.backdrop) {
            const backdrop = this.$('.modal-backdrop');
            const container = this.$('.modal-container');
            
            if (backdrop) {
                this.addEventListener(backdrop, 'click', this.handleBackdropClick);
            }
            
            if (container) {
                this.addEventListener(container, 'click', this.handleContainerClick);
            }
        }

        // 닫기 버튼
        const closeButton = this.$('.modal-close');
        if (closeButton) {
            this.addEventListener(closeButton, 'click', this.handleClose);
        }

        // 푸터 버튼들
        this.$$('.modal-footer .btn').forEach(button => {
            this.addEventListener(button, 'click', this.handleButtonClick);
        });

        // 키보드 이벤트
        if (this.options.keyboard) {
            this.addEventListener(document, 'keydown', this.handleKeydown);
        }

        // 포커스 트랩
        this.addEventListener(this.element, 'keydown', this.handleFocusTrap);
    }

    handleBackdropClick(event) {
        if (event.target === event.currentTarget) {
            this.close();
        }
    }

    handleContainerClick(event) {
        // 컨테이너 클릭 시 버블링 방지
        event.stopPropagation();
    }

    handleClose() {
        this.close();
    }

    handleButtonClick(event) {
        const button = event.currentTarget;
        const action = button.getAttribute('data-action');
        
        // 버튼 액션 이벤트 발생
        this.emit('modal:button-click', {
            action,
            modal: this,
            button
        });

        // 기본 액션 처리
        switch (action) {
            case 'cancel':
            case 'close':
                this.close();
                break;
            case 'confirm':
            case 'ok':
                this.confirm();
                break;
        }
    }

    handleKeydown(event) {
        if (event.key === 'Escape' && this.isOpen) {
            this.close();
        }
    }

    handleFocusTrap(event) {
        if (event.key !== 'Tab') return;

        const focusableElements = this.getFocusableElements();
        const firstElement = focusableElements[0];
        const lastElement = focusableElements[focusableElements.length - 1];

        if (event.shiftKey) {
            // Shift + Tab
            if (document.activeElement === firstElement) {
                event.preventDefault();
                lastElement.focus();
            }
        } else {
            // Tab
            if (document.activeElement === lastElement) {
                event.preventDefault();
                firstElement.focus();
            }
        }
    }

    getFocusableElements() {
        const selector = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
        return Array.from(this.element.querySelectorAll(selector))
            .filter(el => !el.disabled && !el.hidden);
    }

    /**
     * 모달 열기
     */
    open() {
        if (this.isOpen) return;

        // 현재 포커스된 요소 저장
        this.focusedElementBeforeOpen = document.activeElement;

        // 렌더링
        this.render();

        // body 스크롤 방지
        document.body.style.overflow = 'hidden';
        document.body.classList.add('modal-open');

        // 애니메이션 클래스 추가
        this.element.classList.add('modal-opening');
        
        // 애니메이션 완료 후 상태 변경
        setTimeout(() => {
            this.element.classList.remove('modal-opening');
            this.element.classList.add('modal-open');
            this.isOpen = true;
            
            // 첫 번째 포커스 가능한 요소에 포커스
            const focusableElements = this.getFocusableElements();
            if (focusableElements.length > 0) {
                focusableElements[0].focus();
            }
            
            // 열림 이벤트 발생
            this.emit('modal:open', this);
        }, 10);

        // 전역 모달 이벤트 발생
        this.emit(EVENTS.SHOW_MODAL, this);
    }

    /**
     * 모달 닫기
     */
    close() {
        if (!this.isOpen) return;

        // 닫힘 애니메이션
        this.element.classList.add('modal-closing');
        this.element.classList.remove('modal-open');

        setTimeout(() => {
            this.isOpen = false;
            
            // body 스크롤 복원
            document.body.style.overflow = '';
            document.body.classList.remove('modal-open');
            
            // 이전 포커스 복원
            if (this.focusedElementBeforeOpen) {
                this.focusedElementBeforeOpen.focus();
            }
            
            // 닫힘 이벤트 발생
            this.emit('modal:close', this);
            
            // 모달 제거
            this.destroy();
        }, 300); // 애니메이션 시간과 맞춤

        // 전역 모달 이벤트 발생
        this.emit(EVENTS.HIDE_MODAL, this);
    }

    /**
     * 확인 처리
     */
    confirm() {
        this.emit('modal:confirm', this);
        this.close();
    }

    /**
     * 콘텐츠 업데이트
     * @param {string} content - 새로운 콘텐츠
     */
    updateContent(content) {
        this.options.content = content;
        const modalBody = this.$('.modal-body');
        if (modalBody) {
            modalBody.innerHTML = content;
        }
    }

    /**
     * 제목 업데이트
     * @param {string} title - 새로운 제목
     */
    updateTitle(title) {
        this.options.title = title;
        const modalTitle = this.$('.modal-title');
        if (modalTitle) {
            modalTitle.textContent = title;
        }
    }

    /**
     * 버튼 활성화/비활성화
     * @param {string} action - 버튼 액션
     * @param {boolean} disabled - 비활성화 여부
     */
    setButtonDisabled(action, disabled) {
        const button = this.$(`[data-action="${action}"]`);
        if (button) {
            button.disabled = disabled;
        }
    }

    /**
     * 로딩 상태 설정
     * @param {boolean} loading - 로딩 상태
     */
    setLoading(loading) {
        if (loading) {
            this.addClass('modal-loading');
        } else {
            this.removeClass('modal-loading');
        }
    }

    destroy() {
        // body 클래스 정리
        document.body.classList.remove('modal-open');
        document.body.style.overflow = '';
        
        super.destroy();
    }
}

/**
 * 모달 팩토리 함수들
 */
export const ModalFactory = {
    /**
     * 알림 모달
     * @param {string} message - 메시지
     * @param {string} title - 제목
     */
    alert(message, title = '알림') {
        return new Modal({
            type: 'alert',
            title,
            content: `<p>${message}</p>`,
            size: 'small'
        });
    },

    /**
     * 확인 모달
     * @param {string} message - 메시지
     * @param {string} title - 제목
     */
    confirm(message, title = '확인') {
        return new Modal({
            type: 'confirm',
            title,
            content: `<p>${message}</p>`,
            size: 'small'
        });
    },

    /**
     * 커스텀 모달
     * @param {Object} options - 모달 옵션
     */
    custom(options) {
        return new Modal(options);
    },

    /**
     * 로딩 모달
     * @param {string} message - 로딩 메시지
     */
    loading(message = '처리 중...') {
        return new Modal({
            type: 'loading',
            content: `
                <div class="loading-modal">
                    <div class="spinner"></div>
                    <p>${message}</p>
                </div>
            `,
            size: 'small',
            closable: false,
            backdrop: false,
            keyboard: false
        });
    }
};