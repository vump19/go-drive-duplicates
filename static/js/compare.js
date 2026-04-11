/**
 * compare.js - 다중 비교 작업 관리 모듈
 * JobManager: 상태 관리 + 폴링
 * CompareUI: 카드 기반 렌더링
 */

// ============ JobManager: 다중 작업 상태 관리 ============
const JobManager = {
    jobs: new Map(), // Map<id, JobState>
    pollingTimer: null,
    POLL_INTERVAL: 1500,
    nextLocalId: 1, // 로컬 ID (진행 중 작업 구분용)

    addJob(state) {
        this.jobs.set(state.id, state);
        this.startPolling();
        CompareUI.renderAllJobs();
    },

    getJob(id) {
        return this.jobs.get(id);
    },

    getActiveJobs() {
        return [...this.jobs.values()].filter(j =>
            j.status === 'running' || j.deletionProgressId || j.moveProgressId
        );
    },

    getAllJobs(filter) {
        let list = [...this.jobs.values()];
        // 최신 작업이 위에 오도록
        list.sort((a, b) => (b.startTime || 0) - (a.startTime || 0));
        if (filter && filter !== 'all') {
            list = list.filter(j => j.status === filter);
        }
        return list;
    },

    startPolling() {
        if (this.pollingTimer) return;
        this.pollingTimer = setInterval(() => this.pollAllActiveJobs(), this.POLL_INTERVAL);
    },

    stopPolling() {
        if (this.pollingTimer) {
            clearInterval(this.pollingTimer);
            this.pollingTimer = null;
        }
    },

    async pollAllActiveJobs() {
        const active = this.getActiveJobs();
        if (active.length === 0) {
            this.stopPolling();
            return;
        }

        const promises = active.map(async (job) => {
            try {
                // 삭제 진행 중
                if (job.deletionProgressId) {
                    const prog = await window.API.Comparison.getComparisonProgress(job.deletionProgressId);
                    if (prog) {
                        job.deletionProgress = prog;
                        if (prog.status === 'completed' || prog.status === 'failed') {
                            job.deletionProgressId = null;
                            if (prog.status === 'completed') {
                                // 삭제 완료 시 로컬 데이터 업데이트
                                if (job.comparisonResult) {
                                    if (job._deletionType === 'folder') {
                                        job.comparisonResult.duplicateFiles = [];
                                        job.comparisonResult.duplicateCount = 0;
                                        job.comparisonResult.uniqueInTarget = [];
                                    } else {
                                        job.comparisonResult.duplicateFiles = [];
                                        job.comparisonResult.duplicateCount = 0;
                                    }
                                }
                            }
                        }
                        CompareUI.updateJobCard(job.id);
                    }
                    return;
                }

                // 이동 진행 중
                if (job.moveProgressId) {
                    const prog = await window.API.Comparison.getMoveProgress(job.moveProgressId);
                    if (prog) {
                        job.moveProgress = prog;
                        if (prog.status === 'completed' || prog.status === 'failed') {
                            job.moveProgressId = null;
                            if (prog.status === 'completed' && job.comparisonResult) {
                                job.comparisonResult.uniqueInTarget = [];
                            }
                        }
                        CompareUI.updateJobCard(job.id);
                    }
                    return;
                }

                // 비교 진행 중
                if (job.status === 'running') {
                    let prog;
                    if (job.type === 'single-folder') {
                        prog = await window.API.Comparison.getSingleFolderProgress(job.progressId);
                    } else {
                        prog = await window.API.Comparison.getComparisonProgress(job.progressId);
                    }
                    if (prog) {
                        job.progress = prog;
                        if (prog.status === 'completed') {
                            job.status = 'completed';
                            await loadJobResult(job);
                        } else if (prog.status === 'failed') {
                            job.status = 'failed';
                            job.errorMessage = prog.errorMessage || '알 수 없는 오류';
                        } else if (prog.status === 'cancelled') {
                            job.status = 'cancelled';
                        }
                        CompareUI.updateJobCard(job.id);
                    }
                }
            } catch (err) {
                console.error(`작업 ${job.id} 폴링 오류:`, err);
            }
        });

        await Promise.all(promises);
    }
};

// ============ 결과 로드 ============
async function loadJobResult(job) {
    try {
        if (job.type === 'single-folder') {
            const prog = await window.API.Comparison.getSingleFolderProgress(job.progressId);
            console.log('단일 폴더 결과 로드:', JSON.stringify(prog).substring(0, 200));
            const result = (prog.metadata && prog.metadata.result) || prog.result;
            if (result) {
                job.singleFolderResult = result;
            } else {
                // 결과가 없는 경우 빈 결과 설정
                job.singleFolderResult = { duplicateGroups: [], totalFiles: 0, duplicateFiles: 0, wastedSpace: 0 };
            }
        } else {
            const result = await window.API.Comparison.loadSavedComparison(
                job.sourceFolderId, job.targetFolderId
            );
            if (result) {
                job.comparisonResult = result;
                job.comparisonId = result.id;
            }
        }
    } catch (err) {
        console.error('결과 로드 실패:', err);
    }
}

// ============ 폴더 정보 조회 ============
async function getFolderInfo(folderId) {
    try {
        const baseUrl = window.API_BASE || 'http://localhost:9090';
        const response = await fetch(`${baseUrl}/api/explorer/file-info?fileId=${encodeURIComponent(folderId)}`);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return await response.json();
    } catch (error) {
        console.warn(`폴더 정보 조회 실패 (${folderId}):`, error);
        return null;
    }
}

// ============ CompareUI: 카드 기반 렌더링 ============
const CompareUI = {
    currentFilter: 'all',

    renderAllJobs() {
        const container = document.getElementById('jobList');
        if (!container) return;

        const jobs = JobManager.getAllJobs(this.currentFilter);

        if (jobs.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <i class="fas fa-folder-open"></i>
                    <p>${this.currentFilter === 'all' ? '비교 작업이 없습니다. 위에서 새 비교를 시작하세요.' : '해당 상태의 작업이 없습니다.'}</p>
                </div>
            `;
            return;
        }

        container.innerHTML = jobs.map(job => this.renderJobCard(job)).join('');
    },

    renderJobCard(job) {
        const statusBadge = this.getStatusBadge(job);
        const title = this.getJobTitle(job);
        const summary = this.getJobSummary(job);
        const isExpanded = job.expanded || false;

        let progressBarHtml = '';
        if (job.status === 'running' && job.progress) {
            const pct = job.progress.totalItems > 0
                ? Math.round((job.progress.processedItems / job.progress.totalItems) * 100)
                : 0;
            progressBarHtml = `
                <div class="job-progress-bar-container">
                    <div class="job-progress-bar">
                        <div class="job-progress-fill" style="width: ${pct}%"></div>
                    </div>
                    <div class="job-progress-info">
                        <span>${job.progress.currentStep || '진행 중...'}</span>
                        <span>${pct}% | ${formatElapsedTime(job.startTime)}</span>
                    </div>
                </div>
            `;
        }

        // 삭제/이동 진행 바
        if (job.deletionProgressId && job.deletionProgress) {
            const dp = job.deletionProgress;
            const total = dp.totalItems || dp.total || 0;
            const processed = dp.processedItems || dp.completed || 0;
            const pct = total > 0 ? Math.round((processed / total) * 100) : 0;
            progressBarHtml = `
                <div class="job-progress-bar-container">
                    <div class="job-progress-bar">
                        <div class="job-progress-fill" style="width: ${pct}%; background: linear-gradient(90deg, #ef4444, #f87171);"></div>
                    </div>
                    <div class="job-progress-info">
                        <span>${dp.currentStep || '삭제 진행 중...'}</span>
                        <span>${pct}%</span>
                    </div>
                </div>
            `;
        }
        if (job.moveProgressId && job.moveProgress) {
            const mp = job.moveProgress;
            const pct = mp.totalItems > 0 ? Math.round((mp.processedItems / mp.totalItems) * 100) : 0;
            progressBarHtml = `
                <div class="job-progress-bar-container">
                    <div class="job-progress-bar">
                        <div class="job-progress-fill" style="width: ${pct}%; background: linear-gradient(90deg, #22c55e, #4ade80);"></div>
                    </div>
                    <div class="job-progress-info">
                        <span>${mp.currentStep || '이동 진행 중...'}</span>
                        <span>${pct}%</span>
                    </div>
                </div>
            `;
        }

        let actionsHtml = '';
        if (job.status === 'running') {
            actionsHtml = `<button onclick="event.stopPropagation(); cancelJob(${job.id})" title="작업 취소"><i class="fas fa-stop"></i></button>`;
        } else if (job.status === 'completed' || job.status === 'failed' || job.status === 'cancelled') {
            actionsHtml = `<button class="danger" onclick="event.stopPropagation(); deleteJobResult(${job.id})" title="기록 삭제"><i class="fas fa-times"></i></button>`;
        }

        // 펼침 영역 내용
        let bodyHtml = '';
        if (isExpanded) {
            if (job.status === 'running') {
                bodyHtml = this.renderProgressDetail(job);
            } else if (job.status === 'completed') {
                if (job.type === 'single-folder') {
                    bodyHtml = this.renderSingleFolderResult(job);
                } else {
                    bodyHtml = this.renderComparisonResult(job);
                }
            } else if (job.status === 'failed') {
                bodyHtml = `<div style="color: #ef4444; padding: 12px;"><i class="fas fa-exclamation-circle"></i> ${job.errorMessage || '오류가 발생했습니다.'}</div>`;
            }

            // 삭제/이동 진행 표시
            if (job.deletionProgressId || (job.deletionProgress && job.deletionProgress.status === 'completed')) {
                bodyHtml += this.renderDeletionProgress(job);
            }
            if (job.moveProgressId || (job.moveProgress && job.moveProgress.status === 'completed')) {
                bodyHtml += this.renderMoveProgress(job);
            }
        }

        return `
            <div class="job-card" id="job-card-${job.id}">
                <div class="job-card-header" onclick="toggleJobCard(${job.id})">
                    ${statusBadge}
                    <div class="job-title">${title}</div>
                    <div class="job-summary">${summary}</div>
                    <div class="job-actions">${actionsHtml}</div>
                    <div class="job-toggle ${isExpanded ? 'expanded' : ''}"><i class="fas fa-chevron-down"></i></div>
                </div>
                ${progressBarHtml}
                <div class="job-card-body ${isExpanded ? 'open' : ''}">${bodyHtml}</div>
            </div>
        `;
    },

    updateJobCard(id) {
        const job = JobManager.getJob(id);
        if (!job) return;

        const cardEl = document.getElementById(`job-card-${id}`);
        if (!cardEl) {
            // 카드가 아직 없으면 전체 렌더링
            this.renderAllJobs();
            return;
        }

        // 전체 카드를 교체
        const temp = document.createElement('div');
        temp.innerHTML = this.renderJobCard(job);
        const newCard = temp.firstElementChild;
        cardEl.replaceWith(newCard);
    },

    getStatusBadge(job) {
        const map = {
            running: '<span class="job-status-badge running"><i class="fas fa-spinner fa-spin"></i> 진행중</span>',
            completed: '<span class="job-status-badge completed"><i class="fas fa-check"></i> 완료</span>',
            failed: '<span class="job-status-badge failed"><i class="fas fa-times"></i> 실패</span>',
            cancelled: '<span class="job-status-badge cancelled"><i class="fas fa-ban"></i> 취소</span>'
        };
        return map[job.status] || '';
    },

    getJobTitle(job) {
        if (job.type === 'single-folder') {
            return `${job.folderName || '폴더'} 내 중복 검색`;
        }
        return `${job.sourceFolderName || '기준폴더'} → ${job.targetFolderName || '대상폴더'}`;
    },

    getJobSummary(job) {
        if (job.deletionProgressId) {
            return '<span style="color: #ef4444;"><i class="fas fa-trash"></i> 삭제 중...</span>';
        }
        if (job.moveProgressId) {
            return '<span style="color: #22c55e;"><i class="fas fa-arrow-right"></i> 이동 중...</span>';
        }
        if (job.status === 'completed') {
            if (job.type === 'single-folder' && job.singleFolderResult) {
                const groups = job.singleFolderResult.duplicateGroups || [];
                const total = groups.reduce((s, g) => s + g.files.length, 0);
                return `${groups.length}개 중복 그룹 | ${total}개 파일`;
            }
            if (job.comparisonResult) {
                const r = job.comparisonResult;
                return `${r.duplicationPercentage.toFixed(1)}% 중복 | ${r.duplicateCount || 0}개 파일 | ${formatFileSize(r.duplicateSize || 0)} 절약`;
            }
        }
        if (job.status === 'running' && job.progress) {
            const pct = job.progress.totalItems > 0
                ? Math.round((job.progress.processedItems / job.progress.totalItems) * 100) : 0;
            return `${pct}%`;
        }
        return '';
    },

    // ---- 펼침 영역 렌더 함수들 ----

    renderProgressDetail(job) {
        const p = job.progress || {};
        const meta = p.metadata || {};
        const scannedFiles = meta.scannedFileCount || 0;
        const scannedFolders = meta.scannedFolderCount || 0;

        return `
            <div style="display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 12px; margin-bottom: 12px;">
                <div style="background: #f8fafc; padding: 10px; border-radius: 8px; text-align: center;">
                    <div style="font-size: 0.8rem; color: #94a3b8;">현재 작업</div>
                    <div style="font-weight: 600; color: #1e293b; font-size: 0.9rem;">${p.currentStep || '시작 중...'}</div>
                </div>
                <div style="background: #f8fafc; padding: 10px; border-radius: 8px; text-align: center;">
                    <div style="font-size: 0.8rem; color: #94a3b8;">스캔된 파일</div>
                    <div style="font-weight: 600; color: #3b82f6;">${scannedFolders > 0 ? `${scannedFolders}폴더 / ` : ''}${scannedFiles || p.processedItems || 0}개</div>
                </div>
                <div style="background: #f8fafc; padding: 10px; border-radius: 8px; text-align: center;">
                    <div style="font-size: 0.8rem; color: #94a3b8;">경과 시간</div>
                    <div style="font-weight: 600; color: #1e293b;">${formatElapsedTime(job.startTime)}</div>
                </div>
            </div>
        `;
    },

    renderComparisonResult(job) {
        const r = job.comparisonResult;
        if (!r) return '<div style="color: #94a3b8; padding: 12px;">결과를 불러오는 중...</div>';

        const rateClass = r.duplicationPercentage >= 80 ? 'high' : r.duplicationPercentage >= 30 ? 'medium' : 'low';
        const targetId = r.targetFolderId || r.TargetFolderID || r.target_folder_id || r.targetFolderID;
        let uniqueCount = r.uniqueInTarget ? r.uniqueInTarget.length : 0;

        // uniqueInTarget이 null이면 비동기로 비중복 파일 개수를 로드
        if (!r.uniqueInTarget && r.id && !r._uniqueLoading) {
            r._uniqueLoading = true;
            window.API.Comparison.getUniqueFiles(r.id).then(data => {
                if (data && data.uniqueFiles) {
                    r.uniqueInTarget = data.uniqueFiles;
                    r._uniqueCount = data.totalCount || data.uniqueFiles.length;
                }
                r._uniqueLoading = false;
                CompareUI.updateJobCard(job.id);
            }).catch(() => { r._uniqueLoading = false; });
        }
        if (r._uniqueCount) uniqueCount = r._uniqueCount;

        let html = `
            <div class="result-summary">
                <div class="result-folder-info">
                    <div class="folder-label">기준 폴더</div>
                    <div class="folder-name">${r.sourceFolderName || '알 수 없음'}</div>
                    <div class="folder-stats">${r.sourceFileCount}개 파일 | ${formatFileSize(r.sourceTotalSize)}</div>
                </div>
                <div class="result-arrow"><i class="fas fa-arrow-right"></i></div>
                <div class="result-folder-info">
                    <div class="folder-label">대상 폴더</div>
                    <div class="folder-name">${r.targetFolderName || '알 수 없음'}</div>
                    <div class="folder-stats">${r.targetFileCount}개 파일 | ${formatFileSize(r.targetTotalSize)}</div>
                </div>
            </div>
            <div class="result-dup-rate">
                <div class="rate ${rateClass}">${r.duplicationPercentage.toFixed(1)}%</div>
                <div class="info">${r.duplicateCount || 0}개 중복 파일 | ${formatFileSize(r.duplicateSize || 0)} 절약 가능</div>
            </div>
        `;

        // 액션 버튼
        let buttonsHtml = '<div class="action-buttons">';
        if (r.duplicationPercentage === 100 && targetId) {
            buttonsHtml += `<button class="btn-delete-folder" onclick="event.stopPropagation(); deleteTargetFolder(${job.id}, '${targetId}')"><i class="fas fa-folder-minus"></i> 대상 폴더 전체 삭제</button>`;
        } else if (r.duplicateCount > 0) {
            buttonsHtml += `<button class="btn-delete-files" onclick="event.stopPropagation(); deleteDuplicateFiles(${job.id})"><i class="fas fa-files-medical"></i> 중복 파일만 삭제 (${r.duplicateCount}개)</button>`;
        }
        if (uniqueCount > 0) {
            buttonsHtml += `<button class="btn-move-unique" onclick="event.stopPropagation(); moveUniqueFiles(${job.id})"><i class="fas fa-folder-open"></i> 비중복 파일 이동 (${uniqueCount}개)</button>`;
        }
        buttonsHtml += '</div>';

        if (r.duplicateCount > 0 || uniqueCount > 0) {
            buttonsHtml += `
                <div style="margin-bottom: 12px;">
                    <label style="font-size: 0.85rem; color: #475569; cursor: pointer;">
                        <input type="checkbox" id="deleteEmptyFolders-${job.id}" checked>
                        삭제 후 빈 폴더도 함께 정리
                    </label>
                </div>
            `;
        }

        html += buttonsHtml;

        // 중복 파일 목록
        if (r.duplicateFiles && r.duplicateFiles.length > 0) {
            html += this.renderDuplicateFilesList(job, job._dupPage || 1);
        }

        return html;
    },

    renderDuplicateFilesList(job, page) {
        const files = job.comparisonResult?.duplicateFiles || [];
        if (files.length === 0) return '';

        const perPage = 10;
        const totalPages = Math.ceil(files.length / perPage);
        page = Math.max(1, Math.min(page, totalPages));
        const start = (page - 1) * perPage;
        const pageFiles = files.slice(start, start + perPage);

        let html = `
            <div class="dup-files-container">
                <div class="dup-files-header">
                    <span>중복 파일 목록 (총 ${files.length}개)</span>
                    <span>페이지 ${page}/${totalPages}</span>
                </div>
        `;

        pageFiles.forEach(file => {
            html += `
                <div class="dup-file-item">
                    <div class="dup-file-name">${file.name}</div>
                    <div class="dup-file-meta">${formatFileSize(file.size)} | ${new Date(file.modifiedTime).toLocaleDateString('ko-KR')}</div>
                </div>
            `;
        });

        // 페이지네이션
        if (totalPages > 1) {
            html += '<div class="pagination">';
            html += `<button ${page <= 1 ? 'disabled' : ''} onclick="event.stopPropagation(); changeDupPage(${job.id}, ${page - 1})"><i class="fas fa-chevron-left"></i></button>`;
            // 페이지 번호 (최대 5개)
            let startPage = Math.max(1, page - 2);
            let endPage = Math.min(totalPages, startPage + 4);
            if (endPage - startPage < 4) startPage = Math.max(1, endPage - 4);
            for (let i = startPage; i <= endPage; i++) {
                html += `<button class="${i === page ? 'active' : ''}" onclick="event.stopPropagation(); changeDupPage(${job.id}, ${i})">${i}</button>`;
            }
            html += `<button ${page >= totalPages ? 'disabled' : ''} onclick="event.stopPropagation(); changeDupPage(${job.id}, ${page + 1})"><i class="fas fa-chevron-right"></i></button>`;
            html += '</div>';
        }

        html += '</div>';
        return html;
    },

    renderSingleFolderResult(job) {
        const result = job.singleFolderResult;
        if (!result) {
            // 결과가 아직 없으면 비동기로 로드 시도
            if (job.progressId && !job._loadingResult) {
                job._loadingResult = true;
                loadJobResult(job).then(() => {
                    job._loadingResult = false;
                    CompareUI.updateJobCard(job.id);
                });
            }
            return '<div style="color: #94a3b8; padding: 12px;"><i class="fas fa-spinner fa-spin"></i> 결과를 불러오는 중...</div>';
        }

        const groups = result.duplicateGroups || [];
        if (groups.length === 0) {
            return `
                <div style="text-align: center; padding: 20px; color: #22c55e;">
                    <i class="fas fa-check-circle" style="font-size: 2rem; margin-bottom: 8px;"></i>
                    <div style="font-size: 1rem; font-weight: 600;">중복 파일이 없습니다!</div>
                </div>
            `;
        }

        const totalDups = groups.reduce((s, g) => s + g.files.length, 0);
        const totalSize = groups.reduce((s, g) => s + (g.size * g.files.length), 0);
        const savingsSize = totalSize - groups.reduce((s, g) => s + g.size, 0);

        let html = `
            <div style="background: #f0fdf4; padding: 12px 16px; border-radius: 8px; margin-bottom: 16px;">
                <strong style="color: #15803d;">${groups.length}개 중복 그룹</strong> |
                ${totalDups}개 파일 | 절약 가능: ${formatFileSize(savingsSize)}
            </div>
        `;

        groups.forEach((group, gi) => {
            const sorted = [...group.files].sort((a, b) => {
                return new Date(a.createdTime || a.modifiedTime || 0) - new Date(b.createdTime || b.modifiedTime || 0);
            });

            html += `<div class="dup-group">`;
            html += `
                <div class="dup-group-header">
                    <h4><i class="fas fa-layer-group"></i> 그룹 ${gi + 1} (${group.files.length}개, ${formatFileSize(group.size)} 각각)</h4>
                    <label style="font-size: 0.8rem; cursor: pointer;">
                        <input type="checkbox" class="group-select-all" onchange="toggleGroupSelection(${job.id}, ${gi}, this.checked)"> 모두 선택
                    </label>
                </div>
                <div class="dup-group-files">
            `;

            sorted.forEach((file, fi) => {
                const isOriginal = fi === 0;
                const created = file.createdTime ? new Date(file.createdTime).toLocaleDateString('ko-KR') : '-';
                const modified = file.modifiedTime ? new Date(file.modifiedTime).toLocaleDateString('ko-KR') : '-';

                html += `
                    <div class="dup-group-file ${isOriginal ? 'original' : ''}" data-job="${job.id}" data-group="${gi}">
                        <input type="checkbox" class="sf-file-select" data-job="${job.id}" data-group="${gi}" data-file-id="${file.id}">
                        <div style="flex: 1; min-width: 0;">
                            <div style="font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                                ${isOriginal ? '<i class="fas fa-crown original-crown" title="원본 추천"></i>' : ''}
                                ${file.name}
                            </div>
                            ${file.path ? `<div style="font-size: 0.78rem; color: #6366f1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${file.path}"><i class="fas fa-folder-open" style="margin-right: 3px;"></i>${file.path.substring(0, file.path.lastIndexOf('/')) || '/'}</div>` : ''}
                            <div style="font-size: 0.8rem; color: #94a3b8;">
                                생성: ${created} | 수정: ${modified} | ${formatFileSize(file.size)}
                            </div>
                        </div>
                        ${file.webViewLink ? `<a href="${file.webViewLink}" target="_blank" style="color: #3b82f6; font-size: 0.85rem;" onclick="event.stopPropagation();"><i class="fas fa-external-link-alt"></i></a>` : ''}
                    </div>
                `;
            });

            html += '</div></div>';
        });

        html += `
            <div style="margin-top: 16px; display: flex; gap: 10px; align-items: center;">
                <button class="btn-delete-files" onclick="event.stopPropagation(); deleteSelectedSingleFolderFiles(${job.id})">
                    <i class="fas fa-trash"></i> 선택한 파일 휴지통으로 이동
                </button>
                <label style="font-size: 0.85rem; color: #475569; cursor: pointer;">
                    <input type="checkbox" id="sfDeleteEmptyFolders-${job.id}" checked> 이동 후 빈 폴더 정리
                </label>
            </div>
        `;

        return html;
    },

    renderDeletionProgress(job) {
        const dp = job.deletionProgress;
        if (!dp) return '';

        if (dp.status === 'completed') {
            let html = `
                <div class="deletion-progress-area">
                    <div style="text-align: center; color: #22c55e; font-weight: 600; font-size: 1.1rem;">
                        <i class="fas fa-check-circle"></i> 삭제가 완료되었습니다!
                    </div>
                </div>
            `;
            return html;
        }

        const total = dp.totalItems || dp.total || 0;
        const processed = dp.processedItems || dp.completed || 0;
        const pct = total > 0 ? Math.round((processed / total) * 100) : 0;

        let html = `
            <div class="deletion-progress-area">
                <h4><i class="fas fa-trash"></i> 삭제 진행 중...</h4>
                <div class="job-progress-bar">
                    <div class="job-progress-fill" style="width: ${pct}%; background: linear-gradient(90deg, #ef4444, #f87171);"></div>
                </div>
                <div class="job-progress-info">
                    <span>${dp.currentStep || '삭제 중...'}</span>
                    <span>${pct}%</span>
                </div>
        `;

        // 빈 폴더 정리 현황
        const cleanupFolders = dp.metadata?.cleanupFolders;
        if (cleanupFolders && Array.isArray(cleanupFolders)) {
            html += `
                <div style="margin-top: 10px; font-size: 0.82rem; color: #64748b;">
                    빈 폴더 정리: ${dp.metadata.cleanupProcessed || 0}/${dp.metadata.cleanupTotal || 0}
                </div>
                <div style="max-height: 120px; overflow-y: auto; border: 1px solid #e5e7eb; border-radius: 6px; padding: 4px; margin-top: 4px;">
                    ${cleanupFolders.map(f => {
                        const icon = f.status === 'deleted' ? '✅' : f.status === 'skipped' ? '⏭️' : '❌';
                        return `<div class="cleanup-folder-item"><span>${icon}</span><span>${f.name}</span></div>`;
                    }).join('')}
                </div>
            `;
        }

        html += '</div>';
        return html;
    },

    renderMoveProgress(job) {
        const mp = job.moveProgress;
        if (!mp) return '';

        if (mp.status === 'completed') {
            return `
                <div class="deletion-progress-area">
                    <div style="text-align: center; color: #22c55e; font-weight: 600; font-size: 1.1rem;">
                        <i class="fas fa-check-circle"></i> 파일 이동이 완료되었습니다! (${mp.processedItems || 0}개 이동)
                    </div>
                </div>
            `;
        }

        const pct = mp.totalItems > 0 ? Math.round((mp.processedItems / mp.totalItems) * 100) : 0;
        return `
            <div class="deletion-progress-area">
                <h4><i class="fas fa-arrow-right" style="color: #22c55e;"></i> 파일 이동 중...</h4>
                <div class="job-progress-bar">
                    <div class="job-progress-fill" style="width: ${pct}%; background: linear-gradient(90deg, #22c55e, #4ade80);"></div>
                </div>
                <div class="job-progress-info">
                    <span>${mp.currentStep || '이동 중...'}</span>
                    <span>${pct}%</span>
                </div>
                <div style="text-align: center; margin-top: 10px;">
                    <button onclick="event.stopPropagation(); cancelMoveUniqueFiles(${job.id})" style="padding: 6px 16px; background: #ef4444; color: white; border: none; border-radius: 6px; cursor: pointer;">
                        <i class="fas fa-stop"></i> 이동 취소
                    </button>
                </div>
            </div>
        `;
    }
};

// ============ 탭 전환 ============
function switchTab(tabName) {
    document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(panel => panel.classList.remove('active'));

    document.getElementById(`tab-${tabName}`).classList.add('active');
    document.getElementById(`panel-${tabName}`).classList.add('active');
}

// URL 해시로 탭 자동 선택
function checkHashTab() {
    if (window.location.hash === '#single-folder') {
        switchTab('single-folder');
    }
}

// ============ 필터 ============
function setFilter(filter) {
    CompareUI.currentFilter = filter;
    document.querySelectorAll('.filter-btn').forEach(btn => {
        btn.classList.toggle('active', btn.textContent.trim() === {
            all: '전체', running: '진행중', completed: '완료', failed: '실패'
        }[filter]);
    });
    CompareUI.renderAllJobs();
}

// ============ 새 비교 시작 ============
async function startNewComparison() {
    const sourceUrl = document.getElementById('sourceFolderUrl')?.value.trim();
    const targetUrl = document.getElementById('targetFolderUrl')?.value.trim();

    if (!sourceUrl || !targetUrl) {
        alert('기준 폴더와 대상 폴더 URL을 모두 입력해주세요.');
        return;
    }

    try {
        const [sourceResp, targetResp] = await Promise.all([
            window.API.Comparison.extractFolderIdFromUrl(sourceUrl),
            window.API.Comparison.extractFolderIdFromUrl(targetUrl)
        ]);
        const sourceFolderId = sourceResp.folderId;
        const targetFolderId = targetResp.folderId;

        const excludeInput = document.getElementById('excludeFolderNames')?.value.trim() || '';
        const excludeFolderNames = excludeInput
            ? excludeInput.split(',').map(n => n.trim()).filter(n => n.length > 0)
            : [];

        const options = {
            includeSubfolders: document.getElementById('includeSubfolders')?.checked ?? true,
            deepComparison: document.getElementById('deepComparison')?.checked ?? true,
            forceNewComparison: document.getElementById('forceNewComparison')?.checked || false,
            excludeFolderNames
        };

        const response = await window.API.Comparison.compareFolders(sourceFolderId, targetFolderId, options);

        const jobId = JobManager.nextLocalId++;
        const job = {
            id: jobId,
            type: 'comparison',
            status: 'running',
            sourceFolderId,
            targetFolderId,
            sourceFolderName: sourceFolderId.substring(0, 12) + '...',
            targetFolderName: targetFolderId.substring(0, 12) + '...',
            startTime: Date.now(),
            progress: null,
            comparisonResult: null,
            comparisonId: null,
            expanded: true,
            deletionProgressId: null,
            deletionProgress: null,
            moveProgressId: null,
            moveProgress: null,
            _deletionType: null
        };

        if (response && response.progress) {
            job.progressId = response.progress.id;
        } else if (response && response.comparisonResult) {
            // 기존 결과 즉시 반환
            job.status = 'completed';
            job.comparisonResult = response.comparisonResult;
            job.comparisonId = response.comparisonResult.id;
        }

        // 폴더 이름 비동기 로드
        loadFolderNames(job, sourceFolderId, targetFolderId);

        JobManager.addJob(job);

        // 입력 필드 초기화
        document.getElementById('sourceFolderUrl').value = '';
        document.getElementById('targetFolderUrl').value = '';

    } catch (error) {
        console.error('폴더 비교 시작 오류:', error);
        alert('폴더 비교 중 오류가 발생했습니다: ' + error.message);
    }
}

async function loadFolderNames(job, sourceId, targetId) {
    const [sourceInfo, targetInfo] = await Promise.all([
        getFolderInfo(sourceId),
        getFolderInfo(targetId)
    ]);
    if (sourceInfo) job.sourceFolderName = sourceInfo.name || job.sourceFolderName;
    if (targetInfo) job.targetFolderName = targetInfo.name || job.targetFolderName;
    CompareUI.updateJobCard(job.id);
}

// ============ 단일 폴더 중복 검색 ============
async function startSingleFolderSearch() {
    const folderUrl = document.getElementById('singleFolderUrl')?.value.trim();
    if (!folderUrl) {
        alert('폴더 URL을 입력해주세요.');
        return;
    }

    try {
        const extractResp = await window.API.Comparison.extractFolderIdFromUrl(folderUrl);
        const folderId = extractResp.folderId;

        const options = {
            includeSubfolders: document.getElementById('includeSubfoldersOption')?.checked ?? true,
            minFileSize: parseInt(parseFloat(document.getElementById('minFileSizeOption')?.value || '1024') * 1024),
            forceNewScan: document.getElementById('forceNewScanOption')?.checked || false
        };

        const data = await window.API.Comparison.findSingleFolderDuplicates(folderId, options);

        const jobId = JobManager.nextLocalId++;
        const job = {
            id: jobId,
            type: 'single-folder',
            status: 'running',
            folderId,
            folderName: folderId.substring(0, 12) + '...',
            startTime: Date.now(),
            progressId: data.progressId || data.progress?.ID,
            progress: null,
            singleFolderResult: null,
            expanded: true,
            deletionProgressId: null,
            deletionProgress: null,
            moveProgressId: null,
            moveProgress: null
        };

        // 폴더 이름 비동기 로드
        getFolderInfo(folderId).then(info => {
            if (info) {
                job.folderName = info.name || job.folderName;
                CompareUI.updateJobCard(job.id);
            }
        });

        JobManager.addJob(job);

        document.getElementById('singleFolderUrl').value = '';

    } catch (error) {
        console.error('단일 폴더 중복 검색 오류:', error);
        alert('중복 검색 중 오류가 발생했습니다: ' + error.message);
    }
}

// ============ 작업 관리 ============
function toggleJobCard(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job) return;
    job.expanded = !job.expanded;
    CompareUI.updateJobCard(jobId);
}

async function cancelJob(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job || job.status !== 'running') return;

    if (!confirm('작업을 중단하시겠습니까?')) return;

    try {
        await window.API.Comparison.cancelComparison(job.progressId);
        job.status = 'cancelled';
        CompareUI.updateJobCard(jobId);
    } catch (error) {
        console.error('작업 취소 실패:', error);
        alert('작업 취소 중 오류가 발생했습니다: ' + error.message);
    }
}

async function deleteJobResult(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job) return;

    if (job.comparisonId) {
        try {
            await window.API.Comparison.deleteComparisonResult(job.comparisonId);
        } catch (e) {
            console.warn('비교 결과 삭제 실패 (무시):', e);
        }
    }

    JobManager.jobs.delete(jobId);
    CompareUI.renderAllJobs();
}

// ============ 중복 파일 페이지 전환 ============
function changeDupPage(jobId, page) {
    const job = JobManager.getJob(jobId);
    if (!job || !job.comparisonResult) return;
    job._dupPage = page;
    CompareUI.updateJobCard(jobId);
}

// ============ 삭제 작업 ============
function deleteTargetFolder(jobId, targetFolderId) {
    const job = JobManager.getJob(jobId);
    if (!job) return;

    const message = `
        <div style="margin-bottom: 16px;">
            <strong style="color: #ef4444;">대상 폴더 전체 삭제</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280;">대상 폴더를 삭제하시겠습니까?</div>
    `;

    showDeleteConfirmation(message, async (data) => {
        try {
            const permanent = data?.permanentDelete || false;
            const compId = job.comparisonId || job.comparisonResult?.id;
            const response = await window.API.Comparison.deleteTargetFolder(compId, targetFolderId, permanent);
            if (response && response.progress) {
                job.deletionProgressId = response.progress.id;
                job._deletionType = 'folder';
                job.expanded = true;
                JobManager.startPolling();
                CompareUI.updateJobCard(jobId);
            }
        } catch (error) {
            console.error('폴더 삭제 오류:', error);
            alert('폴더 삭제 중 오류: ' + error.message);
        }
    }, null, true);
}

function deleteDuplicateFiles(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job || !job.comparisonResult) return;

    const fileCount = job.comparisonResult.duplicateFiles?.length || 0;
    const deleteEmptyEl = document.getElementById(`deleteEmptyFolders-${jobId}`);

    const message = `
        <div style="margin-bottom: 16px;">
            <strong>중복 파일 삭제</strong>
        </div>
        <div style="font-size: 0.9rem; color: #6b7280;">${fileCount}개의 중복 파일을 삭제하시겠습니까?</div>
    `;

    showDeleteConfirmation(message, async (data) => {
        try {
            const permanent = data?.permanentDelete || false;
            const fileIds = job.comparisonResult.duplicateFiles.map(f => f.id);
            const deleteEmpty = deleteEmptyEl?.checked || false;
            const compId = job.comparisonId || job.comparisonResult?.id;
            const response = await window.API.Comparison.deleteDuplicateFiles(compId, fileIds, deleteEmpty, permanent);
            if (response && response.progress) {
                job.deletionProgressId = response.progress.id;
                job._deletionType = 'files';
                job.expanded = true;
                JobManager.startPolling();
                CompareUI.updateJobCard(jobId);
            }
        } catch (error) {
            console.error('파일 삭제 오류:', error);
            alert('파일 삭제 중 오류: ' + error.message);
        }
    }, null, true);
}

// ============ 비중복 파일 이동 ============
async function moveUniqueFiles(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job || !job.comparisonResult) return;

    const compId = job.comparisonId || job.comparisonResult?.id;

    if (!confirm('비중복 파일을 기준 폴더로 이동하시겠습니까?')) return;

    try {
        const response = await window.API.Comparison.moveUniqueFiles(compId, {
            preservePath: true,
            onConflict: 'rename'
        });
        const progressId = response.progress?.id || response.progressId;
        if (progressId) {
            job.moveProgressId = progressId;
            job.expanded = true;
            JobManager.startPolling();
            CompareUI.updateJobCard(jobId);
        }
    } catch (error) {
        console.error('파일 이동 오류:', error);
        alert('파일 이동 중 오류: ' + error.message);
    }
}

async function cancelMoveUniqueFiles(jobId) {
    const job = JobManager.getJob(jobId);
    if (!job || !job.moveProgressId) return;

    if (!confirm('파일 이동을 취소하시겠습니까?')) return;

    try {
        await window.API.Comparison.cancelMoveUniqueFiles(job.moveProgressId);
        job.moveProgressId = null;
        CompareUI.updateJobCard(jobId);
    } catch (error) {
        console.error('이동 취소 실패:', error);
    }
}

// ============ 단일 폴더 파일 삭제 ============
async function deleteSelectedSingleFolderFiles(jobId) {
    const checkboxes = document.querySelectorAll(`.sf-file-select[data-job="${jobId}"]:checked`);
    if (checkboxes.length === 0) {
        alert('휴지통으로 이동할 파일을 선택해주세요.');
        return;
    }

    const fileIds = Array.from(checkboxes).map(cb => cb.getAttribute('data-file-id'));
    if (!confirm(`선택한 ${fileIds.length}개의 파일을 휴지통으로 이동하시겠습니까?`)) return;

    try {
        await window.API.Cleanup.deleteFiles(fileIds);

        const deleteEmpty = document.getElementById(`sfDeleteEmptyFolders-${jobId}`)?.checked;
        if (deleteEmpty) {
            await window.API.Cleanup.cleanupEmptyFolders();
        }

        alert(`${fileIds.length}개 파일이 휴지통으로 이동되었습니다.`);

        // UI에서 제거
        fileIds.forEach(id => {
            const el = document.querySelector(`[data-file-id="${id}"]`);
            if (el) el.closest('.dup-group-file')?.remove();
        });
    } catch (error) {
        console.error('파일 삭제 오류:', error);
        alert('파일 삭제 중 오류: ' + error.message);
    }
}

function toggleGroupSelection(jobId, groupIndex, checked) {
    const checkboxes = document.querySelectorAll(`.sf-file-select[data-job="${jobId}"][data-group="${groupIndex}"]:not([disabled])`);
    checkboxes.forEach(cb => cb.checked = checked);
}

// ============ 초기화 ============
document.addEventListener('DOMContentLoaded', async () => {
    // 해시로 탭 자동 선택
    checkHashTab();

    // 폴더 URL 입력에 경로 자동 표시 연결
    ['sourceFolderUrl', 'targetFolderUrl', 'singleFolderUrl'].forEach(id => {
        const el = document.getElementById(id);
        if (el) attachFolderResolver(el);
    });

    // 진행 중인 작업 로드
    await loadPendingComparisons();

    // 최근 완료된 작업 로드
    await loadRecentComparisons();

    // 최근 완료된 단일 폴더 중복 검색 결과 로드
    await loadRecentSingleFolderResults();

    // 활성 이동 작업 복구
    await restoreActiveMoveOperations();

    // 전체 렌더링
    CompareUI.renderAllJobs();
});

async function loadPendingComparisons() {
    try {
        const pending = await window.API.Comparison.getPendingComparisons();
        if (pending && pending.length > 0) {
            for (const p of pending) {
                const meta = p.metadata || {};
                const jobId = JobManager.nextLocalId++;
                const job = {
                    id: jobId,
                    type: meta.type === 'single-folder' ? 'single-folder' : 'comparison',
                    status: 'running',
                    sourceFolderId: meta.sourceFolderId,
                    targetFolderId: meta.targetFolderId,
                    sourceFolderName: meta.sourceFolderId ? meta.sourceFolderId.substring(0, 12) + '...' : '-',
                    targetFolderName: meta.targetFolderId ? meta.targetFolderId.substring(0, 12) + '...' : '-',
                    folderId: meta.folderId,
                    folderName: meta.folderId ? meta.folderId.substring(0, 12) + '...' : '-',
                    startTime: new Date(p.startTime).getTime(),
                    progressId: p.id,
                    progress: p,
                    comparisonResult: null,
                    comparisonId: null,
                    singleFolderResult: null,
                    expanded: false,
                    deletionProgressId: null,
                    deletionProgress: null,
                    moveProgressId: null,
                    moveProgress: null
                };

                // 폴더 이름 로드
                if (meta.sourceFolderId && meta.targetFolderId) {
                    loadFolderNames(job, meta.sourceFolderId, meta.targetFolderId);
                } else if (meta.folderId) {
                    getFolderInfo(meta.folderId).then(info => {
                        if (info) {
                            job.folderName = info.name || job.folderName;
                            CompareUI.updateJobCard(job.id);
                        }
                    });
                }

                JobManager.addJob(job);
            }
        }
    } catch (error) {
        console.warn('진행 중 작업 로드 실패:', error);
    }
}

async function restoreActiveMoveOperations() {
    try {
        const activeOps = await window.API.Comparison.getActiveMoveOperations();
        if (!activeOps || activeOps.length === 0) return;

        for (const op of activeOps) {
            if (!op.comparisonId || !op.progressId) continue;

            // Find the job that matches this comparisonId
            const matchedJob = [...JobManager.jobs.values()].find(j =>
                j.comparisonId === op.comparisonId ||
                (j.comparisonResult && j.comparisonResult.id === op.comparisonId)
            );

            if (matchedJob && !matchedJob.moveProgressId) {
                matchedJob.moveProgressId = op.progressId;
                matchedJob.expanded = true;
                console.log(`🔄 이동 작업 복구: job=${matchedJob.id}, progressId=${op.progressId}, comparisonId=${op.comparisonId}`);
            }
        }

        // Start polling if any operations were restored
        if (JobManager.getActiveJobs().length > 0) {
            JobManager.startPolling();
        }
    } catch (error) {
        console.warn('활성 이동 작업 복구 실패:', error);
    }
}

async function loadRecentSingleFolderResults() {
    try {
        const results = await window.API.Comparison.getRecentSingleFolderResults(20);
        if (results && Array.isArray(results) && results.length > 0) {
            for (const p of results) {
                // 이미 추가된 작업이면 건너뜀
                const existing = [...JobManager.jobs.values()].find(j =>
                    j.type === 'single-folder' && j.progressId === p.id
                );
                if (existing) continue;

                const meta = p.metadata || {};
                const result = meta.result || null;
                const folderId = meta.folderId || '';

                const jobId = JobManager.nextLocalId++;
                const job = {
                    id: jobId,
                    type: 'single-folder',
                    status: 'completed',
                    folderId: folderId,
                    folderName: folderId ? folderId.substring(0, 12) + '...' : '-',
                    startTime: new Date(p.startTime).getTime(),
                    progressId: p.id,
                    progress: p,
                    singleFolderResult: result,
                    expanded: false,
                    deletionProgressId: null,
                    deletionProgress: null,
                    moveProgressId: null,
                    moveProgress: null
                };

                // 폴더 이름 비동기 로드
                if (folderId) {
                    getFolderInfo(folderId).then(info => {
                        if (info) {
                            job.folderName = info.name || job.folderName;
                            CompareUI.updateJobCard(job.id);
                        }
                    });
                }

                JobManager.jobs.set(jobId, job);
            }
        }
    } catch (error) {
        console.warn('단일 폴더 중복 검색 결과 로드 실패:', error);
    }
}

async function loadRecentComparisons() {
    try {
        const recent = await window.API.Comparison.getRecentComparisons(20);
        if (recent && Array.isArray(recent) && recent.length > 0) {
            for (const r of recent) {
                // 이미 진행 중으로 추가된 것은 건너뜀
                const existing = [...JobManager.jobs.values()].find(j =>
                    j.comparisonId === r.id ||
                    (j.sourceFolderId === r.sourceFolderId && j.targetFolderId === r.targetFolderId)
                );
                if (existing) {
                    // 진행 중이던 작업이 완료된 경우 결과 업데이트
                    if (existing.status === 'running' || !existing.comparisonResult) {
                        existing.comparisonResult = r;
                        existing.comparisonId = r.id;
                        if (existing.status === 'running') existing.status = 'completed';
                    }
                    continue;
                }

                const jobId = JobManager.nextLocalId++;
                const job = {
                    id: jobId,
                    type: 'comparison',
                    status: 'completed',
                    sourceFolderId: r.sourceFolderId,
                    targetFolderId: r.targetFolderId,
                    sourceFolderName: r.sourceFolderName || r.sourceFolderId?.substring(0, 12) + '...',
                    targetFolderName: r.targetFolderName || r.targetFolderId?.substring(0, 12) + '...',
                    startTime: new Date(r.createdAt || r.created_at).getTime(),
                    comparisonResult: r,
                    comparisonId: r.id,
                    expanded: false,
                    deletionProgressId: null,
                    deletionProgress: null,
                    moveProgressId: null,
                    moveProgress: null
                };
                JobManager.jobs.set(jobId, job);
            }
        }
    } catch (error) {
        console.warn('최근 비교 결과 로드 실패:', error);
    }
}
