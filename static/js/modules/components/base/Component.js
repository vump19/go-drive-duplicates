import { eventBus } from '../../core/EventBus.js';
import { getState, watchState } from '../../core/StateManager.js';

/**
 * 베이스 컴포넌트 클래스 - 모든 컴포넌트의 기본 클래스
 */
export class Component {
    constructor(element = null) {
        this.element = element;
        this.children = new Map();
        this.eventListeners = [];
        this.stateWatchers = [];
        this.isDestroyed = false;
        this.props = {};
        this.state = {};
        
        // 고유 ID 생성
        this.id = this.constructor.name + '_' + Math.random().toString(36).substr(2, 9);
        
        this.init();
    }

    /**
     * 컴포넌트 초기화
     */
    init() {
        if (this.element) {
            this.element.setAttribute('data-component-id', this.id);
        }
        
        this.setupEventListeners();
        this.setupStateWatchers();
        this.onInit();
    }

    /**
     * 초기화 시 호출되는 메서드 (서브클래스에서 오버라이드)
     */
    onInit() {}

    /**
     * HTML 템플릿 반환 (서브클래스에서 구현)
     */
    template() {
        return '<div></div>';
    }

    /**
     * 컴포넌트 렌더링
     */
    render() {
        if (this.isDestroyed) {
            console.warn(`[${this.constructor.name}] 파괴된 컴포넌트를 렌더링하려고 시도함`);
            return;
        }

        const html = this.template();
        
        if (this.element) {
            this.element.innerHTML = html;
            this.afterRender();
        }

        return html;
    }

    /**
     * 렌더링 후 호출되는 메서드 (서브클래스에서 오버라이드)
     */
    afterRender() {
        this.bindEvents();
    }

    /**
     * 이벤트 바인딩
     */
    bindEvents() {
        // 서브클래스에서 구현
    }

    /**
     * 이벤트 리스너 설정
     */
    setupEventListeners() {
        // 서브클래스에서 구현
    }

    /**
     * 상태 워처 설정
     */
    setupStateWatchers() {
        // 서브클래스에서 구현
    }

    /**
     * DOM 이벤트 리스너 추가
     * @param {Element} element - 이벤트를 바인딩할 요소
     * @param {string} event - 이벤트 타입
     * @param {Function} handler - 이벤트 핸들러
     * @param {Object} options - 이벤트 옵션
     */
    addEventListener(element, event, handler, options = {}) {
        if (!element) return;

        const boundHandler = handler.bind(this);
        element.addEventListener(event, boundHandler, options);
        
        this.eventListeners.push({
            element,
            event,
            handler: boundHandler,
            options
        });
    }

    /**
     * 이벤트 버스 이벤트 리스너 추가
     * @param {string} event - 이벤트 이름
     * @param {Function} handler - 이벤트 핸들러
     * @param {Object} options - 옵션
     */
    onEvent(event, handler, options = {}) {
        const unsubscribe = eventBus.on(event, handler.bind(this), options);
        this.eventListeners.push({ unsubscribe });
        return unsubscribe;
    }

    /**
     * 이벤트 발생
     * @param {string} event - 이벤트 이름
     * @param {*} data - 이벤트 데이터
     */
    emit(event, data) {
        eventBus.emit(event, data);
    }

    /**
     * 상태 워처 추가
     * @param {string} key - 상태 키
     * @param {Function} callback - 콜백 함수
     */
    watch(key, callback) {
        const unwatch = watchState(key, callback.bind(this));
        this.stateWatchers.push(unwatch);
        return unwatch;
    }

    /**
     * 상태 값 가져오기
     * @param {string} key - 상태 키
     */
    getState(key) {
        return getState(key);
    }

    /**
     * 자식 컴포넌트 추가
     * @param {string} name - 자식 컴포넌트 이름
     * @param {Component} component - 자식 컴포넌트 인스턴스
     */
    addChild(name, component) {
        this.children.set(name, component);
        return component;
    }

    /**
     * 자식 컴포넌트 가져오기
     * @param {string} name - 자식 컴포넌트 이름
     */
    getChild(name) {
        return this.children.get(name);
    }

    /**
     * 자식 컴포넌트 제거
     * @param {string} name - 자식 컴포넌트 이름
     */
    removeChild(name) {
        const child = this.children.get(name);
        if (child) {
            child.destroy();
            this.children.delete(name);
        }
    }

    /**
     * 요소 선택
     * @param {string} selector - CSS 선택자
     * @returns {Element|null}
     */
    $(selector) {
        if (!this.element) return null;
        return this.element.querySelector(selector);
    }

    /**
     * 요소들 선택
     * @param {string} selector - CSS 선택자
     * @returns {NodeList}
     */
    $$(selector) {
        if (!this.element) return [];
        return this.element.querySelectorAll(selector);
    }

    /**
     * 클래스 추가
     * @param {string} className - 클래스 이름
     */
    addClass(className) {
        if (this.element) {
            this.element.classList.add(className);
        }
    }

    /**
     * 클래스 제거
     * @param {string} className - 클래스 이름
     */
    removeClass(className) {
        if (this.element) {
            this.element.classList.remove(className);
        }
    }

    /**
     * 클래스 토글
     * @param {string} className - 클래스 이름
     */
    toggleClass(className) {
        if (this.element) {
            this.element.classList.toggle(className);
        }
    }

    /**
     * 클래스 존재 확인
     * @param {string} className - 클래스 이름
     */
    hasClass(className) {
        return this.element ? this.element.classList.contains(className) : false;
    }

    /**
     * 컴포넌트 표시
     */
    show() {
        if (this.element) {
            this.element.style.display = '';
            this.removeClass('hidden');
        }
    }

    /**
     * 컴포넌트 숨김
     */
    hide() {
        if (this.element) {
            this.addClass('hidden');
        }
    }

    /**
     * 컴포넌트 표시/숨김 토글
     */
    toggle() {
        if (this.hasClass('hidden')) {
            this.show();
        } else {
            this.hide();
        }
    }

    /**
     * props 업데이트
     * @param {Object} newProps - 새로운 props
     */
    updateProps(newProps) {
        const oldProps = { ...this.props };
        this.props = { ...this.props, ...newProps };
        this.onPropsChange(this.props, oldProps);
    }

    /**
     * props 변경 시 호출되는 메서드 (서브클래스에서 오버라이드)
     * @param {Object} newProps - 새로운 props
     * @param {Object} oldProps - 이전 props
     */
    onPropsChange(newProps, oldProps) {
        // 기본적으로 다시 렌더링
        this.render();
    }

    /**
     * 내부 상태 업데이트
     * @param {Object} newState - 새로운 상태
     */
    setState(newState) {
        const oldState = { ...this.state };
        this.state = { ...this.state, ...newState };
        this.onStateChange(this.state, oldState);
    }

    /**
     * 내부 상태 변경 시 호출되는 메서드 (서브클래스에서 오버라이드)
     * @param {Object} newState - 새로운 상태
     * @param {Object} oldState - 이전 상태
     */
    onStateChange(newState, oldState) {
        // 기본적으로 다시 렌더링
        this.render();
    }

    /**
     * 컴포넌트 파괴
     */
    destroy() {
        if (this.isDestroyed) return;

        // 자식 컴포넌트들 파괴
        this.children.forEach(child => child.destroy());
        this.children.clear();

        // 이벤트 리스너 제거
        this.eventListeners.forEach(listener => {
            if (listener.unsubscribe) {
                listener.unsubscribe();
            } else if (listener.element && listener.handler) {
                listener.element.removeEventListener(
                    listener.event, 
                    listener.handler, 
                    listener.options
                );
            }
        });
        this.eventListeners = [];

        // 상태 워처 제거
        this.stateWatchers.forEach(unwatch => unwatch());
        this.stateWatchers = [];

        // 요소에서 제거
        if (this.element && this.element.parentNode) {
            this.element.parentNode.removeChild(this.element);
        }

        this.isDestroyed = true;
        this.onDestroy();
    }

    /**
     * 파괴 시 호출되는 메서드 (서브클래스에서 오버라이드)
     */
    onDestroy() {}

    /**
     * 컴포넌트 정보 반환
     */
    toString() {
        return `${this.constructor.name}(${this.id})`;
    }
}