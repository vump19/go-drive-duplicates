let progressInterval;
let duplicatesInterval;

let currentSettings = {};

// 간단한 경로 계산 시스템
let currentPage = 1;
let totalPages = 1;

// 일괄 삭제 관련 함수들
let filesToDelete = [];

// 폴더 비교 관련 변수들
let comparisonResult = null;
let duplicatesFromComparison = [];
let comparisonProgressInterval = null;

window.onload = function() {
    loadSettings();
    checkForSavedData();
    setupSettingsControls();
};

function checkForSavedData() {
    console.log('🔍 저장된 데이터 확인 중...');
    
    // 기존 중복 검사 진행 상황 확인
    fetch('/progress')
        .then(response => response.json())
        .then(data => {
            console.log('📊 진행 상태 데이터:', data);
            if (data.progress && (data.progress.Status === 'running' || data.progress.Status === 'completed')) {
                document.getElementById('resumeBtn').style.display = 'inline-block';
                document.getElementById('resetBtn').style.display = 'inline-block';
                document.getElementById('duplicates-section').style.display = 'block';
                
                if (data.progress.Status === 'completed' && data.duplicates) {
                    console.log('✅ 완료된 검사 결과 표시');
                    displayResults({duplicates: data.duplicates, totalFiles: data.progress.TotalFiles});
                    displayLiveDuplicates(data.duplicates);
                    // 완료된 상태에서도 중복 파일 로드
                    loadLiveDuplicates();
                } else if (data.progress.Status === 'running') {
                    console.log('🔄 진행 중인 작업 발견');
                    startProgressMonitoring();
                    document.getElementById('duplicates-section').style.display = 'block';
                    loadLiveDuplicates();
                }
            } else {
                console.log('ℹ️ 저장된 진행 상태 없음');
            }
        })
        .catch(err => console.error('❌ 진행 상태 확인 오류:', err));
    
    // 폴더 비교 작업 진행 상황 확인
    fetch('/comparison-progress')
        .then(response => response.json())
        .then(data => {
            console.log('📂 폴더 비교 진행 상태:', data);
            if (data.status === 'running' || data.status === 'paused') {
                console.log('🔄 진행 중이거나 일시정지된 폴더 비교 작업 발견');
                showResumeComparisonNotification(data);
                startComparisonProgressMonitoring();
            } else if (data.status === 'completed') {
                console.log('✅ 완료된 폴더 비교 작업 발견');
                checkComparisonResult();
            }
        })
        .catch(err => console.log('ℹ️ 폴더 비교 진행 상태 없음 또는 오류:', err));
    
    // 저장된 폴더 비교 작업 확인
    checkSavedComparisonTasks();
}

function displayResults(data) {
    const resultsDiv = document.getElementById('results');
    if (data.duplicates && data.duplicates.length > 0) {
        resultsDiv.innerHTML = 
            '<div class="stats">' +
                '<h3>🎯 검사 완료!</h3>' +
                '<p><strong>' + data.duplicates.length + '개</strong>의 중복 그룹을 발견했습니다.</p>' +
                '<p>총 <strong>' + data.totalFiles + '개</strong> 파일을 검사했습니다.</p>' +
            '</div>';
    }
}

function loadLiveDuplicates(page = 1) {
    console.log('🔍 중복 파일 로드 중... (페이지 ' + page + ')');
    fetch('/duplicates?page=' + page + '&limit=20')
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                console.log('❌ 중복 로드 오류:', data.error);
                return;
            }
            console.log('📊 중복 데이터 받음:', data.duplicates ? data.duplicates.length : 0, '개 그룹');
            currentPage = data.page || 1;
            totalPages = data.totalPages || 1;
            displayLiveDuplicates(data.duplicates || [], data);
        })
        .catch(error => {
            console.log('❌ 중복 로드 오류:', error);
        });
}

function manualRefresh() {
    console.log('🔄 수동 새로고침 시작');
    document.getElementById('duplicates-section').style.display = 'block';
    loadLiveDuplicates();
}

function loadSinglePath(fileId) {
    const pathElement = document.getElementById('path-' + fileId);
    if (pathElement) {
        pathElement.textContent = '조회 중...';
        pathElement.style.color = '#007bff';
    }
    
    fetch('/file-path?id=' + fileId)
        .then(response => response.json())
        .then(data => {
            if (pathElement) {
                const button = pathElement.nextElementSibling;
                
                if (data.status === 'deleted') {
                    // 파일이 삭제된 경우
                    pathElement.innerHTML = '❌ 파일이 삭제됨';
                    pathElement.style.color = '#dc3545';
                    pathElement.style.fontWeight = 'bold';
                    
                    // 파일 항목 전체에 삭제 표시
                    const fileItem = pathElement.closest('.file-item');
                    if (fileItem) {
                        fileItem.style.opacity = '0.6';
                        fileItem.style.backgroundColor = '#ffebee';
                        fileItem.style.border = '1px solid #f44336';
                        fileItem.style.borderRadius = '3px';
                        fileItem.style.padding = '5px';
                        
                        // 파일 제목에도 삭제 표시 추가
                        const fileLink = fileItem.querySelector('.file-link');
                        if (fileLink) {
                            fileLink.style.textDecoration = 'line-through';
                            fileLink.style.color = '#999';
                        }
                    }
                    
                } else if (data.status === 'trashed') {
                    // 파일이 휴지통에 있는 경우
                    pathElement.innerHTML = '🗑️ 휴지통에 있음 (' + (data.name || 'Unknown') + ')';
                    pathElement.style.color = '#ff9800';
                    pathElement.style.fontWeight = 'bold';
                    
                    // 파일 항목 전체에 휴지통 표시
                    const fileItem = pathElement.closest('.file-item');
                    if (fileItem) {
                        fileItem.style.opacity = '0.7';
                        fileItem.style.backgroundColor = '#fff3e0';
                        fileItem.style.border = '1px solid #ff9800';
                        fileItem.style.borderRadius = '3px';
                        fileItem.style.padding = '5px';
                        
                        // 파일 제목에도 휴지통 표시 추가
                        const fileLink = fileItem.querySelector('.file-link');
                        if (fileLink) {
                            fileLink.style.color = '#ff9800';
                        }
                    }
                    
                } else if (data.status === 'exists' && data.path) {
                    // 파일이 정상적으로 존재하는 경우
                    pathElement.textContent = data.path;
                    pathElement.style.color = '#000';
                    pathElement.style.fontStyle = 'normal';
                    
                } else {
                    // 기타 오류
                    pathElement.textContent = data.message || '조회 실패';
                    pathElement.style.color = '#dc3545';
                }
                
                // 경로 조회 버튼 숨기기
                if (button && button.tagName === 'BUTTON') {
                    button.style.display = 'none';
                }
            }
        })
        .catch(error => {
            console.log('경로 계산 실패:', fileId, error);
            if (pathElement) {
                pathElement.textContent = '조회 실패';
                pathElement.style.color = '#dc3545';
            }
        });
}

function updateParents() {
    if (!confirm('데이터베이스의 모든 파일에 대해 경로 정보를 업데이트합니다. 시간이 오래 걸릴 수 있습니다. 계속하시겠습니까?')) {
        return;
    }
    
    fetch('/update-parents', { method: 'POST' })
        .then(response => response.json())
        .then(data => {
            alert('경로 정보 업데이트가 시작되었습니다. 서버 로그를 확인하세요.');
        })
        .catch(error => {
            alert('업데이트 시작 실패: ' + error);
        });
}

function scanDuplicates() {
    if (!confirm('새로운 중복 파일 검사를 시작합니다. 시간이 오래 걸릴 수 있습니다. 계속하시겠습니까?')) {
        return;
    }
    
    document.getElementById('scanBtn').disabled = true;
    document.getElementById('scanBtn').textContent = '검사 중...';
    
    fetch('/scan', { method: 'POST' })
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                alert('검사 시작 실패: ' + data.error);
            } else {
                alert('중복 파일 검사가 시작되었습니다.');
                // 진행 상황 모니터링 시작
                startProgressMonitoring();
            }
        })
        .catch(error => {
            alert('검사 시작 실패: ' + error);
        })
        .finally(() => {
            document.getElementById('scanBtn').disabled = false;
            document.getElementById('scanBtn').textContent = '중복 파일 검사 시작';
        });
}

function resumeScan() {
    if (!confirm('저장된 작업을 재개합니다. 계속하시겠습니까?')) {
        return;
    }
    
    fetch('/resume', { method: 'POST' })
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                alert('작업 재개 실패: ' + data.error);
            } else {
                alert('저장된 작업을 재개합니다.');
                startProgressMonitoring();
            }
        });
}

function resetData() {
    if (!confirm('모든 데이터를 삭제합니다. 이 작업은 되돌릴 수 없습니다. 정말 계속하시겠습니까?')) {
        return;
    }
    
    fetch('/reset', { method: 'POST' })
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                alert('데이터 삭제 실패: ' + data.error);
            } else {
                alert('모든 데이터가 삭제되었습니다.');
                location.reload();
            }
        });
}

function startProgressMonitoring() {
    if (progressInterval) {
        clearInterval(progressInterval);
    }
    
    progressInterval = setInterval(checkProgress, 2000);
    document.getElementById('progress').style.display = 'block';
}

function checkProgress() {
    fetch('/progress')
        .then(response => response.json())
        .then(data => {
            if (data.progress) {
                updateProgressDisplay(data.progress);
                
                if (data.progress.Status === 'completed') {
                    clearInterval(progressInterval);
                    alert('중복 파일 검사가 완료되었습니다!');
                    location.reload();
                }
            }
        });
}

function updateProgressDisplay(progress) {
    const progressDiv = document.getElementById('progress');
    const percentage = progress.TotalFiles > 0 ? (progress.ProcessedFiles / progress.TotalFiles * 100).toFixed(1) : 0;
    progressDiv.innerHTML = 
        '<div style="background: #f8f9fa; padding: 20px; border-radius: 5px; margin: 20px 0;">' +
            '<h3>진행 상황</h3>' +
            '<p>상태: ' + progress.Status + '</p>' +
            '<p>처리된 파일: ' + progress.ProcessedFiles + ' / ' + progress.TotalFiles + '</p>' +
            '<div style="width: 100%; background: #e9ecef; border-radius: 5px; height: 20px;">' +
                '<div style="width: ' + percentage + '%; background: #007bff; height: 100%; border-radius: 5px;"></div>' +
            '</div>' +
        '</div>';
}

function applyWorkerSettings() {
    const workerCount = getCurrentWorkerCount();
    
    fetch('/set-workers', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: 'workers=' + workerCount
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('설정 실패: ' + data.error);
        } else {
            alert('해시 계산 워커 수가 ' + workerCount + '개로 설정되었습니다.');
            // 로컬 스토리지에 저장
            localStorage.setItem('workerCount', workerCount);
        }
    })
    .catch(error => {
        alert('설정 중 오류 발생: ' + error);
    });
}

function getCurrentWorkerCount() {
    return parseInt(document.getElementById('workerValue').textContent) || 5;
}

function setWorkerCount(count) {
    count = Math.max(1, Math.min(20, count)); // 1-20 범위로 제한
    
    document.getElementById('workerValue').textContent = count;
    document.getElementById('workerSlider').value = count;
    
    updateWorkerButtons();
    updateRecommendedBadge(count);
}

function updateWorkerValue(value) {
    setWorkerCount(parseInt(value));
}

function increaseWorkers() {
    const current = getCurrentWorkerCount();
    setWorkerCount(current + 1);
}

function decreaseWorkers() {
    const current = getCurrentWorkerCount();
    setWorkerCount(current - 1);
}

function updateWorkerButtons() {
    const current = getCurrentWorkerCount();
    const decreaseBtn = document.getElementById('workerDecreaseBtn');
    const increaseBtn = document.getElementById('workerIncreaseBtn');
    
    decreaseBtn.disabled = current <= 1;
    increaseBtn.disabled = current >= 20;
}

function updateRecommendedBadge(count) {
    const recommendedBadge = document.getElementById('recommendedBadge');
    const recommendedValue = Math.max(1, Math.floor(navigator.hardwareConcurrency / 2));
    
    if (count === recommendedValue) {
        recommendedBadge.style.display = 'inline-block';
    } else {
        recommendedBadge.style.display = 'none';
    }
}

// 중복 검사 워커 관련 함수들
function setDuplicateWorkerCount(count) {
    count = Math.max(1, Math.min(20, count)); // 1-20 범위로 제한
    
    document.getElementById('duplicateWorkerValue').textContent = count;
    document.getElementById('duplicateWorkerSlider').value = count;
    
    updateDuplicateRecommendedBadge(count);
}

function updateDuplicateRecommendedBadge(count) {
    const recommendedBadge = document.getElementById('duplicateRecommendedBadge');
    const recommendedValue = Math.max(1, Math.floor(navigator.hardwareConcurrency / 2));
    
    if (count === recommendedValue) {
        recommendedBadge.style.display = 'inline-block';
    } else {
        recommendedBadge.style.display = 'none';
    }
}

function applyDuplicateWorkerSettings() {
    const workerCount = parseInt(document.getElementById('duplicateWorkerValue').textContent);
    
    // 로컬 스토리지에 저장
    localStorage.setItem('duplicateWorkerCount', workerCount);
    
    // 기존 코드와 호환성을 위해 기존 API 사용 (별도의 API를 만들 수도 있음)
    fetch('/set-workers', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: 'workers=' + workerCount + '&type=duplicate'
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('설정 실패: ' + data.error);
        } else {
            alert('중복 파일 검사 워커 수가 ' + workerCount + '개로 설정되었습니다.');
        }
    })
    .catch(error => {
        // 서버 API가 없어도 로컬에는 저장됨
        alert('중복 파일 검사 워커 수가 ' + workerCount + '개로 설정되었습니다.');
        console.log('로컬 저장만 완료 (서버 API 없음):', error);
    });
}

// 현재 설정된 중복 검사 워커 수 가져오기
function getCurrentDuplicateWorkerCount() {
    const savedCount = localStorage.getItem('duplicateWorkerCount');
    return savedCount ? parseInt(savedCount) : 3; // 기본값 3
}

function setupSettingsControls() {
    // 해시 워커 설정 로드
    let savedWorkerCount = localStorage.getItem('workerCount');
    if (!savedWorkerCount) {
        // 기본값: CPU 코어 수의 절반, 최소 1개, 최대 5개
        savedWorkerCount = Math.max(1, Math.min(5, Math.floor(navigator.hardwareConcurrency / 2)));
    } else {
        savedWorkerCount = parseInt(savedWorkerCount);
    }
    
    // 중복 검사 워커 설정 로드
    let savedDuplicateWorkerCount = localStorage.getItem('duplicateWorkerCount');
    if (!savedDuplicateWorkerCount) {
        // 기본값: CPU 코어 수의 절반, 최소 1개, 최대 5개
        savedDuplicateWorkerCount = Math.max(1, Math.min(5, Math.floor(navigator.hardwareConcurrency / 2)));
    } else {
        savedDuplicateWorkerCount = parseInt(savedDuplicateWorkerCount);
    }
    
    // 초기값 설정
    setWorkerCount(savedWorkerCount);
    setDuplicateWorkerCount(savedDuplicateWorkerCount);
    
    // 슬라이더 이벤트 리스너들
    const slider = document.getElementById('workerSlider');
    slider.addEventListener('input', function() {
        updateWorkerValue(this.value);
    });
    
    const duplicateSlider = document.getElementById('duplicateWorkerSlider');
    const duplicateValueDisplay = document.getElementById('duplicateWorkerValue');
    const duplicateRecommendedBadge = document.getElementById('duplicateRecommendedBadge');
    
    duplicateSlider.addEventListener('input', function() {
        duplicateValueDisplay.textContent = this.value;
        
        // 권장 설정 표시 (CPU 코어 수의 절반)
        const recommendedValue = Math.max(1, Math.floor(navigator.hardwareConcurrency / 2));
        if (parseInt(this.value) === recommendedValue) {
            duplicateRecommendedBadge.style.display = 'inline-block';
        } else {
            duplicateRecommendedBadge.style.display = 'none';
        }
    });
}

function loadSettings() {
    fetch('/settings')
        .then(response => response.json())
        .then(data => {
            currentSettings = data;
            
            // 서버에서 현재 해시 워커 수 설정 로드
            if (data.currentWorkers) {
                setWorkerCount(data.currentWorkers);
                console.log('⚙️ 서버에서 해시 워커 설정 로드:', data.currentWorkers);
            }
            
            // 로컬 저장된 중복 검사 워커 수 로드
            const savedDuplicateWorkerCount = localStorage.getItem('duplicateWorkerCount');
            if (savedDuplicateWorkerCount) {
                setDuplicateWorkerCount(parseInt(savedDuplicateWorkerCount));
                console.log('⚙️ 로컬에서 중복 검사 워커 설정 로드:', savedDuplicateWorkerCount);
            }
        })
        .catch(error => {
            console.log('❌ 설정 로드 오류:', error);
        });
}

function searchFilesToDelete() {
    const folderUrl = document.getElementById('folderUrl').value.trim();
    const regexPattern = document.getElementById('regexPattern').value.trim();
    
    if (!folderUrl) {
        alert('폴더 URL을 입력해주세요.');
        return;
    }
    
    if (!regexPattern) {
        alert('필터 패턴을 입력해주세요.');
        return;
    }
    
    // 폴더 ID 추출
    const folderIdMatch = folderUrl.match(/folders\/([a-zA-Z0-9-_]+)/);
    if (!folderIdMatch) {
        alert('올바른 Google Drive 폴더 URL을 입력해주세요.');
        return;
    }
    
    const folderId = folderIdMatch[1];
    
    document.getElementById('searchBtn').disabled = true;
    document.getElementById('searchBtn').textContent = '검색 중...';
    
    fetch('/search-files-to-delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            folderId: folderId,
            regexPattern: regexPattern
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('검색 실패: ' + data.error);
            return;
        }
        
        filesToDelete = data.files || [];
        displayFilesToDelete(filesToDelete);
    })
    .catch(error => {
        alert('검색 중 오류 발생: ' + error);
    })
    .finally(() => {
        document.getElementById('searchBtn').disabled = false;
        document.getElementById('searchBtn').textContent = '🔍 삭제 대상 파일 검색';
    });
}

function displayFilesToDelete(files) {
    const previewDiv = document.getElementById('delete-preview');
    const filesDiv = document.getElementById('files-to-delete');
    
    if (files.length === 0) {
        alert('지정된 패턴과 일치하는 파일이 없습니다.');
        previewDiv.style.display = 'none';
        return;
    }
    
    let html = '<div style="background: #f8f9fa; padding: 15px; border-radius: 5px; margin-bottom: 15px;">';
    html += '<p><strong>총 ' + files.length + '개 파일이 삭제 대상으로 선택되었습니다.</strong></p>';
    html += '</div>';
    
    html += '<div style="max-height: 400px; overflow-y: auto; border: 1px solid #ddd; border-radius: 3px;">';
    files.forEach((file, index) => {
        html += '<div style="padding: 10px; border-bottom: 1px solid #eee; ' + (index % 2 === 0 ? 'background: #f9f9f9;' : '') + '">';
        html += '<div style="font-weight: bold;">📄 ' + file.name + '</div>';
        html += '<div style="font-size: 12px; color: #666;">크기: ' + formatFileSize(file.size) + '</div>';
        if (file.modifiedTime) {
            html += '<div style="font-size: 12px; color: #666;">수정일: ' + new Date(file.modifiedTime).toLocaleDateString('ko-KR') + '</div>';
        }
        html += '</div>';
    });
    html += '</div>';
    
    filesDiv.innerHTML = html;
    previewDiv.style.display = 'block';
}

function confirmBulkDelete() {
    if (filesToDelete.length === 0) {
        alert('삭제할 파일이 없습니다.');
        return;
    }
    
    const confirmMsg = '정말로 ' + filesToDelete.length + '개 파일을 삭제하시겠습니까?\n\n⚠️ 이 작업은 되돌릴 수 없습니다!';
    if (!confirm(confirmMsg)) {
        return;
    }
    
    document.getElementById('confirmDeleteBtn').disabled = true;
    document.getElementById('confirmDeleteBtn').textContent = '삭제 중...';
    
    const fileIds = filesToDelete.map(file => file.id);
    
    fetch('/bulk-delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            fileIds: fileIds
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('삭제 실패: ' + data.error);
            return;
        }
        
        alert('성공적으로 ' + data.deletedCount + '개 파일을 삭제했습니다.');
        cancelBulkDelete();
    })
    .catch(error => {
        alert('삭제 중 오류 발생: ' + error);
    })
    .finally(() => {
        document.getElementById('confirmDeleteBtn').disabled = false;
        document.getElementById('confirmDeleteBtn').textContent = '❌ 선택된 파일들 삭제';
    });
}

function cancelBulkDelete() {
    document.getElementById('delete-preview').style.display = 'none';
    filesToDelete = [];
    document.getElementById('folderUrl').value = '';
    document.getElementById('regexPattern').value = '';
}

function displayLiveDuplicates(duplicates, pageInfo) {
    console.log('🖥️ 중복 파일 표시 중:', duplicates.length, '개 그룹');
    const liveDuplicatesDiv = document.getElementById('live-duplicates');
    
    // 페이지네이션 정보 표시
    let html = '';
    if (pageInfo && pageInfo.totalPages > 1) {
        html += '<div style="background: #f8f9fa; padding: 15px; margin-bottom: 20px; border-radius: 5px; text-align: center;">';
        html += '<p><strong>페이지 ' + pageInfo.page + ' / ' + pageInfo.totalPages + '</strong> (총 ' + pageInfo.total + '개 중복 그룹)</p>';
        html += '<div>';
        
        // 이전 페이지 버튼
        if (pageInfo.page > 1) {
            html += '<button onclick="loadLiveDuplicates(' + (pageInfo.page - 1) + ')" style="margin: 5px; padding: 8px 12px;">← 이전</button>';
        }
        
        // 페이지 번호들 (현재 페이지 주변만)
        let startPage = Math.max(1, pageInfo.page - 2);
        let endPage = Math.min(pageInfo.totalPages, pageInfo.page + 2);
        
        for (let i = startPage; i <= endPage; i++) {
            if (i === pageInfo.page) {
                html += '<button style="margin: 5px; padding: 8px 12px; background: #007bff; color: white; font-weight: bold;">' + i + '</button>';
            } else {
                html += '<button onclick="loadLiveDuplicates(' + i + ')" style="margin: 5px; padding: 8px 12px;">' + i + '</button>';
            }
        }
        
        // 다음 페이지 버튼
        if (pageInfo.page < pageInfo.totalPages) {
            html += '<button onclick="loadLiveDuplicates(' + (pageInfo.page + 1) + ')" style="margin: 5px; padding: 8px 12px;">다음 →</button>';
        }
        
        html += '</div></div>';
    }
    
    if (duplicates.length === 0) {
        liveDuplicatesDiv.innerHTML = html + '<p style="color: #666;">아직 중복 파일이 발견되지 않았습니다...</p>';
        return;
    }
    duplicates.forEach((group, index) => {
        html += '<div class="file-group" id="group-' + index + '">';
        html += '<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">';
        html += '<h3>중복 그룹 ' + (index + 1) + ' (' + group.length + '개 파일)</h3>';
        
        // 그룹의 첫 번째 파일에서 해시값 가져오기 (모든 파일이 같은 해시를 가짐)
        if (group.length > 0 && group[0].hash) {
            html += '<button class="remove-group-btn" onclick="removeDuplicateGroup(\'' + group[0].hash + '\', ' + index + ')" ';
            html += 'style="background: #6c757d; color: white; border: none; padding: 5px 10px; border-radius: 3px; cursor: pointer; font-size: 12px;">';
            html += '🗑️ 그룹 제거</button>';
        }
        html += '</div>';
        
        group.forEach((file, fileIndex) => {
            html += '<div class="file-item">';
            html += '<a href="' + file.webViewLink + '" target="_blank" class="file-link">';
            html += '📄 ' + file.name;
            html += '</a>';
            html += '<button class="delete-btn" onclick="deleteFile(\'' + file.id + '\', \'' + file.name + '\')">삭제</button>';
            html += '<div class="file-info">';
            
            // 경로 정보 표시
            if (file.path && file.path !== '' && file.path !== '경로 미확인') {
                html += '📁 경로: ' + file.path;
            } else {
                html += '📁 경로: <span id="path-' + file.id + '" style="color: #666; font-style: italic;">경로 미확인</span> ';
                html += '<button onclick="loadSinglePath(\'' + file.id + '\')" style="font-size: 10px; padding: 2px 5px;">경로 조회</button>';
            }
            
            html += '<br>💾 크기: ' + formatFileSize(file.size);
            if (file.modifiedTime) {
                html += ' | 📅 수정일: ' + new Date(file.modifiedTime).toLocaleDateString('ko-KR');
            }
            html += '</div>';
            html += '</div>';
        });
        
        html += '</div>';
    });
    
    console.log('✅ HTML 생성 완료, 화면에 표시');
    liveDuplicatesDiv.innerHTML = html;
}

function deleteFile(fileId, fileName) {
    if (!confirm('정말로 "' + fileName + '" 파일을 삭제하시겠습니까?\n\n⚠️ 이 작업은 되돌릴 수 없습니다!')) {
        return;
    }
    
    fetch('/delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: 'fileId=' + encodeURIComponent(fileId)
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('삭제 실패: ' + data.error);
        } else {
            alert('파일이 성공적으로 삭제되었습니다.');
            // 페이지 새로고침 대신 해당 요소만 제거
            location.reload();
        }
    })
    .catch(error => {
        alert('삭제 중 오류 발생: ' + error);
    });
}

function removeDuplicateGroup(groupHash, groupIndex) {
    if (!confirm('이 중복 그룹을 목록에서 제거하시겠습니까?\n\n⚠️ 그룹의 모든 파일이 목록에서 사라지지만, 실제 파일은 삭제되지 않습니다.')) {
        return;
    }
    
    console.log('🗑️ 중복 그룹 제거 요청:', groupHash);
    
    fetch('/remove-duplicate-group', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            groupHash: groupHash
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('그룹 제거 실패: ' + data.error);
        } else {
            alert('중복 그룹이 목록에서 제거되었습니다.');
            // 해당 그룹 요소를 DOM에서 제거
            const groupElement = document.getElementById('group-' + groupIndex);
            if (groupElement) {
                groupElement.remove();
            }
            // 페이지 새로고침으로 업데이트된 목록 표시
            loadLiveDuplicates();
        }
    })
    .catch(error => {
        console.error('그룹 제거 중 오류:', error);
        alert('그룹 제거 중 오류 발생: ' + error);
    });
}

function cleanupDeletedFiles() {
    if (!confirm('데이터베이스에서 삭제된 파일들을 정리하시겠습니까?\n\n⚠️ 이 작업은 시간이 오래 걸릴 수 있으며, Google Drive에서 이미 삭제된 파일들을 목록에서 제거합니다.')) {
        return;
    }
    
    console.log('🧹 삭제된 파일 정리 시작');
    
    fetch('/cleanup-deleted-files', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        }
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('삭제된 파일 정리 실패: ' + data.error);
        } else {
            alert('삭제된 파일 정리가 백그라운드에서 시작되었습니다.\n진행 상황은 서버 로그에서 확인할 수 있습니다.');
        }
    })
    .catch(error => {
        console.error('삭제된 파일 정리 중 오류:', error);
        alert('삭제된 파일 정리 중 오류 발생: ' + error);
    });
}

function cleanupEmptyFolders() {
    if (!confirm('전체 Google Drive에서 빈 폴더들을 정리하시겠습니까?\n\n⚠️ 이 작업은 시간이 오래 걸릴 수 있으며, 모든 빈 폴더가 삭제됩니다.\n\n주의: 중요한 폴더 구조가 있다면 신중하게 결정하세요.')) {
        return;
    }
    
    console.log('📂 빈 폴더 정리 시작');
    
    fetch('/cleanup-empty-folders', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        }
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('빈 폴더 정리 실패: ' + data.error);
        } else {
            alert('빈 폴더 정리가 백그라운드에서 시작되었습니다.\n\n정리 과정:\n1. 파일 삭제 시 자동으로 빈 폴더 정리\n2. 수동 전체 빈 폴더 정리\n\n진행 상황은 서버 로그에서 확인할 수 있습니다.');
        }
    })
    .catch(error => {
        console.error('빈 폴더 정리 중 오류:', error);
        alert('빈 폴더 정리 중 오류 발생: ' + error);
    });
}

function confirmDeleteTargetFolder(folderId, folderName) {
    const confirmMsg = `정말로 "${folderName}" 폴더 전체를 삭제하시겠습니까?\n\n` +
                      `⚠️ 경고:\n` +
                      `• 폴더의 모든 파일과 하위 폴더가 영구적으로 삭제됩니다\n` +
                      `• 이 작업은 되돌릴 수 없습니다\n` +
                      `• 95% 이상이 중복 파일이므로 폴더 전체 삭제를 권장합니다\n\n` +
                      `계속하시겠습니까?`;
    
    if (!confirm(confirmMsg)) {
        return;
    }
    
    console.log('🗑️ 대상 폴더 전체 삭제 요청:', folderName, folderId);
    
    fetch('/delete-target-folder', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            folderId: folderId,
            folderName: folderName
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('폴더 삭제 실패: ' + data.error);
        } else {
            alert(`"${folderName}" 폴더 삭제가 시작되었습니다.\n\n백그라운드에서 처리되며, 완료되면 서버 로그에서 확인할 수 있습니다.`);
            
            // 비교 결과 화면 숨기기
            const previewDiv = document.getElementById('comparison-preview');
            if (previewDiv) {
                previewDiv.style.display = 'none';
            }
        }
    })
    .catch(error => {
        console.error('폴더 삭제 중 오류:', error);
        alert('폴더 삭제 중 오류 발생: ' + error);
    });
}

function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 폴더 비교 관련 함수들
function compareFolders() {
    const sourceFolderUrl = document.getElementById('sourceFolderUrl').value.trim();
    const targetFolderUrl = document.getElementById('targetFolderUrl').value.trim();
    
    if (!sourceFolderUrl) {
        alert('기준 폴더 URL을 입력해주세요.');
        return;
    }
    
    if (!targetFolderUrl) {
        alert('대상 폴더 URL을 입력해주세요.');
        return;
    }
    
    if (sourceFolderUrl === targetFolderUrl) {
        alert('기준 폴더와 대상 폴더는 다른 폴더여야 합니다.');
        return;
    }
    
    // UI 업데이트
    document.getElementById('compareFoldersBtn').disabled = true;
    document.getElementById('compareFoldersBtn').textContent = '비교 중...';
    
    // 진행 상황 표시 영역 생성
    showComparisonProgress();
    
    // 진행 상황 모니터링 시작
    startComparisonProgressMonitoring();
    
    // 백그라운드에서 폴더 비교 시작
    fetch('/compare-folders', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            sourceFolderUrl: sourceFolderUrl,
            targetFolderUrl: targetFolderUrl
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('폴더 비교 실패: ' + data.error);
            hideComparisonProgress();
            return;
        }
        
        if (data.status === 'started') {
            console.log('폴더 비교 시작됨:', data.message);
            // 진행 상황 모니터링이 이미 시작되어 있음
            return;
        }
        
        // 이미 완료된 결과가 반환된 경우 (빠른 완료)
        stopComparisonProgressMonitoring();
        hideComparisonProgress();
        
        comparisonResult = data;
        duplicatesFromComparison = data.duplicatesInTarget || [];
        displayComparisonResult(data);
    })
    .catch(error => {
        alert('폴더 비교 중 오류 발생: ' + error);
        stopComparisonProgressMonitoring();
        hideComparisonProgress();
    })
    .finally(() => {
        document.getElementById('compareFoldersBtn').disabled = false;
        document.getElementById('compareFoldersBtn').textContent = '🔍 폴더 비교 및 중복 파일 검색';
    });
}

function showComparisonProgress() {
    const progressHtml = `
        <div id="comparison-progress" style="background: #f8f9fa; padding: 20px; border-radius: 5px; margin: 20px 0; border-left: 4px solid #007bff;">
            <h4>📊 폴더 비교 진행 중...</h4>
            <div id="comparison-progress-content">
                <p id="current-step">초기화 중...</p>
                <div id="progress-details"></div>
                <div id="progress-bar-container" style="width: 100%; background: #e9ecef; border-radius: 5px; height: 20px; margin: 10px 0;">
                    <div id="progress-bar" style="width: 0%; background: #007bff; height: 100%; border-radius: 5px; transition: width 0.3s;"></div>
                </div>
            </div>
        </div>
    `;
    
    // 기존 진행 상황이 있으면 제거
    const existingProgress = document.getElementById('comparison-progress');
    if (existingProgress) {
        existingProgress.remove();
    }
    
    // 폴더 비교 섹션 뒤에 추가
    const folderSection = document.getElementById('folder-comparison-section');
    folderSection.insertAdjacentHTML('afterend', progressHtml);
}

function hideComparisonProgress() {
    const progressDiv = document.getElementById('comparison-progress');
    if (progressDiv) {
        progressDiv.remove();
    }
}

function startComparisonProgressMonitoring() {
    if (comparisonProgressInterval) {
        clearInterval(comparisonProgressInterval);
    }
    
    comparisonProgressInterval = setInterval(checkComparisonProgress, 1000); // 1초마다 체크
}

function stopComparisonProgressMonitoring() {
    if (comparisonProgressInterval) {
        clearInterval(comparisonProgressInterval);
        comparisonProgressInterval = null;
    }
}

function checkComparisonProgress() {
    fetch('/comparison-progress')
        .then(response => response.json())
        .then(data => {
            if (data.status === 'idle') {
                return; // 아직 시작되지 않음
            }
            
            updateComparisonProgressDisplay(data);
            
            // 진행상황 저장
            lastComparisonProgress = data;
            
            if (data.status === 'completed' || data.status === 'error') {
                stopComparisonProgressMonitoring();
                
                if (data.status === 'error') {
                    alert('폴더 비교 중 오류 발생: ' + (data.errorMessage || '알 수 없는 오류'));
                    hideComparisonProgress();
                    resetComparisonUI();
                } else if (data.status === 'completed') {
                    // 완료된 경우 결과를 가져와서 표시
                    hideComparisonProgress();
                    fetchComparisonResult();
                }
            }
        })
        .catch(error => {
            console.log('진행 상황 확인 오류:', error);
        });
}

function updateComparisonProgressDisplay(progress) {
    const currentStepEl = document.getElementById('current-step');
    const progressDetailsEl = document.getElementById('progress-details');
    const progressBarEl = document.getElementById('progress-bar');
    
    if (!currentStepEl || !progressDetailsEl || !progressBarEl) {
        return;
    }
    
    // 현재 단계 표시 (더 명확하게)
    const currentStep = progress.currentStep || '진행 중...';
    currentStepEl.textContent = currentStep;
    
    // 상세 정보 표시 (성능 정보 포함)
    let details = '';
    
    // 진행 단계 분석하여 시각적 표시
    let stageIcon = '📋';
    let stageText = '';
    
    if (currentStep.includes('스캔 중') || currentStep.includes('조회')) {
        stageIcon = '📁';
        stageText = '폴더 스캔 단계';
    } else if (currentStep.includes('해시 계산') || currentStep.includes('계산 중')) {
        stageIcon = '🔑';
        stageText = '해시 계산 단계';
    } else if (currentStep.includes('중복 검출') || currentStep.includes('비교')) {
        stageIcon = '🔄';
        stageText = '중복 파일 검출 단계';
    } else if (currentStep.includes('완료')) {
        stageIcon = '✅';
        stageText = '작업 완료';
    }
    
    details += `<div style="background: #f8f9fa; padding: 10px; border-radius: 5px; margin-bottom: 15px;">
        <p style="margin: 0; font-weight: bold;">${stageIcon} ${stageText}</p>
    </div>`;
    
    // 폴더 스캔 정보 (더 상세하게)
    if (progress.sourceFolderScanned > 0 || progress.sourceFolderTotal > 0) {
        const sourceProgress = progress.sourceFolderTotal > 0 ? 
            ` (${((progress.sourceFolderScanned / progress.sourceFolderTotal) * 100).toFixed(1)}%)` : '';
        const scanStatus = progress.sourceFolderTotal > 0 && progress.sourceFolderScanned >= progress.sourceFolderTotal ? 
            ' ✅ 스캔 완료' : ' 📁 스캔 중';
        details += `<p>📁 기준 폴더: ${progress.sourceFolderScanned}/${progress.sourceFolderTotal || '?'}개${sourceProgress}${scanStatus}</p>`;
    }
    if (progress.targetFolderScanned > 0 || progress.targetFolderTotal > 0) {
        const targetProgress = progress.targetFolderTotal > 0 ? 
            ` (${((progress.targetFolderScanned / progress.targetFolderTotal) * 100).toFixed(1)}%)` : '';
        const scanStatus = progress.targetFolderTotal > 0 && progress.targetFolderScanned >= progress.targetFolderTotal ? 
            ' ✅ 스캔 완료' : ' 📁 스캔 중';
        details += `<p>📁 대상 폴더: ${progress.targetFolderScanned}/${progress.targetFolderTotal || '?'}개${targetProgress}${scanStatus}</p>`;
    }
    
    // 해시 계산 정보 (성능 정보 추출)
    if (progress.hashesCalculated > 0) {
        const currentStep = progress.currentStep || '';
        let hashInfo = `🔑 해시 계산: ${progress.hashesCalculated}`;
        
        // totalHashesToCalc가 있으면 진행률 표시
        if (progress.totalHashesToCalc > 0) {
            const hashProgress = ((progress.hashesCalculated / progress.totalHashesToCalc) * 100).toFixed(1);
            hashInfo += `/${progress.totalHashesToCalc}개 (${hashProgress}%)`;
        } else {
            hashInfo += '개 완료';
        }
        
        // currentStep에서 속도와 ETA 정보 추출
        const speedMatch = currentStep.match(/(\d+\.?\d*\/s)/);
        const etaMatch = currentStep.match(/ETA:\s*([^)]+)/);
        
        if (speedMatch) {
            hashInfo += ` - 속도: ⚡${speedMatch[1]}`;
        }
        if (etaMatch) {
            hashInfo += ` - 완료 예상: 🕒${etaMatch[1]}`;
        }
        
        details += `<p>${hashInfo}</p>`;
    }
    
    // 중복 파일 발견 정보
    if (progress.duplicatesFound > 0) {
        details += `<p>🔄 중복 파일 발견: ${progress.duplicatesFound}개</p>`;
    }
    
    // 해시 계산 준비 중 표시
    if (currentStep.includes('해시 계산 시작') || currentStep.includes('준비 중')) {
        details += `<div style="background: #d1ecf1; padding: 10px; margin-top: 10px; border-radius: 3px; border-left: 4px solid #0dcaf0;">
            <p style="margin: 0;"><strong>⏳ 해시 계산 준비 중:</strong> 파일 정보 검증 및 워커 초기화 중입니다...</p>
        </div>`;
    }
    
    // 성능 팁 표시
    if (progress.hashesCalculated > 50) {
        const currentStep = progress.currentStep || '';
        const speedMatch = currentStep.match(/(\d+\.?\d*)/);
        if (speedMatch && parseFloat(speedMatch[1]) < 1.0) {
            details += `<div style="background: #fff3cd; padding: 10px; margin-top: 10px; border-radius: 3px; border-left: 4px solid #ffc107;">
                <p style="margin: 0;"><strong>💡 성능 팁:</strong> 해시 계산이 느린 것 같습니다. 워커 수를 줄이거나 네트워크 상태를 확인해보세요.</p>
            </div>`;
        } else if (speedMatch && parseFloat(speedMatch[1]) > 5.0) {
            details += `<div style="background: #d4edda; padding: 10px; margin-top: 10px; border-radius: 3px; border-left: 4px solid #28a745;">
                <p style="margin: 0;"><strong>🚀 좋은 성능:</strong> 해시 계산이 빠르게 진행되고 있습니다!</p>
            </div>`;
        }
    }
    
    progressDetailsEl.innerHTML = details;
    
    // 진행률 계산 및 표시
    let progressPercentage = 0;
    if (progress.sourceFolderTotal > 0 && progress.targetFolderTotal > 0) {
        const sourceProgress = progress.sourceFolderScanned / progress.sourceFolderTotal;
        const targetProgress = progress.targetFolderScanned / progress.targetFolderTotal;
        progressPercentage = ((sourceProgress + targetProgress) / 2) * 100;
    } else if (progress.sourceFolderTotal > 0) {
        progressPercentage = (progress.sourceFolderScanned / progress.sourceFolderTotal) * 50;
    }
    
    progressBarEl.style.width = Math.min(progressPercentage, 100) + '%';
}

function fetchComparisonResult() {
    // 완료된 비교 결과를 서버에서 가져오기
    fetch('/comparison-result')
        .then(response => response.json())
        .then(data => {
            if (data.error) {
                console.log('결과 가져오기 실패:', data.error);
                // 진행상황에서 얻은 정보로 대체
                const progress = getLastComparisonProgress();
                if (progress && progress.duplicatesFound >= 0) {
                    if (progress.duplicatesFound === 0) {
                        displayNoResultsMessage();
                    } else {
                        alert(`폴더 비교 완료! ${progress.duplicatesFound}개의 중복 파일을 발견했습니다.`);
                    }
                }
            } else {
                // 실제 결과 표시
                comparisonResult = data;
                duplicatesFromComparison = data.duplicatesInTarget || [];
                displayComparisonResult(data);
            }
        })
        .catch(error => {
            console.log('결과 가져오기 오류:', error);
            // 진행상황 정보로 대체
            const progress = getLastComparisonProgress();
            if (progress && progress.duplicatesFound >= 0) {
                if (progress.duplicatesFound === 0) {
                    displayNoResultsMessage();
                } else {
                    alert(`폴더 비교 완료! ${progress.duplicatesFound}개의 중복 파일을 발견했습니다.`);
                }
            }
        })
        .finally(() => {
            resetComparisonUI();
        });
}

function displayNoResultsMessage() {
    const previewDiv = document.getElementById('comparison-preview');
    const summaryDiv = document.getElementById('comparison-summary');
    
    let html = '<div style="background: #d4edda; padding: 15px; border-radius: 5px; color: #155724; margin: 20px 0;">';
    html += '<h4>✅ 폴더 비교 완료!</h4>';
    html += '<p><strong>중복 파일이 없습니다!</strong></p>';
    html += '<p>대상 폴더에는 기준 폴더와 중복되는 파일이 없습니다.</p>';
    html += '</div>';
    
    summaryDiv.innerHTML = html;
    document.getElementById('duplicates-list').innerHTML = '';
    previewDiv.style.display = 'block';
    
    // 삭제 버튼 숨기기
    document.getElementById('confirmDeleteDuplicatesBtn').style.display = 'none';
}

function resetComparisonUI() {
    document.getElementById('compareFoldersBtn').disabled = false;
    document.getElementById('compareFoldersBtn').textContent = '🔍 폴더 비교 및 중복 파일 검색';
}

// 마지막 진행상황을 저장하기 위한 변수
let lastComparisonProgress = null;

function getLastComparisonProgress() {
    return lastComparisonProgress;
}

function displayComparisonResult(result) {
    const previewDiv = document.getElementById('comparison-preview');
    const summaryDiv = document.getElementById('comparison-summary');
    const duplicatesListDiv = document.getElementById('duplicates-list');
    
    // 요약 정보 표시
    let summaryHtml = '<div style="background: #f8f9fa; padding: 15px; border-radius: 5px; margin-bottom: 15px;">';
    summaryHtml += '<h4>📊 비교 결과 요약</h4>';
    summaryHtml += '<p><strong>기준 폴더 파일 수:</strong> ' + result.sourceFiles.length + '개</p>';
    summaryHtml += '<p><strong>대상 폴더 파일 수:</strong> ' + result.targetFiles.length + '개</p>';
    summaryHtml += '<p><strong>대상 폴더의 중복 파일:</strong> <span style="color: #dc3545; font-weight: bold;">' + result.totalDuplicates + '개</span></p>';
    if (result.duplicationPercentage !== undefined) {
        summaryHtml += '<p><strong>중복 비율:</strong> <span style="color: #dc3545; font-weight: bold;">' + result.duplicationPercentage.toFixed(1) + '%</span></p>';
    }
    summaryHtml += '</div>';
    
    // 폴더 전체 삭제 권장 표시
    if (result.canDeleteTargetFolder && result.targetFolderName) {
        summaryHtml += '<div style="background: #fff3cd; padding: 15px; border-radius: 5px; margin-bottom: 15px; border-left: 4px solid #ffc107;">';
        summaryHtml += '<h4>💡 효율적인 삭제 방법 권장</h4>';
        summaryHtml += '<p><strong>대상 폴더의 ' + result.duplicationPercentage.toFixed(1) + '%가 중복됩니다!</strong></p>';
        summaryHtml += '<p>개별 파일을 삭제하는 대신 <strong>"' + result.targetFolderName + '"</strong> 폴더 전체를 삭제하는 것이 더 효율적입니다.</p>';
        summaryHtml += '<div style="margin-top: 15px;">';
        summaryHtml += '<button onclick="confirmDeleteTargetFolder(\'' + result.targetFolderID + '\', \'' + result.targetFolderName + '\')" ';
        summaryHtml += 'style="background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 3px; cursor: pointer; margin-right: 10px; font-weight: bold;">';
        summaryHtml += '🗑️ 폴더 전체 삭제 (권장)</button>';
        summaryHtml += '<span style="color: #856404; font-size: 12px;">⚠️ 폴더의 모든 내용이 삭제됩니다</span>';
        summaryHtml += '</div>';
        summaryHtml += '<hr style="margin: 15px 0; border: none; border-top: 1px solid #e9ecef;">';
        summaryHtml += '<p style="margin: 0; font-size: 14px; color: #6c757d;"><strong>또는</strong> 아래에서 개별 파일만 삭제할 수도 있습니다:</p>';
        summaryHtml += '</div>';
    }
    
    if (result.totalDuplicates === 0) {
        summaryHtml += '<div style="background: #d4edda; padding: 15px; border-radius: 5px; color: #155724;">';
        summaryHtml += '<p><strong>✅ 중복 파일이 없습니다!</strong></p>';
        summaryHtml += '<p>대상 폴더에는 기준 폴더와 중복되는 파일이 없습니다.</p>';
        summaryHtml += '</div>';
        
        summaryDiv.innerHTML = summaryHtml;
        duplicatesListDiv.innerHTML = '';
        previewDiv.style.display = 'block';
        
        // 삭제 버튼 숨기기
        document.getElementById('confirmDeleteDuplicatesBtn').style.display = 'none';
        return;
    }
    
    summaryDiv.innerHTML = summaryHtml;
    
    // 중복 파일 목록 표시
    let duplicatesHtml = '<div style="background: #fff3cd; padding: 15px; border-radius: 5px; margin-bottom: 15px;">';
    duplicatesHtml += '<h4>🗑️ 삭제될 중복 파일 목록</h4>';
    duplicatesHtml += '<p style="color: #856404;"><strong>⚠️ 주의:</strong> 아래 파일들이 대상 폴더에서 영구적으로 삭제됩니다.</p>';
    duplicatesHtml += '</div>';
    
    duplicatesHtml += '<div style="max-height: 400px; overflow-y: auto; border: 1px solid #ddd; border-radius: 3px;">';
    result.duplicatesInTarget.forEach((file, index) => {
        duplicatesHtml += '<div style="padding: 10px; border-bottom: 1px solid #eee; ' + (index % 2 === 0 ? 'background: #f9f9f9;' : '') + '">';
        duplicatesHtml += '<div style="font-weight: bold;">📄 ' + file.name + '</div>';
        duplicatesHtml += '<div style="font-size: 12px; color: #666;">크기: ' + formatFileSize(file.size) + '</div>';
        if (file.path) {
            duplicatesHtml += '<div style="font-size: 12px; color: #666;">경로: ' + file.path + '</div>';
        }
        if (file.modifiedTime) {
            duplicatesHtml += '<div style="font-size: 12px; color: #666;">수정일: ' + new Date(file.modifiedTime).toLocaleDateString('ko-KR') + '</div>';
        }
        duplicatesHtml += '</div>';
    });
    duplicatesHtml += '</div>';
    
    // 빈 폴더 정리 옵션 추가
    duplicatesHtml += '<div style="background: #e9ecef; padding: 15px; border-radius: 5px; margin-top: 15px;">';
    duplicatesHtml += '<h5>🗂️ 추가 정리 옵션</h5>';
    duplicatesHtml += '<label style="display: flex; align-items: center; cursor: pointer;">';
    duplicatesHtml += '<input type="checkbox" id="cleanupEmptyFoldersCheck" checked style="margin-right: 8px; transform: scale(1.2);">';
    duplicatesHtml += '<span>파일 삭제 후 빈 폴더도 함께 정리</span>';
    duplicatesHtml += '</label>';
    duplicatesHtml += '<p style="margin: 8px 0 0 28px; font-size: 12px; color: #6c757d;">';
    duplicatesHtml += '체크하면 중복 파일 삭제로 인해 비워진 폴더들도 자동으로 삭제됩니다.';
    duplicatesHtml += '</p>';
    duplicatesHtml += '</div>';
    
    duplicatesListDiv.innerHTML = duplicatesHtml;
    previewDiv.style.display = 'block';
    
    // 삭제 버튼 표시
    document.getElementById('confirmDeleteDuplicatesBtn').style.display = 'inline-block';
}

function confirmDeleteDuplicates() {
    if (!duplicatesFromComparison || duplicatesFromComparison.length === 0) {
        alert('삭제할 중복 파일이 없습니다.');
        return;
    }
    
    // 빈 폴더 정리 옵션 확인
    const cleanupEmptyFolders = document.getElementById('cleanupEmptyFoldersCheck')?.checked || false;
    
    let confirmMsg = '정말로 대상 폴더에서 ' + duplicatesFromComparison.length + '개의 중복 파일을 삭제하시겠습니까?\\n\\n⚠️ 이 작업은 되돌릴 수 없습니다!';
    if (cleanupEmptyFolders) {
        confirmMsg += '\\n\\n📂 파일 삭제 후 빈 폴더도 함께 정리됩니다.';
    }
    
    if (!confirm(confirmMsg)) {
        return;
    }
    
    document.getElementById('confirmDeleteDuplicatesBtn').disabled = true;
    document.getElementById('confirmDeleteDuplicatesBtn').textContent = '삭제 중...';
    
    const fileIds = duplicatesFromComparison.map(file => file.id);
    
    fetch('/delete-duplicates-from-target', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            fileIds: fileIds,
            cleanupEmptyFolders: cleanupEmptyFolders
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('삭제 실패: ' + data.error);
            return;
        }
        
        alert('성공적으로 ' + data.deletedCount + '개의 중복 파일을 삭제했습니다.');
        cancelFolderComparison();
    })
    .catch(error => {
        alert('삭제 중 오류 발생: ' + error);
    })
    .finally(() => {
        document.getElementById('confirmDeleteDuplicatesBtn').disabled = false;
        document.getElementById('confirmDeleteDuplicatesBtn').textContent = '❌ 대상 폴더에서 중복 파일 삭제';
    });
}

function cancelFolderComparison() {
    document.getElementById('comparison-preview').style.display = 'none';
    comparisonResult = null;
    duplicatesFromComparison = [];
    document.getElementById('sourceFolderUrl').value = '';
    document.getElementById('targetFolderUrl').value = '';
    document.getElementById('comparison-summary').innerHTML = '';
    document.getElementById('duplicates-list').innerHTML = '';
}

// 폴더 비교 재개 알림 함수
function showResumeComparisonNotification(progressData) {
    const notificationHtml = `
        <div id="resume-comparison-notification" style="background: #fff3cd; padding: 15px; border-radius: 5px; margin: 20px 0; border-left: 4px solid #ffc107;">
            <h3 style="margin-top: 0;">🔄 진행 중인 폴더 비교 작업이 발견되었습니다</h3>
            <p><strong>현재 단계:</strong> ${progressData.currentStep || '알 수 없음'}</p>
            <p><strong>진행 상황:</strong> 
                기준 폴더 ${progressData.sourceFolderScanned || 0}개 스캔됨, 
                대상 폴더 ${progressData.targetFolderScanned || 0}개 스캔됨, 
                중복 파일 ${progressData.duplicatesFound || 0}개 발견됨
            </p>
            <button onclick="resumeComparison()" style="background: #28a745; color: white; border: none; padding: 10px 20px; border-radius: 3px; cursor: pointer; margin-right: 10px;">
                🔄 작업 재개
            </button>
            <button onclick="hideResumeNotification()" style="background: #6c757d; color: white; border: none; padding: 10px 20px; border-radius: 3px; cursor: pointer;">
                나중에
            </button>
        </div>
    `;
    
    const container = document.querySelector('.container');
    const header = document.querySelector('.header');
    header.insertAdjacentHTML('afterend', notificationHtml);
}

function resumeComparison() {
    hideResumeNotification();
    startComparisonProgressMonitoring();
    showComparisonProgress();
}

function hideResumeNotification() {
    const notification = document.getElementById('resume-comparison-notification');
    if (notification) {
        notification.remove();
    }
}

function checkComparisonResult() {
    fetch('/comparison-result')
        .then(response => response.json())
        .then(data => {
            if (data && !data.error) {
                console.log('✅ 완료된 비교 결과 로드');
                comparisonResult = data;
                duplicatesFromComparison = data.duplicatesInTarget || [];
                showComparisonResult();
            }
        })
        .catch(err => console.log('ℹ️ 비교 결과 없음:', err));
}

function showComparisonResult() {
    if (!comparisonResult) return;
    
    // 폴더 비교 섹션을 보이게 함
    document.getElementById('folder-comparison-section').scrollIntoView({ behavior: 'smooth' });
    
    // 결과 표시
    displayComparisonPreview(comparisonResult);
}

// 저장된 폴더 비교 작업 확인 및 표시
function checkSavedComparisonTasks() {
    fetch('/saved-comparison-tasks')
        .then(response => response.json())
        .then(data => {
            if (data.tasks && data.tasks.length > 0) {
                console.log('📂 저장된 폴더 비교 작업 발견:', data.tasks.length + '개');
                showSavedComparisonTasks(data.tasks);
            } else {
                console.log('ℹ️ 저장된 폴더 비교 작업 없음');
            }
        })
        .catch(err => console.log('ℹ️ 저장된 작업 조회 오류:', err));
}

function showSavedComparisonTasks(tasks) {
    const section = document.getElementById('saved-comparison-section');
    const infoDiv = document.getElementById('saved-comparison-info');
    
    if (tasks.length === 0) {
        section.style.display = 'none';
        return;
    }
    
    // 가장 최근 작업을 표시
    const latestTask = tasks[0];
    
    let statusText = '';
    let statusColor = '';
    switch (latestTask.status) {
        case 'running':
            statusText = '실행 중';
            statusColor = '#007bff';
            break;
        case 'paused':
            statusText = '일시 정지됨';
            statusColor = '#ffc107';
            break;
        case 'completed':
            statusText = '완료됨';
            statusColor = '#28a745';
            break;
        case 'pending':
            statusText = '대기 중';
            statusColor = '#6c757d';
            break;
        default:
            statusText = latestTask.status;
            statusColor = '#6c757d';
    }
    
    const progressText = `기준 폴더: ${latestTask.sourceFolderScanned || 0}/${latestTask.sourceFolderTotal || 0}, 대상 폴더: ${latestTask.targetFolderScanned || 0}/${latestTask.targetFolderTotal || 0}, 중복 파일: ${latestTask.duplicatesFound || 0}개`;
    
    infoDiv.innerHTML = `
        <p><strong>상태:</strong> <span style="color: ${statusColor};">${statusText}</span></p>
        <p><strong>현재 단계:</strong> ${latestTask.currentStep || '알 수 없음'}</p>
        <p><strong>진행 상황:</strong> ${progressText}</p>
        <p><strong>마지막 업데이트:</strong> ${new Date(latestTask.updatedAt).toLocaleString()}</p>
        ${latestTask.errorMessage ? `<p style="color: #dc3545;"><strong>오류:</strong> ${latestTask.errorMessage}</p>` : ''}
    `;
    
    section.style.display = 'block';
    
    // 저장된 작업 정보를 전역 변수에 보관
    window.savedComparisonTask = latestTask;
}

function loadSavedComparison() {
    if (!window.savedComparisonTask) {
        alert('저장된 작업 정보가 없습니다.');
        return;
    }
    
    const task = window.savedComparisonTask;
    console.log('🔍 저장된 작업 정보:', task); // 디버깅용
    
    // 폴더 ID 검증
    if (!task.sourceFolderId || !task.targetFolderId) {
        console.error('❌ 폴더 ID가 없습니다:', {
            sourceFolderId: task.sourceFolderId,
            targetFolderId: task.targetFolderId
        });
        alert('저장된 작업의 폴더 정보가 올바르지 않습니다.');
        return;
    }
    
    // URL 입력 필드에 폴더 ID 설정 (실제 URL로 변환)
    const sourceFolderUrl = `https://drive.google.com/drive/folders/${task.sourceFolderId}`;
    const targetFolderUrl = `https://drive.google.com/drive/folders/${task.targetFolderId}`;
    
    console.log('📁 생성된 URL:', { sourceFolderUrl, targetFolderUrl }); // 디버깅용
    
    document.getElementById('sourceFolderUrl').value = sourceFolderUrl;
    document.getElementById('targetFolderUrl').value = targetFolderUrl;
    
    // 저장된 작업 섹션 숨기기
    document.getElementById('saved-comparison-section').style.display = 'none';
    
    // 작업 상태에 따라 다른 동작 수행
    if (task.status === 'completed') {
        // 완료된 작업이면 결과를 직접 로드
        checkComparisonResult();
        alert('완료된 작업을 불러왔습니다. 결과를 확인하세요.');
    } else if (task.status === 'running') {
        // 실행 중인 작업이면 진행 상황 모니터링 시작
        startComparisonProgressMonitoring();
        showComparisonProgress();
        alert('실행 중인 작업을 불러왔습니다. 진행 상황을 확인하세요.');
    } else {
        // 다른 상태면 새로 시작할 수 있도록 폴더 URL만 설정
        alert('저장된 폴더 정보를 불러왔습니다. "폴더 비교 및 중복 파일 검색" 버튼을 클릭하여 작업을 계속하세요.');
    }
}

function clearSavedComparison() {
    if (!confirm('저장된 폴더 비교 작업을 모두 삭제하시겠습니까?')) {
        return;
    }
    
    fetch('/clear-saved-comparison', {
        method: 'POST'
    })
    .then(response => response.json())
    .then(data => {
        if (data.error) {
            alert('삭제 실패: ' + data.error);
        } else {
            alert(data.message);
            document.getElementById('saved-comparison-section').style.display = 'none';
            window.savedComparisonTask = null;
        }
    })
    .catch(error => {
        alert('삭제 중 오류 발생: ' + error);
    });
}

function hideSavedComparisonSection() {
    document.getElementById('saved-comparison-section').style.display = 'none';
}