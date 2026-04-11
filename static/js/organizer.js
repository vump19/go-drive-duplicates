// 스마트 파일 정리 - 프론트엔드 JavaScript

let currentRuleSetId = null;
let currentSessionId = 'session_' + Date.now();
let currentJobId = null;
let progressInterval = null;
let isWatching = false;

// ============================================================
// 초기화
// ============================================================

document.addEventListener('DOMContentLoaded', () => {
    loadRuleSets();
});

// ============================================================
// 탭 전환
// ============================================================

function switchTab(tabName) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
    document.querySelector(`.tab[data-tab="${tabName}"]`).classList.add('active');
    document.getElementById(`tab-${tabName}`).classList.add('active');

    if (tabName === 'logs' && currentRuleSetId) {
        loadLogs();
    }
}

// ============================================================
// 규칙 세트 CRUD
// ============================================================

async function loadRuleSets() {
    try {
        const ruleSets = await API.Organizer.getRuleSets();
        renderRuleSets(ruleSets || []);
    } catch (e) {
        console.error('규칙 세트 로드 실패:', e);
    }
}

function renderRuleSets(ruleSets) {
    const list = document.getElementById('rulesetList');
    const empty = document.getElementById('rulesetEmpty');

    if (!ruleSets || ruleSets.length === 0) {
        list.innerHTML = '';
        list.appendChild(empty);
        empty.style.display = 'block';
        return;
    }

    empty.style.display = 'none';
    list.innerHTML = ruleSets.map(rs => `
        <li class="ruleset-item ${rs.id === currentRuleSetId ? 'active' : ''}" onclick="selectRuleSet(${rs.id})">
            <div>
                <div class="name">${escapeHtml(rs.name)}</div>
                <div class="meta">${escapeHtml(rs.description || '설명 없음')}</div>
            </div>
            <div>
                ${rs.isActive ? '<span class="badge">감시중</span>' : ''}
                <button class="btn btn-sm" onclick="event.stopPropagation();deleteRuleSet(${rs.id})" title="삭제" style="background:none;border:none;color:#dc3545;cursor:pointer;">
                    <i class="fas fa-trash"></i>
                </button>
            </div>
        </li>
    `).join('');
}

function showCreateRuleSetModal() {
    document.getElementById('rsName').value = '';
    document.getElementById('rsDescription').value = '';
    document.getElementById('rsWatchFolder').value = '';
    document.getElementById('createRuleSetModal').classList.add('show');
}

async function createRuleSet() {
    const name = document.getElementById('rsName').value.trim();
    if (!name) { showNotify('이름을 입력하세요', 'error'); return; }

    try {
        const rs = await API.Organizer.createRuleSet({
            name,
            description: document.getElementById('rsDescription').value.trim(),
            watchFolder: document.getElementById('rsWatchFolder').value.trim()
        });
        closeModal('createRuleSetModal');
        showNotify('규칙 세트가 생성되었습니다', 'success');
        await loadRuleSets();
        selectRuleSet(rs.id);
    } catch (e) {
        showNotify('생성 실패: ' + e.message, 'error');
    }
}

async function deleteRuleSet(id) {
    if (!confirm('이 규칙 세트를 삭제하시겠습니까?')) return;
    try {
        await API.Organizer.deleteRuleSet(id);
        if (currentRuleSetId === id) {
            currentRuleSetId = null;
            document.getElementById('rulesSection').style.display = 'none';
            updateActionButtons();
        }
        showNotify('규칙 세트가 삭제되었습니다', 'success');
        await loadRuleSets();
    } catch (e) {
        showNotify('삭제 실패: ' + e.message, 'error');
    }
}

async function selectRuleSet(id) {
    currentRuleSetId = id;
    await loadRuleSets();
    await loadRules();
    document.getElementById('rulesSection').style.display = 'block';
    updateActionButtons();
}

// ============================================================
// 규칙 CRUD
// ============================================================

async function loadRules() {
    if (!currentRuleSetId) return;
    try {
        const rs = await API.Organizer.getRuleSet(currentRuleSetId);
        renderRules(rs.rules || []);
    } catch (e) {
        console.error('규칙 로드 실패:', e);
    }
}

function renderRules(rules) {
    const list = document.getElementById('rulesList');
    const count = document.getElementById('rulesCount');
    count.textContent = `(${rules.length}개)`;

    if (!rules || rules.length === 0) {
        list.innerHTML = '<div class="empty-state" style="padding:16px;"><p>규칙이 없습니다. AI 채팅으로 규칙을 생성하거나 직접 추가하세요.</p></div>';
        return;
    }

    list.innerHTML = rules.map(rule => {
        const actionIcon = rule.action === 'move' ? 'fa-arrow-right' : 'fa-copy';
        const actionText = rule.action === 'move' ? '이동' : '복사';
        return `
            <div class="rule-item">
                <div>
                    <span class="rule-name">${escapeHtml(rule.name)}</span>
                    <span class="rule-action"><i class="fas ${actionIcon}"></i> ${actionText}</span>
                </div>
                <div class="rule-btns">
                    <button onclick="toggleRuleEnabled(${rule.id}, ${!rule.enabled})" title="${rule.enabled ? '비활성화' : '활성화'}">
                        <i class="fas ${rule.enabled ? 'fa-toggle-on' : 'fa-toggle-off'}" style="color:${rule.enabled ? '#28a745' : '#ccc'}"></i>
                    </button>
                    <button onclick="deleteRule(${rule.id})" title="삭제">
                        <i class="fas fa-trash" style="color:#dc3545"></i>
                    </button>
                </div>
            </div>
        `;
    }).join('');
}

function showCreateRuleModal() {
    document.getElementById('ruleName').value = '';
    document.getElementById('ruleDesc').value = '';
    document.getElementById('ruleCondValue').value = '';
    document.getElementById('ruleTarget').value = '';
    document.getElementById('createRuleModal').classList.add('show');
}

async function createRule() {
    const name = document.getElementById('ruleName').value.trim();
    const targetPath = document.getElementById('ruleTarget').value.trim();
    if (!name || !targetPath) { showNotify('이름과 대상 폴더를 입력하세요', 'error'); return; }

    const conditions = [{
        field: document.getElementById('ruleCondField').value,
        operator: document.getElementById('ruleCondOp').value,
        value: document.getElementById('ruleCondValue').value.trim()
    }];

    try {
        await API.Organizer.createRule({
            ruleSetId: currentRuleSetId,
            name,
            description: document.getElementById('ruleDesc').value.trim(),
            conditions: JSON.stringify(conditions),
            action: document.getElementById('ruleAction').value,
            targetPath,
            enabled: true,
            priority: 0
        });
        closeModal('createRuleModal');
        showNotify('규칙이 추가되었습니다', 'success');
        await loadRules();
    } catch (e) {
        showNotify('규칙 추가 실패: ' + e.message, 'error');
    }
}

async function deleteRule(id) {
    if (!confirm('이 규칙을 삭제하시겠습니까?')) return;
    try {
        await API.Organizer.deleteRule(id);
        showNotify('규칙이 삭제되었습니다', 'success');
        await loadRules();
    } catch (e) {
        showNotify('삭제 실패: ' + e.message, 'error');
    }
}

async function toggleRuleEnabled(id, enabled) {
    try {
        await API.Organizer.updateRule({ id, enabled });
        await loadRules();
    } catch (e) {
        showNotify('업데이트 실패: ' + e.message, 'error');
    }
}

// ============================================================
// AI 채팅
// ============================================================

async function sendChat() {
    const input = document.getElementById('chatInput');
    const message = input.value.trim();
    if (!message) return;

    input.value = '';
    addChatMessage('user', message);

    const sendBtn = document.getElementById('btnSend');
    sendBtn.disabled = true;

    try {
        const aiProviders = getLocalAIProviders();
        const response = await API.Organizer.chat(currentSessionId, message, '', aiProviders);
        addChatMessage('assistant', response.content, response.ruleSuggestions);
    } catch (e) {
        addChatMessage('assistant', '오류가 발생했습니다: ' + e.message);
    } finally {
        sendBtn.disabled = false;
        input.focus();
    }
}

function addChatMessage(role, content, suggestions) {
    const container = document.getElementById('chatMessages');
    const div = document.createElement('div');
    div.className = `chat-message ${role}`;

    let html = `<div class="bubble">${formatChatContent(content)}</div>`;

    if (suggestions && suggestions.length > 0) {
        html += suggestions.map((s, i) => `
            <div class="suggestion-card">
                <h4><i class="fas fa-lightbulb"></i> ${escapeHtml(s.name)}</h4>
                <p>${escapeHtml(s.description || '')}</p>
                <div class="conditions">${formatConditions(s.conditions)}</div>
                <button class="btn btn-primary btn-sm" onclick='applySuggestion(${JSON.stringify(s).replace(/'/g, "&#39;")})'>
                    <i class="fas fa-check"></i> 규칙 적용
                </button>
            </div>
        `).join('');
    }

    html += `<div class="time">${new Date().toLocaleTimeString('ko-KR')}</div>`;
    div.innerHTML = html;
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

function formatChatContent(content) {
    // Basic markdown-like formatting
    return escapeHtml(content)
        .replace(/\n/g, '<br>')
        .replace(/```json[\s\S]*?```/g, '') // Remove JSON code blocks (shown as cards)
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/`(.*?)`/g, '<code style="background:#f0f0f0;padding:1px 4px;border-radius:3px;">$1</code>');
}

function formatConditions(conditions) {
    if (!conditions || conditions.length === 0) return '조건 없음';
    return conditions.map(c => `${c.field} ${c.operator} "${c.value}"`).join(' AND ');
}

async function applySuggestion(suggestion) {
    if (!currentRuleSetId) {
        showNotify('먼저 규칙 세트를 선택하세요', 'error');
        return;
    }

    try {
        await API.Organizer.applySuggestions(currentRuleSetId, [suggestion]);
        showNotify('규칙이 추가되었습니다', 'success');
        await loadRules();
    } catch (e) {
        showNotify('규칙 적용 실패: ' + e.message, 'error');
    }
}

// ============================================================
// 미리보기
// ============================================================

async function doPreview() {
    const folderId = document.getElementById('targetFolderId').value.trim();
    if (!folderId || !currentRuleSetId) {
        showNotify('규칙 세트와 대상 폴더를 선택하세요', 'error');
        return;
    }

    switchTab('preview');
    document.getElementById('previewEmpty').style.display = 'none';
    document.getElementById('previewResult').style.display = 'none';

    try {
        const result = await API.Organizer.previewOrganize(currentRuleSetId, folderId);
        renderPreview(result);
    } catch (e) {
        showNotify('미리보기 실패: ' + e.message, 'error');
        document.getElementById('previewEmpty').style.display = 'block';
    }
}

function renderPreview(result) {
    document.getElementById('previewResult').style.display = 'block';

    document.getElementById('previewStats').innerHTML = `
        <div class="stat-card"><div class="value">${result.totalFiles}</div><div class="label">전체 파일</div></div>
        <div class="stat-card"><div class="value" style="color:#28a745">${result.matchedFiles}</div><div class="label">매칭됨</div></div>
        <div class="stat-card"><div class="value" style="color:#6c757d">${result.unmatchedFiles}</div><div class="label">매칭 안됨</div></div>
        <div class="stat-card"><div class="value" style="color:#667eea">${result.items.length}</div><div class="label">정리 대상</div></div>
    `;

    const tbody = document.getElementById('previewTableBody');
    tbody.innerHTML = result.items.map(item => `
        <tr>
            <td>${escapeHtml(item.fileName)}</td>
            <td>${formatFileSize(item.fileSize)}</td>
            <td>${escapeHtml(item.ruleName)}</td>
            <td>${item.action === 'move' ? '이동' : '복사'}</td>
            <td style="font-size:11px;color:#666;">${escapeHtml(item.targetPath)}</td>
        </tr>
    `).join('');
}

// ============================================================
// 실행 & 진행 상황
// ============================================================

async function doExecute() {
    const folderId = document.getElementById('targetFolderId').value.trim();
    if (!folderId || !currentRuleSetId) {
        showNotify('규칙 세트와 대상 폴더를 선택하세요', 'error');
        return;
    }

    if (!confirm('파일 정리를 실행하시겠습니까?')) return;

    switchTab('progress');

    try {
        const job = await API.Organizer.executeOrganize(currentRuleSetId, folderId);
        currentJobId = job.jobId;
        document.getElementById('progressEmpty').style.display = 'none';
        document.getElementById('progressResult').style.display = 'block';
        startProgressMonitoring();
    } catch (e) {
        showNotify('실행 실패: ' + e.message, 'error');
    }
}

function startProgressMonitoring() {
    if (progressInterval) clearInterval(progressInterval);
    progressInterval = setInterval(async () => {
        if (!currentJobId) { clearInterval(progressInterval); return; }

        try {
            const job = await API.Organizer.getOrganizeProgress(currentJobId);
            if (!job) return;
            renderProgress(job);

            if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
                clearInterval(progressInterval);
                progressInterval = null;
                if (job.status === 'completed') showNotify('파일 정리가 완료되었습니다!', 'success');
                if (job.status === 'failed') showNotify('파일 정리 실패: ' + job.errorMessage, 'error');
            }
        } catch (e) {
            // silent
        }
    }, 1000);
}

function renderProgress(job) {
    const pct = job.totalFiles > 0 ? Math.round((job.processedFiles / job.totalFiles) * 100) : 0;
    document.getElementById('progressBar').style.width = pct + '%';
    document.getElementById('progressBar').textContent = pct + '%';

    document.getElementById('progressStats').innerHTML = `
        <div class="stat-card"><div class="value">${job.processedFiles}/${job.totalFiles}</div><div class="label">처리됨</div></div>
        <div class="stat-card"><div class="value" style="color:#28a745">${job.movedFiles}</div><div class="label">이동/복사</div></div>
        <div class="stat-card"><div class="value" style="color:#6c757d">${job.skippedFiles}</div><div class="label">건너뜀</div></div>
        <div class="stat-card"><div class="value" style="color:#dc3545">${job.failedFiles}</div><div class="label">실패</div></div>
    `;
}

async function cancelOrganize() {
    if (!currentJobId) return;
    try {
        await API.Organizer.cancelOrganize(currentJobId);
        showNotify('작업이 취소되었습니다', 'success');
    } catch (e) {
        showNotify('취소 실패: ' + e.message, 'error');
    }
}

// ============================================================
// 감시 폴더
// ============================================================

async function toggleWatch() {
    if (!currentRuleSetId) return;

    try {
        if (isWatching) {
            await API.Organizer.stopWatching(currentRuleSetId);
            isWatching = false;
            document.getElementById('btnWatch').innerHTML = '<i class="fas fa-eye"></i> 감시 시작';
            showNotify('폴더 감시가 중지되었습니다', 'success');
        } else {
            await API.Organizer.startWatching(currentRuleSetId);
            isWatching = true;
            document.getElementById('btnWatch').innerHTML = '<i class="fas fa-stop"></i> 감시 중지';
            showNotify('폴더 감시가 시작되었습니다', 'success');
        }
    } catch (e) {
        showNotify('감시 설정 실패: ' + e.message, 'error');
    }
}

// ============================================================
// 로그
// ============================================================

async function loadLogs() {
    if (!currentRuleSetId) return;

    try {
        const result = await API.Organizer.getLogs(currentRuleSetId, 100, 0);
        const logs = result.logs || [];

        if (logs.length === 0) {
            document.getElementById('logsEmpty').style.display = 'block';
            document.getElementById('logsTable').style.display = 'none';
            return;
        }

        document.getElementById('logsEmpty').style.display = 'none';
        document.getElementById('logsTable').style.display = 'table';

        document.getElementById('logsTableBody').innerHTML = logs.map(l => `
            <tr>
                <td>${new Date(l.executedAt).toLocaleString('ko-KR')}</td>
                <td>${escapeHtml(l.ruleName)}</td>
                <td title="${escapeHtml(l.fileId)}">${escapeHtml(l.fileName)}</td>
                <td>${formatFileSize(l.fileSize)}</td>
                <td class="action-${l.action}">${actionLabel(l.action)}</td>
            </tr>
        `).join('');
    } catch (e) {
        console.error('로그 로드 실패:', e);
    }
}

function actionLabel(action) {
    const labels = { moved: '이동', copied: '복사', skipped: '건너뜀', failed: '실패' };
    return labels[action] || action;
}

async function exportLogs() {
    if (!currentRuleSetId) { showNotify('규칙 세트를 선택하세요', 'error'); return; }

    try {
        const result = await API.Organizer.exportLogs(currentRuleSetId);
        showNotify('스프레드시트로 내보냈습니다', 'success');
        if (result.webLink) {
            window.open(result.webLink, '_blank');
        }
    } catch (e) {
        showNotify('내보내기 실패: ' + e.message, 'error');
    }
}

// ============================================================
// UI 헬퍼
// ============================================================

function updateActionButtons() {
    const hasFolderId = document.getElementById('targetFolderId').value.trim() !== '';
    const hasRuleSet = currentRuleSetId !== null;
    document.getElementById('btnPreview').disabled = !(hasFolderId && hasRuleSet);
    document.getElementById('btnExecute').disabled = !(hasFolderId && hasRuleSet);
    document.getElementById('btnWatch').disabled = !hasRuleSet;
}

document.getElementById('targetFolderId')?.addEventListener('input', updateActionButtons);

function closeModal(id) {
    document.getElementById(id).classList.remove('show');
}

// Close modals on overlay click
document.querySelectorAll('.modal-overlay').forEach(overlay => {
    overlay.addEventListener('click', (e) => {
        if (e.target === overlay) overlay.classList.remove('show');
    });
});

function showNotify(message, type = 'success') {
    const div = document.createElement('div');
    div.className = `notification ${type}`;
    div.textContent = message;
    document.body.appendChild(div);
    setTimeout(() => div.remove(), 3000);
}

function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function formatFileSize(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// ============================================================
// AI API 키 설정 (localStorage)
// ============================================================

const AI_PROVIDERS_KEY = 'ai_providers';
const AI_PROVIDER_ORDER_KEY = 'ai_provider_order';

const PROVIDER_DEFS = {
    claude: { label: 'Claude (Anthropic)', placeholder: 'sk-ant-...', defaultModel: 'claude-sonnet-4-5-20250929' },
    gemini: { label: 'Gemini (Google)', placeholder: 'AIza...', defaultModel: 'gemini-2.0-flash' },
    openai: { label: 'OpenAI (ChatGPT)', placeholder: 'sk-...', defaultModel: 'gpt-4o' },
};

function getProviderOrder() {
    try {
        const order = JSON.parse(localStorage.getItem(AI_PROVIDER_ORDER_KEY));
        if (Array.isArray(order) && order.length === 3) return order;
    } catch {}
    return ['claude', 'gemini', 'openai'];
}

function saveProviderOrder(order) {
    localStorage.setItem(AI_PROVIDER_ORDER_KEY, JSON.stringify(order));
}

function getLocalAIProviders() {
    try {
        const data = localStorage.getItem(AI_PROVIDERS_KEY);
        if (!data) return null;
        const saved = JSON.parse(data);
        const order = getProviderOrder();
        // 저장된 순서에 따라 정렬
        const sorted = [];
        for (const name of order) {
            const p = saved.find(s => s.name === name);
            if (p && p.apiKey && p.apiKey.trim()) sorted.push(p);
        }
        // 순서에 없는 프로바이더도 포함
        for (const p of saved) {
            if (p.apiKey && p.apiKey.trim() && !sorted.find(s => s.name === p.name)) sorted.push(p);
        }
        return sorted.length > 0 ? sorted : null;
    } catch {
        return null;
    }
}

function saveLocalAIProviders(providers) {
    localStorage.setItem(AI_PROVIDERS_KEY, JSON.stringify(providers));
    updateSettingsBadge();
}

// 현재 모달의 프로바이더 순서
let _modalProviderOrder = [];

function showAPIKeySettings() {
    const saved = (() => {
        try { return JSON.parse(localStorage.getItem(AI_PROVIDERS_KEY)) || []; } catch { return []; }
    })();
    _modalProviderOrder = getProviderOrder();
    renderProviderList(saved);
    document.getElementById('apiKeySettingsModal').classList.add('show');
}

function renderProviderList(saved) {
    const container = document.getElementById('providerList');
    const total = _modalProviderOrder.length;
    container.innerHTML = _modalProviderOrder.map((name, idx) => {
        const def = PROVIDER_DEFS[name];
        const p = saved.find(s => s.name === name);
        const apiKey = p ? p.apiKey : '';
        const model = p ? (p.model || '') : '';
        const rank = idx + 1;
        const rankBg = rank === 1 ? '#d97706' : '#6b7280';
        return `
        <div class="provider-card" data-name="${name}" style="border: 1px solid #e0e0e0; border-radius: 8px; padding: 12px; margin-bottom: 12px;">
            <div style="display:flex; align-items:center; justify-content:space-between; margin-bottom:8px;">
                <div style="display:flex; align-items:center; gap:8px;">
                    <span class="provider-rank" style="background:${rankBg}; color:white; font-size:11px; padding:2px 8px; border-radius:10px; font-weight:600;">${rank}순위</span>
                    <strong style="font-size:14px;">${def.label}</strong>
                </div>
                <div style="display:flex; gap:2px;">
                    <button class="btn btn-sm" onclick="moveProvider('${name}',-1)" style="background:none; border:1px solid #ddd; border-radius:4px; padding:2px 6px; cursor:pointer; color:#555;${idx === 0 ? 'opacity:0.3; pointer-events:none;' : ''}" title="위로">
                        <i class="fas fa-chevron-up"></i>
                    </button>
                    <button class="btn btn-sm" onclick="moveProvider('${name}',1)" style="background:none; border:1px solid #ddd; border-radius:4px; padding:2px 6px; cursor:pointer; color:#555;${idx === total - 1 ? 'opacity:0.3; pointer-events:none;' : ''}" title="아래로">
                        <i class="fas fa-chevron-down"></i>
                    </button>
                </div>
            </div>
            <div class="form-group" style="margin-bottom:8px;">
                <label style="font-size:12px;">API Key</label>
                <input type="password" id="setting_${name}_key" value="${escapeHtml(apiKey)}" placeholder="${def.placeholder}" style="width:100%; padding:8px 10px; border:1px solid #ddd; border-radius:6px; font-size:13px;">
            </div>
            <div class="form-group" style="margin-bottom:0;">
                <label style="font-size:12px;">모델 <span style="color:#999;">(빈칸 = ${def.defaultModel})</span></label>
                <input type="text" id="setting_${name}_model" value="${escapeHtml(model)}" placeholder="${def.defaultModel}" style="width:100%; padding:8px 10px; border:1px solid #ddd; border-radius:6px; font-size:13px;">
            </div>
        </div>`;
    }).join('');
}

function moveProvider(name, direction) {
    // 현재 입력값을 먼저 수집
    const currentValues = collectModalValues();
    const idx = _modalProviderOrder.indexOf(name);
    const newIdx = idx + direction;
    if (newIdx < 0 || newIdx >= _modalProviderOrder.length) return;
    // swap
    [_modalProviderOrder[idx], _modalProviderOrder[newIdx]] = [_modalProviderOrder[newIdx], _modalProviderOrder[idx]];
    renderProviderList(currentValues);
}

function collectModalValues() {
    return _modalProviderOrder.map(name => ({
        name,
        apiKey: (document.getElementById(`setting_${name}_key`)?.value || '').trim(),
        model: (document.getElementById(`setting_${name}_model`)?.value || '').trim(),
    }));
}

function saveAPIKeySettings() {
    const all = collectModalValues();
    const providers = all.filter(p => p.apiKey);
    saveLocalAIProviders(providers);
    saveProviderOrder(_modalProviderOrder);
    closeModal('apiKeySettingsModal');
    showNotify('API 키가 저장되었습니다', 'success');
}

function clearAPIKeySettings() {
    if (!confirm('모든 API 키를 삭제하시겠습니까?')) return;
    localStorage.removeItem(AI_PROVIDERS_KEY);
    localStorage.removeItem(AI_PROVIDER_ORDER_KEY);
    _modalProviderOrder = ['claude', 'gemini', 'openai'];
    renderProviderList([]);
    updateSettingsBadge();
    showNotify('API 키가 삭제되었습니다', 'success');
}

function updateSettingsBadge() {
    const badge = document.getElementById('settingsBadge');
    if (!badge) return;
    const providers = getLocalAIProviders();
    if (providers && providers.length > 0) {
        badge.textContent = providers.length;
        badge.style.display = 'inline-flex';
    } else {
        badge.style.display = 'none';
    }
}

// 페이지 로드 시 뱃지 업데이트
document.addEventListener('DOMContentLoaded', () => {
    updateSettingsBadge();
});
