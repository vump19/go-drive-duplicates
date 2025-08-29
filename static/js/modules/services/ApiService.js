/**
 * API 서비스
 * 백엔드 API와의 통신을 담당하는 중앙화된 서비스
 */

export class ApiService {
    constructor(configManager = null) {
        this.configManager = configManager;
        this.baseURL = this.getBaseURL();
        this.defaultTimeout = 30000; // 30초
        this.retryAttempts = 3;
        this.retryDelay = 1000; // 1초
        
        console.log(`🔗 API 서비스 초기화: ${this.baseURL}`);
    }

    /**
     * 설정에 따른 기본 URL 결정
     */
    getBaseURL() {
        // 설정 매니저가 있고 설정이 로드된 경우 사용
        if (this.configManager && this.configManager.config) {
            const url = this.configManager.getBackendURL();
            console.log('[ApiService] ConfigManager에서 URL 사용:', url);
            return url;
        }
        
        // 설정 파일에서 직접 백엔드 포트 가져오기 시도
        try {
            const config = this.loadConfigFromFile();
            if (config && config.backend) {
                const protocol = config.backend.protocol || 'http';
                const host = config.backend.host || 'localhost';
                const port = config.backend.port || 9090;
                const url = `${protocol}://${host}:${port}`;
                console.log('[ApiService] 설정 파일에서 URL 생성:', url);
                return url;
            }
        } catch (error) {
            console.warn('[ApiService] 설정 파일 로드 실패, 기본값 사용:', error.message);
        }
        
        // 설정이 없는 경우 환경에 따른 기본 URL 사용
        if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
            const fallbackUrl = 'http://localhost:9090';
            console.log('[ApiService] 기본 URL 사용:', fallbackUrl);
            return fallbackUrl;
        }
        
        // 프로덕션 환경에서는 현재 도메인 사용
        return location.origin;
    }
    
    /**
     * 설정 파일에서 백엔드 설정 로드
     */
    loadConfigFromFile() {
        // localStorage나 sessionStorage에서 설정 확인
        const storedConfig = localStorage.getItem('frontendConfig') || sessionStorage.getItem('frontendConfig');
        if (storedConfig) {
            return JSON.parse(storedConfig);
        }
        
        // 기본 설정 값 반환 (frontend.yaml의 기본값과 동일)
        return {
            backend: {
                protocol: 'http',
                host: 'localhost',
                port: 9090,
                api_path: '/api',
                timeout: 30000,
                retries: 3
            }
        };
    }
    
    /**
     * 설정 파일을 비동기로 로드하고 ApiService를 재초기화
     */
    async loadConfigAsync() {
        try {
            // 설정 파일 fetch 시도
            const response = await fetch('./config/frontend.yaml');
            if (response.ok) {
                const yamlText = await response.text();
                // YAML 파싱은 간단히 처리 (실제 구현에서는 yaml 라이브러리 사용 권장)
                console.log('[ApiService] YAML 설정 파일 로드됨, ConfigManager 사용 권장');
            }
        } catch (error) {
            console.warn('[ApiService] 설정 파일 비동기 로드 실패:', error.message);
        }
    }

    /**
     * HTTP 요청을 보내는 기본 메서드
     */
    async request(url, options = {}) {
        const config = {
            timeout: this.defaultTimeout,
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            ...options
        };

        // 타임아웃 처리
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), config.timeout);
        config.signal = controller.signal;

        try {
            const response = await fetch(this.baseURL + url, config);
            clearTimeout(timeoutId);

            if (!response.ok) {
                throw new ApiError(response.status, response.statusText, await response.text());
            }

            const contentType = response.headers.get('Content-Type');
            if (contentType && contentType.includes('application/json')) {
                return await response.json();
            }
            
            return await response.text();

        } catch (error) {
            clearTimeout(timeoutId);
            
            if (error.name === 'AbortError') {
                throw new ApiError(408, 'Request Timeout', '요청 시간이 초과되었습니다');
            }
            
            throw error;
        }
    }

    /**
     * 재시도 로직이 포함된 요청
     */
    async requestWithRetry(url, options = {}) {
        let lastError;
        
        for (let attempt = 1; attempt <= this.retryAttempts; attempt++) {
            try {
                return await this.request(url, options);
            } catch (error) {
                lastError = error;
                
                // 4xx 에러는 재시도하지 않음
                if (error.status >= 400 && error.status < 500) {
                    throw error;
                }
                
                if (attempt < this.retryAttempts) {
                    await this.delay(this.retryDelay * attempt);
                    console.warn(`API 요청 재시도 ${attempt}/${this.retryAttempts}: ${url}`);
                }
            }
        }
        
        throw lastError;
    }

    /**
     * GET 요청
     */
    async get(url, params = {}) {
        const query = new URLSearchParams(params).toString();
        const fullUrl = query ? `${url}?${query}` : url;
        return this.requestWithRetry(fullUrl, { method: 'GET' });
    }

    /**
     * POST 요청
     */
    async post(url, data = null) {
        return this.requestWithRetry(url, {
            method: 'POST',
            body: data ? JSON.stringify(data) : null
        });
    }

    /**
     * PUT 요청
     */
    async put(url, data = null) {
        return this.requestWithRetry(url, {
            method: 'PUT',
            body: data ? JSON.stringify(data) : null
        });
    }

    /**
     * DELETE 요청
     */
    async delete(url) {
        return this.requestWithRetry(url, { method: 'DELETE' });
    }

    /**
     * 파일 업로드
     */
    async uploadFile(url, file, progressCallback = null) {
        const formData = new FormData();
        formData.append('file', file);

        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            
            xhr.upload.addEventListener('progress', (event) => {
                if (event.lengthComputable && progressCallback) {
                    const progress = Math.round((event.loaded / event.total) * 100);
                    progressCallback(progress);
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        const response = JSON.parse(xhr.responseText);
                        resolve(response);
                    } catch {
                        resolve(xhr.responseText);
                    }
                } else {
                    reject(new ApiError(xhr.status, xhr.statusText, xhr.responseText));
                }
            });

            xhr.addEventListener('error', () => {
                reject(new ApiError(0, 'Network Error', '네트워크 오류가 발생했습니다'));
            });

            xhr.open('POST', this.baseURL + url);
            xhr.send(formData);
        });
    }

    /**
     * 스트리밍 요청 (Server-Sent Events)
     */
    async stream(url, onMessage, onError = null) {
        return new Promise((resolve, reject) => {
            const eventSource = new EventSource(this.baseURL + url);
            
            eventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    onMessage(data);
                } catch (error) {
                    onMessage(event.data);
                }
            };

            eventSource.onerror = (error) => {
                if (onError) {
                    onError(error);
                } else {
                    reject(error);
                }
                eventSource.close();
            };

            eventSource.addEventListener('close', () => {
                eventSource.close();
                resolve();
            });

            // 연결 해제 함수 반환
            return () => eventSource.close();
        });
    }

    // === 파일 스캔 API ===

    /**
     * 전체 드라이브 스캔 시작
     */
    async startFullScan(options = {}) {
        return this.post('/api/files/scan', options);
    }

    /**
     * 특정 폴더 스캔 시작
     */
    async startFolderScan(folderId, options = {}) {
        return this.post('/api/files/scan/folder', { folderId, ...options });
    }

    /**
     * 스캔 진행 상황 조회
     */
    async getScanProgress() {
        return this.get('/api/files/scan/progress');
    }

    /**
     * 스캔 중지
     */
    async stopScan() {
        return this.post('/api/files/scan/stop');
    }

    /**
     * 해시 계산 시작
     */
    async startHashCalculation(options = {}) {
        return this.post('/api/files/hash/calculate', options);
    }

    // === 중복 파일 API ===

    /**
     * 중복 파일 검색
     */
    async findDuplicates(options = {}) {
        return this.post('/api/duplicates/find', options);
    }

    /**
     * 중복 그룹 목록 조회
     */
    async getDuplicateGroups(page = 1, limit = 50) {
        return this.get('/api/duplicates/groups', { page, limit });
    }

    /**
     * 특정 중복 그룹 상세 조회
     */
    async getDuplicateGroup(groupId) {
        return this.get(`/api/duplicates/group`, { id: groupId });
    }

    // === 폴더 비교 API ===

    /**
     * 폴더 비교 시작
     */
    async compareFolders(sourceId, targetId, options = {}) {
        return this.post('/api/compare/folders', {
            sourceId,
            targetId,
            ...options
        });
    }

    /**
     * 비교 결과 목록 조회
     */
    async getComparisonResults() {
        return this.get('/api/compare/results');
    }

    /**
     * 특정 비교 결과 상세 조회
     */
    async getComparisonResult(resultId) {
        return this.get('/api/compare/result', { id: resultId });
    }

    // === 파일 정리 API ===

    /**
     * 파일 삭제
     */
    async deleteFiles(fileIds, options = {}) {
        return this.post('/api/cleanup/files', { fileIds, options });
    }

    /**
     * 중복 파일 정리
     */
    async cleanupDuplicates(groupIds, options = {}) {
        return this.post('/api/cleanup/duplicates', { groupIds, options });
    }

    /**
     * 패턴 기반 파일 삭제
     */
    async deleteByPattern(pattern, options = {}) {
        return this.post('/api/cleanup/pattern', { pattern, options });
    }

    /**
     * 빈 폴더 정리
     */
    async cleanupEmptyFolders(options = {}) {
        return this.post('/api/cleanup/folders', options);
    }

    // === 통계 API ===

    /**
     * 파일 통계 조회
     */
    async getFileStats() {
        return this.get('/api/stats/files');
    }

    /**
     * 중복 파일 통계 조회
     */
    async getDuplicateStats() {
        return this.get('/api/stats/duplicates');
    }

    /**
     * 시스템 상태 조회
     */
    async getSystemStatus() {
        return this.get('/api/status');
    }

    // === 헬스 체크 API ===

    /**
     * 서버 상태 확인
     */
    async healthCheck() {
        return this.get('/health');
    }

    /**
     * 데이터베이스 상태 확인
     */
    async checkDatabase() {
        return this.get('/health/db');
    }

    /**
     * Google Drive 연결 상태 확인
     */
    async checkStorage() {
        return this.get('/health/storage');
    }

    // === 유틸리티 메서드 ===

    /**
     * 지연 함수
     */
    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    /**
     * 진행 상황 폴링
     */
    async pollProgress(progressUrl, callback, interval = 1000) {
        const poll = async () => {
            try {
                const progress = await this.get(progressUrl);
                callback(progress);
                
                if (!progress.completed && !progress.error) {
                    setTimeout(poll, interval);
                }
            } catch (error) {
                callback({ error: error.message });
            }
        };
        
        await poll();
    }

    /**
     * 배치 요청 처리
     */
    async batchRequest(requests, batchSize = 5) {
        const results = [];
        
        for (let i = 0; i < requests.length; i += batchSize) {
            const batch = requests.slice(i, i + batchSize);
            const batchResults = await Promise.allSettled(
                batch.map(request => this.request(request.url, request.options))
            );
            
            results.push(...batchResults);
            
            // 배치 간 지연
            if (i + batchSize < requests.length) {
                await this.delay(100);
            }
        }
        
        return results;
    }
}

/**
 * API 에러 클래스
 */
export class ApiError extends Error {
    constructor(status, statusText, message) {
        super(message || statusText);
        this.name = 'ApiError';
        this.status = status;
        this.statusText = statusText;
    }

    toString() {
        return `${this.name}: ${this.status} ${this.statusText} - ${this.message}`;
    }
}

/**
 * 설정이 적용된 API 서비스 인스턴스 생성 함수
 */
export function createApiService(configManager) {
    return new ApiService(configManager);
}

// 기본 인스턴스는 App.js에서 초기화하므로 제거
// export const apiService = new ApiService();