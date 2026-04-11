/**
 * Dual Panel Explorer - API Client
 * Handles all communication with the backend API
 */

// API Base Configuration - 동적으로 설정됨
let EXPLORER_API_BASE = 'http://localhost:9090';
let explorerConfigLoaded = false;

// 설정 로드 함수
async function loadExplorerConfig() {
    if (explorerConfigLoaded) return;

    try {
        const response = await fetch('config/frontend.yaml');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const yamlText = await response.text();
        const config = parseExplorerYAML(yamlText);

        if (config.backend) {
            const { protocol, host, port } = config.backend;
            EXPLORER_API_BASE = `${protocol}://${host}:${port}`;
            console.log(`🔧 Explorer API 설정 로드: ${EXPLORER_API_BASE}`);
        }
        explorerConfigLoaded = true;
    } catch (error) {
        console.warn('⚠️  Explorer 설정 파일 로드 실패, 기본 설정 사용:', error.message);
        // 기본값: 개발 환경에서는 9090 포트 사용
        if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
            EXPLORER_API_BASE = 'http://localhost:9090';
        }
        explorerConfigLoaded = true;
    }
}

// 간단한 YAML 파서
function parseExplorerYAML(yamlText) {
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
            currentSection[key] = parseExplorerValue(valueStr);
        }
    }

    return result;
}

function parseExplorerValue(valueStr) {
    if (valueStr === 'true') return true;
    if (valueStr === 'false') return false;
    if (valueStr === 'null') return null;
    if (!isNaN(valueStr) && !isNaN(parseFloat(valueStr))) {
        return valueStr.includes('.') ? parseFloat(valueStr) : parseInt(valueStr, 10);
    }
    return valueStr.replace(/['\"]/g, '');
}

// 설정 로드 (페이지 로드 시 실행)
loadExplorerConfig();

const ExplorerAPI = {
    // Base URL getter - 동적으로 설정된 값 사용
    get baseURL() {
        return `${EXPLORER_API_BASE}/api/explorer`;
    },

    /**
     * Get folder tree structure
     * @param {string} folderId - Folder ID to get tree from
     * @param {number} depth - Maximum depth to traverse
     * @returns {Promise<Object>} Folder tree object
     */
    async getFolderTree(folderId = 'root', depth = 3) {
        try {
            const params = new URLSearchParams({
                folderId: folderId,
                depth: depth.toString()
            });

            const response = await fetch(`${this.baseURL}/folder-tree?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to fetch folder tree: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error fetching folder tree:', error);
            throw error;
        }
    },

    /**
     * List folder contents with pagination and filtering
     * @param {string} folderId - Folder ID to list contents
     * @param {Object} options - List options (sortBy, order, search, pageSize, pageToken)
     * @returns {Promise<Object>} File list response
     */
    async listFolderContents(folderId, options = {}) {
        try {
            const params = new URLSearchParams({
                folderId: folderId,
                ...options
            });

            const response = await fetch(`${this.baseURL}/folder-contents?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to list folder contents: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error listing folder contents:', error);
            throw error;
        }
    },

    /**
     * Copy files to target folder
     * @param {Array<string>} fileIds - Array of file IDs to copy
     * @param {string} sourceFolderId - Source folder ID
     * @param {string} targetFolderId - Target folder ID
     * @returns {Promise<Object>} Copy operation response
     */
    async copyFiles(fileIds, sourceFolderId, targetFolderId) {
        try {
            const response = await fetch(`${this.baseURL}/copy-files`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    fileIds: fileIds,
                    sourceFolderId: sourceFolderId,
                    targetFolderId: targetFolderId,
                    operationType: 'copy'
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to copy files: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error copying files:', error);
            throw error;
        }
    },

    /**
     * Move files to target folder
     * @param {Array<string>} fileIds - Array of file IDs to move
     * @param {string} sourceFolderId - Source folder ID
     * @param {string} targetFolderId - Target folder ID
     * @returns {Promise<Object>} Move operation response
     */
    async moveFiles(fileIds, sourceFolderId, targetFolderId) {
        try {
            const response = await fetch(`${this.baseURL}/move-files`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    fileIds: fileIds,
                    sourceFolderId: sourceFolderId,
                    targetFolderId: targetFolderId,
                    operationType: 'move'
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to move files: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error moving files:', error);
            throw error;
        }
    },

    /**
     * Delete files
     * @param {Array<string>} fileIds - Array of file IDs to delete
     * @param {boolean} permanentDelete - true = permanent delete, false = move to trash
     * @returns {Promise<Object>} Delete operation response
     */
    async deleteFiles(fileIds, permanentDelete = false) {
        try {
            const response = await fetch(`${this.baseURL}/delete-files`, {
                method: 'POST', // Using POST instead of DELETE for body support
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    fileIds: fileIds,
                    permanentDelete: permanentDelete
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to delete files: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error deleting files:', error);
            throw error;
        }
    },

    /**
     * Rename a file
     * @param {string} fileId - File ID to rename
     * @param {string} newName - New name for the file
     * @returns {Promise<Object>} Rename response
     */
    async renameFile(fileId, newName) {
        try {
            const response = await fetch(`${this.baseURL}/rename-file`, {
                method: 'POST', // Using POST for consistency
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    fileId: fileId,
                    newName: newName
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to rename file: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error renaming file:', error);
            throw error;
        }
    },

    /**
     * Create a new folder
     * @param {string} parentFolderId - Parent folder ID
     * @param {string} folderName - Name for the new folder
     * @returns {Promise<Object>} Created folder object
     */
    async createFolder(parentFolderId, folderName) {
        try {
            const response = await fetch(`${this.baseURL}/create-folder`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    parentFolderId: parentFolderId,
                    folderName: folderName
                })
            });

            if (!response.ok) {
                throw new Error(`Failed to create folder: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error creating folder:', error);
            throw error;
        }
    },

    /**
     * Get file information
     * @param {string} fileId - File ID to get information
     * @returns {Promise<Object>} File information object
     */
    async getFileInfo(fileId) {
        try {
            const params = new URLSearchParams({ fileId: fileId });
            const response = await fetch(`${this.baseURL}/file-info?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to get file info: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error getting file info:', error);
            throw error;
        }
    },

    /**
     * Search files
     * @param {string} query - Search query
     * @param {Object} options - Search options (sortBy, order, pageSize, pageToken)
     * @returns {Promise<Object>} Search results
     */
    async searchFiles(query, options = {}) {
        try {
            const params = new URLSearchParams({
                query: query,
                ...options
            });

            const response = await fetch(`${this.baseURL}/search?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to search files: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error searching files:', error);
            throw error;
        }
    },

    /**
     * Get folder size (total size including all subfolders)
     * @param {string} folderId - Folder ID to calculate size
     * @returns {Promise<Object>} Folder size information
     */
    async getFolderSize(folderId) {
        try {
            const params = new URLSearchParams({
                folderId: folderId
            });

            const response = await fetch(`${this.baseURL}/folder-size?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to get folder size: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error getting folder size:', error);
            throw error;
        }
    },

    /**
     * Get operation progress
     * @param {number} operationId - Operation ID to check progress
     * @returns {Promise<Object>} Operation progress object
     */
    async getOperationProgress(operationId) {
        try {
            const params = new URLSearchParams({
                operationId: operationId.toString()
            });

            const response = await fetch(`${this.baseURL}/operation/progress?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to get operation progress: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error getting operation progress:', error);
            throw error;
        }
    },

    /**
     * Cancel an operation
     * @param {number} operationId - Operation ID to cancel
     * @returns {Promise<Object>} Cancel response
     */
    async cancelOperation(operationId) {
        try {
            const params = new URLSearchParams({
                operationId: operationId.toString()
            });

            const response = await fetch(`${this.baseURL}/operation/cancel?${params}`, {
                method: 'POST'
            });

            if (!response.ok) {
                throw new Error(`Failed to cancel operation: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error canceling operation:', error);
            throw error;
        }
    },

    /**
     * Poll operation progress until completion
     * @param {number} operationId - Operation ID to poll
     * @param {Function} onProgress - Callback function for progress updates
     * @param {number} interval - Polling interval in milliseconds (default: 1000)
     * @returns {Promise<Object>} Final operation status
     */
    async pollOperationProgress(operationId, onProgress, interval = 1000) {
        return new Promise((resolve, reject) => {
            const poll = async () => {
                try {
                    const progress = await this.getOperationProgress(operationId);

                    if (onProgress) {
                        onProgress(progress);
                    }

                    // Check if operation is complete
                    if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'cancelled') {
                        resolve(progress);
                        return;
                    }

                    // Continue polling
                    setTimeout(poll, interval);
                } catch (error) {
                    reject(error);
                }
            };

            poll();
        });
    }
};

/**
 * Utility functions for file operations
 */
const FileUtils = {
    /**
     * Format file size in human-readable format
     * @param {number} bytes - Size in bytes
     * @returns {string} Formatted size string
     */
    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';

        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));

        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    },

    /**
     * Format date in user-friendly format
     * @param {string|Date} date - Date to format
     * @returns {string} Formatted date string
     */
    formatDate(date) {
        const d = new Date(date);
        const now = new Date();
        const diffMs = now - d;
        const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

        if (diffDays === 0) {
            return 'Today ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
        } else if (diffDays === 1) {
            return 'Yesterday ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
        } else if (diffDays < 7) {
            return d.toLocaleDateString('en-US', { weekday: 'short' }) + ' ' +
                   d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
        } else {
            return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
        }
    },

    /**
     * Get icon class for file type
     * @param {string} mimeType - MIME type of the file
     * @param {boolean} isFolder - Whether it's a folder
     * @returns {string} Font Awesome icon class
     */
    getFileIcon(mimeType, isFolder) {
        if (isFolder) {
            return 'fas fa-folder folder';
        }

        // Document types
        if (mimeType.includes('document') || mimeType.includes('word')) {
            return 'fas fa-file-word document';
        }
        if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) {
            return 'fas fa-file-excel document';
        }
        if (mimeType.includes('presentation') || mimeType.includes('powerpoint')) {
            return 'fas fa-file-powerpoint document';
        }
        if (mimeType.includes('pdf')) {
            return 'fas fa-file-pdf document';
        }
        if (mimeType.includes('text')) {
            return 'fas fa-file-alt document';
        }

        // Image types
        if (mimeType.startsWith('image/')) {
            return 'fas fa-file-image image';
        }

        // Video types
        if (mimeType.startsWith('video/')) {
            return 'fas fa-file-video video';
        }

        // Audio types
        if (mimeType.startsWith('audio/')) {
            return 'fas fa-file-audio audio';
        }

        // Archive types
        if (mimeType.includes('zip') || mimeType.includes('rar') || mimeType.includes('tar') ||
            mimeType.includes('7z') || mimeType.includes('compressed')) {
            return 'fas fa-file-archive archive';
        }

        // Code types
        if (mimeType.includes('javascript') || mimeType.includes('json') ||
            mimeType.includes('html') || mimeType.includes('css') ||
            mimeType.includes('xml') || mimeType.includes('python') ||
            mimeType.includes('java') || mimeType.includes('php')) {
            return 'fas fa-file-code code';
        }

        // Default
        return 'fas fa-file';
    },

    /**
     * Get file type name from MIME type
     * @param {string} mimeType - MIME type of the file
     * @param {boolean} isFolder - Whether it's a folder
     * @returns {string} File type name
     */
    getFileType(mimeType, isFolder) {
        if (isFolder) {
            return 'Folder';
        }

        if (mimeType.includes('document')) return 'Document';
        if (mimeType.includes('spreadsheet')) return 'Spreadsheet';
        if (mimeType.includes('presentation')) return 'Presentation';
        if (mimeType.includes('pdf')) return 'PDF';
        if (mimeType.startsWith('image/')) return 'Image';
        if (mimeType.startsWith('video/')) return 'Video';
        if (mimeType.startsWith('audio/')) return 'Audio';
        if (mimeType.includes('zip') || mimeType.includes('rar')) return 'Archive';
        if (mimeType.includes('text')) return 'Text';

        return 'File';
    },

    /**
     * Extract folder ID from Google Drive URL
     * @param {string} url - Google Drive folder URL
     * @returns {string|null} Folder ID or null if not found
     */
    extractFolderIdFromUrl(url) {
        const patterns = [
            /folders\/([a-zA-Z0-9_-]+)/,
            /id=([a-zA-Z0-9_-]+)/,
            /^([a-zA-Z0-9_-]+)$/
        ];

        for (const pattern of patterns) {
            const match = url.match(pattern);
            if (match) {
                return match[1];
            }
        }

        return null;
    },

    /**
     * Debounce function for search inputs
     * @param {Function} func - Function to debounce
     * @param {number} wait - Wait time in milliseconds
     * @returns {Function} Debounced function
     */
    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }
};

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ExplorerAPI, FileUtils };
}

/**
 * Database-based Explorer API
 * For browsing scanned files from database
 */
const ExplorerDBAPI = {
    // Base URL getter - 동적으로 설정된 값 사용
    get baseURL() {
        return `${EXPLORER_API_BASE}/api/files/db`;
    },

    /**
     * Get folder tree from database
     * @param {string} parentId - Parent folder ID
     * @param {number} depth - Maximum depth
     * @returns {Promise<Object>} Folder tree
     */
    async getFolderTree(parentId = 'root', depth = 3) {
        try {
            const params = new URLSearchParams({
                parentId: parentId,
                depth: depth.toString()
            });

            const response = await fetch(`${this.baseURL}/tree?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to fetch DB folder tree: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error fetching DB folder tree:', error);
            throw error;
        }
    },

    /**
     * List files from database
     * @param {string} parentId - Parent folder ID
     * @returns {Promise<Object>} File list
     */
    async listFiles(parentId = 'root') {
        try {
            const params = new URLSearchParams({
                parentId: parentId
            });

            const response = await fetch(`${this.baseURL}/list?${params}`);

            if (!response.ok) {
                throw new Error(`Failed to fetch DB file list: ${response.statusText}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Error fetching DB file list:', error);
            throw error;
        }
    }
};

// Export both APIs
window.ExplorerAPI = ExplorerAPI;
window.ExplorerDBAPI = ExplorerDBAPI;
