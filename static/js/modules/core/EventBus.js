/**
 * 이벤트 버스 - 컴포넌트 간 통신을 위한 중앙 이벤트 관리
 */
export class EventBus {
    constructor() {
        this.events = new Map();
        this.debug = (typeof process !== 'undefined' && process?.env?.NODE_ENV === 'development') || 
                     (typeof window !== 'undefined' && window.isDevelopment);
    }

    /**
     * 이벤트 리스너 등록
     * @param {string} event - 이벤트 이름
     * @param {Function} callback - 콜백 함수
     * @param {Object} options - 옵션 {once: boolean, priority: number}
     */
    on(event, callback, options = {}) {
        if (!this.events.has(event)) {
            this.events.set(event, []);
        }

        const listener = {
            callback,
            once: options.once || false,
            priority: options.priority || 0,
            id: Symbol('listener')
        };

        const listeners = this.events.get(event);
        listeners.push(listener);
        
        // 우선순위에 따라 정렬 (높은 숫자가 먼저 실행)
        listeners.sort((a, b) => b.priority - a.priority);

        if (this.debug) {
            console.log(`[EventBus] 이벤트 리스너 등록: ${event}`, { priority: listener.priority });
        }

        // 리스너 제거를 위한 함수 반환
        return () => this.off(event, listener.id);
    }

    /**
     * 일회성 이벤트 리스너 등록
     * @param {string} event - 이벤트 이름
     * @param {Function} callback - 콜백 함수
     */
    once(event, callback) {
        return this.on(event, callback, { once: true });
    }

    /**
     * 이벤트 리스너 제거
     * @param {string} event - 이벤트 이름
     * @param {Symbol} listenerId - 리스너 ID (optional)
     */
    off(event, listenerId = null) {
        if (!this.events.has(event)) return;

        const listeners = this.events.get(event);

        if (listenerId) {
            // 특정 리스너 제거
            const index = listeners.findIndex(l => l.id === listenerId);
            if (index !== -1) {
                listeners.splice(index, 1);
                if (this.debug) {
                    console.log(`[EventBus] 이벤트 리스너 제거: ${event}`);
                }
            }
        } else {
            // 모든 리스너 제거
            this.events.delete(event);
            if (this.debug) {
                console.log(`[EventBus] 모든 이벤트 리스너 제거: ${event}`);
            }
        }

        // 빈 이벤트 정리
        if (listeners && listeners.length === 0) {
            this.events.delete(event);
        }
    }

    /**
     * 이벤트 발생
     * @param {string} event - 이벤트 이름
     * @param {*} data - 이벤트 데이터
     */
    emit(event, data = null) {
        if (!this.events.has(event)) {
            if (this.debug) {
                console.log(`[EventBus] 이벤트 리스너 없음: ${event}`);
            }
            return;
        }

        const listeners = this.events.get(event);
        const toRemove = [];

        if (this.debug) {
            console.log(`[EventBus] 이벤트 발생: ${event}`, { 
                listenerCount: listeners.length, 
                data 
            });
        }

        listeners.forEach(listener => {
            try {
                listener.callback(data, event);
                
                // once 리스너는 실행 후 제거
                if (listener.once) {
                    toRemove.push(listener.id);
                }
            } catch (error) {
                console.error(`[EventBus] 이벤트 핸들러 오류 (${event}):`, error);
            }
        });

        // once 리스너들 제거
        toRemove.forEach(id => this.off(event, id));
    }

    /**
     * 비동기 이벤트 발생 (Promise 반환)
     * @param {string} event - 이벤트 이름
     * @param {*} data - 이벤트 데이터
     * @returns {Promise<Array>} 모든 핸들러의 결과
     */
    async emitAsync(event, data = null) {
        if (!this.events.has(event)) {
            return [];
        }

        const listeners = this.events.get(event);
        const toRemove = [];
        const promises = [];

        listeners.forEach(listener => {
            const result = listener.callback(data, event);
            
            if (result instanceof Promise) {
                promises.push(result);
            }
            
            if (listener.once) {
                toRemove.push(listener.id);
            }
        });

        // once 리스너들 제거
        toRemove.forEach(id => this.off(event, id));

        return Promise.all(promises);
    }

    /**
     * 모든 이벤트 리스너 제거
     */
    clear() {
        this.events.clear();
        if (this.debug) {
            console.log('[EventBus] 모든 이벤트 리스너 제거됨');
        }
    }

    /**
     * 등록된 이벤트 목록 반환
     */
    getEvents() {
        return Array.from(this.events.keys());
    }

    /**
     * 특정 이벤트의 리스너 수 반환
     */
    getListenerCount(event) {
        return this.events.has(event) ? this.events.get(event).length : 0;
    }
}

// 전역 이벤트 버스 인스턴스
export const eventBus = new EventBus();

// 자주 사용되는 이벤트 상수들
export const EVENTS = {
    // 라우팅 관련
    ROUTE_CHANGE: 'route:change',
    ROUTE_BEFORE_CHANGE: 'route:before-change',
    
    // 상태 관리
    STATE_CHANGE: 'state:change',
    STATE_RESET: 'state:reset',
    
    // UI 관련
    SHOW_LOADING: 'ui:show-loading',
    HIDE_LOADING: 'ui:hide-loading',
    SHOW_TOAST: 'ui:show-toast',
    SHOW_MODAL: 'ui:show-modal',
    HIDE_MODAL: 'ui:hide-modal',
    
    // API 관련
    API_REQUEST_START: 'api:request-start',
    API_REQUEST_END: 'api:request-end',
    API_REQUEST_ERROR: 'api:request-error',
    
    // 파일 관련
    FILE_SCAN_START: 'file:scan-start',
    FILE_SCAN_PROGRESS: 'file:scan-progress',
    FILE_SCAN_COMPLETE: 'file:scan-complete',
    
    // 중복 파일 관련
    DUPLICATE_SEARCH_START: 'duplicate:search-start',
    DUPLICATE_SEARCH_PROGRESS: 'duplicate:search-progress',
    DUPLICATE_SEARCH_COMPLETE: 'duplicate:search-complete',
    
    // 정리 관련
    CLEANUP_START: 'cleanup:start',
    CLEANUP_PROGRESS: 'cleanup:progress',
    CLEANUP_COMPLETE: 'cleanup:complete',
    
    // 시스템 관련
    SYSTEM_ERROR: 'system:error',
    SYSTEM_STATUS_CHANGE: 'system:status-change'
};