// API Base Configuration
let API_BASE = 'http://localhost:9090'; // 기본값으로 백엔드 포트 설정
window.API_BASE = API_BASE; // 다른 스크립트에서 접근 가능하도록 window에 노출

// 설정 로드 함수
async function loadConfig() {
    try {
        const response = await fetch('config/frontend.yaml');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const yamlText = await response.text();
        const config = parseSimpleYAML(yamlText);
        
        if (config.backend) {
            const { protocol, host, port } = config.backend;
            API_BASE = `${protocol}://${host}:${port}`;
            window.API_BASE = API_BASE; // window에도 업데이트
            console.log(`🔧 설정 파일에서 백엔드 URL 로드: ${API_BASE}`);
        }
    } catch (error) {
        console.warn('⚠️  설정 파일 로드 실패, 기본 설정 사용:', error.message);
        // 기본값: 개발 환경에서는 9090 포트 사용
        if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
            API_BASE = 'http://localhost:9090';
            window.API_BASE = API_BASE;
        }
    }
}

// 간단한 YAML 파서
function parseSimpleYAML(yamlText) {
    const result = {};
    const lines = yamlText.split('\n');
    let currentSection = result;
    let sectionStack = [{ obj: result, indent: -1 }];

    for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith('#')) continue;

        const indent = line.length - line.trimStart().length;
        const colonIndex = trimmed.indexOf(':');
        if (colonIndex === -1) continue;

        const key = trimmed.substring(0, colonIndex).trim();
        const valueStr = trimmed.substring(colonIndex + 1).trim();

        // 적절한 부모 찾기
        while (sectionStack.length > 1 && sectionStack[sectionStack.length - 1].indent >= indent) {
            sectionStack.pop();
        }
        currentSection = sectionStack[sectionStack.length - 1].obj;

        if (valueStr === '') {
            currentSection[key] = {};
            sectionStack.push({ obj: currentSection[key], indent });
        } else {
            currentSection[key] = parseValue(valueStr);
        }
    }

    return result;
}

function parseValue(valueStr) {
    if (valueStr === 'true') return true;
    if (valueStr === 'false') return false;
    if (valueStr === 'null') return null;
    if (!isNaN(valueStr) && !isNaN(parseFloat(valueStr))) {
        return valueStr.includes('.') ? parseFloat(valueStr) : parseInt(valueStr, 10);
    }
    return valueStr.replace(/['"]/g, '');
}

// 설정 로드 (페이지 로드 시 실행)
loadConfig();

// API Helper Functions
async function apiCall(endpoint, options = {}) {
    const url = `${API_BASE}${endpoint}`;
    const silent = options.silent || false;
    const config = {
        headers: {
            'Content-Type': 'application/json',
            ...options.headers
        },
        ...options
    };
    delete config.silent; // Remove silent from fetch config

    try {
        if (!silent) {
            console.log(`API Call: ${config.method || 'GET'} ${url}`);
        }
        const response = await fetch(url, config);

        if (!response.ok) {
            const errorText = await response.text();
            const error = new Error(`HTTP ${response.status}: ${errorText}`);
            error.status = response.status;
            throw error;
        }

        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
            return await response.json();
        } else {
            return await response.text();
        }
    } catch (error) {
        // 404 에러이고 silent 모드면 로그 출력 안 함
        if (!silent || error.status !== 404) {
            console.error('API Error:', error);
        }
        throw error;
    }
}

// Health Check APIs
const HealthAPI = {
    async checkServer() {
        return apiCall('/health');
    },

    async checkDatabase() {
        return apiCall('/health/db');
    },

    async checkStorage() {
        return apiCall('/health/storage');
    }
};

// File APIs
const FileAPI = {
    async startScan() {
        return apiCall('/api/files/scan', {
            method: 'POST'
        });
    },

    async scanFolder(folderId) {
        return apiCall('/api/files/scan/folder', {
            method: 'POST',
            body: JSON.stringify({ folderId })
        });
    },

    async getScanProgress() {
        return apiCall('/api/files/scan/progress', { silent: true });
    },

    async calculateHashes() {
        return apiCall('/api/files/hash/calculate', {
            method: 'POST'
        });
    },

    async getHashProgress() {
        return apiCall('/api/files/hash/progress', { silent: true });
    },

    async getStatistics() {
        return apiCall('/api/files/statistics');
    },

    async checkFilesExist(fileIds) {
        return apiCall('/api/files/check-exists', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ fileIds })
        });
    }
};

// Duplicate APIs
const DuplicateAPI = {
    async findDuplicates(options = {}) {
        return apiCall('/api/duplicates/find', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                calculateHashes: options.calculateHashes !== undefined ? options.calculateHashes : false,
                forceRecalculate: options.forceRecalculate || false,
                minFileSize: options.minFileSize || 1024,
                maxResults: options.maxResults || 1000
            })
        });
    },

    async getDuplicateGroups(page = 1, limit = 20) {
        return apiCall(`/api/duplicates/groups?page=${page}&limit=${limit}`);
    },

    async getDuplicateGroup(id) {
        return apiCall(`/api/duplicates/group?id=${id}`);
    },

    async getDuplicateGroupByHash(hash) {
        return apiCall(`/api/duplicates/group/by-hash?hash=${encodeURIComponent(hash)}`);
    },

    async deleteDuplicateGroup(id) {
        return apiCall(`/api/duplicates/group/delete?id=${id}`, {
            method: 'DELETE'
        });
    },

    async getDuplicateProgress() {
        return apiCall('/api/duplicates/progress', { silent: true });
    },

    async validateAndCleanupDuplicateGroups(options = {}) {
        return apiCall('/api/duplicates/validate', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                workerCount: options.workerCount || 4
            })
        });
    },

    async getFilePath(fileId) {
        return apiCall(`/api/duplicates/file/path?fileId=${fileId}`);
    },

    async trashFile(fileId) {
        return apiCall(`/api/duplicates/file/trash?fileId=${fileId}`, { method: 'POST' });
    },

    async quickDelete(groupId) {
        return apiCall(`/api/duplicates/group/${groupId}/quick-clean`, {
            method: 'DELETE'
        });
    },

    async checkResumableTasks() {
        return apiCall('/api/duplicates/resume-check');
    },

    async resetProgress() {
        return apiCall('/api/duplicates/reset', {
            method: 'DELETE'
        });
    },

    async updateMemo(groupId, memo) {
        return apiCall('/api/duplicates/group/memo', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ groupId, memo })
        });
    }
};

// Comparison APIs
const ComparisonAPI = {
    async compareFolders(sourceFolderId, targetFolderId, options = {}) {
        return apiCall('/api/compare/folders', {
            method: 'POST',
            body: JSON.stringify({
                sourceFolderId,
                targetFolderId,
                includeSubfolders: options.includeSubfolders || true,
                deepComparison: options.deepComparison || true,
                forceNewComparison: options.forceNewComparison || false,
                excludeFolderNames: options.excludeFolderNames || []
            })
        });
    },

    async getComparisonProgress(comparisonId) {
        const endpoint = comparisonId ?
            `/api/compare/progress?id=${comparisonId}` :
            '/api/compare/progress';
        return apiCall(endpoint, { silent: true });
    },

    async loadSavedComparison(sourceFolderId, targetFolderId) {
        return apiCall(`/api/compare/result/load?sourceFolderId=${sourceFolderId}&targetFolderId=${targetFolderId}`);
    },

    async getPendingComparisons() {
        return apiCall('/api/compare/pending');
    },

    async resumeComparison(progressId) {
        return apiCall('/api/compare/resume', {
            method: 'POST',
            body: JSON.stringify({ progressId })
        });
    },

    async deleteTargetFolder(comparisonId, targetFolderId, permanentDelete = false) {
        return apiCall('/api/compare/delete/target-folder', {
            method: 'POST',
            body: JSON.stringify({
                comparisonId,
                targetFolderId,
                deleteEmptyFolders: true,
                permanentDelete: permanentDelete
            })
        });
    },

    async deleteDuplicateFiles(comparisonId, fileIds, deleteEmptyFolders = false, permanentDelete = false) {
        return apiCall('/api/compare/delete/duplicate-files', {
            method: 'POST',
            body: JSON.stringify({
                comparisonId,
                fileIds,
                deleteEmptyFolders,
                permanentDelete: permanentDelete
            })
        });
    },

    async extractFolderIdFromUrl(url) {
        return apiCall('/api/utils/extract-folder-id', {
            method: 'POST',
            body: JSON.stringify({ url })
        });
    },

    async resolveFolderPath(idOrUrl) {
        return apiCall(`/api/utils/resolve-folder?id=${encodeURIComponent(idOrUrl)}`);
    },

    async findSingleFolderDuplicates(folderId, options = {}) {
        return apiCall('/api/compare/single-folder/duplicates', {
            method: 'POST',
            body: JSON.stringify({
                folderId,
                includeSubfolders: options.includeSubfolders || true,
                minFileSize: options.minFileSize || 0,
                forceNewScan: options.forceNewScan || false
            })
        });
    },

    async getSingleFolderProgress(progressId) {
        return apiCall(`/api/compare/single-folder/progress?progressId=${progressId}`);
    },

    async getRecentSingleFolderResults(limit = 20) {
        return apiCall(`/api/compare/single-folder/recent?limit=${limit}`);
    },

    async getRecentComparisons(limit = 20) {
        return apiCall(`/api/compare/results/recent?limit=${limit}`);
    },

    async cancelComparison(progressId) {
        return apiCall('/api/compare/cancel', {
            method: 'POST',
            body: JSON.stringify({ progressId })
        });
    },

    async getUniqueFiles(comparisonId) {
        return apiCall(`/api/compare/unique-files?comparisonId=${comparisonId}`);
    },

    async moveUniqueFiles(comparisonId, options = {}) {
        return apiCall('/api/compare/move-unique-files', {
            method: 'POST',
            body: JSON.stringify({
                comparisonId,
                preservePath: options.preservePath !== false,
                onConflict: options.onConflict || 'rename'
            })
        });
    },

    async getMoveProgress(progressId) {
        return apiCall(`/api/compare/move-unique-files/progress?progressId=${progressId}`, { silent: true });
    },

    async cancelMoveUniqueFiles(progressId) {
        return apiCall(`/api/compare/move-unique-files/cancel?progressId=${progressId}`, {
            method: 'POST'
        });
    },

    async getActiveMoveOperations() {
        return apiCall('/api/compare/move-unique-files/active', { silent: true });
    },

    async deleteComparisonResult(id) {
        return apiCall(`/api/compare/result/delete?id=${id}`, { method: 'DELETE' });
    }
};

// Cleanup APIs
const CleanupAPI = {
    async deleteFiles(fileIds) {
        return apiCall('/api/cleanup/files', {
            method: 'POST',
            body: JSON.stringify({ fileIds })
        });
    },

    async deleteDuplicatesFromGroup(groupId, keepFileId) {
        return apiCall('/api/cleanup/duplicates', {
            method: 'POST',
            body: JSON.stringify({
                groupId,
                keepFileId
            })
        });
    },

    async deleteByPattern(pattern) {
        return apiCall('/api/cleanup/pattern', {
            method: 'POST',
            body: JSON.stringify({ pattern })
        });
    },

    async searchByPattern(pattern) {
        return apiCall('/api/cleanup/search', {
            method: 'POST',
            body: JSON.stringify({ pattern })
        });
    },

    async cleanupEmptyFolders() {
        return apiCall('/api/cleanup/folders', {
            method: 'POST'
        });
    },

    async getCleanupProgress() {
        return apiCall('/api/cleanup/progress', { silent: true });
    }
};


// Utility Functions
function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatNumber(num) {
    // 전체 숫자를 쉼표로 구분하여 표시
    return num.toLocaleString();
}

function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString('ko-KR');
}

function formatDuration(seconds) {
    if (seconds < 60) {
        return `${Math.round(seconds)}초`;
    } else if (seconds < 3600) {
        return `${Math.round(seconds / 60)}분`;
    } else {
        return `${Math.round(seconds / 3600)}시간`;
    }
}

// Error Handler
function handleAPIError(error, context = '') {
    console.error(`API Error in ${context}:`, error);
    
    let message = '알 수 없는 오류가 발생했습니다.';
    
    if (error.message) {
        if (error.message.includes('Failed to fetch')) {
            message = '서버에 연결할 수 없습니다. 서버가 실행 중인지 확인하세요.';
        } else if (error.message.includes('HTTP 404')) {
            message = '요청한 리소스를 찾을 수 없습니다.';
        } else if (error.message.includes('HTTP 500')) {
            message = '서버 내부 오류가 발생했습니다.';
        } else {
            message = error.message;
        }
    }
    
    showNotification(message, 'error');
    return message;
}

// Response Validators
function validateResponse(response, context = '') {
    if (!response) {
        throw new Error(`${context}: 응답이 없습니다`);
    }
    
    if (response.error) {
        throw new Error(`${context}: ${response.error}`);
    }
    
    return response;
}

// Batch API Calls
async function batchApiCalls(calls, concurrency = 3) {
    const results = [];
    const errors = [];
    
    for (let i = 0; i < calls.length; i += concurrency) {
        const batch = calls.slice(i, i + concurrency);
        const promises = batch.map(async (call, index) => {
            try {
                const result = await call();
                return { index: i + index, result, success: true };
            } catch (error) {
                return { index: i + index, error, success: false };
            }
        });
        
        const batchResults = await Promise.all(promises);
        
        batchResults.forEach(item => {
            if (item.success) {
                results[item.index] = item.result;
            } else {
                errors[item.index] = item.error;
            }
        });
    }
    
    return { results, errors };
}

// Polling Helper
function pollUntilComplete(apiCall, checkComplete, interval = 2000, maxAttempts = 30) {
    return new Promise((resolve, reject) => {
        let attempts = 0;
        
        const poll = async () => {
            try {
                attempts++;
                const result = await apiCall();
                
                if (checkComplete(result)) {
                    resolve(result);
                } else if (attempts >= maxAttempts) {
                    reject(new Error('최대 시도 횟수에 도달했습니다'));
                } else {
                    setTimeout(poll, interval);
                }
            } catch (error) {
                reject(error);
            }
        };
        
        poll();
    });
}

// Verified Duplicates API
const VerifiedDuplicateAPI = {
    async markAsVerified(request) {
        return apiCall('/api/verified-duplicates', {
            method: 'POST',
            body: JSON.stringify(request)
        });
    },

    async getVerifiedDuplicates(status = null) {
        const url = status ? `/api/verified-duplicates?status=${encodeURIComponent(status)}` : '/api/verified-duplicates';
        return apiCall(url);
    },

    async updateStatus(id, status, description = '') {
        return apiCall(`/api/verified-duplicates/${id}`, {
            method: 'PUT',
            body: JSON.stringify({ status, description })
        });
    },

    async remove(id) {
        return apiCall(`/api/verified-duplicates/${id}`, {
            method: 'DELETE'
        });
    },

    async getByHash(hash) {
        // Special handling for 404 - it means the hash is not verified, which is a valid state
        const url = `${API_BASE}/api/verified-duplicates/hash/${encodeURIComponent(hash)}`;
        try {
            const response = await fetch(url, {
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            // 404 is expected when hash is not in verified_duplicates table
            if (response.status === 404) {
                return null; // Not verified - this is a normal case, not an error
            }

            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('검증 정보 조회 실패:', error);
            throw error;
        }
    },

    async bulkMark(requests) {
        return apiCall('/api/verified-duplicates/bulk', {
            method: 'POST',
            body: JSON.stringify(requests)
        });
    },

    async getExcludedHashes() {
        return apiCall('/api/verified-duplicates/excluded-hashes');
    }
};

// Export APIs
const ExportAPI = {
    /**
     * 폴더 파일 목록을 Google 스프레드시트로 내보내기 (SSE)
     * @param {string} folderId - Google Drive 폴더 ID
     * @param {object} options - 옵션 (includeSubfolders)
     * @param {function} onProgress - 진행 상황 콜백
     * @param {function} onComplete - 완료 콜백
     * @param {function} onError - 오류 콜백
     * @returns {AbortController} - 요청 취소용 컨트롤러
     */
    exportFolderToSpreadsheet(folderId, options = {}, onProgress, onComplete, onError) {
        const controller = new AbortController();

        fetch(`${API_BASE}/api/export/folder-to-spreadsheet`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                folderId: folderId,
                includeSubfolders: options.includeSubfolders !== false
            }),
            signal: controller.signal
        }).then(async response => {
            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || '내보내기 요청 실패');
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                buffer += decoder.decode(value, { stream: true });

                const lines = buffer.split('\n');
                buffer = lines.pop() || '';

                let eventType = null;
                let eventData = null;

                for (const line of lines) {
                    if (line.startsWith(':')) continue;
                    if (line.startsWith('event: ')) {
                        eventType = line.slice(7).trim();
                    } else if (line.startsWith('data: ')) {
                        try {
                            eventData = JSON.parse(line.slice(6));
                        } catch (e) {
                            console.error('SSE 데이터 파싱 오류:', e);
                        }
                    } else if (line === '' && eventType && eventData) {
                        if (eventType === 'complete') {
                            if (onComplete) onComplete(eventData);
                        } else if (eventType === 'error') {
                            if (onError) onError(eventData.message || '알 수 없는 오류');
                        } else {
                            if (onProgress) onProgress(eventData);
                        }
                        eventType = null;
                        eventData = null;
                    }
                }
            }
        }).catch(error => {
            if (error.name !== 'AbortError') {
                console.error('스프레드시트 내보내기 오류:', error);
                if (onError) onError(error.message);
            }
        });

        return controller;
    }
};

// Consolidate APIs
const ConsolidateAPI = {
    async start(folderId, options = {}) {
        return apiCall('/api/consolidate/start', {
            method: 'POST',
            body: JSON.stringify({
                folderId: folderId,
                includeSubfolders: options.includeSubfolders !== false,
                maxFileSizeMB: options.maxFileSizeMB || 10,
                fileExtensions: options.fileExtensions || []
            })
        });
    },

    async getProgress(jobId) {
        return apiCall(`/api/consolidate/progress?jobId=${jobId}`, { silent: true });
    },

    async cancel(jobId) {
        return apiCall(`/api/consolidate/cancel?jobId=${jobId}`, {
            method: 'POST'
        });
    }
};

// Smart Organizer APIs
const OrganizerAPI = {
    // 채팅
    async chat(sessionId, message, folderContext = '', aiProviders = null) {
        const body = { sessionId, message, folderContext };
        if (aiProviders && aiProviders.length > 0) {
            body.aiProviders = aiProviders;
        }
        return apiCall('/api/organizer/chat', {
            method: 'POST',
            body: JSON.stringify(body)
        });
    },
    async getChatHistory(sessionId) {
        return apiCall(`/api/organizer/chat/history?sessionId=${encodeURIComponent(sessionId)}`);
    },
    async deleteChatSession(sessionId) {
        return apiCall(`/api/organizer/chat/session?sessionId=${encodeURIComponent(sessionId)}`, {
            method: 'DELETE'
        });
    },

    // 규칙 세트
    async createRuleSet(data) {
        return apiCall('/api/organizer/rulesets', {
            method: 'POST',
            body: JSON.stringify(data)
        });
    },
    async getRuleSets() {
        return apiCall('/api/organizer/rulesets');
    },
    async getRuleSet(id) {
        return apiCall(`/api/organizer/rulesets/detail?id=${id}`);
    },
    async updateRuleSet(data) {
        return apiCall('/api/organizer/rulesets', {
            method: 'PUT',
            body: JSON.stringify(data)
        });
    },
    async deleteRuleSet(id) {
        return apiCall(`/api/organizer/rulesets?id=${id}`, { method: 'DELETE' });
    },
    async backupRuleSet(ruleSetId) {
        return apiCall('/api/organizer/rulesets/backup', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId })
        });
    },
    async restoreRuleSet(fileId) {
        return apiCall('/api/organizer/rulesets/restore', {
            method: 'POST',
            body: JSON.stringify({ fileId })
        });
    },

    // 규칙
    async createRule(data) {
        return apiCall('/api/organizer/rules', {
            method: 'POST',
            body: JSON.stringify(data)
        });
    },
    async updateRule(data) {
        return apiCall('/api/organizer/rules', {
            method: 'PUT',
            body: JSON.stringify(data)
        });
    },
    async deleteRule(id) {
        return apiCall(`/api/organizer/rules?id=${id}`, { method: 'DELETE' });
    },
    async applySuggestions(ruleSetId, suggestions) {
        return apiCall('/api/organizer/rules/apply-suggestions', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId, suggestions })
        });
    },

    // 파일 정리
    async previewOrganize(ruleSetId, folderId) {
        return apiCall('/api/organizer/organize/preview', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId, folderId })
        });
    },
    async executeOrganize(ruleSetId, folderId) {
        return apiCall('/api/organizer/organize/execute', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId, folderId })
        });
    },
    async getOrganizeProgress(jobId) {
        return apiCall(`/api/organizer/organize/progress?jobId=${jobId}`, { silent: true });
    },
    async cancelOrganize(jobId) {
        return apiCall(`/api/organizer/organize/cancel?jobId=${jobId}`, { method: 'POST' });
    },

    // 감시
    async startWatching(ruleSetId) {
        return apiCall('/api/organizer/watch/start', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId })
        });
    },
    async stopWatching(ruleSetId) {
        return apiCall('/api/organizer/watch/stop', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId })
        });
    },
    async getWatchStatus() {
        return apiCall('/api/organizer/watch/status');
    },

    // 로그
    async getLogs(ruleSetId, limit = 50, offset = 0) {
        return apiCall(`/api/organizer/logs?ruleSetId=${ruleSetId}&limit=${limit}&offset=${offset}`);
    },
    async exportLogs(ruleSetId, parentFolderId = '') {
        return apiCall('/api/organizer/logs/export', {
            method: 'POST',
            body: JSON.stringify({ ruleSetId, parentFolderId })
        });
    },

    // 폴더 탐색
    async listFolderFiles(folderId = 'root') {
        return apiCall(`/api/organizer/folders?folderId=${folderId}`);
    }
};

// Notification APIs
const NotificationAPI = {
    async getTelegramSettings() {
        return apiCall('/api/notifications/telegram/settings');
    },

    async updateTelegramSettings(settings) {
        return apiCall('/api/notifications/telegram/settings', {
            method: 'PUT',
            body: JSON.stringify(settings)
        });
    },

    async testTelegram() {
        return apiCall('/api/notifications/telegram/test', {
            method: 'POST'
        });
    }
};

// Sharing Management APIs
const SharingAPI = {
    async startScan(folderId, includeSubfolders = true) {
        return apiCall('/api/sharing/scan/start', {
            method: 'POST',
            body: JSON.stringify({ folderId, includeSubfolders })
        });
    },

    async getScanProgress(jobId) {
        return apiCall(`/api/sharing/scan/progress?jobId=${jobId}`, { silent: true });
    },

    async getScanResults(jobId) {
        return apiCall(`/api/sharing/scan/results?jobId=${jobId}`);
    },

    async cancelScan(jobId) {
        return apiCall(`/api/sharing/scan/cancel?jobId=${jobId}`, { method: 'POST' });
    },

    async changePermissions(fileIds, action, targetEmail, newRole = '') {
        return apiCall('/api/sharing/permissions/change', {
            method: 'POST',
            body: JSON.stringify({ fileIds, action, targetEmail, newRole })
        });
    },

    async getChangeProgress(jobId) {
        return apiCall(`/api/sharing/permissions/progress?jobId=${jobId}`, { silent: true });
    }
};

// Audio Conversion APIs
const AudioConvertAPI = {
    async start(folderUrl, folderId, options) {
        return apiCall('/api/audio-convert/start', {
            method: 'POST',
            body: JSON.stringify({
                folderUrl: folderUrl || '',
                folderId: folderId || '',
                options: options
            })
        });
    },

    async getProgress(jobId) {
        return apiCall(`/api/audio-convert/progress?jobId=${jobId}`, { silent: true });
    },

    async cancel(jobId) {
        return apiCall(`/api/audio-convert/cancel?jobId=${jobId}`, {
            method: 'POST'
        });
    },

    async getFormats() {
        return apiCall('/api/audio-convert/formats');
    }
};

// Export for use in other files
window.API = {
    Health: HealthAPI,
    File: FileAPI,
    Duplicate: DuplicateAPI,
    Comparison: ComparisonAPI,
    Cleanup: CleanupAPI,
    VerifiedDuplicate: VerifiedDuplicateAPI,
    Export: ExportAPI,
    Consolidate: ConsolidateAPI,
    Organizer: OrganizerAPI,
    Notification: NotificationAPI,
    Sharing: SharingAPI,
    AudioConvert: AudioConvertAPI
};

window.APIUtils = {
    formatFileSize,
    formatNumber,
    formatDate,
    formatDuration,
    handleAPIError,
    validateResponse,
    batchApiCalls,
    pollUntilComplete
};