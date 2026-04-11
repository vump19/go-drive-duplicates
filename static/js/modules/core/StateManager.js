import { eventBus, EVENTS } from './EventBus.js';

/**
 * 상태 관리자 - 애플리케이션의 중앙 상태 관리
 */
export class StateManager {
    constructor() {
        this.state = new Map();
        this.watchers = new Map();
        this.middleware = [];
        this.debug = (typeof process !== 'undefined' && process?.env?.NODE_ENV === 'development') || 
                     (typeof window !== 'undefined' && window.isDevelopment);
        
        // 기본 상태 초기화
        this.initializeDefaultState();
    }

    /**
     * 기본 상태 초기화
     */
    initializeDefaultState() {
        this.state.set('app', {
            initialized: false,
            loading: false,
            currentRoute: 'dashboard',
            theme: 'light'
        });

        this.state.set('system', {
            server: { status: 'unknown', lastCheck: null },
            database: { status: 'unknown', lastCheck: null },
            storage: { status: 'unknown', lastCheck: null }
        });

        this.state.set('stats', {
            totalFiles: 0,
            duplicateGroups: 0,
            wastedSpace: 0,
            lastUpdated: null
        });

        this.state.set('fileScan', {
            isRunning: false,
            progress: null,
            lastScan: null
        });

        this.state.set('duplicates', {
            groups: [],
            currentPage: 1,
            pageSize: 20,
            totalGroups: 0,
            totalPages: 1,
            isSearching: false
        });

        this.state.set('cleanup', {
            isRunning: false,
            progress: null,
            lastCleanup: null
        });

        this.state.set('ui', {
            modals: [],
            toasts: [],
            sidebarOpen: false
        });
    }

    /**
     * 상태 값 가져오기
     * @param {string} key - 상태 키 (예: 'app.currentRoute')
     * @returns {*} 상태 값
     */
    get(key) {
        const keys = key.split('.');
        let value = this.state.get(keys[0]);

        for (let i = 1; i < keys.length && value !== undefined; i++) {
            value = value[keys[i]];
        }

        return value;
    }

    /**
     * 상태 값 설정
     * @param {string} key - 상태 키
     * @param {*} value - 설정할 값
     * @param {boolean} silent - 이벤트 발생 억제
     */
    set(key, value, silent = false) {
        const keys = key.split('.');
        const namespace = keys[0];
        
        if (!this.state.has(namespace)) {
            this.state.set(namespace, {});
        }

        const oldValue = this.get(key);
        
        // 미들웨어 실행
        for (const middleware of this.middleware) {
            const result = middleware(key, value, oldValue);
            if (result !== undefined) {
                value = result;
            }
        }

        // 중첩된 키 설정
        if (keys.length === 1) {
            this.state.set(namespace, value);
        } else {
            const stateObj = this.state.get(namespace);
            let current = stateObj;
            
            for (let i = 1; i < keys.length - 1; i++) {
                if (!current[keys[i]]) {
                    current[keys[i]] = {};
                }
                current = current[keys[i]];
            }
            
            current[keys[keys.length - 1]] = value;
        }

        if (this.debug) {
            console.log(`[StateManager] 상태 변경: ${key}`, { oldValue, newValue: value });
        }

        // 이벤트 발생 및 워처 실행
        if (!silent) {
            this.notifyWatchers(key, value, oldValue);
            eventBus.emit(EVENTS.STATE_CHANGE, { key, value, oldValue });
        }
    }

    /**
     * 상태 업데이트 (부분 업데이트)
     * @param {string} key - 상태 키
     * @param {Object} updates - 업데이트할 속성들
     */
    update(key, updates) {
        const current = this.get(key);
        if (typeof current === 'object' && current !== null) {
            this.set(key, { ...current, ...updates });
        } else {
            this.set(key, updates);
        }
    }

    /**
     * 상태 워처 등록
     * @param {string} key - 감시할 상태 키
     * @param {Function} callback - 변경 시 실행할 콜백
     * @returns {Function} 워처 제거 함수
     */
    watch(key, callback) {
        if (!this.watchers.has(key)) {
            this.watchers.set(key, []);
        }
        
        const watcherId = Symbol('watcher');
        this.watchers.get(key).push({ id: watcherId, callback });

        if (this.debug) {
            console.log(`[StateManager] 워처 등록: ${key}`);
        }

        // 워처 제거 함수 반환
        return () => this.unwatch(key, watcherId);
    }

    /**
     * 상태 워처 제거
     * @param {string} key - 상태 키
     * @param {Symbol} watcherId - 워처 ID
     */
    unwatch(key, watcherId) {
        if (this.watchers.has(key)) {
            const watchers = this.watchers.get(key);
            const index = watchers.findIndex(w => w.id === watcherId);
            if (index !== -1) {
                watchers.splice(index, 1);
                if (watchers.length === 0) {
                    this.watchers.delete(key);
                }
            }
        }
    }

    /**
     * 워처들에게 변경 알림
     * @param {string} key - 변경된 키
     * @param {*} newValue - 새 값
     * @param {*} oldValue - 이전 값
     */
    notifyWatchers(key, newValue, oldValue) {
        // 정확한 키 매치
        if (this.watchers.has(key)) {
            this.watchers.get(key).forEach(watcher => {
                try {
                    watcher.callback(newValue, oldValue, key);
                } catch (error) {
                    console.error(`[StateManager] 워처 실행 오류 (${key}):`, error);
                }
            });
        }

        // 부모 키들도 확인 (예: 'app.loading' 변경 시 'app' 워처들도 실행)
        const keyParts = key.split('.');
        for (let i = keyParts.length - 1; i > 0; i--) {
            const parentKey = keyParts.slice(0, i).join('.');
            if (this.watchers.has(parentKey)) {
                const parentValue = this.get(parentKey);
                this.watchers.get(parentKey).forEach(watcher => {
                    try {
                        watcher.callback(parentValue, parentValue, parentKey);
                    } catch (error) {
                        console.error(`[StateManager] 부모 워처 실행 오류 (${parentKey}):`, error);
                    }
                });
            }
        }
    }

    /**
     * 미들웨어 추가
     * @param {Function} middleware - 미들웨어 함수
     */
    addMiddleware(middleware) {
        this.middleware.push(middleware);
    }

    /**
     * 전체 상태 리셋
     */
    reset() {
        this.state.clear();
        this.watchers.clear();
        this.initializeDefaultState();
        
        if (this.debug) {
            console.log('[StateManager] 상태 리셋됨');
        }
        
        eventBus.emit(EVENTS.STATE_RESET);
    }

    /**
     * 상태를 localStorage에 저장
     * @param {Array<string>} keys - 저장할 상태 키들 (기본: 모든 상태)
     */
    persist(keys = null) {
        try {
            const stateToPersist = {};
            const keysToSave = keys || Array.from(this.state.keys());
            
            keysToSave.forEach(key => {
                if (this.state.has(key)) {
                    stateToPersist[key] = this.state.get(key);
                }
            });

            localStorage.setItem('app-state', JSON.stringify(stateToPersist));
            
            if (this.debug) {
                console.log('[StateManager] 상태 저장됨:', keysToSave);
            }
        } catch (error) {
            console.error('[StateManager] 상태 저장 실패:', error);
        }
    }

    /**
     * localStorage에서 상태 복원
     * @param {Array<string>} keys - 복원할 상태 키들 (기본: 모든 저장된 상태)
     */
    restore(keys = null) {
        try {
            const savedState = localStorage.getItem('app-state');
            if (!savedState) return;

            const parsedState = JSON.parse(savedState);
            const keysToRestore = keys || Object.keys(parsedState);
            
            keysToRestore.forEach(key => {
                if (parsedState[key] !== undefined) {
                    this.state.set(key, parsedState[key]);
                }
            });

            if (this.debug) {
                console.log('[StateManager] 상태 복원됨:', keysToRestore);
            }
        } catch (error) {
            console.error('[StateManager] 상태 복원 실패:', error);
        }
    }

    /**
     * 현재 상태를 JSON으로 직렬화
     */
    serialize() {
        const serialized = {};
        this.state.forEach((value, key) => {
            serialized[key] = value;
        });
        return JSON.stringify(serialized);
    }

    /**
     * 상태 스냅샷 생성
     */
    createSnapshot() {
        const snapshot = new Map();
        this.state.forEach((value, key) => {
            snapshot.set(key, JSON.parse(JSON.stringify(value)));
        });
        return snapshot;
    }

    /**
     * 스냅샷에서 상태 복원
     */
    restoreSnapshot(snapshot) {
        this.state = snapshot;
        eventBus.emit(EVENTS.STATE_CHANGE, { type: 'snapshot-restore' });
    }
}

// 전역 상태 관리자 인스턴스
export const stateManager = new StateManager();

// 편의 함수들
export const getState = (key) => stateManager.get(key);
export const setState = (key, value, silent = false) => stateManager.set(key, value, silent);
export const updateState = (key, updates) => stateManager.update(key, updates);
export const watchState = (key, callback) => stateManager.watch(key, callback);