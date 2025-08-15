let progressInterval;
let duplicatesInterval;

let currentSettings = {};

// 간단한 경로 계산 시스템
let currentPage = 1;
let totalPages = 1;

// 일괄 삭제 관련 함수들
let filesToDelete = [];

window.onload = function() {
    loadSettings();
    checkForSavedData();
    setupSettingsControls();
};

function checkForSavedData() {
    console.log('🔍 저장된 데이터 확인 중...');
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
        .catch(error => {
            console.log('❌ 진행 상태 확인 오류:', error);
        });
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
            if (data.path && pathElement) {
                pathElement.textContent = data.path;
                pathElement.style.color = '#000';
                pathElement.style.fontStyle = 'normal';
                // 경로 조회 버튼 숨기기
                const button = pathElement.nextElementSibling;
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
    const workerCount = document.getElementById('workerSlider').value;
    
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
            alert('작업자 수가 ' + workerCount + '개로 설정되었습니다.');
        }
    });
}

function setupSettingsControls() {
    const slider = document.getElementById('workerSlider');
    const valueDisplay = document.getElementById('workerValue');
    const recommendedBadge = document.getElementById('recommendedBadge');
    
    slider.addEventListener('input', function() {
        valueDisplay.textContent = this.value;
        
        // 권장 설정 표시 (CPU 코어 수의 절반)
        const recommendedValue = Math.max(1, Math.floor(navigator.hardwareConcurrency / 2));
        if (parseInt(this.value) === recommendedValue) {
            recommendedBadge.style.display = 'inline-block';
        } else {
            recommendedBadge.style.display = 'none';
        }
    });
    
    // 초기값 설정
    slider.value = Math.max(1, Math.floor(navigator.hardwareConcurrency / 2));
    valueDisplay.textContent = slider.value;
}

function loadSettings() {
    fetch('/settings')
        .then(response => response.json())
        .then(data => {
            currentSettings = data;
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
        html += '<div class="file-group">';
        html += '<h3>중복 그룹 ' + (index + 1) + ' (' + group.length + '개 파일)</h3>';
        
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

function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}