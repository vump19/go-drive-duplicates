/**
 * 공유 관리 모듈
 */

let currentScanJobId = null;
let scanPollTimer = null;
let scanResults = [];
let selectedFileIds = new Set();

// Initialize folder resolver on page load
document.addEventListener('DOMContentLoaded', () => {
    const folderInput = document.getElementById('folderInput');
    if (folderInput) {
        attachFolderResolver(folderInput);
    }
});

// ===== Scan Functions =====

async function startSharingScan() {
    const folderInput = document.getElementById('folderInput');
    const folderId = folderInput.value.trim();
    if (!folderId) {
        alert('폴더 URL 또는 ID를 입력하세요.');
        return;
    }

    const includeSubfolders = document.getElementById('includeSubfolders').checked;

    // Disable button
    const scanBtn = document.getElementById('scanBtn');
    scanBtn.disabled = true;
    scanBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 시작 중...';

    try {
        const result = await window.API.Sharing.startScan(folderId, includeSubfolders);
        currentScanJobId = result.jobId;

        // Show progress card
        document.getElementById('progressCard').classList.remove('hidden');
        document.getElementById('resultsCard').classList.add('hidden');

        // Start polling
        startScanPolling();
    } catch (err) {
        alert('스캔 시작 실패: ' + (err.message || err));
        scanBtn.disabled = false;
        scanBtn.innerHTML = '<i class="fas fa-search"></i> 스캔 시작';
    }
}

function startScanPolling() {
    if (scanPollTimer) clearInterval(scanPollTimer);
    scanPollTimer = setInterval(pollScanProgress, 1000);
}

async function pollScanProgress() {
    if (!currentScanJobId) return;

    try {
        const progress = await window.API.Sharing.getScanProgress(currentScanJobId);
        updateScanProgress(progress);

        if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'cancelled') {
            clearInterval(scanPollTimer);
            scanPollTimer = null;

            if (progress.status === 'completed') {
                await loadScanResults();
            } else {
                document.getElementById('progressCard').querySelector('h2').innerHTML =
                    `<i class="fas fa-${progress.status === 'cancelled' ? 'ban' : 'exclamation-triangle'}"></i> 스캔 ${progress.status === 'cancelled' ? '취소됨' : '실패'}`;
            }

            // Re-enable button
            const scanBtn = document.getElementById('scanBtn');
            scanBtn.disabled = false;
            scanBtn.innerHTML = '<i class="fas fa-search"></i> 스캔 시작';
        }
    } catch (err) {
        // Ignore polling errors
    }
}

function updateScanProgress(data) {
    const total = data.totalItems || 0;
    const scanned = data.scannedItems || 0;
    const shared = data.sharedItems || 0;
    const pct = total > 0 ? Math.round((scanned / total) * 100) : 0;

    document.getElementById('scanProgressBar').style.width = pct + '%';
    document.getElementById('scanProgressBar').textContent = pct + '%';
    document.getElementById('scannedCount').textContent = scanned.toLocaleString();
    document.getElementById('totalCount').textContent = total.toLocaleString();
    document.getElementById('sharedCount').textContent = shared.toLocaleString();
}

async function loadScanResults() {
    try {
        const data = await window.API.Sharing.getScanResults(currentScanJobId);
        scanResults = data.results || [];

        // Show results card
        document.getElementById('progressCard').classList.add('hidden');
        document.getElementById('resultsCard').classList.remove('hidden');

        // Update summary
        const totalItems = data.totalItems || data.scannedItems || 0;
        const sharedItems = data.sharedItems || scanResults.length;
        const uniqueEmails = new Set();
        scanResults.forEach(item => {
            item.permissions.forEach(p => {
                if (!p.isOwner && p.emailAddress) {
                    uniqueEmails.add(p.emailAddress);
                }
            });
        });

        document.getElementById('summaryTotal').textContent = totalItems.toLocaleString();
        document.getElementById('summaryShared').textContent = sharedItems.toLocaleString();
        document.getElementById('summaryUsers').textContent = uniqueEmails.size.toLocaleString();

        // Render table
        renderResultsTable(scanResults);

        // Auto-fill target email if only one shared user
        if (uniqueEmails.size === 1) {
            document.getElementById('targetEmail').value = [...uniqueEmails][0];
        }
    } catch (err) {
        alert('결과 로드 실패: ' + (err.message || err));
    }
}

async function cancelScan() {
    if (!currentScanJobId) return;
    try {
        await window.API.Sharing.cancelScan(currentScanJobId);
    } catch (err) {
        // ignore
    }
}

// ===== Results Rendering =====

function renderResultsTable(results, filter = null) {
    const tbody = document.getElementById('resultsBody');
    tbody.innerHTML = '';

    const emailFilter = filter?.email || document.getElementById('emailFilter').value.trim().toLowerCase();
    const roleFilter = filter?.role || document.getElementById('roleFilter').value;

    let filteredResults = results;

    if (emailFilter || roleFilter) {
        filteredResults = results.filter(item => {
            return item.permissions.some(p => {
                if (p.isOwner) return false;
                const emailMatch = !emailFilter || (p.emailAddress && p.emailAddress.toLowerCase().includes(emailFilter));
                const roleMatch = !roleFilter || p.role === roleFilter;
                return emailMatch && roleMatch;
            });
        });
    }

    filteredResults.forEach(item => {
        // Main row
        const tr = document.createElement('tr');
        tr.dataset.fileId = item.fileId;

        const isChecked = selectedFileIds.has(item.fileId);
        const icon = item.isFolder ? 'fa-folder' : getFileIcon(item.mimeType);
        const iconColor = item.isFolder ? '#f59e0b' : '#80848e';
        const nonOwnerPerms = item.permissions.filter(p => !p.isOwner);

        tr.innerHTML = `
            <td><input type="checkbox" ${isChecked ? 'checked' : ''} onchange="toggleFileSelection('${item.fileId}', this)"></td>
            <td>
                <button class="toggle-btn" onclick="togglePermissionDetails('${item.fileId}')">
                    <i class="fas fa-chevron-right" id="toggle-icon-${item.fileId}"></i>
                </button>
            </td>
            <td>
                <i class="fas ${icon} file-icon" style="color: ${iconColor};"></i>
                ${item.webViewLink ? `<a href="${item.webViewLink}" target="_blank" class="file-link">${escapeHtml(item.fileName)}</a>` : escapeHtml(item.fileName)}
            </td>
            <td style="color: #80848e; font-size: 0.85rem;">${escapeHtml(item.path)}</td>
            <td>
                <div class="perm-badges">
                    ${nonOwnerPerms.map(p => `<span class="perm-badge ${p.role}" title="${p.emailAddress}">${p.displayName || p.emailAddress || p.type} (${translateRole(p.role)})</span>`).join('')}
                </div>
            </td>
        `;
        tbody.appendChild(tr);

        // Detail row (hidden by default)
        const detailTr = document.createElement('tr');
        detailTr.className = 'perm-detail-row hidden';
        detailTr.id = `detail-${item.fileId}`;
        detailTr.innerHTML = `
            <td colspan="5">
                <ul class="perm-detail-list">
                    ${item.permissions.map(p => `
                        <li>
                            <i class="fas ${p.isOwner ? 'fa-crown' : 'fa-user'}" style="color: ${p.isOwner ? '#f59e0b' : '#5865f2'};"></i>
                            <strong>${escapeHtml(p.displayName || p.emailAddress || p.type)}</strong>
                            ${p.emailAddress ? `<span style="color: #80848e;">(${p.emailAddress})</span>` : ''}
                            <span class="perm-badge ${p.role}">${translateRole(p.role)}</span>
                            <span style="color: #80848e; font-size: 0.8rem;">${p.type}</span>
                        </li>
                    `).join('')}
                </ul>
            </td>
        `;
        tbody.appendChild(detailTr);
    });

    updateBulkActionBar();
}

function togglePermissionDetails(fileId) {
    const detailRow = document.getElementById(`detail-${fileId}`);
    const icon = document.getElementById(`toggle-icon-${fileId}`);
    if (detailRow) {
        detailRow.classList.toggle('hidden');
        if (icon) {
            icon.className = detailRow.classList.contains('hidden') ? 'fas fa-chevron-right' : 'fas fa-chevron-down';
        }
    }
}

function applyFilters() {
    renderResultsTable(scanResults);
}

// ===== Selection Functions =====

function toggleFileSelection(fileId, checkbox) {
    if (checkbox.checked) {
        selectedFileIds.add(fileId);
    } else {
        selectedFileIds.delete(fileId);
    }
    updateBulkActionBar();
}

function toggleSelectAll(checkbox) {
    const checkboxes = document.querySelectorAll('#resultsBody input[type="checkbox"]');
    checkboxes.forEach(cb => {
        cb.checked = checkbox.checked;
        const fileId = cb.closest('tr')?.dataset?.fileId;
        if (fileId) {
            if (checkbox.checked) {
                selectedFileIds.add(fileId);
            } else {
                selectedFileIds.delete(fileId);
            }
        }
    });
    updateBulkActionBar();
}

function selectAllVisible() {
    const checkboxes = document.querySelectorAll('#resultsBody input[type="checkbox"]');
    checkboxes.forEach(cb => {
        cb.checked = true;
        const fileId = cb.closest('tr')?.dataset?.fileId;
        if (fileId) selectedFileIds.add(fileId);
    });
    document.getElementById('selectAllCheckbox').checked = true;
    updateBulkActionBar();
}

function deselectAll() {
    selectedFileIds.clear();
    const checkboxes = document.querySelectorAll('#resultsBody input[type="checkbox"]');
    checkboxes.forEach(cb => cb.checked = false);
    document.getElementById('selectAllCheckbox').checked = false;
    updateBulkActionBar();
}

function updateBulkActionBar() {
    const bar = document.getElementById('bulkActionBar');
    const count = selectedFileIds.size;
    document.getElementById('selectedCountText').textContent = count;
    if (count > 0) {
        bar.classList.remove('hidden');
    } else {
        bar.classList.add('hidden');
    }
}

// ===== Bulk Actions =====

async function bulkRemoveSharing() {
    const targetEmail = document.getElementById('targetEmail').value.trim();
    if (!targetEmail) {
        alert('대상 이메일을 입력하세요.');
        return;
    }
    if (selectedFileIds.size === 0) {
        alert('파일을 선택하세요.');
        return;
    }

    if (!confirm(`선택한 ${selectedFileIds.size}개 파일에서 ${targetEmail}의 공유를 해제합니다.\n계속하시겠습니까?`)) {
        return;
    }

    await executePermissionChange([...selectedFileIds], 'remove', targetEmail);
}

async function bulkChangeRole(newRole) {
    const targetEmail = document.getElementById('targetEmail').value.trim();
    if (!targetEmail) {
        alert('대상 이메일을 입력하세요.');
        return;
    }
    if (selectedFileIds.size === 0) {
        alert('파일을 선택하세요.');
        return;
    }

    const roleName = translateRole(newRole);
    if (!confirm(`선택한 ${selectedFileIds.size}개 파일에서 ${targetEmail}의 권한을 ${roleName}(으)로 변경합니다.\n계속하시겠습니까?`)) {
        return;
    }

    await executePermissionChange([...selectedFileIds], 'changeRole', targetEmail, newRole);
}

async function executePermissionChange(fileIds, action, targetEmail, newRole = '') {
    showChangeModal(fileIds.length);

    try {
        const result = await window.API.Sharing.changePermissions(fileIds, action, targetEmail, newRole);
        startChangePolling(result.jobId);
    } catch (err) {
        alert('권한 변경 실패: ' + (err.message || err));
        hideChangeModal();
    }
}

// ===== Change Progress Modal =====

let changePollTimer = null;

function showChangeModal(totalItems) {
    document.getElementById('changeModal').classList.remove('hidden');
    document.getElementById('changeProgressFill').style.width = '0%';
    document.getElementById('changeSuccessCount').textContent = '0';
    document.getElementById('changeFailureCount').textContent = '0';
    document.getElementById('changeProcessedCount').textContent = '0';
    document.getElementById('changeTotalCount').textContent = totalItems;
    document.getElementById('changeCompleteArea').classList.add('hidden');
}

function hideChangeModal() {
    document.getElementById('changeModal').classList.add('hidden');
    if (changePollTimer) {
        clearInterval(changePollTimer);
        changePollTimer = null;
    }
}

function closeChangeModal() {
    hideChangeModal();
    // Refresh scan results
    if (currentScanJobId) {
        loadScanResults();
    }
}

function startChangePolling(jobId) {
    if (changePollTimer) clearInterval(changePollTimer);
    changePollTimer = setInterval(async () => {
        try {
            const progress = await window.API.Sharing.getChangeProgress(jobId);
            updateChangeProgress(progress);

            if (progress.status === 'completed' || progress.status === 'failed' || progress.status === 'cancelled') {
                clearInterval(changePollTimer);
                changePollTimer = null;
                document.getElementById('changeCompleteArea').classList.remove('hidden');
                document.querySelector('#changeModal h3').innerHTML =
                    `<i class="fas fa-check-circle" style="color: #3ba55d;"></i> 변경 완료`;
            }
        } catch (err) {
            // Ignore
        }
    }, 500);
}

function updateChangeProgress(data) {
    const total = data.totalItems || 1;
    const processed = data.processedItems || 0;
    const pct = Math.round((processed / total) * 100);

    document.getElementById('changeProgressFill').style.width = pct + '%';
    document.getElementById('changeSuccessCount').textContent = data.successCount || 0;
    document.getElementById('changeFailureCount').textContent = data.failureCount || 0;
    document.getElementById('changeProcessedCount').textContent = processed;
}

// ===== Helper Functions =====

function translateRole(role) {
    const map = {
        'owner': '소유자',
        'writer': '편집자',
        'reader': '뷰어',
        'commenter': '댓글 작성자'
    };
    return map[role] || role;
}

function getFileIcon(mimeType) {
    if (!mimeType) return 'fa-file';
    if (mimeType.includes('image')) return 'fa-file-image';
    if (mimeType.includes('video')) return 'fa-file-video';
    if (mimeType.includes('audio')) return 'fa-file-audio';
    if (mimeType.includes('pdf')) return 'fa-file-pdf';
    if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) return 'fa-file-excel';
    if (mimeType.includes('document') || mimeType.includes('word')) return 'fa-file-word';
    if (mimeType.includes('presentation') || mimeType.includes('powerpoint')) return 'fa-file-powerpoint';
    if (mimeType.includes('zip') || mimeType.includes('archive') || mimeType.includes('compressed')) return 'fa-file-archive';
    if (mimeType.includes('text')) return 'fa-file-alt';
    return 'fa-file';
}

function showNotification(message, type = 'info') {
    // Simple alert fallback
    if (type === 'error') {
        console.error(message);
    } else {
        console.log(message);
    }
}
