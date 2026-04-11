// Global State
let currentTab = 'dashboard';
let currentPage = 1;
let pageSize = 20;
let totalPages = 1;
let duplicateGroups = [];
let progressChart = null;

// Initialize Application
document.addEventListener('DOMContentLoaded', function() {
    console.log('Google Drive 중복 파일 검사기 시작');
    
    // Initialize the dashboard
    initializeDashboard();
    
    // Set up periodic updates
    setInterval(updateDashboard, 30000); // Update every 30 seconds
    
    // Check initial system status
    checkSystemStatus();
    
    // Set up keyboard event handlers
    document.addEventListener('keydown', function(event) {
        if (event.key === 'Escape') {
            const modal = document.getElementById('group-details-modal');
            if (modal && !modal.classList.contains('hidden')) {
                closeGroupDetailsModal();
            }
        }
    });
});

// Tab Management
function showTab(tabName) {
    // Hide all tabs
    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    
    // Remove active class from all buttons
    document.querySelectorAll('.tab-button').forEach(btn => {
        btn.classList.remove('active');
    });
    
    // Show selected tab
    document.getElementById(tabName).classList.add('active');
    
    // Add active class to clicked button
    event.target.classList.add('active');
    
    currentTab = tabName;
    
    // Load tab-specific data
    loadTabData(tabName);
}

function loadTabData(tabName) {
    switch(tabName) {
        case 'dashboard':
            updateDashboard();
            break;
        case 'scan':
            refreshScanProgress();
            break;
        case 'duplicates':
            refreshDuplicates();
            break;
        case 'cleanup':
            updateCleanupProgress();
            break;
        case 'settings':
            loadSettings();
            break;
    }
}

// Dashboard Functions
async function initializeDashboard() {
    await checkSystemStatus();
    await updateStatistics();
    await updateRecentOperations();
    initProgressChart();
}

async function updateDashboard() {
    if (currentTab === 'dashboard') {
        await checkSystemStatus();
        await updateStatistics();
        await updateRecentOperations();
        updateProgressChart();
    }
}

async function checkSystemStatus() {
    const serverStatus = document.getElementById('server-status');
    const dbStatus = document.getElementById('db-status');
    const storageStatus = document.getElementById('storage-status');
    
    // Set checking status
    serverStatus.textContent = '확인 중...';
    serverStatus.className = 'status-value checking';
    
    try {
        // Check server health
        const serverHealth = await API.Health.checkServer();
        serverStatus.textContent = '정상';
        serverStatus.className = 'status-value healthy';
    } catch (error) {
        serverStatus.textContent = '오류';
        serverStatus.className = 'status-value unhealthy';
    }
    
    try {
        // Check database health
        const dbHealth = await API.Health.checkDatabase();
        dbStatus.textContent = '정상';
        dbStatus.className = 'status-value healthy';
    } catch (error) {
        dbStatus.textContent = '오류';
        dbStatus.className = 'status-value unhealthy';
    }
    
    try {
        // Check storage health
        const storageHealth = await API.Health.checkStorage();
        storageStatus.textContent = '정상';
        storageStatus.className = 'status-value healthy';
    } catch (error) {
        storageStatus.textContent = '오류';
        storageStatus.className = 'status-value unhealthy';
    }
}

async function updateStatistics() {
    try {
        // Get duplicate groups to calculate statistics
        const groupsData = await API.Duplicate.getDuplicateGroups(1, 100); // Get more groups for better stats
        
        if (groupsData && Array.isArray(groupsData)) {
            // Calculate statistics from the groups
            let totalDuplicateFiles = 0;
            let totalWastedSpace = 0;
            
            groupsData.forEach(group => {
                totalDuplicateFiles += group.count || 0;
                const individualFileSize = group.files && group.files.length > 0 ? 
                    group.files[0].size : (group.totalSize || 0) / (group.count || 1);
                totalWastedSpace += (group.count - 1) * individualFileSize;
            });
            
            // Update statistics display
            document.getElementById('total-files').textContent = APIUtils.formatNumber(totalDuplicateFiles);
            document.getElementById('duplicate-groups').textContent = APIUtils.formatNumber(groupsData.length);
            document.getElementById('wasted-space').textContent = APIUtils.formatFileSize(totalWastedSpace);
        } else {
            document.getElementById('total-files').textContent = '0';
            document.getElementById('duplicate-groups').textContent = '0';
            document.getElementById('wasted-space').textContent = '0 B';
        }
        
    } catch (error) {
        console.error('Failed to update statistics:', error);
        document.getElementById('total-files').textContent = '오류';
        document.getElementById('duplicate-groups').textContent = '오류';
        document.getElementById('wasted-space').textContent = '오류';
    }
}

async function updateRecentOperations() {
    const container = document.getElementById('recent-operations');
    
    try {
        // This would need a dedicated API endpoint for recent operations
        container.innerHTML = '<p class="text-center">최근 작업 내역 API가 필요합니다.</p>';
    } catch (error) {
        container.innerHTML = '<p class="text-center">작업 내역을 불러올 수 없습니다.</p>';
    }
}

function initProgressChart() {
    const ctx = document.getElementById('progress-chart').getContext('2d');
    
    progressChart = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: ['완료된 작업', '진행 중인 작업', '대기 중인 작업'],
            datasets: [{
                data: [0, 0, 0],
                backgroundColor: ['#28a745', '#007bff', '#ffc107'],
                borderWidth: 0
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    position: 'bottom'
                }
            }
        }
    });
}

function updateProgressChart() {
    if (progressChart) {
        // This would be updated with real progress data
        progressChart.data.datasets[0].data = [5, 2, 1];
        progressChart.update();
    }
}

// File Scan Functions
async function startFileScan() {
    const button = document.getElementById('start-scan-btn');
    button.disabled = true;
    button.textContent = '스캔 시작 중...';
    
    try {
        showLoading('파일 스캔을 시작하는 중...');
        await API.File.startScan();
        showNotification('파일 스캔이 시작되었습니다.', 'success');
        
        // Start monitoring progress
        setTimeout(refreshScanProgress, 2000);
        
    } catch (error) {
        APIUtils.handleAPIError(error, '파일 스캔 시작');
    } finally {
        hideLoading();
        button.disabled = false;
        button.textContent = '전체 스캔 시작';
    }
}

async function calculateHashes() {
    const button = document.getElementById('calculate-hashes-btn');
    button.disabled = true;
    button.textContent = '해시 계산 중...';
    
    try {
        showLoading('해시 계산을 시작하는 중...');
        await API.File.calculateHashes();
        showNotification('해시 계산이 시작되었습니다.', 'success');
        
        // Start monitoring progress
        setTimeout(refreshScanProgress, 2000);
        
    } catch (error) {
        APIUtils.handleAPIError(error, '해시 계산 시작');
    } finally {
        hideLoading();
        button.disabled = false;
        button.textContent = '해시 계산';
    }
}

async function scanFolder() {
    const folderIdInput = document.getElementById('folder-id-input');
    const folderId = folderIdInput.value.trim();
    
    if (!folderId) {
        showNotification('폴더 ID를 입력하세요.', 'warning');
        return;
    }
    
    try {
        showLoading('폴더 스캔을 시작하는 중...');
        await API.File.scanFolder(folderId);
        showNotification('폴더 스캔이 시작되었습니다.', 'success');
        
        // Clear input and refresh progress
        folderIdInput.value = '';
        setTimeout(refreshScanProgress, 2000);
        
    } catch (error) {
        APIUtils.handleAPIError(error, '폴더 스캔');
    } finally {
        hideLoading();
    }
}

async function refreshScanProgress() {
    const container = document.getElementById('scan-progress');
    
    try {
        const progress = await API.File.getScanProgress();
        
        if (progress) {
            container.innerHTML = createProgressDisplay(progress);
        } else {
            container.innerHTML = '<p>스캔 진행 정보가 없습니다.</p>';
        }
    } catch (error) {
        container.innerHTML = '<p>진행 상황을 불러올 수 없습니다.</p>';
        console.error('Failed to refresh scan progress:', error);
    }
}

// Duplicate Functions
async function findDuplicates() {
    try {
        showLoading('중복 파일을 검색하는 중...');
        await API.Duplicate.findDuplicates();
        showNotification('중복 파일 검색이 시작되었습니다.', 'success');
        
        // Refresh the duplicates list after a delay
        setTimeout(refreshDuplicates, 3000);
        
    } catch (error) {
        APIUtils.handleAPIError(error, '중복 파일 검색');
    } finally {
        hideLoading();
    }
}

async function refreshDuplicates() {
    const container = document.getElementById('duplicate-groups-container');
    container.innerHTML = '<p class="loading">중복 그룹을 불러오는 중...</p>';
    
    try {
        const data = await API.Duplicate.getDuplicateGroups(currentPage, pageSize);
        
        // 새로운 페이지네이션 응답 형식 처리
        if (data && data.groups && Array.isArray(data.groups)) {
            duplicateGroups = data.groups;
            totalPages = data.totalPages || 1;
            
            if (data.groups.length > 0) {
                displayDuplicateGroups(data.groups);
                updatePagination(data.totalGroups, data.currentPage, data.totalPages, data.hasNext, data.hasPrev);
            } else {
                container.innerHTML = '<p class="text-center">현재 페이지에 중복 그룹이 없습니다.</p>';
                updatePagination(data.totalGroups, data.currentPage, data.totalPages, data.hasNext, data.hasPrev);
            }
        } else if (data && Array.isArray(data)) {
            // 이전 API 형식과의 호환성 (배열 직접 반환)
            duplicateGroups = data;
            totalPages = Math.max(1, Math.ceil(data.length / pageSize));
            
            if (data.length > 0) {
                displayDuplicateGroups(data);
                updatePagination(data.length);
            } else {
                container.innerHTML = '<p class="text-center">중복 그룹이 없습니다.</p>';
            }
        } else {
            container.innerHTML = '<p class="text-center">중복 그룹을 불러올 수 없습니다.</p>';
        }
    } catch (error) {
        container.innerHTML = '<p class="text-center">중복 그룹을 불러올 수 없습니다.</p>';
        APIUtils.handleAPIError(error, '중복 그룹 조회');
    }
}

function displayDuplicateGroups(groups) {
    const container = document.getElementById('duplicate-groups-container');
    
    if (!groups || groups.length === 0) {
        container.innerHTML = '<p class="text-center">중복 그룹이 없습니다.</p>';
        return;
    }
    
    let html = '';
    
    groups.forEach((group, index) => {
        html += createDuplicateGroupHTML(group, index);
    });
    
    container.innerHTML = html;
}

function createDuplicateGroupHTML(group, index) {
    const totalSize = group.totalSize || 0;
    const count = group.count || (group.files ? group.files.length : 0);
    // 낭비 공간 계산: (파일 수 - 1) * 개별 파일 크기
    const individualFileSize = group.files && group.files.length > 0 ? group.files[0].size : totalSize / count;
    const wastedSpace = (count - 1) * individualFileSize;
    
    let html = `
        <div class="duplicate-group" style="border: 1px solid #ddd; margin: 10px 0; padding: 15px; border-radius: 5px; background: #f9f9f9;">
            <div class="group-header" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                <div class="group-info">
                    <strong style="font-size: 18px; color: #333;">그룹 ${group.id || index + 1}</strong>
                    <br>
                    <small style="color: #666;">
                        파일 수: <span style="font-weight: bold; color: #e74c3c;">${count}개</span> | 
                        개별 크기: <span style="font-weight: bold;">${APIUtils.formatFileSize(individualFileSize)}</span> | 
                        총 크기: <span style="font-weight: bold;">${APIUtils.formatFileSize(totalSize)}</span> | 
                        <span style="color: #e74c3c; font-weight: bold;">절약 가능: ${APIUtils.formatFileSize(wastedSpace)}</span>
                    </small>
                </div>
                <div class="group-actions">
                    <button class="btn btn-info" onclick="showGroupDetails(${group.id})" style="margin-right: 5px;">상세 보기</button>
                    <button class="btn btn-danger" onclick="deleteDuplicateGroup(${group.id})">그룹 삭제</button>
                </div>
            </div>
    `;
    
    if (group.files && group.files.length > 0) {
        html += '<div class="group-files" style="border-top: 1px solid #eee; padding-top: 10px;">';
        html += `
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
                <div style="font-weight: bold; color: #555;">중복 파일 목록 (${group.files.length}개):</div>
                <div style="display: flex; gap: 10px; align-items: center;">
                    <span style="font-size: 12px; color: #666;">정렬:</span>
                    <select id="sort-files" onchange="sortFiles(this.value)" style="padding: 2px 5px; font-size: 12px; border: 1px solid #ddd; border-radius: 3px;">
                        <option value="created-asc">생성일 (오래된 순)</option>
                        <option value="created-desc">생성일 (최신 순)</option>
                        <option value="modified-asc">수정일 (오래된 순)</option>
                        <option value="modified-desc">수정일 (최신 순)</option>
                        <option value="name-asc">이름 (가나다 순)</option>
                    </select>
                </div>
            </div>
            <div id="files-container">
        `;
        
        // 파일을 생성일 오래된 순으로 기본 정렬
        const sortedFiles = sortFilesByCreatedDate(group.files);
        sortedFiles.forEach((file, fileIndex) => {
            html += createFileItemHTML(file, fileIndex);
        });
        html += '</div></div>';
    }
    
    html += '</div>';
    
    return html;
}

function createFileItemHTML(file, fileIndex) {
    const createdDate = file.createdTime ? new Date(file.createdTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';
    const modifiedDate = file.modifiedTime ? new Date(file.modifiedTime).toLocaleDateString('ko-KR') : '날짜 정보 없음';
    const mimeTypeDisplay = file.mimeType || 'application/octet-stream';
    const fileTypeIcon = getFileTypeIcon(mimeTypeDisplay);
    
    // 첫 번째 파일 (가장 오래된 원본)을 다르게 표시
    const isOriginal = fileIndex === 0;
    const borderColor = isOriginal ? '#10b981' : '#e0e0e0';
    const backgroundColor = isOriginal ? '#f0fdf4' : 'white';
    const originalBadge = isOriginal ? '<span style="background: #10b981; color: white; font-size: 10px; padding: 2px 6px; border-radius: 10px; margin-left: 8px;">원본 추천</span>' : '';
    
    return `
        <div class="file-item" style="display: flex; justify-content: space-between; align-items: center; padding: 8px; margin: 5px 0; background: ${backgroundColor}; border: 2px solid ${borderColor}; border-radius: 5px; position: relative;">
            <div class="file-info" style="flex: 1; min-width: 0;">
                <div class="file-name" style="font-weight: 500; color: #333; display: flex; align-items: center;">
                    <span style="margin-right: 8px;">${fileTypeIcon}</span>
                    <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">${file.name || '알 수 없는 파일'}</span>
                    ${originalBadge}
                </div>
                <div class="file-details" style="font-size: 12px; color: #666; margin-top: 2px;">
                    <span style="font-weight: 600; color: #2563eb;">📅 생성일: ${createdDate}</span> | 
                    <span>✏️ 수정일: ${modifiedDate}</span> | 
                    <span>📄 타입: ${mimeTypeDisplay}</span>
                    ${file.webViewLink ? ` | <a href="${file.webViewLink}" target="_blank" style="color: #4285f4;">Drive에서 보기</a>` : ''}
                </div>
                ${isOriginal ? '<div style="font-size: 11px; color: #059669; font-weight: 600; margin-top: 3px;">💡 가장 오래된 파일로 원본일 가능성이 높습니다.</div>' : ''}
            </div>
            <div class="file-size" style="font-weight: bold; color: #555; margin-left: 10px;">
                ${APIUtils.formatFileSize(file.size || 0)}
            </div>
        </div>
    `;
}

function getFileTypeIcon(mimeType) {
    if (mimeType.startsWith('image/')) return '🖼️';
    if (mimeType.startsWith('video/')) return '🎥';
    if (mimeType.startsWith('audio/')) return '🎵';
    if (mimeType.includes('pdf')) return '📄';
    if (mimeType.includes('zip') || mimeType.includes('rar') || mimeType.includes('compressed')) return '📦';
    if (mimeType.includes('document') || mimeType.includes('word')) return '📝';
    if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) return '📊';
    if (mimeType.includes('presentation') || mimeType.includes('powerpoint')) return '📽️';
    return '📄';
}

// 파일 정렬 함수들
function sortFilesByCreatedDate(files, ascending = true) {
    return [...files].sort((a, b) => {
        const dateA = new Date(a.createdTime || '1970-01-01');
        const dateB = new Date(b.createdTime || '1970-01-01');
        return ascending ? dateA - dateB : dateB - dateA;
    });
}

function sortFilesByModifiedDate(files, ascending = true) {
    return [...files].sort((a, b) => {
        const dateA = new Date(a.modifiedTime || '1970-01-01');
        const dateB = new Date(b.modifiedTime || '1970-01-01');
        return ascending ? dateA - dateB : dateB - dateA;
    });
}

function sortFilesByName(files, ascending = true) {
    return [...files].sort((a, b) => {
        const nameA = (a.name || '').toLowerCase();
        const nameB = (b.name || '').toLowerCase();
        return ascending ? nameA.localeCompare(nameB) : nameB.localeCompare(nameA);
    });
}

// 현재 표시된 그룹의 파일들을 저장할 변수
let currentGroupFiles = [];

// 파일 정렬 함수 (드롭다운에서 호출)
function sortFiles(sortType) {
    let sortedFiles;
    
    switch(sortType) {
        case 'created-asc':
            sortedFiles = sortFilesByCreatedDate(currentGroupFiles, true);
            break;
        case 'created-desc':
            sortedFiles = sortFilesByCreatedDate(currentGroupFiles, false);
            break;
        case 'modified-asc':
            sortedFiles = sortFilesByModifiedDate(currentGroupFiles, true);
            break;
        case 'modified-desc':
            sortedFiles = sortFilesByModifiedDate(currentGroupFiles, false);
            break;
        case 'name-asc':
            sortedFiles = sortFilesByName(currentGroupFiles, true);
            break;
        default:
            sortedFiles = sortFilesByCreatedDate(currentGroupFiles, true);
    }
    
    // 파일 목록 컨테이너를 다시 렌더링
    const container = document.getElementById('files-container');
    if (container) {
        let html = '';
        sortedFiles.forEach((file, fileIndex) => {
            html += createFileItemHTML(file, fileIndex);
        });
        container.innerHTML = html;
    }
}

let currentGroupId = null; // 현재 표시 중인 그룹 ID 저장

async function showGroupDetails(groupId) {
    try {
        showLoading('그룹 상세 정보를 불러오는 중...');
        const group = await API.Duplicate.getDuplicateGroup(groupId);
        
        if (group) {
            currentGroupId = groupId; // 현재 그룹 ID 저장
            displayGroupDetailsModal(group);
        }
    } catch (error) {
        APIUtils.handleAPIError(error, '그룹 상세 정보 조회');
    } finally {
        hideLoading();
    }
}

function displayGroupDetailsModal(group) {
    const modal = document.getElementById('group-details-modal');
    const content = document.getElementById('group-details-content');
    
    // 현재 그룹의 파일들을 전역 변수에 저장 (정렬을 위해)
    currentGroupFiles = group.files || [];
    
    // 그룹 요약 정보 생성
    const wastedSpace = group.files && group.files.length > 0 ? 
        (group.count - 1) * group.files[0].size : 0;
    
    const totalSize = group.totalSize || (group.files && group.files.length > 0 ? 
        group.files[0].size * group.count : 0);
    
    const oldestFile = group.files ? getOldestFile(group.files) : null;
    const newestFile = group.files ? getNewestFile(group.files) : null;
    
    let html = `
        <div class="group-info">
            <div class="group-summary">
                <div class="group-summary-grid">
                    <div class="summary-item">
                        <div class="summary-value">${group.count || 0}</div>
                        <div class="summary-label">중복 파일 수</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value">${APIUtils.formatFileSize(totalSize)}</div>
                        <div class="summary-label">총 크기</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value" style="color: #ef4444;">${APIUtils.formatFileSize(wastedSpace)}</div>
                        <div class="summary-label">절약 가능 용량</div>
                    </div>
                    <div class="summary-item">
                        <div class="summary-value" style="font-size: 1rem; font-family: monospace;">${group.hash ? group.hash.substring(0, 12) + '...' : 'N/A'}</div>
                        <div class="summary-label">파일 해시</div>
                    </div>
                </div>
            </div>
            
            ${oldestFile || newestFile ? `
                <div style="margin-bottom: 16px; padding: 12px; background: #f8fafc; border-radius: 6px; font-size: 0.875rem; color: #6b7280;">
                    ${oldestFile ? `<div><strong>가장 오래된 파일:</strong> ${oldestFile.name} (${APIUtils.formatDate(oldestFile.modifiedTime)})</div>` : ''}
                    ${newestFile ? `<div><strong>가장 최신 파일:</strong> ${newestFile.name} (${APIUtils.formatDate(newestFile.modifiedTime)})</div>` : ''}
                </div>
            ` : ''}
        </div>
        
        <div class="files-list">
            <h3><i class="fas fa-list"></i> 중복 파일 목록 (${group.files ? group.files.length : 0}개)</h3>
    `;
    
    if (group.files && group.files.length > 0) {
        group.files.forEach((file, index) => {
            const fileTypeIcon = getFileTypeIcon(file.mimeType || 'application/octet-stream');
            const modifiedDate = file.modifiedTime ? APIUtils.formatDate(file.modifiedTime) : '날짜 정보 없음';
            
            html += `
                <div class="file-item-modal">
                    <div class="file-info-modal">
                        <div class="file-name-modal">
                            <span class="file-type-icon">${fileTypeIcon}</span>
                            <span class="file-name-text" title="${file.name || '알 수 없는 파일'}">${file.name || '알 수 없는 파일'}</span>
                        </div>
                        <div class="file-details-modal">
                            <span><i class="fas fa-calendar"></i> ${modifiedDate}</span>
                            <span><i class="fas fa-tag"></i> ${file.mimeType || 'application/octet-stream'}</span>
                            ${file.path ? `<span><i class="fas fa-folder"></i> ${file.path}</span>` : ''}
                        </div>
                    </div>
                    <div class="file-actions-modal">
                        <div class="file-size-modal">${APIUtils.formatFileSize(file.size || 0)}</div>
                        ${file.webViewLink ? `<a href="${file.webViewLink}" target="_blank" class="btn-sm btn-primary" title="Google Drive에서 열기"><i class="fas fa-external-link-alt"></i></a>` : ''}
                        <button class="btn-sm btn-danger-sm" onclick="deleteFileFromGroup('${file.id}', '${group.id}')" title="파일 삭제">
                            <i class="fas fa-trash"></i>
                        </button>
                    </div>
                </div>
            `;
        });
    } else {
        html += '<p style="text-align: center; color: #6b7280; padding: 20px;">파일 정보를 불러올 수 없습니다.</p>';
    }
    
    html += '</div>';
    
    content.innerHTML = html;
    modal.classList.remove('hidden');
}

function closeGroupDetailsModal() {
    const modal = document.getElementById('group-details-modal');
    modal.classList.add('hidden');
    currentGroupId = null;
}

function getOldestFile(files) {
    if (!files || files.length === 0) return null;
    return files.reduce((oldest, file) => {
        if (!oldest.modifiedTime) return file;
        if (!file.modifiedTime) return oldest;
        return new Date(file.modifiedTime) < new Date(oldest.modifiedTime) ? file : oldest;
    });
}

function getNewestFile(files) {
    if (!files || files.length === 0) return null;
    return files.reduce((newest, file) => {
        if (!newest.modifiedTime) return file;
        if (!file.modifiedTime) return newest;
        return new Date(file.modifiedTime) > new Date(newest.modifiedTime) ? file : newest;
    });
}

async function deleteCurrentGroup() {
    if (!currentGroupId) return;
    
    if (!confirm(`이 중복 그룹(ID: ${currentGroupId})을 삭제하시겠습니까?`)) {
        return;
    }
    
    try {
        await API.Duplicate.deleteDuplicateGroup(currentGroupId);
        showNotification('중복 그룹이 삭제되었습니다.', 'success');
        closeGroupDetailsModal();
        refreshDuplicates(); // 목록 새로고침
    } catch (error) {
        APIUtils.handleAPIError(error, '중복 그룹 삭제');
    }
}

async function deleteFileFromGroup(fileId, groupId) {
    if (!confirm('이 파일을 삭제하시겠습니까?')) return;
    
    try {
        showLoading('파일을 삭제하는 중...');
        // 개별 파일 삭제 API 호출 (구현 필요)
        await API.Cleanup.deleteFiles([fileId]);
        showNotification('파일이 삭제되었습니다.', 'success');
        
        // 그룹 정보 다시 로드
        showGroupDetails(groupId);
    } catch (error) {
        APIUtils.handleAPIError(error, '파일 삭제');
    } finally {
        hideLoading();
    }
}

async function deleteDuplicateGroup(groupId) {
    if (!confirm('이 중복 그룹을 삭제하시겠습니까?')) {
        return;
    }
    
    try {
        showLoading('중복 그룹을 삭제하는 중...');
        await API.Duplicate.deleteDuplicateGroup(groupId);
        showNotification('중복 그룹이 삭제되었습니다.', 'success');
        
        // Refresh the list
        refreshDuplicates();
        
    } catch (error) {
        APIUtils.handleAPIError(error, '중복 그룹 삭제');
    } finally {
        hideLoading();
    }
}

async function getDuplicateProgress() {
    try {
        const progress = await API.Duplicate.getDuplicateProgress();
        
        if (progress) {
            const message = `진행률: ${progress.processedItems || 0}/${progress.totalItems || 0} (${progress.status || 'unknown'})`;
            showNotification(message, 'info');
        } else {
            showNotification('진행 상황 정보가 없습니다.', 'warning');
        }
    } catch (error) {
        APIUtils.handleAPIError(error, '중복 검색 진행 상황 확인');
    }
}

// Cleanup Functions
async function deleteSpecificFiles() {
    const input = document.getElementById('file-ids-input');
    const fileIds = input.value.trim().split('\n').filter(id => id.trim());
    
    if (fileIds.length === 0) {
        showNotification('삭제할 파일 ID를 입력하세요.', 'warning');
        return;
    }
    
    if (!confirm(`${fileIds.length}개의 파일을 삭제하시겠습니까?`)) {
        return;
    }
    
    try {
        showLoading('파일을 삭제하는 중...');
        await API.Cleanup.deleteFiles(fileIds);
        showNotification(`${fileIds.length}개의 파일이 삭제되었습니다.`, 'success');
        
        // Clear input
        input.value = '';
        
    } catch (error) {
        APIUtils.handleAPIError(error, '파일 삭제');
    } finally {
        hideLoading();
    }
}

async function searchByPattern() {
    const patternInput = document.getElementById('pattern-input');
    const pattern = patternInput.value.trim();
    
    if (!pattern) {
        showNotification('검색 패턴을 입력하세요.', 'warning');
        return;
    }
    
    try {
        showLoading('패턴에 매칭되는 파일을 검색하는 중...');
        const results = await API.Cleanup.searchByPattern(pattern);
        
        const container = document.getElementById('pattern-results');
        
        if (results && results.files && results.files.length > 0) {
            let html = `<h4>검색 결과: ${results.files.length}개 파일</h4>`;
            results.files.forEach(file => {
                html += `<div class="file-item"><div class="file-info"><div class="file-name">${file.name}</div><div class="file-path">${file.path || ''}</div></div><div class="file-size">${APIUtils.formatFileSize(file.size || 0)}</div></div>`;
            });
            container.innerHTML = html;
        } else {
            container.innerHTML = '<p>패턴에 매칭되는 파일이 없습니다.</p>';
        }
        
    } catch (error) {
        document.getElementById('pattern-results').innerHTML = '<p>검색 중 오류가 발생했습니다.</p>';
        APIUtils.handleAPIError(error, '패턴 검색');
    } finally {
        hideLoading();
    }
}

async function deleteByPattern() {
    const patternInput = document.getElementById('pattern-input');
    const pattern = patternInput.value.trim();
    
    if (!pattern) {
        showNotification('삭제 패턴을 입력하세요.', 'warning');
        return;
    }
    
    if (!confirm(`패턴 "${pattern}"에 매칭되는 모든 파일을 삭제하시겠습니까?`)) {
        return;
    }
    
    try {
        showLoading('패턴에 매칭되는 파일을 삭제하는 중...');
        const result = await API.Cleanup.deleteByPattern(pattern);
        
        if (result && result.deletedCount !== undefined) {
            showNotification(`${result.deletedCount}개의 파일이 삭제되었습니다.`, 'success');
        } else {
            showNotification('파일 삭제가 완료되었습니다.', 'success');
        }
        
        // Clear results
        document.getElementById('pattern-results').innerHTML = '';
        
    } catch (error) {
        APIUtils.handleAPIError(error, '패턴 기반 파일 삭제');
    } finally {
        hideLoading();
    }
}

async function cleanupEmptyFolders() {
    if (!confirm('빈 폴더를 모두 삭제하시겠습니까?')) {
        return;
    }
    
    try {
        showLoading('빈 폴더를 정리하는 중...');
        const result = await API.Cleanup.cleanupEmptyFolders();
        
        if (result && result.deletedCount !== undefined) {
            showNotification(`${result.deletedCount}개의 빈 폴더가 삭제되었습니다.`, 'success');
        } else {
            showNotification('빈 폴더 정리가 완료되었습니다.', 'success');
        }
        
    } catch (error) {
        APIUtils.handleAPIError(error, '빈 폴더 정리');
    } finally {
        hideLoading();
    }
}

async function updateCleanupProgress() {
    const container = document.getElementById('cleanup-progress');
    
    try {
        const progress = await API.Cleanup.getCleanupProgress();
        
        if (progress) {
            container.innerHTML = createProgressDisplay(progress);
        } else {
            container.innerHTML = '<p>정리 작업 정보가 없습니다.</p>';
        }
    } catch (error) {
        container.innerHTML = '<p>정리 진행 상황을 불러올 수 없습니다.</p>';
        console.error('Failed to update cleanup progress:', error);
    }
}

// Pagination Functions
function updatePagination(totalGroups, pageCurrent, pagesTotal, hasNext, hasPrev) {
    // Support both new pagination format and legacy format
    if (arguments.length >= 3) {
        // New paginated API format
        totalPages = pagesTotal;
        currentPage = pageCurrent;
        
        // Update page info display
        const pageInfoElement = document.getElementById('page-info');
        if (pageInfoElement) {
            pageInfoElement.textContent = `페이지 ${currentPage} / ${totalPages} (총 ${totalGroups}개 그룹)`;
        }
        
        // Update navigation buttons
        const prevBtn = document.getElementById('prev-btn');
        const nextBtn = document.getElementById('next-btn');
        if (prevBtn) prevBtn.disabled = !hasPrev;
        if (nextBtn) nextBtn.disabled = !hasNext;
    } else {
        // Legacy format (totalGroups only)
        totalPages = Math.ceil(totalGroups / pageSize);
        
        const pageInfoElement = document.getElementById('page-info');
        if (pageInfoElement) {
            pageInfoElement.textContent = `페이지 ${currentPage} / ${totalPages}`;
        }
        
        const prevBtn = document.getElementById('prev-btn');
        const nextBtn = document.getElementById('next-btn');
        if (prevBtn) prevBtn.disabled = currentPage <= 1;
        if (nextBtn) nextBtn.disabled = currentPage >= totalPages;
    }
}

function prevPage() {
    if (currentPage > 1) {
        currentPage--;
        refreshDuplicates();
    }
}

function nextPage() {
    if (currentPage < totalPages) {
        currentPage++;
        refreshDuplicates();
    }
}

function changePageSize() {
    const newPageSize = parseInt(document.getElementById('page-size').value);
    if (newPageSize !== pageSize) {
        pageSize = newPageSize;
        currentPage = 1;
        refreshDuplicates();
    }
}

// Settings Functions
function loadSettings() {
    // Load current settings (would come from API)
    document.getElementById('worker-count').value = 4;
    document.getElementById('batch-size').value = 100;
    document.getElementById('api-base-url').textContent = API_BASE;
}

function applySettings() {
    const workerCount = parseInt(document.getElementById('worker-count').value);
    const batchSize = parseInt(document.getElementById('batch-size').value);
    
    // This would send settings to the API
    showNotification('설정이 적용되었습니다.', 'success');
}

// Utility Functions
function createProgressDisplay(progress) {
    const percentage = progress.totalItems > 0 ? 
        Math.round((progress.processedItems / progress.totalItems) * 100) : 0;
    
    return `
        <div class="progress-text">
            상태: ${progress.status || 'unknown'} | 
            진행률: ${progress.processedItems || 0}/${progress.totalItems || 0} (${percentage}%)
        </div>
        <div class="progress-bar">
            <div class="progress-fill" style="width: ${percentage}%"></div>
        </div>
        <div class="progress-text">
            현재 단계: ${progress.currentStep || '정보 없음'}
        </div>
        ${progress.startTime ? `<div class="progress-text">시작 시간: ${APIUtils.formatDate(progress.startTime)}</div>` : ''}
    `;
}

function showNotification(message, type = 'info') {
    const notification = document.getElementById('notification');
    const messageElement = document.getElementById('notification-message');
    
    messageElement.textContent = message;
    notification.className = `notification ${type}`;
    notification.classList.remove('hidden');
    
    // Auto hide after 5 seconds
    setTimeout(() => {
        hideNotification();
    }, 5000);
}

function hideNotification() {
    document.getElementById('notification').classList.add('hidden');
}

function showLoading(message = '처리 중...') {
    document.getElementById('loading-message').textContent = message;
    document.getElementById('loading-overlay').classList.remove('hidden');
}

function hideLoading() {
    document.getElementById('loading-overlay').classList.add('hidden');
}

// Export functions for global access
window.showTab = showTab;
window.startFileScan = startFileScan;
window.calculateHashes = calculateHashes;
window.scanFolder = scanFolder;
window.refreshScanProgress = refreshScanProgress;
window.findDuplicates = findDuplicates;
window.refreshDuplicates = refreshDuplicates;
window.showGroupDetails = showGroupDetails;
window.deleteDuplicateGroup = deleteDuplicateGroup;
window.getDuplicateProgress = getDuplicateProgress;
window.deleteSpecificFiles = deleteSpecificFiles;
window.searchByPattern = searchByPattern;
window.deleteByPattern = deleteByPattern;
window.cleanupEmptyFolders = cleanupEmptyFolders;
window.prevPage = prevPage;
window.nextPage = nextPage;
window.changePageSize = changePageSize;
window.applySettings = applySettings;
window.hideNotification = hideNotification;
window.closeGroupDetailsModal = closeGroupDetailsModal;
window.deleteCurrentGroup = deleteCurrentGroup;
window.deleteFileFromGroup = deleteFileFromGroup;