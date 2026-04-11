/**
 * Folder Resolve Utility
 * 폴더 URL/ID 입력 시 실제 Google Drive 경로를 자동 표시하는 공통 유틸리티
 */

/**
 * 폴더 입력 필드에 경로 자동 표시 기능을 연결합니다.
 * @param {HTMLInputElement} inputElement - 폴더 URL/ID 입력 필드
 * @param {Object} options - 옵션
 * @param {string} options.placeholder - 경로 표시 영역의 placeholder 텍스트
 */
function attachFolderResolver(inputElement, options = {}) {
    if (!inputElement || inputElement.dataset.folderResolverAttached) return;
    inputElement.dataset.folderResolverAttached = 'true';

    // 경로 표시용 div 생성
    const pathDisplay = document.createElement('div');
    pathDisplay.className = 'folder-resolve-path';
    pathDisplay.style.cssText = 'font-size:12px; color:#666; margin-top:3px; min-height:0; transition: all 0.2s;';
    inputElement.parentNode.insertBefore(pathDisplay, inputElement.nextSibling);

    let lastResolved = '';

    async function resolve() {
        const value = inputElement.value.trim();
        if (!value || value === lastResolved) {
            if (!value) {
                pathDisplay.textContent = '';
                pathDisplay.style.minHeight = '0';
                lastResolved = '';
            }
            return;
        }
        lastResolved = value;

        pathDisplay.innerHTML = '<i class="fas fa-spinner fa-spin" style="font-size:11px;"></i> <span style="color:#999;">경로 확인 중...</span>';
        pathDisplay.style.minHeight = '18px';

        try {
            const result = await window.API.Comparison.resolveFolderPath(value);
            // 입력값이 바뀌지 않았을 때만 결과 표시
            if (inputElement.value.trim() === value) {
                pathDisplay.innerHTML = `<span style="color:#2563eb;">📂 ${escapeHtml(result.path)}</span>`;
            }
        } catch (err) {
            if (inputElement.value.trim() === value) {
                pathDisplay.innerHTML = '<span style="color:#d97706;">⚠️ 경로를 확인할 수 없습니다</span>';
            }
        }
    }

    inputElement.addEventListener('blur', resolve);
    inputElement.addEventListener('change', resolve);

    // 이미 값이 있으면 즉시 해석
    if (inputElement.value.trim()) {
        resolve();
    }
}

/**
 * HTML 이스케이프 유틸
 */
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
