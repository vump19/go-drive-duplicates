// iframe 내에서 로드된 경우 body에 'iframe-mode' 클래스 추가
if (window.self !== window.top) {
    document.documentElement.classList.add('iframe-mode');
}

// 도움말 패널 토글
function toggleHelpPanel(el) {
    const panel = el.closest('.help-panel');
    if (panel) panel.classList.toggle('open');
}
