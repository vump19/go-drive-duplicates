/**
 * 유틸리티 함수들
 * 공통으로 사용되는 헬퍼 함수들
 */

// 파일 크기를 사람이 읽기 쉬운 형태로 포맷
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 숫자를 현지화 형태로 포맷
function formatNumber(num) {
    return num.toLocaleString();
}

// URL에서 폴더명 추출
function extractFolderNameFromUrl(url) {
    if (!url) return null;
    
    // URL에서 폴더 ID 추출
    const patterns = [
        /\/folders\/([a-zA-Z0-9_-]+)/,
        /[?&]id=([a-zA-Z0-9_-]+)/
    ];
    
    for (const pattern of patterns) {
        const match = url.match(pattern);
        if (match) {
            return match[1].substring(0, 10) + '...'; // ID의 앞 10자리만 표시
        }
    }
    
    return url.length > 20 ? url.substring(0, 20) + '...' : url;
}

// 현재 시간을 포맷된 문자열로 반환
function formatTime() {
    const now = new Date();
    return now.toLocaleTimeString('ko-KR', { 
        hour12: false, 
        hour: '2-digit', 
        minute: '2-digit', 
        second: '2-digit' 
    });
}

// 경과 시간을 포맷
function formatElapsedTime(startTime) {
    if (!startTime) return '0초';
    
    const elapsedSeconds = Math.floor((Date.now() - startTime) / 1000);
    const hours = Math.floor(elapsedSeconds / 3600);
    const minutes = Math.floor((elapsedSeconds % 3600) / 60);
    const seconds = elapsedSeconds % 60;
    
    if (hours > 0) {
        return `${hours}시간 ${minutes}분 ${seconds}초`;
    } else if (minutes > 0) {
        return `${minutes}분 ${seconds}초`;
    } else {
        return `${seconds}초`;
    }
}

// 파일 타입에 따른 아이콘 반환
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

// 상태에 따른 색상 반환
function getStatusColor(status) {
    switch (status) {
        case 'running': return '#28a745';
        case 'paused': return '#ffc107';
        case 'failed': return '#dc3545';
        case 'completed': return '#10b981';
        default: return '#6c757d';
    }
}

// 단계에 따른 텍스트 반환
function getPhaseText(phase) {
    switch (phase) {
        case 'initialized': return '초기화됨';
        case 'scanning_source': return '기준 폴더 스캔 중';
        case 'source_completed': return '기준 폴더 완료';
        case 'scanning_target': return '대상 폴더 스캔 중';
        case 'target_completed': return '대상 폴더 완료';
        case 'calculating_hashes': return '해시 계산 중';
        case 'hashes_completed': return '해시 계산 완료';
        case 'comparing_files': return '파일 비교 중';
        case 'saving_results': return '결과 저장 중';
        case 'completed': return '완료됨';
        default: return '알 수 없음';
    }
}