/**
 * 파일 정리 페이지
 * 중복 파일 삭제 및 정리 작업 관리
 */

import { Component } from '../components/base/Component.js';
import { ProgressBar } from '../components/widgets/ProgressBar.js';
import { Modal } from '../components/widgets/Modal.js';
import { Toast } from '../components/widgets/Toast.js';

export class Cleanup extends Component {
    constructor() {
        super();
        this.selectedFiles = new Set();
        this.duplicateGroups = [];
        this.isProcessing = false;
        this.progressModal = null;
        this.confirmModal = null;
    }

    async render() {
        return `
            <div class="cleanup-page">
                <!-- 페이지 헤더 -->
                <div class="page-header">
                    <h1 class="page-title">
                        <i class="fas fa-trash-alt"></i>
                        파일 정리
                    </h1>
                    <p class="page-description">
                        중복 파일을 선택하여 삭제하고 저장 공간을 확보하세요
                    </p>
                </div>

                <!-- 정리 옵션 -->
                <div class="card mb-6">
                    <div class="card-header">
                        <h2 class="card-title">
                            <i class="fas fa-cog"></i>
                            정리 옵션
                        </h2>
                    </div>
                    <div class="card-body">
                        <div class="cleanup-options grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="deleteEmptyFolders" checked>
                                    빈 폴더 자동 삭제
                                </label>
                                <div class="form-help">
                                    파일 삭제 후 비어있게 된 폴더를 자동으로 정리합니다
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="createBackup">
                                    삭제 전 백업 생성
                                </label>
                                <div class="form-help">
                                    삭제할 파일들의 백업을 먼저 생성합니다
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="confirmEach">
                                    개별 확인
                                </label>
                                <div class="form-help">
                                    각 파일마다 삭제 여부를 개별적으로 확인합니다
                                </div>
                            </div>
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="skipLargeFiles">
                                    대용량 파일 건너뛰기
                                </label>
                                <div class="form-help">
                                    100MB 이상의 파일은 자동으로 건너뜁니다
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- 중복 그룹 목록 -->
                <div class="card">
                    <div class="card-header">
                        <h2 class="card-title">
                            <i class="fas fa-clone"></i>
                            중복 파일 그룹
                        </h2>
                        <div class="card-actions">
                            <button class="btn btn-secondary btn-sm" id="selectAllBtn">
                                <i class="fas fa-check-square"></i>
                                전체 선택
                            </button>
                            <button class="btn btn-secondary btn-sm" id="selectNoneBtn">
                                <i class="fas fa-square"></i>
                                선택 해제
                            </button>
                            <button class="btn btn-primary btn-sm" id="refreshBtn">
                                <i class="fas fa-sync-alt"></i>
                                새로고침
                            </button>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="duplicate-groups" id="duplicateGroups">
                            ${this.renderDuplicateGroups()}
                        </div>
                    </div>
                </div>

                <!-- 선택된 파일 요약 -->
                <div class="card mt-6" id="selectionSummary" style="display: none;">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="fas fa-info-circle"></i>
                            선택된 파일 요약
                        </h3>
                    </div>
                    <div class="card-body">
                        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                            <div class="stats-card">
                                <div class="stats-value" id="selectedCount">0</div>
                                <div class="stats-label">선택된 파일</div>
                            </div>
                            <div class="stats-card">
                                <div class="stats-value" id="selectedSize">0 MB</div>
                                <div class="stats-label">회수 가능한 용량</div>
                            </div>
                            <div class="stats-card">
                                <div class="stats-value" id="estimatedTime">0분</div>
                                <div class="stats-label">예상 소요 시간</div>
                            </div>
                        </div>
                        <div class="mt-4">
                            <button class="btn btn-danger" id="deleteSelectedBtn" disabled>
                                <i class="fas fa-trash"></i>
                                선택된 파일 삭제
                            </button>
                        </div>
                    </div>
                </div>

                <!-- 최근 정리 기록 -->
                <div class="card mt-6">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="fas fa-history"></i>
                            최근 정리 기록
                        </h3>
                    </div>
                    <div class="card-body">
                        <div class="cleanup-history" id="cleanupHistory">
                            ${this.renderCleanupHistory()}
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    renderDuplicateGroups() {
        if (!this.duplicateGroups || this.duplicateGroups.length === 0) {
            return `
                <div class="empty-state text-center py-8">
                    <div class="empty-icon text-6xl mb-4 opacity-50">
                        <i class="fas fa-search"></i>
                    </div>
                    <h3 class="empty-title text-lg font-semibold mb-2">중복 파일이 없습니다</h3>
                    <p class="empty-description text-sm text-secondary">
                        먼저 파일 스캔을 실행하여 중복 파일을 찾아주세요
                    </p>
                    <button class="btn btn-primary mt-4" onclick="router.navigate('/scan')">
                        <i class="fas fa-search"></i>
                        파일 스캔하기
                    </button>
                </div>
            `;
        }

        return this.duplicateGroups.map(group => `
            <div class="duplicate-group card mb-4" data-group-id="${group.id}">
                <div class="card-header">
                    <div class="flex items-center gap-3">
                        <label class="form-checkbox">
                            <input type="checkbox" class="group-checkbox" data-group-id="${group.id}">
                            그룹 선택
                        </label>
                        <div class="group-info">
                            <div class="group-title font-semibold">${group.filename}</div>
                            <div class="group-meta text-sm text-secondary">
                                ${group.files.length}개 파일 • ${this.formatFileSize(group.totalSize)} • 
                                ${group.files.length - 1}개 중복
                            </div>
                        </div>
                    </div>
                    <button class="btn btn-ghost btn-sm expand-toggle" data-group-id="${group.id}">
                        <i class="fas fa-chevron-down"></i>
                    </button>
                </div>
                <div class="group-files" style="display: none;">
                    ${group.files.map((file, index) => `
                        <div class="file-item p-4 border-t border-primary ${index === 0 ? 'original-file' : ''}">
                            <div class="flex items-center justify-between">
                                <div class="file-info flex-1">
                                    <div class="flex items-center gap-3">
                                        ${index === 0 ? '' : `
                                            <label class="form-checkbox">
                                                <input type="checkbox" class="file-checkbox" 
                                                       data-file-id="${file.id}" data-group-id="${group.id}">
                                            </label>
                                        `}
                                        <div>
                                            <div class="file-name font-medium ${index === 0 ? 'text-success' : ''}">
                                                ${file.name}
                                                ${index === 0 ? '<span class="badge badge-success ml-2">원본</span>' : ''}
                                            </div>
                                            <div class="file-path text-sm text-secondary truncate">
                                                ${file.path}
                                            </div>
                                            <div class="file-meta text-xs text-tertiary">
                                                ${this.formatFileSize(file.size)} • 수정일: ${this.formatDate(file.modifiedTime)}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                                <div class="file-actions flex items-center gap-2">
                                    <button class="btn btn-ghost btn-sm" onclick="window.open('${file.webViewLink}', '_blank')">
                                        <i class="fas fa-external-link-alt"></i>
                                        보기
                                    </button>
                                    ${index === 0 ? '' : `
                                        <button class="btn btn-danger btn-sm" data-file-id="${file.id}">
                                            <i class="fas fa-trash"></i>
                                            삭제
                                        </button>
                                    `}
                                </div>
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `).join('');
    }

    renderCleanupHistory() {
        const history = this.getCleanupHistory();
        
        if (history.length === 0) {
            return `
                <div class="empty-state text-center py-4">
                    <p class="text-secondary">정리 기록이 없습니다</p>
                </div>
            `;
        }

        return history.map(record => `
            <div class="history-item p-4 border-b border-primary last:border-b-0">
                <div class="flex items-center justify-between">
                    <div class="history-info">
                        <div class="history-title font-medium">
                            ${record.deletedFiles}개 파일 삭제
                        </div>
                        <div class="history-meta text-sm text-secondary">
                            ${this.formatFileSize(record.freedSpace)} 확보 • ${this.formatDate(record.timestamp)}
                        </div>
                    </div>
                    <div class="history-status">
                        <span class="badge ${record.success ? 'badge-success' : 'badge-error'}">
                            ${record.success ? '성공' : '실패'}
                        </span>
                    </div>
                </div>
            </div>
        `).join('');
    }

    setupEventListeners() {
        // 새로고침 버튼
        this.addClickListener('#refreshBtn', () => this.loadDuplicateGroups());

        // 전체 선택/해제 버튼
        this.addClickListener('#selectAllBtn', () => this.selectAll());
        this.addClickListener('#selectNoneBtn', () => this.selectNone());

        // 그룹 확장/축소
        this.addEventListener('.expand-toggle', 'click', (e) => {
            const groupId = e.target.closest('.expand-toggle').dataset.groupId;
            this.toggleGroup(groupId);
        });

        // 그룹 체크박스
        this.addEventListener('.group-checkbox', 'change', (e) => {
            const groupId = e.target.dataset.groupId;
            this.toggleGroupSelection(groupId, e.target.checked);
        });

        // 파일 체크박스
        this.addEventListener('.file-checkbox', 'change', (e) => {
            const fileId = e.target.dataset.fileId;
            this.toggleFileSelection(fileId, e.target.checked);
        });

        // 개별 파일 삭제
        this.addEventListener('[data-file-id]', 'click', (e) => {
            if (e.target.closest('.btn-danger')) {
                const fileId = e.target.closest('[data-file-id]').dataset.fileId;
                this.deleteFile(fileId);
            }
        });

        // 선택된 파일 삭제
        this.addClickListener('#deleteSelectedBtn', () => this.deleteSelectedFiles());

        // 상태 변화 감지
        this.stateManager.watch('selectedFiles', () => this.updateSelectionSummary());
    }

    async loadDuplicateGroups() {
        try {
            const response = await fetch('/api/duplicates/groups');
            if (!response.ok) throw new Error('중복 그룹을 불러올 수 없습니다');
            
            this.duplicateGroups = await response.json();
            this.updateDuplicateGroups();
            
        } catch (error) {
            console.error('중복 그룹 로드 실패:', error);
            Toast.show('중복 그룹을 불러오는데 실패했습니다', 'error');
        }
    }

    updateDuplicateGroups() {
        const container = this.element.querySelector('#duplicateGroups');
        if (container) {
            container.innerHTML = this.renderDuplicateGroups();
        }
    }

    toggleGroup(groupId) {
        const groupElement = this.element.querySelector(`[data-group-id="${groupId}"]`);
        const filesContainer = groupElement.querySelector('.group-files');
        const toggleButton = groupElement.querySelector('.expand-toggle i');
        
        if (filesContainer.style.display === 'none') {
            filesContainer.style.display = 'block';
            toggleButton.className = 'fas fa-chevron-up';
        } else {
            filesContainer.style.display = 'none';
            toggleButton.className = 'fas fa-chevron-down';
        }
    }

    toggleGroupSelection(groupId, selected) {
        const group = this.duplicateGroups.find(g => g.id === groupId);
        if (!group) return;

        // 원본 파일을 제외한 모든 파일 선택/해제
        group.files.slice(1).forEach(file => {
            const checkbox = this.element.querySelector(`[data-file-id="${file.id}"]`);
            if (checkbox) {
                checkbox.checked = selected;
                this.toggleFileSelection(file.id, selected);
            }
        });
    }

    toggleFileSelection(fileId, selected) {
        if (selected) {
            this.selectedFiles.add(fileId);
        } else {
            this.selectedFiles.delete(fileId);
        }
        
        this.stateManager.set('selectedFiles', Array.from(this.selectedFiles));
        this.updateSelectionSummary();
    }

    selectAll() {
        this.duplicateGroups.forEach(group => {
            group.files.slice(1).forEach(file => {
                this.selectedFiles.add(file.id);
                const checkbox = this.element.querySelector(`[data-file-id="${file.id}"]`);
                if (checkbox) checkbox.checked = true;
            });
        });
        this.updateSelectionSummary();
    }

    selectNone() {
        this.selectedFiles.clear();
        this.element.querySelectorAll('.file-checkbox').forEach(checkbox => {
            checkbox.checked = false;
        });
        this.updateSelectionSummary();
    }

    updateSelectionSummary() {
        const summary = this.element.querySelector('#selectionSummary');
        const deleteBtn = this.element.querySelector('#deleteSelectedBtn');
        
        if (this.selectedFiles.size === 0) {
            summary.style.display = 'none';
            deleteBtn.disabled = true;
            return;
        }

        summary.style.display = 'block';
        deleteBtn.disabled = false;

        // 선택된 파일 정보 계산
        let totalSize = 0;
        this.duplicateGroups.forEach(group => {
            group.files.forEach(file => {
                if (this.selectedFiles.has(file.id)) {
                    totalSize += file.size;
                }
            });
        });

        // UI 업데이트
        this.element.querySelector('#selectedCount').textContent = this.selectedFiles.size;
        this.element.querySelector('#selectedSize').textContent = this.formatFileSize(totalSize);
        this.element.querySelector('#estimatedTime').textContent = 
            Math.ceil(this.selectedFiles.size / 10) + '분';
    }

    async deleteFile(fileId) {
        if (!confirm('이 파일을 삭제하시겠습니까?')) return;

        try {
            const response = await fetch(`/api/cleanup/files`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ fileIds: [fileId] })
            });

            if (!response.ok) throw new Error('파일 삭제 실패');

            Toast.show('파일이 성공적으로 삭제되었습니다', 'success');
            this.loadDuplicateGroups();
            
        } catch (error) {
            console.error('파일 삭제 실패:', error);
            Toast.show('파일 삭제에 실패했습니다', 'error');
        }
    }

    async deleteSelectedFiles() {
        if (this.selectedFiles.size === 0) return;

        const options = this.getCleanupOptions();
        const confirmMessage = `${this.selectedFiles.size}개의 파일을 삭제하시겠습니까?`;
        
        if (!confirm(confirmMessage)) return;

        this.showProgressModal();
        
        try {
            const response = await fetch('/api/cleanup/files', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    fileIds: Array.from(this.selectedFiles),
                    options
                })
            });

            if (!response.ok) throw new Error('파일 삭제 실패');

            const result = await response.json();
            
            this.hideProgressModal();
            Toast.show(`${result.deletedCount}개 파일이 삭제되었습니다`, 'success');
            
            // 정리 기록 저장
            this.saveCleanupRecord(result);
            
            // UI 새로고침
            this.selectedFiles.clear();
            this.loadDuplicateGroups();
            this.updateSelectionSummary();
            
        } catch (error) {
            console.error('파일 삭제 실패:', error);
            this.hideProgressModal();
            Toast.show('파일 삭제에 실패했습니다', 'error');
        }
    }

    getCleanupOptions() {
        return {
            deleteEmptyFolders: this.element.querySelector('#deleteEmptyFolders').checked,
            createBackup: this.element.querySelector('#createBackup').checked,
            confirmEach: this.element.querySelector('#confirmEach').checked,
            skipLargeFiles: this.element.querySelector('#skipLargeFiles').checked
        };
    }

    showProgressModal() {
        this.progressModal = new Modal({
            title: '파일 삭제 중...',
            content: `
                <div class="text-center py-6">
                    <div class="loading-spinner mx-auto mb-4"></div>
                    <p>파일을 삭제하고 있습니다. 잠시만 기다려주세요.</p>
                </div>
            `,
            showClose: false
        });
        this.progressModal.show();
    }

    hideProgressModal() {
        if (this.progressModal) {
            this.progressModal.hide();
            this.progressModal = null;
        }
    }

    saveCleanupRecord(result) {
        const history = this.getCleanupHistory();
        history.unshift({
            timestamp: new Date().toISOString(),
            deletedFiles: result.deletedCount,
            freedSpace: result.freedSpace,
            success: true
        });
        
        // 최대 10개 기록만 유지
        if (history.length > 10) {
            history.splice(10);
        }
        
        localStorage.setItem('cleanupHistory', JSON.stringify(history));
        
        // UI 업데이트
        const historyContainer = this.element.querySelector('#cleanupHistory');
        if (historyContainer) {
            historyContainer.innerHTML = this.renderCleanupHistory();
        }
    }

    getCleanupHistory() {
        try {
            return JSON.parse(localStorage.getItem('cleanupHistory') || '[]');
        } catch {
            return [];
        }
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatDate(dateString) {
        return new Date(dateString).toLocaleDateString('ko-KR', {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
    }

    async onMount() {
        await this.loadDuplicateGroups();
    }

    onUnmount() {
        if (this.progressModal) {
            this.progressModal.hide();
        }
        if (this.confirmModal) {
            this.confirmModal.hide();
        }
    }
}