/**
 * 설정 페이지
 * 애플리케이션 설정 및 환경설정 관리
 */

import { Component } from '../components/base/Component.js';
import { Toast } from '../components/widgets/Toast.js';

export class Settings extends Component {
    constructor() {
        super();
        this.settings = this.loadSettings();
        this.originalSettings = { ...this.settings };
    }

    async render() {
        return `
            <div class="settings-page">
                <!-- 페이지 헤더 -->
                <div class="page-header">
                    <h1 class="page-title">
                        <i class="fas fa-cog"></i>
                        설정
                    </h1>
                    <p class="page-description">
                        애플리케이션 동작을 사용자 환경에 맞게 조정하세요
                    </p>
                </div>

                <!-- 설정 폼 -->
                <form id="settingsForm" class="settings-form">
                    <!-- 일반 설정 -->
                    <div class="card mb-6">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-user-cog"></i>
                                일반 설정
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-group">
                                <label class="form-label" for="language">언어</label>
                                <select class="form-select" id="language" name="language">
                                    <option value="ko" ${this.settings.language === 'ko' ? 'selected' : ''}>한국어</option>
                                    <option value="en" ${this.settings.language === 'en' ? 'selected' : ''}>English</option>
                                    <option value="ja" ${this.settings.language === 'ja' ? 'selected' : ''}>日本語</option>
                                </select>
                                <div class="form-help">사용자 인터페이스에서 사용할 언어를 선택하세요</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="theme">테마</label>
                                <select class="form-select" id="theme" name="theme">
                                    <option value="auto" ${this.settings.theme === 'auto' ? 'selected' : ''}>시스템 설정 따르기</option>
                                    <option value="light" ${this.settings.theme === 'light' ? 'selected' : ''}>라이트 모드</option>
                                    <option value="dark" ${this.settings.theme === 'dark' ? 'selected' : ''}>다크 모드</option>
                                </select>
                                <div class="form-help">애플리케이션의 외관 테마를 선택하세요</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableNotifications" name="enableNotifications" 
                                           ${this.settings.enableNotifications ? 'checked' : ''}>
                                    알림 활성화
                                </label>
                                <div class="form-help">작업 완료 및 오류에 대한 알림을 표시합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableSounds" name="enableSounds" 
                                           ${this.settings.enableSounds ? 'checked' : ''}>
                                    사운드 효과
                                </label>
                                <div class="form-help">버튼 클릭 및 알림에 사운드 효과를 재생합니다</div>
                            </div>
                        </div>
                    </div>

                    <!-- 스캔 설정 -->
                    <div class="card mb-6">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-search"></i>
                                스캔 설정
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-group">
                                <label class="form-label" for="maxFileSize">최대 파일 크기 (MB)</label>
                                <input type="number" class="form-input" id="maxFileSize" name="maxFileSize" 
                                       value="${this.settings.maxFileSize}" min="1" max="10240">
                                <div class="form-help">이 크기보다 큰 파일은 스캔에서 제외됩니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="workerCount">동시 작업 수</label>
                                <input type="number" class="form-input" id="workerCount" name="workerCount" 
                                       value="${this.settings.workerCount}" min="1" max="16">
                                <div class="form-help">동시에 처리할 파일 수 (CPU 성능에 따라 조정)</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="hashAlgorithm">해시 알고리즘</label>
                                <select class="form-select" id="hashAlgorithm" name="hashAlgorithm">
                                    <option value="md5" ${this.settings.hashAlgorithm === 'md5' ? 'selected' : ''}>MD5 (빠름)</option>
                                    <option value="sha1" ${this.settings.hashAlgorithm === 'sha1' ? 'selected' : ''}>SHA1 (권장)</option>
                                    <option value="sha256" ${this.settings.hashAlgorithm === 'sha256' ? 'selected' : ''}>SHA256 (안전)</option>
                                </select>
                                <div class="form-help">파일 비교에 사용할 해시 알고리즘을 선택하세요</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="skipHiddenFiles" name="skipHiddenFiles" 
                                           ${this.settings.skipHiddenFiles ? 'checked' : ''}>
                                    숨김 파일 건너뛰기
                                </label>
                                <div class="form-help">점(.)으로 시작하는 숨김 파일을 스캔에서 제외합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="skipSystemFiles" name="skipSystemFiles" 
                                           ${this.settings.skipSystemFiles ? 'checked' : ''}>
                                    시스템 파일 건너뛰기
                                </label>
                                <div class="form-help">운영체제 시스템 파일을 스캔에서 제외합니다</div>
                            </div>
                        </div>
                    </div>

                    <!-- 정리 설정 -->
                    <div class="card mb-6">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-trash-alt"></i>
                                정리 설정
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="confirmBeforeDelete" name="confirmBeforeDelete" 
                                           ${this.settings.confirmBeforeDelete ? 'checked' : ''}>
                                    삭제 전 확인
                                </label>
                                <div class="form-help">파일을 삭제하기 전에 항상 확인 메시지를 표시합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="moveToTrash" name="moveToTrash" 
                                           ${this.settings.moveToTrash ? 'checked' : ''}>
                                    휴지통으로 이동
                                </label>
                                <div class="form-help">파일을 완전히 삭제하지 않고 휴지통으로 이동합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="autoCleanEmptyFolders" name="autoCleanEmptyFolders" 
                                           ${this.settings.autoCleanEmptyFolders ? 'checked' : ''}>
                                    빈 폴더 자동 정리
                                </label>
                                <div class="form-help">파일 삭제 후 비어있게 된 폴더를 자동으로 정리합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="backupLocation">백업 위치</label>
                                <div class="input-group">
                                    <input type="text" class="form-input" id="backupLocation" name="backupLocation" 
                                           value="${this.settings.backupLocation}" readonly>
                                    <button type="button" class="btn btn-secondary" id="selectBackupLocation">
                                        <i class="fas fa-folder-open"></i>
                                        선택
                                    </button>
                                </div>
                                <div class="form-help">삭제 전 백업 파일이 저장될 위치를 지정하세요</div>
                            </div>
                        </div>
                    </div>

                    <!-- 성능 설정 -->
                    <div class="card mb-6">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-tachometer-alt"></i>
                                성능 설정
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-group">
                                <label class="form-label" for="cacheSize">캐시 크기 (MB)</label>
                                <input type="number" class="form-input" id="cacheSize" name="cacheSize" 
                                       value="${this.settings.cacheSize}" min="50" max="2048">
                                <div class="form-help">파일 메타데이터 캐시 크기를 조정하여 성능을 향상시킵니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="batchSize">배치 크기</label>
                                <input type="number" class="form-input" id="batchSize" name="batchSize" 
                                       value="${this.settings.batchSize}" min="10" max="1000">
                                <div class="form-help">한 번에 처리할 파일 수를 조정하여 메모리 사용량을 제어합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableProgressSaving" name="enableProgressSaving" 
                                           ${this.settings.enableProgressSaving ? 'checked' : ''}>
                                    진행상황 저장
                                </label>
                                <div class="form-help">스캔 진행상황을 저장하여 중단 시 이어서 실행할 수 있습니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableMemoryOptimization" name="enableMemoryOptimization" 
                                           ${this.settings.enableMemoryOptimization ? 'checked' : ''}>
                                    메모리 최적화
                                </label>
                                <div class="form-help">대용량 파일 처리 시 메모리 사용량을 최적화합니다</div>
                            </div>
                        </div>
                    </div>

                    <!-- 고급 설정 -->
                    <div class="card mb-6">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-code"></i>
                                고급 설정
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableDebugMode" name="enableDebugMode" 
                                           ${this.settings.enableDebugMode ? 'checked' : ''}>
                                    디버그 모드
                                </label>
                                <div class="form-help">개발자 도구에 상세한 로그를 출력합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-checkbox">
                                    <input type="checkbox" id="enableTelemetry" name="enableTelemetry" 
                                           ${this.settings.enableTelemetry ? 'checked' : ''}>
                                    사용량 통계 수집
                                </label>
                                <div class="form-help">익명화된 사용량 통계를 수집하여 서비스 개선에 활용합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="apiTimeout">API 타임아웃 (초)</label>
                                <input type="number" class="form-input" id="apiTimeout" name="apiTimeout" 
                                       value="${this.settings.apiTimeout}" min="10" max="300">
                                <div class="form-help">서버 API 요청 타임아웃 시간을 설정합니다</div>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="logLevel">로그 레벨</label>
                                <select class="form-select" id="logLevel" name="logLevel">
                                    <option value="error" ${this.settings.logLevel === 'error' ? 'selected' : ''}>Error</option>
                                    <option value="warn" ${this.settings.logLevel === 'warn' ? 'selected' : ''}>Warning</option>
                                    <option value="info" ${this.settings.logLevel === 'info' ? 'selected' : ''}>Info</option>
                                    <option value="debug" ${this.settings.logLevel === 'debug' ? 'selected' : ''}>Debug</option>
                                </select>
                                <div class="form-help">로그 출력 수준을 조정합니다</div>
                            </div>
                        </div>
                    </div>

                    <!-- 설정 액션 -->
                    <div class="card">
                        <div class="card-header">
                            <h2 class="card-title">
                                <i class="fas fa-tools"></i>
                                설정 관리
                            </h2>
                        </div>
                        <div class="card-body">
                            <div class="form-actions">
                                <button type="button" class="btn btn-secondary" id="resetBtn">
                                    <i class="fas fa-undo"></i>
                                    기본값으로 재설정
                                </button>
                                <button type="button" class="btn btn-secondary" id="exportBtn">
                                    <i class="fas fa-download"></i>
                                    설정 내보내기
                                </button>
                                <button type="button" class="btn btn-secondary" id="importBtn">
                                    <i class="fas fa-upload"></i>
                                    설정 가져오기
                                </button>
                                <button type="submit" class="btn btn-primary">
                                    <i class="fas fa-save"></i>
                                    설정 저장
                                </button>
                            </div>
                        </div>
                    </div>
                </form>

                <!-- 숨김 파일 입력 -->
                <input type="file" id="importFile" accept=".json" style="display: none;">
            </div>
        `;
    }

    setupEventListeners() {
        // 폼 제출
        this.addEventListener('#settingsForm', 'submit', (e) => {
            e.preventDefault();
            this.saveSettings();
        });

        // 테마 변경 즉시 적용
        this.addEventListener('#theme', 'change', (e) => {
            this.applyTheme(e.target.value);
        });

        // 기본값 재설정
        this.addClickListener('#resetBtn', () => this.resetToDefaults());

        // 설정 내보내기/가져오기
        this.addClickListener('#exportBtn', () => this.exportSettings());
        this.addClickListener('#importBtn', () => this.importSettings());
        this.addEventListener('#importFile', 'change', (e) => this.handleImportFile(e));

        // 백업 위치 선택
        this.addClickListener('#selectBackupLocation', () => this.selectBackupLocation());

        // 실시간 변경 감지
        this.addEventListener('.form-input, .form-select, .form-checkbox input', 'change', () => {
            this.markAsChanged();
        });
    }

    loadSettings() {
        const defaultSettings = {
            // 일반 설정
            language: 'ko',
            theme: 'auto',
            enableNotifications: true,
            enableSounds: false,

            // 스캔 설정
            maxFileSize: 1024,
            workerCount: 4,
            hashAlgorithm: 'sha1',
            skipHiddenFiles: true,
            skipSystemFiles: true,

            // 정리 설정
            confirmBeforeDelete: true,
            moveToTrash: true,
            autoCleanEmptyFolders: true,
            backupLocation: '',

            // 성능 설정
            cacheSize: 256,
            batchSize: 100,
            enableProgressSaving: true,
            enableMemoryOptimization: true,

            // 고급 설정
            enableDebugMode: false,
            enableTelemetry: false,
            apiTimeout: 60,
            logLevel: 'info'
        };

        try {
            const saved = localStorage.getItem('appSettings');
            return saved ? { ...defaultSettings, ...JSON.parse(saved) } : defaultSettings;
        } catch {
            return defaultSettings;
        }
    }

    saveSettings() {
        // 폼 데이터 수집
        const formData = new FormData(this.element.querySelector('#settingsForm'));
        const newSettings = {};

        // 텍스트/숫자 입력값
        for (const [key, value] of formData.entries()) {
            if (key === 'maxFileSize' || key === 'workerCount' || key === 'cacheSize' || 
                key === 'batchSize' || key === 'apiTimeout') {
                newSettings[key] = parseInt(value);
            } else {
                newSettings[key] = value;
            }
        }

        // 체크박스 값 처리
        const checkboxes = this.element.querySelectorAll('input[type="checkbox"]');
        checkboxes.forEach(checkbox => {
            newSettings[checkbox.name] = checkbox.checked;
        });

        // 설정 저장
        this.settings = { ...this.settings, ...newSettings };
        localStorage.setItem('appSettings', JSON.stringify(this.settings));

        // 설정 적용
        this.applySettings();

        Toast.show('설정이 저장되었습니다', 'success');
        this.originalSettings = { ...this.settings };
        this.markAsUnchanged();
    }

    applySettings() {
        // 테마 적용
        this.applyTheme(this.settings.theme);

        // 전역 상태 업데이트
        if (window.stateManager) {
            window.stateManager.set('settings', this.settings);
        }

        // 설정 변경 이벤트 발생
        window.dispatchEvent(new CustomEvent('settingsChanged', {
            detail: this.settings
        }));
    }

    applyTheme(theme) {
        let actualTheme = theme;
        
        if (theme === 'auto') {
            actualTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }

        document.documentElement.setAttribute('data-theme', actualTheme);
        
        // 상태 관리자에 테마 상태 저장
        if (window.stateManager) {
            window.stateManager.set('currentTheme', actualTheme);
        }
    }

    resetToDefaults() {
        if (!confirm('모든 설정을 기본값으로 재설정하시겠습니까?')) return;

        this.settings = this.loadSettings();
        localStorage.removeItem('appSettings');
        
        // 폼 필드 업데이트
        this.updateFormFields();
        this.applySettings();
        
        Toast.show('설정이 기본값으로 재설정되었습니다', 'info');
        this.originalSettings = { ...this.settings };
        this.markAsUnchanged();
    }

    updateFormFields() {
        // 텍스트/숫자 입력값 업데이트
        Object.keys(this.settings).forEach(key => {
            const element = this.element.querySelector(`[name="${key}"]`);
            if (element) {
                if (element.type === 'checkbox') {
                    element.checked = this.settings[key];
                } else {
                    element.value = this.settings[key];
                }
            }
        });
    }

    exportSettings() {
        const dataStr = JSON.stringify(this.settings, null, 2);
        const blob = new Blob([dataStr], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        
        const a = document.createElement('a');
        a.href = url;
        a.download = `drive-duplicates-settings-${new Date().toISOString().split('T')[0]}.json`;
        a.click();
        
        URL.revokeObjectURL(url);
        Toast.show('설정이 내보내기되었습니다', 'success');
    }

    importSettings() {
        this.element.querySelector('#importFile').click();
    }

    handleImportFile(event) {
        const file = event.target.files[0];
        if (!file) return;

        const reader = new FileReader();
        reader.onload = (e) => {
            try {
                const importedSettings = JSON.parse(e.target.result);
                
                // 유효성 검사
                if (this.validateSettings(importedSettings)) {
                    this.settings = { ...this.settings, ...importedSettings };
                    this.updateFormFields();
                    Toast.show('설정이 가져오기되었습니다', 'success');
                    this.markAsChanged();
                } else {
                    Toast.show('유효하지 않은 설정 파일입니다', 'error');
                }
            } catch {
                Toast.show('설정 파일을 읽을 수 없습니다', 'error');
            }
        };
        reader.readAsText(file);
        
        // 파일 입력 리셋
        event.target.value = '';
    }

    validateSettings(settings) {
        // 기본적인 유효성 검사
        const requiredKeys = ['language', 'theme', 'maxFileSize', 'workerCount'];
        return requiredKeys.every(key => key in settings);
    }

    selectBackupLocation() {
        // 실제 구현에서는 파일 선택 다이얼로그를 사용
        const location = prompt('백업 폴더 경로를 입력하세요:', this.settings.backupLocation);
        if (location !== null) {
            this.element.querySelector('#backupLocation').value = location;
            this.markAsChanged();
        }
    }

    markAsChanged() {
        const saveBtn = this.element.querySelector('button[type="submit"]');
        if (saveBtn && !saveBtn.classList.contains('btn-warning')) {
            saveBtn.classList.remove('btn-primary');
            saveBtn.classList.add('btn-warning');
            saveBtn.innerHTML = '<i class="fas fa-exclamation-triangle"></i> 변경사항 저장';
        }
    }

    markAsUnchanged() {
        const saveBtn = this.element.querySelector('button[type="submit"]');
        if (saveBtn) {
            saveBtn.classList.remove('btn-warning');
            saveBtn.classList.add('btn-primary');
            saveBtn.innerHTML = '<i class="fas fa-save"></i> 설정 저장';
        }
    }

    hasUnsavedChanges() {
        return JSON.stringify(this.settings) !== JSON.stringify(this.originalSettings);
    }

    onMount() {
        // 초기 설정 적용
        this.applySettings();
        
        // 시스템 테마 변경 감지
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (this.settings.theme === 'auto') {
                this.applyTheme('auto');
            }
        });
    }

    onUnmount() {
        // 저장되지 않은 변경사항 확인
        if (this.hasUnsavedChanges()) {
            const save = confirm('저장되지 않은 변경사항이 있습니다. 저장하시겠습니까?');
            if (save) {
                this.saveSettings();
            }
        }
    }
}