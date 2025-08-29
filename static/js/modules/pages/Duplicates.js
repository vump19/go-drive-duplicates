import { Component } from '../components/base/Component.js';
import { ModalFactory } from '../components/base/Modal.js';
import { EVENTS } from '../core/EventBus.js';
import { getState, setState } from '../core/StateManager.js';

/**
 * 중복 파일 페이지 컴포넌트
 */
export class Duplicates extends Component {
    constructor(element) {
        super(element);
        this.currentPage = 1;
        this.pageSize = 20;
        this.totalPages = 1;
        this.duplicateGroups = [];
        this.selectedGroups = new Set();
        this.isSearching = false;
        this.sortBy = 'size'; // size, count, name
        this.sortOrder = 'desc'; // asc, desc
        this.filters = {
            minSize: 0,
            fileTypes: [],
            minCount: 2
        };
    }

    /**
     * Render method for App.js compatibility
     */
    render() {
        const html = this.template();
        if (this.element) {
            this.element.innerHTML = html;
            this.afterRender();
            this.bindEvents();
        }
        return html;
    }

    onInit() {
        this.setupStateWatchers();
        this.setupEventListeners();
        this.render();
        this.loadDuplicateGroups();
    }

    setupStateWatchers() {
        // 중복 파일 상태 워치
        this.watch('duplicates', (duplicateState) => {
            this.isSearching = duplicateState.isSearching || false;
            this.duplicateGroups = duplicateState.groups || [];
            this.currentPage = duplicateState.currentPage || 1;
            this.pageSize = duplicateState.pageSize || 20;
            this.totalPages = duplicateState.totalPages || 1;
            
            this.updateSearchStatus();
            this.renderDuplicateGroups();
            this.updatePagination();
        });
    }

    setupEventListeners() {
        // 중복 검색 관련 이벤트
        this.onEvent(EVENTS.DUPLICATE_SEARCH_START, this.handleSearchStart);
        this.onEvent(EVENTS.DUPLICATE_SEARCH_PROGRESS, this.handleSearchProgress);
        this.onEvent(EVENTS.DUPLICATE_SEARCH_COMPLETE, this.handleSearchComplete);
    }

    template() {
        return `
            <div class="duplicates-container">
                <!-- 검색 컨트롤 카드 -->
                <div class="search-control-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-search"></i>
                            중복 파일 검색
                        </h3>
                        <div class="search-status">
                            <span class="status-indicator" data-status="${this.isSearching ? 'running' : 'idle'}">
                                ${this.isSearching ? '검색 중' : '대기'}
                            </span>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="search-description">
                            <p>SHA-256 해시를 기준으로 정확한 중복 파일을 검색합니다. 
                            검색하기 전에 파일 스캔과 해시 계산이 완료되어야 합니다.</p>
                        </div>
                        
                        <div class="search-controls">
                            <button class="btn btn-primary search-btn" ${this.isSearching ? 'disabled' : ''}>
                                <i class="icon-search"></i>
                                ${this.isSearching ? '검색 중...' : '중복 파일 검색'}
                            </button>
                            
                            <button class="btn btn-secondary refresh-btn">
                                <i class="icon-refresh"></i>
                                목록 새로고침
                            </button>
                            
                            <button class="btn btn-info filters-btn">
                                <i class="icon-filter"></i>
                                필터 설정
                            </button>
                        </div>
                    </div>
                </div>

                <!-- 필터 및 정렬 카드 -->
                <div class="filter-sort-card ${this.showFilters ? 'expanded' : ''}">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-filter"></i>
                            필터 및 정렬
                        </h3>
                        <button class="btn btn-icon toggle-filters-btn">
                            <i class="icon-chevron-${this.showFilters ? 'up' : 'down'}"></i>
                        </button>
                    </div>
                    <div class="card-body filter-content">
                        ${this.renderFiltersAndSort()}
                    </div>
                </div>

                <!-- 중복 그룹 목록 카드 -->
                <div class="duplicate-groups-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-copy"></i>
                            중복 그룹 목록
                            <span class="group-count">(${this.duplicateGroups.length}개 그룹)</span>
                        </h3>
                        <div class="list-actions">
                            <button class="btn btn-sm btn-secondary select-all-btn">
                                <i class="icon-check-square"></i>
                                전체 선택
                            </button>
                            <button class="btn btn-sm btn-danger delete-selected-btn" 
                                    ${this.selectedGroups.size === 0 ? 'disabled' : ''}>
                                <i class="icon-trash"></i>
                                선택 삭제 (${this.selectedGroups.size})
                            </button>
                        </div>
                    </div>
                    <div class="card-body">
                        <div class="duplicate-groups-list" id="duplicate-groups-list">
                            ${this.renderDuplicateGroupsList()}
                        </div>
                        
                        <!-- 페이지네이션 -->
                        <div class="pagination-container">
                            ${this.renderPagination()}
                        </div>
                    </div>
                </div>

                <!-- 통계 카드 -->
                <div class="duplicates-stats-card">
                    <div class="card-header">
                        <h3 class="card-title">
                            <i class="icon-chart"></i>
                            중복 파일 통계
                        </h3>
                    </div>
                    <div class="card-body">
                        ${this.renderDuplicateStats()}
                    </div>
                </div>
            </div>
        `;
    }

    renderFiltersAndSort() {
        return `
            <div class="filters-grid">
                <!-- 파일 크기 필터 -->
                <div class="filter-group">
                    <label class="filter-label">최소 파일 크기</label>
                    <div class="input-group">
                        <input type="number" class="form-input min-size-input" 
                               value="${this.filters.minSize}" min="0" step="1">
                        <select class="form-select size-unit-select">
                            <option value="1">B</option>
                            <option value="1024">KB</option>
                            <option value="1048576" selected>MB</option>
                            <option value="1073741824">GB</option>
                        </select>
                    </div>
                </div>

                <!-- 최소 중복 개수 -->
                <div class="filter-group">
                    <label class="filter-label">최소 중복 개수</label>
                    <input type="number" class="form-input min-count-input" 
                           value="${this.filters.minCount}" min="2" max="100">
                </div>

                <!-- 파일 타입 필터 -->
                <div class="filter-group">
                    <label class="filter-label">파일 타입</label>
                    <div class="checkbox-group">
                        <label class="checkbox-label">
                            <input type="checkbox" class="file-type-checkbox" value="image" 
                                   ${this.filters.fileTypes.includes('image') ? 'checked' : ''}>
                            <span class="checkmark"></span>
                            이미지
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" class="file-type-checkbox" value="video" 
                                   ${this.filters.fileTypes.includes('video') ? 'checked' : ''}>
                            <span class="checkmark"></span>
                            비디오
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" class="file-type-checkbox" value="document" 
                                   ${this.filters.fileTypes.includes('document') ? 'checked' : ''}>
                            <span class="checkmark"></span>
                            문서
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" class="file-type-checkbox" value="audio" 
                                   ${this.filters.fileTypes.includes('audio') ? 'checked' : ''}>
                            <span class="checkmark"></span>
                            오디오
                        </label>
                    </div>
                </div>

                <!-- 정렬 옵션 -->
                <div class="filter-group">
                    <label class="filter-label">정렬 기준</label>
                    <select class="form-select sort-by-select">
                        <option value="size" ${this.sortBy === 'size' ? 'selected' : ''}>파일 크기</option>
                        <option value="count" ${this.sortBy === 'count' ? 'selected' : ''}>중복 개수</option>
                        <option value="name" ${this.sortBy === 'name' ? 'selected' : ''}>파일 이름</option>
                        <option value="date" ${this.sortBy === 'date' ? 'selected' : ''}>수정 날짜</option>
                    </select>
                </div>

                <div class="filter-group">
                    <label class="filter-label">정렬 순서</label>
                    <select class="form-select sort-order-select">
                        <option value="desc" ${this.sortOrder === 'desc' ? 'selected' : ''}>내림차순</option>
                        <option value="asc" ${this.sortOrder === 'asc' ? 'selected' : ''}>오름차순</option>
                    </select>
                </div>
            </div>
            
            <div class="filter-actions">
                <button class="btn btn-primary apply-filters-btn">
                    <i class="icon-check"></i>
                    필터 적용
                </button>
                <button class="btn btn-secondary reset-filters-btn">
                    <i class="icon-refresh"></i>
                    초기화
                </button>
            </div>
        `;
    }

    renderDuplicateGroupsList() {
        if (this.duplicateGroups.length === 0) {
            return this.renderEmptyState();
        }

        return this.duplicateGroups.map((group, index) => 
            this.renderDuplicateGroup(group, index)
        ).join('');
    }

    renderEmptyState() {
        if (this.isSearching) {
            return `
                <div class="loading-state">
                    <div class="spinner"></div>
                    <h4>중복 파일을 검색하는 중...</h4>
                    <p>잠시만 기다려주세요.</p>
                </div>
            `;
        }

        return `
            <div class="empty-state">
                <i class="icon-search"></i>
                <h4>중복 파일이 없습니다</h4>
                <p>중복 파일 검색을 실행하거나 필터 조건을 변경해보세요.</p>
                <button class="btn btn-primary start-search-btn">
                    <i class="icon-search"></i>
                    중복 파일 검색 시작
                </button>
            </div>
        `;
    }

    renderDuplicateGroup(group, index) {
        const isSelected = this.selectedGroups.has(group.id);
        const totalSize = group.totalSize || 0;
        const count = group.count || (group.files ? group.files.length : 0);
        const wastedSpace = totalSize * (count - 1);

        return `
            <div class="duplicate-group ${isSelected ? 'selected' : ''}" data-group-id="${group.id}">
                <div class="group-header">
                    <div class="group-select">
                        <input type="checkbox" class="group-checkbox" 
                               ${isSelected ? 'checked' : ''}>
                    </div>
                    <div class="group-info">
                        <div class="group-title">
                            <strong>그룹 ${group.id}</strong>
                            <span class="group-hash" title="SHA-256 해시">
                                ${group.hash ? group.hash.substring(0, 8) + '...' : 'N/A'}
                            </span>
                        </div>
                        <div class="group-summary">
                            <span class="summary-item">
                                <i class="icon-copy"></i>
                                ${count}개 파일
                            </span>
                            <span class="summary-item">
                                <i class="icon-database"></i>
                                ${this.formatFileSize(totalSize)}
                            </span>
                            <span class="summary-item wasted-space">
                                <i class="icon-trash"></i>
                                낭비: ${this.formatFileSize(wastedSpace)}
                            </span>
                        </div>
                    </div>
                    <div class="group-actions">
                        <button class="btn btn-sm btn-info toggle-files-btn" 
                                data-group-id="${group.id}">
                            <i class="icon-eye"></i>
                            파일 보기
                        </button>
                        <button class="btn btn-sm btn-warning keep-one-btn" 
                                data-group-id="${group.id}">
                            <i class="icon-shield"></i>
                            하나만 보관
                        </button>
                        <button class="btn btn-sm btn-danger delete-group-btn" 
                                data-group-id="${group.id}">
                            <i class="icon-trash"></i>
                            그룹 삭제
                        </button>
                    </div>
                </div>
                
                <div class="group-files collapsed" data-group-id="${group.id}">
                    ${group.files ? this.renderGroupFiles(group.files) : ''}
                </div>
            </div>
        `;
    }

    renderGroupFiles(files) {
        return `
            <div class="files-header">
                <span class="files-title">중복 파일 목록</span>
                <span class="files-count">${files.length}개 파일</span>
            </div>
            <div class="files-list">
                ${files.map((file, index) => this.renderFileItem(file, index)).join('')}
            </div>
        `;
    }

    renderFileItem(file, index) {
        return `
            <div class="file-item" data-file-id="${file.id}">
                <div class="file-select">
                    <input type="checkbox" class="file-checkbox">
                </div>
                <div class="file-icon">
                    <i class="icon-${this.getFileIcon(file.mimeType)}"></i>
                </div>
                <div class="file-info">
                    <div class="file-name" title="${file.name}">
                        ${file.name}
                    </div>
                    <div class="file-path" title="${file.path || '경로 없음'}">
                        ${file.path || '경로 없음'}
                    </div>
                    <div class="file-details">
                        <span class="file-size">${this.formatFileSize(file.size || 0)}</span>
                        <span class="file-date">${this.formatDate(file.modifiedTime)}</span>
                    </div>
                </div>
                <div class="file-actions">
                    <button class="btn btn-sm btn-info preview-btn" 
                            data-file-id="${file.id}" title="미리보기">
                        <i class="icon-eye"></i>
                    </button>
                    <button class="btn btn-sm btn-secondary download-btn" 
                            data-file-id="${file.id}" title="다운로드">
                        <i class="icon-download"></i>
                    </button>
                    <button class="btn btn-sm btn-primary open-drive-btn" 
                            data-file-id="${file.id}" title="Google Drive에서 열기">
                        <i class="icon-external-link"></i>
                    </button>
                    <button class="btn btn-sm btn-danger delete-file-btn" 
                            data-file-id="${file.id}" title="삭제">
                        <i class="icon-trash"></i>
                    </button>
                </div>
            </div>
        `;
    }

    renderPagination() {
        if (this.totalPages <= 1) {
            return '';
        }

        const prevDisabled = this.currentPage <= 1;
        const nextDisabled = this.currentPage >= this.totalPages;

        return `
            <div class="pagination">
                <button class="btn btn-sm pagination-btn" 
                        ${prevDisabled ? 'disabled' : ''} 
                        data-action="prev">
                    <i class="icon-chevron-left"></i>
                    이전
                </button>
                
                <div class="pagination-info">
                    <span>페이지 ${this.currentPage} / ${this.totalPages}</span>
                    <select class="form-select page-size-select">
                        <option value="10" ${this.pageSize === 10 ? 'selected' : ''}>10개씩</option>
                        <option value="20" ${this.pageSize === 20 ? 'selected' : ''}>20개씩</option>
                        <option value="50" ${this.pageSize === 50 ? 'selected' : ''}>50개씩</option>
                        <option value="100" ${this.pageSize === 100 ? 'selected' : ''}>100개씩</option>
                    </select>
                </div>
                
                <button class="btn btn-sm pagination-btn" 
                        ${nextDisabled ? 'disabled' : ''} 
                        data-action="next">
                    다음
                    <i class="icon-chevron-right"></i>
                </button>
            </div>
        `;
    }

    renderDuplicateStats() {
        const totalGroups = this.duplicateGroups.length;
        const totalDuplicates = this.duplicateGroups.reduce((sum, group) => 
            sum + (group.count || 0), 0);
        const totalWastedSpace = this.duplicateGroups.reduce((sum, group) => 
            sum + (group.totalSize || 0) * ((group.count || 0) - 1), 0);
        const averageGroupSize = totalGroups > 0 ? 
            (totalDuplicates / totalGroups).toFixed(1) : 0;

        return `
            <div class="stats-grid">
                <div class="stat-item">
                    <div class="stat-icon">
                        <i class="icon-layers"></i>
                    </div>
                    <div class="stat-content">
                        <div class="stat-value">${totalGroups}</div>
                        <div class="stat-label">중복 그룹</div>
                    </div>
                </div>
                
                <div class="stat-item">
                    <div class="stat-icon">
                        <i class="icon-copy"></i>
                    </div>
                    <div class="stat-content">
                        <div class="stat-value">${totalDuplicates}</div>
                        <div class="stat-label">중복 파일</div>
                    </div>
                </div>
                
                <div class="stat-item">
                    <div class="stat-icon">
                        <i class="icon-trash"></i>
                    </div>
                    <div class="stat-content">
                        <div class="stat-value">${this.formatFileSize(totalWastedSpace)}</div>
                        <div class="stat-label">낭비된 공간</div>
                    </div>
                </div>
                
                <div class="stat-item">
                    <div class="stat-icon">
                        <i class="icon-bar-chart"></i>
                    </div>
                    <div class="stat-content">
                        <div class="stat-value">${averageGroupSize}</div>
                        <div class="stat-label">평균 그룹 크기</div>
                    </div>
                </div>
            </div>
        `;
    }

    bindEvents() {
        // 검색 시작 버튼
        const searchBtn = this.$('.search-btn');
        if (searchBtn) {
            this.addEventListener(searchBtn, 'click', this.handleStartSearch);
        }

        // 새로고침 버튼
        const refreshBtn = this.$('.refresh-btn');
        if (refreshBtn) {
            this.addEventListener(refreshBtn, 'click', this.handleRefresh);
        }

        // 필터 토글 버튼
        const filtersBtn = this.$('.filters-btn');
        if (filtersBtn) {
            this.addEventListener(filtersBtn, 'click', this.toggleFilters);
        }

        // 전체 선택 버튼
        const selectAllBtn = this.$('.select-all-btn');
        if (selectAllBtn) {
            this.addEventListener(selectAllBtn, 'click', this.handleSelectAll);
        }

        // 선택 삭제 버튼
        const deleteSelectedBtn = this.$('.delete-selected-btn');
        if (deleteSelectedBtn) {
            this.addEventListener(deleteSelectedBtn, 'click', this.handleDeleteSelected);
        }

        // 필터 적용 버튼
        const applyFiltersBtn = this.$('.apply-filters-btn');
        if (applyFiltersBtn) {
            this.addEventListener(applyFiltersBtn, 'click', this.handleApplyFilters);
        }

        // 그룹 관련 이벤트들
        this.bindGroupEvents();
        
        // 페이지네이션 이벤트들
        this.bindPaginationEvents();
    }

    bindGroupEvents() {
        // 그룹 체크박스
        this.$$('.group-checkbox').forEach(checkbox => {
            this.addEventListener(checkbox, 'change', this.handleGroupSelect);
        });

        // 파일 보기 토글
        this.$$('.toggle-files-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handleToggleFiles);
        });

        // 하나만 보관 버튼
        this.$$('.keep-one-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handleKeepOne);
        });

        // 그룹 삭제 버튼
        this.$$('.delete-group-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handleDeleteGroup);
        });

        // 파일 관련 버튼들
        this.$$('.delete-file-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handleDeleteFile);
        });

        this.$$('.open-drive-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handleOpenInDrive);
        });
    }

    bindPaginationEvents() {
        // 페이지 버튼들
        this.$$('.pagination-btn').forEach(btn => {
            this.addEventListener(btn, 'click', this.handlePagination);
        });

        // 페이지 크기 변경
        const pageSizeSelect = this.$('.page-size-select');
        if (pageSizeSelect) {
            this.addEventListener(pageSizeSelect, 'change', this.handlePageSizeChange);
        }
    }

    async handleStartSearch() {
        if (this.isSearching) return;

        try {
            this.emit(EVENTS.SHOW_LOADING, '중복 파일을 검색하는 중...');
            
            const response = await this.startDuplicateSearch();
            
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'success',
                message: '중복 파일 검색이 시작되었습니다.'
            });

            setState('duplicates.isSearching', true);

        } catch (error) {
            this.emit(EVENTS.HIDE_LOADING);
            this.emit(EVENTS.SHOW_TOAST, {
                type: 'error',
                message: `검색 실패: ${error.message}`
            });
        }
    }

    handleRefresh() {
        this.loadDuplicateGroups();
    }

    toggleFilters() {
        this.showFilters = !this.showFilters;
        const card = this.$('.filter-sort-card');
        if (card) {
            card.classList.toggle('expanded', this.showFilters);
        }

        const icon = this.$('.toggle-filters-btn i');
        if (icon) {
            icon.className = `icon-chevron-${this.showFilters ? 'up' : 'down'}`;
        }
    }

    handleSelectAll() {
        const allSelected = this.selectedGroups.size === this.duplicateGroups.length;
        
        if (allSelected) {
            this.selectedGroups.clear();
        } else {
            this.duplicateGroups.forEach(group => {
                this.selectedGroups.add(group.id);
            });
        }

        this.updateGroupSelections();
        this.updateSelectedActions();
    }

    handleGroupSelect(event) {
        const checkbox = event.target;
        const groupElement = checkbox.closest('.duplicate-group');
        const groupId = parseInt(groupElement.getAttribute('data-group-id'));

        if (checkbox.checked) {
            this.selectedGroups.add(groupId);
            groupElement.classList.add('selected');
        } else {
            this.selectedGroups.delete(groupId);
            groupElement.classList.remove('selected');
        }

        this.updateSelectedActions();
    }

    async handleDeleteSelected() {
        if (this.selectedGroups.size === 0) return;

        const confirmed = await this.showConfirmDialog(
            '선택된 그룹 삭제',
            `선택된 ${this.selectedGroups.size}개 그룹의 모든 파일을 삭제하시겠습니까?`
        );

        if (confirmed) {
            try {
                this.emit(EVENTS.SHOW_LOADING, '선택된 파일들을 삭제하는 중...');
                
                await this.deleteSelectedGroups(Array.from(this.selectedGroups));
                
                this.emit(EVENTS.HIDE_LOADING);
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'success',
                    message: `${this.selectedGroups.size}개 그룹이 삭제되었습니다.`
                });

                this.selectedGroups.clear();
                this.loadDuplicateGroups();

            } catch (error) {
                this.emit(EVENTS.HIDE_LOADING);
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'error',
                    message: `삭제 실패: ${error.message}`
                });
            }
        }
    }

    handleToggleFiles(event) {
        const button = event.target.closest('button');
        const groupId = button.getAttribute('data-group-id');
        const filesContainer = this.$(`.group-files[data-group-id="${groupId}"]`);
        
        if (filesContainer) {
            const isCollapsed = filesContainer.classList.contains('collapsed');
            
            if (isCollapsed) {
                filesContainer.classList.remove('collapsed');
                button.innerHTML = '<i class="icon-eye-off"></i> 파일 숨김';
            } else {
                filesContainer.classList.add('collapsed');
                button.innerHTML = '<i class="icon-eye"></i> 파일 보기';
            }
        }
    }

    async handleKeepOne(event) {
        const button = event.target.closest('button');
        const groupId = parseInt(button.getAttribute('data-group-id'));
        
        const modal = ModalFactory.custom({
            title: '하나만 보관',
            content: this.renderKeepOneModal(groupId),
            size: 'large',
            buttons: [
                { text: '취소', action: 'cancel' },
                { text: '실행', action: 'confirm', type: 'primary' }
            ]
        });

        modal.onEvent('modal:confirm', async () => {
            try {
                await this.keepOnlyOneFile(groupId);
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'success',
                    message: '중복 파일이 정리되었습니다.'
                });
                this.loadDuplicateGroups();
            } catch (error) {
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'error',
                    message: `정리 실패: ${error.message}`
                });
            }
        });

        modal.open();
    }

    async handleDeleteGroup(event) {
        const button = event.target.closest('button');
        const groupId = parseInt(button.getAttribute('data-group-id'));
        
        const confirmed = await this.showConfirmDialog(
            '그룹 삭제',
            '이 중복 그룹의 모든 파일을 삭제하시겠습니까?'
        );

        if (confirmed) {
            try {
                await this.deleteDuplicateGroup(groupId);
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'success',
                    message: '중복 그룹이 삭제되었습니다.'
                });
                this.loadDuplicateGroups();
            } catch (error) {
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'error',
                    message: `삭제 실패: ${error.message}`
                });
            }
        }
    }

    async handleDeleteFile(event) {
        const button = event.target.closest('button');
        const fileId = button.getAttribute('data-file-id');
        
        const confirmed = await this.showConfirmDialog(
            '파일 삭제',
            '이 파일을 삭제하시겠습니까?'
        );

        if (confirmed) {
            try {
                await this.deleteFile(fileId);
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'success',
                    message: '파일이 삭제되었습니다.'
                });
                this.loadDuplicateGroups();
            } catch (error) {
                this.emit(EVENTS.SHOW_TOAST, {
                    type: 'error',
                    message: `삭제 실패: ${error.message}`
                });
            }
        }
    }

    handleOpenInDrive(event) {
        const button = event.target.closest('button');
        const fileId = button.getAttribute('data-file-id');
        
        const driveUrl = `https://drive.google.com/file/d/${fileId}/view`;
        window.open(driveUrl, '_blank');
    }

    renderKeepOneModal(groupId) {
        const group = this.duplicateGroups.find(g => g.id === groupId);
        if (!group || !group.files) return '';

        return `
            <div class="keep-one-modal">
                <p>다음 중복 파일들 중 하나만 선택하여 보관하고 나머지는 삭제합니다:</p>
                <div class="file-selection-list">
                    ${group.files.map((file, index) => `
                        <label class="file-selection-item">
                            <input type="radio" name="keepFile" value="${file.id}" 
                                   ${index === 0 ? 'checked' : ''}>
                            <div class="file-info">
                                <div class="file-name">${file.name}</div>
                                <div class="file-details">
                                    ${this.formatFileSize(file.size)} • ${this.formatDate(file.modifiedTime)}
                                </div>
                                <div class="file-path">${file.path || '경로 없음'}</div>
                            </div>
                        </label>
                    `).join('')}
                </div>
            </div>
        `;
    }

    // API 호출 메서드들
    async startDuplicateSearch() {
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async loadDuplicateGroups() {
        try {
            const response = await this.fetchDuplicateGroups();
            setState('duplicates.groups', response.groups || []);
            setState('duplicates.totalPages', response.totalPages || 1);
        } catch (error) {
            console.error('중복 그룹 로드 실패:', error);
        }
    }

    async fetchDuplicateGroups() {
        // 모의 구현
        return {
            groups: [],
            totalPages: 1,
            totalGroups: 0
        };
    }

    async deleteSelectedGroups(groupIds) {
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async deleteDuplicateGroup(groupId) {
        return new Promise(resolve => setTimeout(resolve, 500));
    }

    async deleteFile(fileId) {
        return new Promise(resolve => setTimeout(resolve, 500));
    }

    async keepOnlyOneFile(groupId) {
        return new Promise(resolve => setTimeout(resolve, 1000));
    }

    async showConfirmDialog(title, message) {
        return new Promise(resolve => {
            const modal = ModalFactory.confirm(message, title);
            modal.onEvent('modal:confirm', () => resolve(true));
            modal.onEvent('modal:close', () => resolve(false));
            modal.open();
        });
    }

    // 유틸리티 메서드들
    updateSearchStatus() {
        const statusIndicator = this.$('.search-status .status-indicator');
        if (statusIndicator) {
            statusIndicator.setAttribute('data-status', this.isSearching ? 'running' : 'idle');
            statusIndicator.textContent = this.isSearching ? '검색 중' : '대기';
        }

        const searchBtn = this.$('.search-btn');
        if (searchBtn) {
            searchBtn.disabled = this.isSearching;
            searchBtn.innerHTML = this.isSearching ? 
                '<i class="icon-spinner"></i> 검색 중...' : 
                '<i class="icon-search"></i> 중복 파일 검색';
        }
    }

    updateGroupSelections() {
        this.$$('.group-checkbox').forEach(checkbox => {
            const groupElement = checkbox.closest('.duplicate-group');
            const groupId = parseInt(groupElement.getAttribute('data-group-id'));
            const isSelected = this.selectedGroups.has(groupId);
            
            checkbox.checked = isSelected;
            groupElement.classList.toggle('selected', isSelected);
        });
    }

    updateSelectedActions() {
        const deleteBtn = this.$('.delete-selected-btn');
        if (deleteBtn) {
            deleteBtn.disabled = this.selectedGroups.size === 0;
            deleteBtn.innerHTML = `
                <i class="icon-trash"></i>
                선택 삭제 (${this.selectedGroups.size})
            `;
        }
    }

    renderDuplicateGroups() {
        const container = this.$('#duplicate-groups-list');
        if (container) {
            container.innerHTML = this.renderDuplicateGroupsList();
            this.bindGroupEvents();
        }
    }

    updatePagination() {
        const container = this.$('.pagination-container');
        if (container) {
            container.innerHTML = this.renderPagination();
            this.bindPaginationEvents();
        }
    }

    getFileIcon(mimeType) {
        if (!mimeType) return 'file';
        
        if (mimeType.startsWith('image/')) return 'image';
        if (mimeType.startsWith('video/')) return 'video';
        if (mimeType.startsWith('audio/')) return 'music';
        if (mimeType.includes('pdf')) return 'file-pdf';
        if (mimeType.includes('document') || mimeType.includes('text')) return 'file-text';
        if (mimeType.includes('spreadsheet')) return 'file-spreadsheet';
        if (mimeType.includes('presentation')) return 'file-presentation';
        
        return 'file';
    }

    formatFileSize(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    formatDate(date) {
        if (!date) return '날짜 없음';
        return new Date(date).toLocaleDateString('ko-KR');
    }

    handleSearchStart() {
        this.isSearching = true;
        this.updateSearchStatus();
    }

    handleSearchProgress() {
        // 진행률 업데이트 로직
    }

    handleSearchComplete() {
        this.isSearching = false;
        this.updateSearchStatus();
        this.loadDuplicateGroups();
    }

    onDestroy() {
        this.selectedGroups.clear();
        super.onDestroy();
    }
}