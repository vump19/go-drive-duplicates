/**
 * Dual Panel Explorer - Main Controller
 * Initializes panels and handles action button events
 */

class ExplorerApp {
    constructor() {
        this.leftPanel = null;
        this.rightPanel = null;
        this.currentOperation = null;
    }

    /**
     * Initialize the application
     */
    async initialize() {
        try {
            // Initialize status bar and modals
            StatusBar.initialize();
            ModalController.initialize();

            // Show loading status
            StatusBar.showMessage('Initializing Explorer...', 'fa-spinner fa-spin');

            // Initialize panels
            this.leftPanel = new PanelController('leftPanel', 'left');
            this.rightPanel = new PanelController('rightPanel', 'right');

            // Load initial data
            await Promise.all([
                this.leftPanel.initialize(),
                this.rightPanel.initialize()
            ]);

            // Attach action button event listeners
            this.attachActionButtonListeners();

            // Setup resizers for panel width adjustment
            this.setupResizers();

            // Check URL parameters for direct navigation
            await this.handleURLParameters();

            // Show ready status
            StatusBar.showSuccess('Explorer ready');

        } catch (error) {
            console.error('Failed to initialize explorer:', error);
            StatusBar.showError('Failed to initialize explorer');
        }
    }

    /**
     * Handle URL parameters for direct navigation
     */
    async handleURLParameters() {
        const urlParams = new URLSearchParams(window.location.search);
        const folderId = urlParams.get('folderId');
        const fileId = urlParams.get('fileId');

        if (folderId) {
            try {
                // Navigate left panel to the specified folder
                await this.leftPanel.navigateToFolder(folderId);

                // If fileId is specified, highlight that file
                if (fileId) {
                    this.leftPanel.highlightFile(fileId);
                    StatusBar.showSuccess(`파일 위치로 이동했습니다`);
                } else {
                    StatusBar.showSuccess(`폴더로 이동했습니다`);
                }
            } catch (error) {
                console.error('Failed to navigate to folder:', error);
                StatusBar.showError('폴더 이동 실패');
            }
        }
    }

    /**
     * Attach event listeners to action buttons
     */
    attachActionButtonListeners() {
        // Copy operations
        document.getElementById('copyLeftToRight').addEventListener('click', () => {
            this.copyFiles(this.leftPanel, this.rightPanel);
        });

        document.getElementById('copyRightToLeft').addEventListener('click', () => {
            this.copyFiles(this.rightPanel, this.leftPanel);
        });

        // Move operations
        document.getElementById('moveLeftToRight').addEventListener('click', () => {
            this.moveFiles(this.leftPanel, this.rightPanel);
        });

        document.getElementById('moveRightToLeft').addEventListener('click', () => {
            this.moveFiles(this.rightPanel, this.leftPanel);
        });

        // Delete operations
        document.getElementById('deleteLeft').addEventListener('click', () => {
            this.deleteFiles(this.leftPanel);
        });

        document.getElementById('deleteRight').addEventListener('click', () => {
            this.deleteFiles(this.rightPanel);
        });

        // Create folder operations
        document.getElementById('createFolderLeft').addEventListener('click', () => {
            this.showCreateFolderDialog(this.leftPanel);
        });

        document.getElementById('createFolderRight').addEventListener('click', () => {
            this.showCreateFolderDialog(this.rightPanel);
        });

        // Operation modal buttons
        document.getElementById('cancelOperation').addEventListener('click', () => {
            this.cancelCurrentOperation();
        });

        document.getElementById('closeOperationModal').addEventListener('click', () => {
            ModalController.closeModal(ModalController.operationModal);
            // Refresh both panels after operation completes
            this.leftPanel.refresh();
            this.rightPanel.refresh();
        });

        // Create folder modal buttons
        document.getElementById('cancelCreateFolder').addEventListener('click', () => {
            ModalController.closeModal(ModalController.createFolderModal);
        });

        document.getElementById('confirmCreateFolder').addEventListener('click', () => {
            this.createFolder();
        });
    }

    /**
     * Setup resizers for panel width adjustment
     */
    setupResizers() {
        // Main panel resizers (between panels)
        this.setupPanelResizer('leftResizer', 'leftPanel', 'before');
        this.setupPanelResizer('rightResizer', 'rightPanel', 'after');

        // Tree resizers (inside panels, between folder tree and file list)
        this.setupTreeResizer('leftTreeResizer', 'leftPanel');
        this.setupTreeResizer('rightTreeResizer', 'rightPanel');
    }

    /**
     * Setup a panel resizer (between main panels)
     */
    setupPanelResizer(resizerId, panelId, direction) {
        const resizer = document.getElementById(resizerId);
        const panel = document.getElementById(panelId);

        if (!resizer || !panel) return;

        let isResizing = false;
        let startX = 0;
        let startWidth = 0;

        resizer.addEventListener('mousedown', (e) => {
            isResizing = true;
            startX = e.clientX;
            startWidth = panel.offsetWidth;

            resizer.classList.add('active');
            document.body.classList.add('resizing');

            e.preventDefault();
        });

        document.addEventListener('mousemove', (e) => {
            if (!isResizing) return;

            const diff = e.clientX - startX;
            let newWidth;

            if (direction === 'before') {
                // Left resizer - affects left panel
                newWidth = startWidth + diff;
            } else {
                // Right resizer - affects right panel (inverse direction)
                newWidth = startWidth - diff;
            }

            // Minimum and maximum width constraints
            newWidth = Math.max(200, Math.min(newWidth, window.innerWidth * 0.6));

            panel.style.flex = 'none';
            panel.style.width = newWidth + 'px';
        });

        document.addEventListener('mouseup', () => {
            if (isResizing) {
                isResizing = false;
                resizer.classList.remove('active');
                document.body.classList.remove('resizing');
            }
        });
    }

    /**
     * Setup a tree resizer (inside panel, between folder tree and file list)
     */
    setupTreeResizer(resizerId, panelId) {
        const resizer = document.getElementById(resizerId);
        const panel = document.getElementById(panelId);

        if (!resizer || !panel) return;

        const treeContainer = panel.querySelector('.folder-tree-container');
        if (!treeContainer) return;

        let isResizing = false;
        let startX = 0;
        let startWidth = 0;

        resizer.addEventListener('mousedown', (e) => {
            isResizing = true;
            startX = e.clientX;
            startWidth = treeContainer.offsetWidth;

            resizer.classList.add('active');
            document.body.classList.add('resizing');

            e.preventDefault();
        });

        document.addEventListener('mousemove', (e) => {
            if (!isResizing) return;

            const diff = e.clientX - startX;
            let newWidth = startWidth + diff;

            // Minimum and maximum width constraints
            newWidth = Math.max(100, Math.min(newWidth, 500));

            treeContainer.style.width = newWidth + 'px';
        });

        document.addEventListener('mouseup', () => {
            if (isResizing) {
                isResizing = false;
                resizer.classList.remove('active');
                document.body.classList.remove('resizing');
            }
        });
    }

    /**
     * Copy files from source panel to target panel
     */
    async copyFiles(sourcePanel, targetPanel) {
        const selectedFiles = sourcePanel.getSelectedFileIds();

        if (selectedFiles.length === 0) {
            StatusBar.showError('No files selected');
            return;
        }

        const confirmDetails = `
            <div>
                <p><strong>Source:</strong> ${sourcePanel.side === 'left' ? 'Left Panel' : 'Right Panel'}</p>
                <p><strong>Target:</strong> ${targetPanel.side === 'left' ? 'Left Panel' : 'Right Panel'}</p>
                <p><strong>Files to copy:</strong> ${selectedFiles.length}</p>
            </div>
        `;

        ModalController.showConfirm(
            'Confirm Copy',
            `Are you sure you want to copy ${selectedFiles.length} file(s)?`,
            confirmDetails,
            async () => {
                try {
                    StatusBar.showMessage('Copying files...', 'fa-spinner fa-spin');

                    const response = await ExplorerAPI.copyFiles(
                        selectedFiles,
                        sourcePanel.currentFolderId,
                        targetPanel.currentFolderId
                    );

                    if (response.operationId) {
                        ModalController.showOperationProgress('Copy', response.operationId);
                        this.currentOperation = response.operationId;
                    } else {
                        StatusBar.showSuccess(`Copied ${selectedFiles.length} file(s)`);
                        await targetPanel.refresh();
                        sourcePanel.clearSelection();
                    }

                } catch (error) {
                    console.error('Copy failed:', error);
                    StatusBar.showError(`Copy failed: ${error.message}`);
                }
            }
        );
    }

    /**
     * Move files from source panel to target panel
     */
    async moveFiles(sourcePanel, targetPanel) {
        const selectedFiles = sourcePanel.getSelectedFileIds();

        if (selectedFiles.length === 0) {
            StatusBar.showError('No files selected');
            return;
        }

        const confirmDetails = `
            <div>
                <p><strong>Source:</strong> ${sourcePanel.side === 'left' ? 'Left Panel' : 'Right Panel'}</p>
                <p><strong>Target:</strong> ${targetPanel.side === 'left' ? 'Left Panel' : 'Right Panel'}</p>
                <p><strong>Files to move:</strong> ${selectedFiles.length}</p>
                <p class="text-danger"><strong>Warning:</strong> Files will be removed from source location</p>
            </div>
        `;

        ModalController.showConfirm(
            'Confirm Move',
            `Are you sure you want to move ${selectedFiles.length} file(s)?`,
            confirmDetails,
            async () => {
                try {
                    StatusBar.showMessage('Moving files...', 'fa-spinner fa-spin');

                    const response = await ExplorerAPI.moveFiles(
                        selectedFiles,
                        sourcePanel.currentFolderId,
                        targetPanel.currentFolderId
                    );

                    if (response.operationId) {
                        ModalController.showOperationProgress('Move', response.operationId);
                        this.currentOperation = response.operationId;
                    } else {
                        StatusBar.showSuccess(`Moved ${selectedFiles.length} file(s)`);
                        await Promise.all([
                            sourcePanel.refresh(),
                            targetPanel.refresh()
                        ]);
                    }

                } catch (error) {
                    console.error('Move failed:', error);
                    StatusBar.showError(`Move failed: ${error.message}`);
                }
            }
        );
    }

    /**
     * Delete files from panel
     */
    async deleteFiles(panel) {
        const selectedFiles = panel.getSelectedFileIds();

        if (selectedFiles.length === 0) {
            StatusBar.showError('No files selected');
            return;
        }

        const confirmDetails = `
            <div>
                <p><strong>패널:</strong> ${panel.side === 'left' ? '왼쪽 패널' : '오른쪽 패널'}</p>
                <p><strong>휴지통으로 이동할 파일:</strong> ${selectedFiles.length}개</p>
            </div>
        `;

        ModalController.showConfirm(
            '휴지통 이동 확인',
            `${selectedFiles.length}개 파일을 휴지통으로 이동하시겠습니까?`,
            confirmDetails,
            async () => {
                try {
                    StatusBar.showMessage('휴지통으로 이동 중...', 'fa-spinner fa-spin');

                    const response = await ExplorerAPI.deleteFiles(selectedFiles, false);

                    if (response.operationId) {
                        ModalController.showOperationProgress('휴지통으로 이동', response.operationId);
                        this.currentOperation = response.operationId;
                    } else {
                        StatusBar.showSuccess(`${selectedFiles.length}개 파일 휴지통으로 이동 완료`);
                        await panel.refresh();
                    }

                } catch (error) {
                    console.error('Delete failed:', error);
                    StatusBar.showError(`휴지통 이동 실패: ${error.message}`);
                }
            }
        );
    }

    /**
     * Show create folder dialog
     */
    showCreateFolderDialog(panel) {
        const locationInfo = document.getElementById('createFolderLocation');
        locationInfo.innerHTML = `
            <i class="fas fa-folder"></i>
            <span>${panel.side === 'left' ? 'Left Panel' : 'Right Panel'} - Current Folder</span>
        `;

        // Clear previous input
        const folderNameInput = document.getElementById('newFolderName');
        folderNameInput.value = '';

        // Store the panel for later use
        this.createFolderPanel = panel;

        ModalController.openModal(ModalController.createFolderModal);

        // Focus on input
        setTimeout(() => folderNameInput.focus(), 100);
    }

    /**
     * Create a new folder
     */
    async createFolder() {
        const folderNameInput = document.getElementById('newFolderName');
        const folderName = folderNameInput.value.trim();

        if (!folderName) {
            StatusBar.showError('Please enter a folder name');
            return;
        }

        const panel = this.createFolderPanel;

        try {
            StatusBar.showMessage('Creating folder...', 'fa-spinner fa-spin');
            ModalController.closeModal(ModalController.createFolderModal);

            await ExplorerAPI.createFolder(panel.currentFolderId, folderName);

            StatusBar.showSuccess(`Folder "${folderName}" created successfully`);
            await panel.refresh();

        } catch (error) {
            console.error('Create folder failed:', error);
            StatusBar.showError(`Create folder failed: ${error.message}`);
        }
    }

    /**
     * Cancel current operation
     */
    async cancelCurrentOperation() {
        if (!this.currentOperation) {
            return;
        }

        try {
            await ExplorerAPI.cancelOperation(this.currentOperation);
            StatusBar.showMessage('Operation cancelled');
            ModalController.closeModal(ModalController.operationModal);
            this.currentOperation = null;

        } catch (error) {
            console.error('Cancel operation failed:', error);
            StatusBar.showError(`Cancel operation failed: ${error.message}`);
        }
    }
}

// File Scan Progress Monitoring
let scanProgressPollingActive = true;
let scanProgressInterval = null;

/**
 * Update file scan progress display
 */
async function updateScanProgress() {
    if (!scanProgressPollingActive) return;

    try {
        const container = document.getElementById('scanProgressContainer');
        const section = document.getElementById('scanProgressSection');
        let progress = null;

        // Check file scan progress
        try {
            const scanProgress = await window.API.File.getScanProgress();
            if (scanProgress && (scanProgress.status === 'running' || scanProgress.status === 'pending')) {
                progress = scanProgress;
            }
        } catch (error) {
            // 404는 정상 (작업 없음)
            if (!error.message || !error.message.includes('HTTP 404')) {
                console.error('파일 스캔 진행 상황 조회 실패:', error);
            }
        }

        // If no file scan, check duplicate search
        if (!progress) {
            try {
                const dupProgress = await window.API.Duplicate.getDuplicateProgress();
                if (dupProgress && (dupProgress.status === 'running' || dupProgress.status === 'pending')) {
                    progress = dupProgress;
                }
            } catch (error) {
                // 404는 정상 (작업 없음)
                if (!error.message || !error.message.includes('HTTP 404')) {
                    console.error('중복 검색 진행 상황 조회 실패:', error);
                }
            }
        }

        // Display progress if available
        if (progress && (progress.status === 'running' || progress.status === 'pending')) {
            section.style.display = 'block';

            // Determine title and icon
            let title = '작업 진행 중';
            let icon = 'fas fa-cog';

            switch (progress.operationType) {
                case 'file_scan':
                    title = '파일 스캔';
                    icon = 'fas fa-folder-open';
                    break;
                case 'duplicate_search':
                    title = '중복 파일 검색';
                    icon = 'fas fa-search';
                    break;
                case 'hash_calculation':
                    title = '해시 계산';
                    icon = 'fas fa-calculator';
                    break;
                default:
                    title = '작업 진행 중';
                    icon = 'fas fa-cog';
            }

            // Get scanned folders count
            let scannedFolders = 0;
            if (progress.metadata && progress.metadata.scannedFolders) {
                scannedFolders = Array.isArray(progress.metadata.scannedFolders)
                    ? progress.metadata.scannedFolders.length
                    : 0;
            }

            // Progress bar style
            const hasTotal = progress.totalItems > 0;
            const percent = hasTotal
                ? Math.round((progress.processedItems / progress.totalItems) * 100)
                : 0;
            let progressBarStyle = hasTotal
                ? `width: ${percent}%`
                : 'width: 100%; animation: progress-indeterminate 5s ease-in-out infinite';

            container.innerHTML = `
                <div class="progress-item">
                    <div class="progress-header">
                        <div class="progress-title">
                            <i class="${icon}"></i> ${title}
                        </div>
                        <div class="progress-percentage">${hasTotal ? percent + '%' : '진행 중...'}</div>
                    </div>
                    <div class="progress-bar">
                        <div class="progress-fill" style="${progressBarStyle}"></div>
                    </div>
                    <div class="progress-details">
                        ${progress.operationType === 'file_scan'
                            ? `<strong style="font-size: 1.1em; color: #3b82f6;">📊 전체 스캔: ${formatNumber(progress.processedItems || 0)}개 파일</strong>`
                            : `${progress.currentStep || '검색 중...'}`
                        }
                        ${scannedFolders > 0 ? `<br><strong>스캔 완료 폴더:</strong> ${formatNumber(scannedFolders)}개` : ''}
                        ${progress.operationType === 'file_scan' && progress.currentStep ? `<br><span style="color: #6b7280;">${progress.currentStep}</span>` : ''}
                        ${progress.metadata && progress.metadata.lastScannedFolderPath ? `<br><strong>현재 폴더:</strong> ${progress.metadata.lastScannedFolderPath}` : ''}
                    </div>
                </div>
            `;
        } else {
            // Hide section if no progress
            section.style.display = 'none';
        }
    } catch (error) {
        console.error('Progress update error:', error);
    }
}

/**
 * Format number with K/M suffixes
 */
function formatNumber(num) {
    // 전체 숫자를 쉼표로 구분하여 표시
    return num.toLocaleString();
}

/**
 * Start progress monitoring
 */
function startProgressMonitoring() {
    scanProgressPollingActive = true;
    updateScanProgress();
    scanProgressInterval = setInterval(updateScanProgress, 5000);
}

/**
 * Stop progress monitoring
 */
function stopProgressMonitoring() {
    scanProgressPollingActive = false;
    if (scanProgressInterval) {
        clearInterval(scanProgressInterval);
        scanProgressInterval = null;
    }
}

// DB Mode State
let isDBMode = false;

/**
 * Toggle between DB mode and Live mode
 */
function toggleDBMode() {
    const toggle = document.getElementById('dbModeToggle');
    const label = document.getElementById('modeLabel');

    isDBMode = toggle.checked;

    if (isDBMode) {
        label.innerHTML = '<i class="fas fa-database"></i> DB 모드 (스캔된 파일)';
        console.log('✅ DB 모드 활성화: 데이터베이스에서 파일 조회');
    } else {
        label.innerHTML = '<i class="fas fa-cloud"></i> Live 모드 (Google Drive)';
        console.log('✅ Live 모드 활성화: Google Drive API 직접 조회');
    }

    // Reload panels with new mode
    if (window.explorerApp) {
        window.explorerApp.leftPanel.refresh();
        window.explorerApp.rightPanel.refresh();
    }
}

/**
 * Get current API based on mode
 */
function getCurrentAPI() {
    return isDBMode ? window.ExplorerDBAPI : window.ExplorerAPI;
}

// Initialize the application when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    const app = new ExplorerApp();
    app.initialize();

    // Start file scan progress monitoring
    startProgressMonitoring();

    // Setup DB mode toggle
    const dbModeToggle = document.getElementById('dbModeToggle');
    if (dbModeToggle) {
        dbModeToggle.addEventListener('change', toggleDBMode);
    }

    // Store app instance globally for debugging
    window.explorerApp = app;
    window.isDBMode = () => isDBMode;
    window.getCurrentAPI = getCurrentAPI;
});
