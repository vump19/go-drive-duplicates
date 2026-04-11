/**
 * Dual Panel Explorer - UI Controller
 * Handles all UI rendering and user interactions
 */

class PanelController {
    constructor(panelId, side) {
        this.panelId = panelId;
        this.side = side; // 'left' or 'right'
        this.currentFolderId = 'root';
        this.currentFolderPath = [];
        this.selectedFiles = new Set();
        this.fileList = [];
        this.folderTree = null;
        this.currentPageToken = null;
        this.currentInputPageToken = null; // Token used to load current page
        this.previousPageTokens = []; // Stack to store previous page tokens
        this.hasNextPage = false;
        this.sortBy = 'name';
        this.sortOrder = 'asc';
        this.searchQuery = '';

        this.initializeElements();
        this.attachEventListeners();
    }

    /**
     * Initialize DOM element references
     */
    initializeElements() {
        const prefix = this.side;

        // Panel elements
        this.panel = document.getElementById(`${prefix}Panel`);
        this.breadcrumb = document.getElementById(`${prefix}Breadcrumb`);
        this.folderTreeElement = document.getElementById(`${prefix}FolderTree`);
        this.fileListBody = document.getElementById(`${prefix}FileListBody`);

        // Control elements
        this.refreshBtn = document.getElementById(`${prefix}Refresh`);
        this.homeBtn = document.getElementById(`${prefix}Home`);
        this.selectAllCheckbox = document.getElementById(`${prefix}SelectAll`);
        this.selectFilesBtn = document.getElementById(`${prefix}SelectFiles`);
        this.selectedCountElement = document.getElementById(`${prefix}SelectedCount`);
        this.searchInput = document.getElementById(`${prefix}Search`);
        this.searchBtn = document.getElementById(`${prefix}SearchBtn`);
        this.sortBySelect = document.getElementById(`${prefix}SortBy`);
        this.sortOrderSelect = document.getElementById(`${prefix}SortOrder`);

        // Pagination elements
        this.prevPageBtn = document.getElementById(`${prefix}PrevPage`);
        this.nextPageBtn = document.getElementById(`${prefix}NextPage`);
        this.pageInfo = document.getElementById(`${prefix}PageInfo`);

        // Footer elements
        this.itemCountElement = document.getElementById(`${prefix}ItemCount`);
        this.totalSizeElement = document.getElementById(`${prefix}TotalSize`);
    }

    /**
     * Attach event listeners to UI elements
     */
    attachEventListeners() {
        // Refresh and navigation
        this.refreshBtn.addEventListener('click', () => this.refresh());
        this.homeBtn.addEventListener('click', () => this.navigateToFolder('root'));

        // Select all
        this.selectAllCheckbox.addEventListener('change', (e) => this.toggleSelectAll(e.target.checked));
        this.selectFilesBtn.addEventListener('click', () => this.selectOnlyFiles());

        // Search
        const debouncedSearch = FileUtils.debounce(() => this.performSearch(), 500);
        this.searchInput.addEventListener('input', debouncedSearch);
        this.searchBtn.addEventListener('click', () => this.performSearch());
        this.searchInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                this.performSearch();
            }
        });

        // Sort
        this.sortBySelect.addEventListener('change', (e) => {
            this.sortBy = e.target.value;
            this.refresh();
        });
        this.sortOrderSelect.addEventListener('change', (e) => {
            this.sortOrder = e.target.value;
            this.refresh();
        });

        // Pagination
        this.prevPageBtn.addEventListener('click', () => this.previousPage());
        this.nextPageBtn.addEventListener('click', () => this.nextPage());
    }

    /**
     * Initialize panel by loading folder tree and contents
     */
    async initialize() {
        try {
            await this.loadFolderTree();
            await this.loadFolderContents();
        } catch (error) {
            this.showError('Failed to initialize panel', error);
        }
    }

    /**
     * Load folder tree from API
     */
    async loadFolderTree() {
        try {
            this.showFolderTreeLoading();
            // Load only 1 level initially for fast loading
            this.folderTree = await ExplorerAPI.getFolderTree('root', 1);
            this.renderFolderTree(this.folderTree);
        } catch (error) {
            this.showError('Failed to load folder tree', error);
            throw error;
        }
    }

    /**
     * Load folder contents from API
     * @param {string} pageToken - Page token for pagination
     * @param {boolean} isGoingBack - Whether we're going back to a previous page
     */
    async loadFolderContents(pageToken = null, isGoingBack = false) {
        try {
            this.showFileListLoading();

            const options = {
                sortBy: this.sortBy,
                sortOrder: this.sortOrder,
                pageSize: '50'
            };

            if (pageToken) {
                options.pageToken = pageToken;
            }

            if (this.searchQuery) {
                options.searchQuery = this.searchQuery;
            }

            const response = await ExplorerAPI.listFolderContents(this.currentFolderId, options);

            // Store the input token used to load this page
            this.currentInputPageToken = pageToken;

            // Sort items: folders first, then files
            this.fileList = this.sortFoldersThenFiles(response.items || []);
            this.currentPageToken = response.nextPageToken || null;
            this.hasNextPage = !!this.currentPageToken;

            this.renderFileList();
            this.updateFooter();
            this.updatePagination();

        } catch (error) {
            this.showError('Failed to load folder contents', error);
            throw error;
        }
    }

    /**
     * Sort items with folders first, then files
     */
    sortFoldersThenFiles(items) {
        // Separate folders and files
        const folders = items.filter(item => item.isFolder);
        const files = items.filter(item => !item.isFolder);

        // Sort folders
        this.sortItems(folders);

        // Sort files
        this.sortItems(files);

        // Return folders first, then files
        return [...folders, ...files];
    }

    /**
     * Sort items based on current sort settings
     */
    sortItems(items) {
        items.sort((a, b) => {
            let comparison = 0;

            switch (this.sortBy) {
                case 'name':
                    comparison = a.name.localeCompare(b.name);
                    break;
                case 'size':
                    comparison = a.size - b.size;
                    break;
                case 'modifiedTime':
                    comparison = new Date(a.modifiedTime) - new Date(b.modifiedTime);
                    break;
                case 'mimeType':
                    comparison = a.mimeType.localeCompare(b.mimeType);
                    break;
                default:
                    comparison = a.name.localeCompare(b.name);
            }

            // Apply sort order
            return this.sortOrder === 'desc' ? -comparison : comparison;
        });
    }

    /**
     * Render folder tree in the UI
     */
    renderFolderTree(node, container = this.folderTreeElement, level = 0) {
        if (level === 0) {
            container.innerHTML = '';
        }

        const folderItem = document.createElement('div');
        folderItem.className = 'folder-item';
        folderItem.dataset.folderId = node.id;

        if (node.id === this.currentFolderId) {
            folderItem.classList.add('active');
        }

        // Toggle button
        const toggle = document.createElement('span');
        toggle.className = 'folder-toggle';

        // Check if folder has subfolders (from API or already loaded children)
        const hasSubfolders = node.hasChildren || node.subfolderCount > 0 || (node.children && node.children.length > 0);

        if (hasSubfolders) {
            toggle.innerHTML = node.isExpanded ? '<i class="fas fa-chevron-down"></i>' : '<i class="fas fa-chevron-right"></i>';
            toggle.addEventListener('click', async (e) => {
                e.stopPropagation();
                await this.toggleFolderExpansion(node, folderItem);
            });
        } else {
            toggle.classList.add('empty');
        }

        // Folder icon
        const icon = document.createElement('i');
        icon.className = 'fas fa-folder folder-icon';

        // Folder name
        const name = document.createElement('span');
        name.className = 'folder-name';
        name.textContent = node.name;
        name.title = node.path;

        // Size button
        const sizeBtn = document.createElement('button');
        sizeBtn.className = 'folder-size-btn';
        sizeBtn.innerHTML = '<i class="fas fa-calculator"></i>';
        sizeBtn.title = '폴더 크기 계산';
        sizeBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.showFolderSize(node.id, node.name, sizeBtn);
        });

        folderItem.appendChild(toggle);
        folderItem.appendChild(icon);
        folderItem.appendChild(name);
        folderItem.appendChild(sizeBtn);

        // Navigate on click
        folderItem.addEventListener('click', () => this.navigateToFolder(node.id, node.path));

        container.appendChild(folderItem);

        // Render children if expanded
        if (node.isExpanded && node.children && node.children.length > 0) {
            const childrenContainer = document.createElement('div');
            childrenContainer.className = 'folder-children';
            container.appendChild(childrenContainer);

            node.children.forEach(child => {
                this.renderFolderTree(child, childrenContainer, level + 1);
            });
        }
    }

    /**
     * Toggle folder expansion in tree (with lazy loading)
     */
    async toggleFolderExpansion(node, folderItem) {
        const toggle = folderItem.querySelector('.folder-toggle');

        // If expanding and children not loaded yet, load them
        if (!node.isExpanded && (!node.children || node.children.length === 0)) {
            // Show loading state
            toggle.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';

            try {
                // Load subfolder tree for this folder
                const subTree = await ExplorerAPI.getFolderTree(node.id, 1);

                // Add children to the node
                if (subTree && subTree.children) {
                    node.children = subTree.children;
                    node.hasChildren = subTree.children.length > 0;
                    node.subfolderCount = subTree.children.length;
                }
            } catch (error) {
                console.error('Failed to load subfolders:', error);
                StatusBar.showError('하위 폴더 로드 실패: ' + error.message);
                toggle.innerHTML = '<i class="fas fa-chevron-right"></i>';
                return;
            }
        }

        // Toggle expansion state
        node.isExpanded = !node.isExpanded;

        // Update toggle icon
        if (node.isExpanded) {
            toggle.innerHTML = '<i class="fas fa-chevron-down"></i>';
        } else {
            toggle.innerHTML = '<i class="fas fa-chevron-right"></i>';
        }

        // Re-render the entire tree to show/hide children
        this.renderFolderTree(this.folderTree);
    }

    /**
     * Render file list in the UI
     */
    renderFileList() {
        this.fileListBody.innerHTML = '';

        if (this.fileList.length === 0) {
            const emptyRow = document.createElement('tr');
            emptyRow.innerHTML = `
                <td colspan="6" class="text-center">
                    <div class="empty-state">
                        <i class="fas fa-folder-open"></i>
                        <span>No files in this folder</span>
                    </div>
                </td>
            `;
            this.fileListBody.appendChild(emptyRow);
            return;
        }

        this.fileList.forEach(file => {
            const row = document.createElement('tr');
            row.dataset.fileId = file.id;

            if (this.selectedFiles.has(file.id)) {
                row.classList.add('selected');
            }

            // Checkbox
            const checkboxCell = document.createElement('td');
            checkboxCell.className = 'col-checkbox';
            const checkbox = document.createElement('input');
            checkbox.type = 'checkbox';
            checkbox.checked = this.selectedFiles.has(file.id);
            checkbox.addEventListener('change', (e) => {
                e.stopPropagation();
                this.toggleFileSelection(file.id, e.target.checked);
            });
            checkboxCell.appendChild(checkbox);

            // Icon
            const iconCell = document.createElement('td');
            iconCell.className = 'col-icon';
            const icon = document.createElement('i');
            icon.className = `file-icon ${FileUtils.getFileIcon(file.mimeType, file.isFolder)}`;
            iconCell.appendChild(icon);

            // Name
            const nameCell = document.createElement('td');
            nameCell.className = 'col-name';
            nameCell.textContent = file.name;
            nameCell.title = file.name;

            // Size
            const sizeCell = document.createElement('td');
            sizeCell.className = 'col-size';
            sizeCell.textContent = file.isFolder ? '-' : FileUtils.formatFileSize(file.size);

            // Modified
            const modifiedCell = document.createElement('td');
            modifiedCell.className = 'col-modified';
            modifiedCell.textContent = FileUtils.formatDate(file.modifiedTime);

            // Type
            const typeCell = document.createElement('td');
            typeCell.className = 'col-type';
            typeCell.textContent = FileUtils.getFileType(file.mimeType, file.isFolder);

            row.appendChild(checkboxCell);
            row.appendChild(iconCell);
            row.appendChild(nameCell);
            row.appendChild(sizeCell);
            row.appendChild(modifiedCell);
            row.appendChild(typeCell);

            // Double-click to navigate into folder
            if (file.isFolder) {
                row.addEventListener('dblclick', () => this.navigateToFolder(file.id, file.name));
                row.style.cursor = 'pointer';
            }

            // Single click to select
            row.addEventListener('click', (e) => {
                if (e.target.type !== 'checkbox') {
                    const checkbox = row.querySelector('input[type="checkbox"]');
                    checkbox.checked = !checkbox.checked;
                    this.toggleFileSelection(file.id, checkbox.checked);
                }
            });

            this.fileListBody.appendChild(row);
        });
    }

    /**
     * Navigate to a folder
     */
    async navigateToFolder(folderId, folderPath = null) {
        try {
            this.currentFolderId = folderId;
            this.currentPageToken = null;
            this.previousPageTokens = []; // Reset pagination when navigating to new folder
            this.selectedFiles.clear();

            // Update breadcrumb
            this.updateBreadcrumb(folderId, folderPath);

            // Load contents
            await this.loadFolderContents();

            // Update tree highlighting
            this.folderTreeElement.querySelectorAll('.folder-item').forEach(item => {
                if (item.dataset.folderId === folderId) {
                    item.classList.add('active');
                } else {
                    item.classList.remove('active');
                }
            });

        } catch (error) {
            this.showError('Failed to navigate to folder', error);
        }
    }

    /**
     * Update breadcrumb navigation
     */
    updateBreadcrumb(folderId, folderPath) {
        this.breadcrumb.innerHTML = '';

        // Root item
        const rootItem = document.createElement('span');
        rootItem.className = 'breadcrumb-item';
        rootItem.innerHTML = '<i class="fas fa-hard-drive"></i> My Drive';
        rootItem.addEventListener('click', () => this.navigateToFolder('root'));
        this.breadcrumb.appendChild(rootItem);

        // Current folder (if not root)
        if (folderId !== 'root' && folderPath) {
            const separator = document.createElement('span');
            separator.className = 'breadcrumb-separator';
            separator.textContent = ' / ';
            this.breadcrumb.appendChild(separator);

            const currentItem = document.createElement('span');
            currentItem.className = 'breadcrumb-item';
            currentItem.textContent = folderPath;
            this.breadcrumb.appendChild(currentItem);
        }
    }

    /**
     * Toggle file selection
     */
    toggleFileSelection(fileId, selected) {
        if (selected) {
            this.selectedFiles.add(fileId);
        } else {
            this.selectedFiles.delete(fileId);
        }

        this.updateSelectedCount();

        // Update row highlighting
        const row = this.fileListBody.querySelector(`tr[data-file-id="${fileId}"]`);
        if (row) {
            if (selected) {
                row.classList.add('selected');
            } else {
                row.classList.remove('selected');
            }
        }
    }

    /**
     * Toggle select all
     */
    toggleSelectAll(selected) {
        if (selected) {
            // Select all items (folders and files) on current page
            this.fileList.forEach(file => {
                this.selectedFiles.add(file.id);
            });
        } else {
            // Deselect all items on current page
            this.fileList.forEach(file => {
                this.selectedFiles.delete(file.id);
            });
        }

        this.renderFileList();
        this.updateSelectedCount();
    }

    /**
     * Select only files (not folders)
     */
    selectOnlyFiles() {
        this.selectedFiles.clear();
        this.fileList.forEach(file => {
            if (!file.isFolder) {
                this.selectedFiles.add(file.id);
            }
        });

        this.renderFileList();
        this.updateSelectedCount();
    }

    /**
     * Update selected count display
     */
    updateSelectedCount() {
        this.selectedCountElement.textContent = `(${this.selectedFiles.size} selected)`;
        this.selectAllCheckbox.checked = this.selectedFiles.size === this.fileList.length && this.fileList.length > 0;
    }

    /**
     * Update footer statistics
     */
    updateFooter() {
        this.itemCountElement.textContent = `${this.fileList.length} items`;

        const totalSize = this.fileList
            .filter(file => !file.isFolder)
            .reduce((sum, file) => sum + file.size, 0);

        this.totalSizeElement.textContent = FileUtils.formatFileSize(totalSize);
    }

    /**
     * Update pagination controls
     */
    updatePagination() {
        this.nextPageBtn.disabled = !this.hasNextPage;
        this.prevPageBtn.disabled = this.previousPageTokens.length === 0;
    }

    /**
     * Navigate to next page
     */
    async nextPage() {
        if (this.hasNextPage && this.currentPageToken) {
            // Save current page token to go back later
            this.previousPageTokens.push(this.currentInputPageToken);
            await this.loadFolderContents(this.currentPageToken);
        }
    }

    /**
     * Navigate to previous page
     */
    async previousPage() {
        if (this.previousPageTokens.length > 0) {
            // Pop and get the previous page token
            const previousToken = this.previousPageTokens.pop();

            // Load the previous page
            await this.loadFolderContents(previousToken, true); // true = going back
        }
    }

    /**
     * Perform search
     */
    async performSearch() {
        this.searchQuery = this.searchInput.value.trim();
        this.currentPageToken = null;
        this.previousPageTokens = []; // Reset pagination on search
        await this.loadFolderContents();
    }

    /**
     * Refresh current view
     */
    async refresh() {
        this.selectedFiles.clear();
        this.currentPageToken = null;
        this.previousPageTokens = []; // Reset pagination on refresh
        await this.loadFolderContents();
    }

    /**
     * Get selected file IDs
     */
    getSelectedFileIds() {
        return Array.from(this.selectedFiles);
    }

    /**
     * Clear selection
     */
    clearSelection() {
        this.selectedFiles.clear();
        this.renderFileList();
        this.updateSelectedCount();
    }

    /**
     * Highlight a specific file in the list
     * @param {string} fileId - File ID to highlight
     */
    highlightFile(fileId) {
        // Find the file row
        const fileRow = this.fileListBody.querySelector(`tr[data-file-id="${fileId}"]`);

        if (fileRow) {
            // Add highlight class
            fileRow.classList.add('highlighted-file');

            // Select the file
            this.toggleFileSelection(fileId, true);
            const checkbox = fileRow.querySelector('input[type="checkbox"]');
            if (checkbox) {
                checkbox.checked = true;
            }

            // Scroll into view
            setTimeout(() => {
                fileRow.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }, 300);

            // Remove highlight after animation
            setTimeout(() => {
                fileRow.classList.remove('highlighted-file');
            }, 3000);
        } else {
            console.warn(`File ${fileId} not found in current view`);
        }
    }

    /**
     * Show loading state for folder tree
     */
    showFolderTreeLoading() {
        this.folderTreeElement.innerHTML = `
            <div class="loading">
                <i class="fas fa-spinner fa-spin"></i>
                <span>Loading folders...</span>
            </div>
        `;
    }

    /**
     * Show loading state for file list
     */
    showFileListLoading() {
        this.fileListBody.innerHTML = `
            <tr class="loading-row">
                <td colspan="6">
                    <i class="fas fa-spinner fa-spin"></i>
                    <span>Loading files...</span>
                </td>
            </tr>
        `;
    }

    /**
     * Show folder size information
     */
    async showFolderSize(folderId, folderName, button) {
        try {
            // Show loading state
            button.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';
            button.disabled = true;
            StatusBar.showMessage(`폴더 크기 계산 중: ${folderName}...`, 'fa-calculator');

            // Get folder size from API
            const sizeInfo = await ExplorerAPI.getFolderSize(folderId);

            // Show size information in status bar
            const sizeText = FileUtils.formatFileSize(sizeInfo.totalSize);
            const message = `📊 ${folderName}: ${sizeText} (${sizeInfo.fileCount} 파일, ${sizeInfo.folderCount} 폴더)`;
            StatusBar.showSuccess(message);

            // Update button to show size
            button.innerHTML = `<span class="folder-size-text">${sizeText}</span>`;
            button.title = `총 크기: ${sizeText}\n파일: ${sizeInfo.fileCount}개\n폴더: ${sizeInfo.folderCount}개`;

            // Reset button after 5 seconds
            setTimeout(() => {
                button.innerHTML = '<i class="fas fa-calculator"></i>';
                button.title = '폴더 크기 계산';
                button.disabled = false;
            }, 5000);

        } catch (error) {
            this.showError('폴더 크기 계산 실패', error);
            button.innerHTML = '<i class="fas fa-calculator"></i>';
            button.disabled = false;
        }
    }

    /**
     * Show error message
     */
    showError(message, error) {
        console.error(message, error);
        StatusBar.showError(`${message}: ${error.message}`);
    }
}

/**
 * Status Bar Controller
 */
const StatusBar = {
    statusBar: null,
    statusMessage: null,
    statusProgress: null,
    progressFill: null,
    progressText: null,

    initialize() {
        this.statusBar = document.getElementById('statusBar');
        this.statusMessage = document.getElementById('statusMessage');
        this.statusProgress = document.getElementById('statusProgress');
        this.progressFill = document.getElementById('progressFill');
        this.progressText = document.getElementById('progressText');
    },

    showMessage(message, icon = 'fa-info-circle') {
        this.statusMessage.innerHTML = `
            <i class="fas ${icon}"></i>
            <span>${message}</span>
        `;
        this.statusProgress.style.display = 'none';
    },

    showError(message) {
        this.showMessage(message, 'fa-exclamation-circle');
    },

    showSuccess(message) {
        this.showMessage(message, 'fa-check-circle');
    },

    showProgress(percentage, text) {
        this.statusProgress.style.display = 'flex';
        this.progressFill.style.width = `${percentage}%`;
        this.progressText.textContent = `${Math.round(percentage)}%`;

        this.statusMessage.innerHTML = `
            <i class="fas fa-spinner fa-spin"></i>
            <span>${text}</span>
        `;
    },

    hideProgress() {
        this.statusProgress.style.display = 'none';
    }
};

/**
 * Modal Controller
 */
const ModalController = {
    operationModal: null,
    createFolderModal: null,
    confirmModal: null,

    initialize() {
        this.operationModal = document.getElementById('operationModal');
        this.createFolderModal = document.getElementById('createFolderModal');
        this.confirmModal = document.getElementById('confirmModal');

        // Close modals on background click
        [this.operationModal, this.createFolderModal, this.confirmModal].forEach(modal => {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    this.closeModal(modal);
                }
            });
        });

        // Close button for create folder modal
        const closeCreateFolderBtn = document.getElementById('closeCreateFolderModal');
        if (closeCreateFolderBtn) {
            closeCreateFolderBtn.addEventListener('click', () => {
                this.closeModal(this.createFolderModal);
            });
        }
    },

    openModal(modal) {
        modal.classList.add('active');
    },

    closeModal(modal) {
        modal.classList.remove('active');
    },

    showOperationProgress(operationType, operationId) {
        const operationTypeElement = document.getElementById('operationType');
        operationTypeElement.innerHTML = `
            <i class="fas fa-spinner fa-spin"></i>
            <span>${operationType} in progress...</span>
        `;

        this.openModal(this.operationModal);

        // Start polling for progress
        this.pollOperationProgress(operationId);
    },

    async pollOperationProgress(operationId) {
        try {
            await ExplorerAPI.pollOperationProgress(operationId, (progress) => {
                this.updateOperationProgress(progress);
            });
        } catch (error) {
            console.error('Error polling operation progress:', error);
            this.showOperationError('Failed to get operation progress');
        }
    },

    updateOperationProgress(progress) {
        const filesProgress = document.getElementById('operationFilesProgress');
        const sizeProgress = document.getElementById('operationSizeProgress');
        const progressFill = document.getElementById('operationProgressFill');
        const progressPercentage = document.getElementById('operationProgressPercentage');

        filesProgress.textContent = `${progress.processedFiles} / ${progress.totalFiles}`;
        sizeProgress.textContent = `${FileUtils.formatFileSize(progress.processedBytes)} / ${FileUtils.formatFileSize(progress.totalBytes)}`;

        const percentage = progress.totalFiles > 0 ? (progress.processedFiles / progress.totalFiles) * 100 : 0;
        progressFill.style.width = `${percentage}%`;
        progressPercentage.textContent = `${Math.round(percentage)}%`;

        // Show close button when complete
        if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'cancelled') {
            document.getElementById('cancelOperation').style.display = 'none';
            document.getElementById('closeOperationModal').style.display = 'block';
        }
    },

    showOperationError(message) {
        const operationDetails = document.getElementById('operationDetails');
        operationDetails.innerHTML = `
            <div class="detail-item error">
                <i class="fas fa-exclamation-circle"></i>
                ${message}
            </div>
        `;
    },

    showConfirm(title, message, details, onConfirm) {
        const confirmTitle = document.getElementById('confirmTitle');
        const confirmMessage = document.getElementById('confirmMessage');
        const confirmDetails = document.getElementById('confirmDetails');
        const proceedBtn = document.getElementById('proceedConfirm');
        const cancelBtn = document.getElementById('cancelConfirm');

        confirmTitle.textContent = title;
        confirmMessage.textContent = message;
        confirmDetails.innerHTML = details || '';

        // Remove old event listeners
        const newProceedBtn = proceedBtn.cloneNode(true);
        proceedBtn.parentNode.replaceChild(newProceedBtn, proceedBtn);

        const newCancelBtn = cancelBtn.cloneNode(true);
        cancelBtn.parentNode.replaceChild(newCancelBtn, cancelBtn);

        // Add new event listeners
        newProceedBtn.addEventListener('click', () => {
            this.closeModal(this.confirmModal);
            onConfirm();
        });

        newCancelBtn.addEventListener('click', () => {
            this.closeModal(this.confirmModal);
        });

        this.openModal(this.confirmModal);
    }
};

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { PanelController, StatusBar, ModalController };
}
