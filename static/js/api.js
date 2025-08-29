// API Base Configuration
let API_BASE = window.location.origin;

// 설정 로드 함수
async function loadConfig() {
    try {
        const response = await fetch('/static/config/frontend.yaml');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const yamlText = await response.text();
        const config = parseSimpleYAML(yamlText);
        
        if (config.backend) {
            const { protocol, host, port } = config.backend;
            API_BASE = `${protocol}://${host}:${port}`;
            console.log(`🔧 설정 파일에서 백엔드 URL 로드: ${API_BASE}`);
        }
    } catch (error) {
        console.warn('⚠️  설정 파일 로드 실패, 기본 설정 사용:', error.message);
        // 기본값: 개발 환경에서는 8080 포트 사용
        if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
            API_BASE = 'http://localhost:8080';
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
    const config = {
        headers: {
            'Content-Type': 'application/json',
            ...options.headers
        },
        ...options
    };

    try {
        console.log(`API Call: ${config.method || 'GET'} ${url}`);
        const response = await fetch(url, config);
        
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }
        
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
            return await response.json();
        } else {
            return await response.text();
        }
    } catch (error) {
        console.error('API Error:', error);
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
        return apiCall('/api/files/scan/progress');
    },

    async calculateHashes() {
        return apiCall('/api/files/hash/calculate', {
            method: 'POST'
        });
    },

    async getHashProgress() {
        return apiCall('/api/files/hash/progress');
    }
};

// Duplicate APIs
const DuplicateAPI = {
    async findDuplicates() {
        return apiCall('/api/duplicates/find', {
            method: 'POST'
        });
    },

    async getDuplicateGroups(page = 1, limit = 20) {
        return apiCall(`/api/duplicates/groups?page=${page}&limit=${limit}`);
    },

    async getDuplicateGroup(id) {
        return apiCall(`/api/duplicates/group?id=${id}`);
    },

    async deleteDuplicateGroup(id) {
        return apiCall(`/api/duplicates/group/delete?id=${id}`, {
            method: 'DELETE'
        });
    },

    async getDuplicateProgress() {
        return apiCall('/api/duplicates/progress');
    }
};

// Comparison APIs
const ComparisonAPI = {
    async compareFolders(sourceFolderId, targetFolderId) {
        return apiCall('/api/compare/folders', {
            method: 'POST',
            body: JSON.stringify({
                sourceFolderId,
                targetFolderId
            })
        });
    },

    async getComparisonProgress() {
        return apiCall('/api/compare/progress');
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
        return apiCall('/api/cleanup/progress');
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
    if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M';
    } else if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
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

// Export for use in other files
window.API = {
    Health: HealthAPI,
    File: FileAPI,
    Duplicate: DuplicateAPI,
    Comparison: ComparisonAPI,
    Cleanup: CleanupAPI
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