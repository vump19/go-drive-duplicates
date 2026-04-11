// Telegram Settings Module

// 페이지 로드 시 텔레그램 설정 불러오기
document.addEventListener('DOMContentLoaded', () => {
    loadTelegramSettings();
});

async function loadTelegramSettings() {
    try {
        const settings = await API.Notification.getTelegramSettings();

        const enabledCheckbox = document.getElementById('telegramEnabled');
        const botTokenInput = document.getElementById('telegramBotToken');
        const chatIdInput = document.getElementById('telegramChatId');
        const formSection = document.getElementById('telegramSettingsForm');

        if (enabledCheckbox) enabledCheckbox.checked = settings.enabled;
        if (botTokenInput) botTokenInput.value = settings.botToken || '';
        if (chatIdInput) chatIdInput.value = settings.chatId || '';
        if (formSection) formSection.style.display = settings.enabled ? 'block' : 'none';
    } catch (error) {
        console.warn('텔레그램 설정 로드 실패:', error.message);
    }
}

function toggleTelegramSettings() {
    const enabled = document.getElementById('telegramEnabled').checked;
    const formSection = document.getElementById('telegramSettingsForm');
    if (formSection) {
        formSection.style.display = enabled ? 'block' : 'none';
    }
}

async function saveTelegramSettings() {
    const enabled = document.getElementById('telegramEnabled').checked;
    const botToken = document.getElementById('telegramBotToken').value.trim();
    const chatId = document.getElementById('telegramChatId').value.trim();

    const statusEl = document.getElementById('telegramStatus');

    try {
        statusEl.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 저장 중...';
        statusEl.className = 'telegram-status saving';
        statusEl.style.display = 'block';

        const result = await API.Notification.updateTelegramSettings({
            enabled,
            botToken,
            chatId
        });

        statusEl.innerHTML = '<i class="fas fa-check-circle"></i> ' + (result.message || '설정이 저장되었습니다');
        statusEl.className = 'telegram-status success';

        // 저장 후 설정 다시 로드 (마스킹된 토큰 표시)
        setTimeout(() => loadTelegramSettings(), 500);
        setTimeout(() => { statusEl.style.display = 'none'; }, 3000);
    } catch (error) {
        statusEl.innerHTML = '<i class="fas fa-exclamation-circle"></i> 저장 실패: ' + error.message;
        statusEl.className = 'telegram-status error';
    }
}

async function testTelegramNotification() {
    const statusEl = document.getElementById('telegramStatus');

    try {
        statusEl.innerHTML = '<i class="fas fa-spinner fa-spin"></i> 테스트 알림 전송 중...';
        statusEl.className = 'telegram-status saving';
        statusEl.style.display = 'block';

        const result = await API.Notification.testTelegram();

        if (result.success) {
            statusEl.innerHTML = '<i class="fas fa-check-circle"></i> ' + (result.message || '테스트 알림이 전송되었습니다');
            statusEl.className = 'telegram-status success';
        } else {
            statusEl.innerHTML = '<i class="fas fa-exclamation-circle"></i> ' + (result.error || '전송 실패');
            statusEl.className = 'telegram-status error';
        }

        setTimeout(() => { statusEl.style.display = 'none'; }, 5000);
    } catch (error) {
        statusEl.innerHTML = '<i class="fas fa-exclamation-circle"></i> 전송 실패: ' + error.message;
        statusEl.className = 'telegram-status error';
    }
}
