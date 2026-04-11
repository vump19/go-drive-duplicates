/**
 * 설정 관리자
 * 프론트엔드 YAML 설정 파일을 로드하고 관리하는 중앙화된 서비스
 */

export class ConfigManager {
    constructor() {
        this.config = null;
        this.configPath = './config/frontend.yaml';
        this.defaultConfig = this.getDefaultConfig();
    }

    /**
     * 기본 설정값
     */
    getDefaultConfig() {
        return {
            frontend: {
                port: 3000,
                host: 'localhost',
                title: 'Google Drive Duplicates Finder',
                theme: 'light'
            },
            backend: {
                protocol: 'http',
                host: 'localhost',
                port: 9090,
                api_path: '/api',
                timeout: 30000,
                retries: 3
            },
            features: {
                enable_auto_scan: true,
                enable_hash_verification: true,
                enable_bulk_operations: true,
                max_file_size: 1073741824,
                supported_formats: ['image', 'video', 'document', 'archive']
            },
            ui: {
                pagination: {
                    default_page_size: 50,
                    max_page_size: 200
                },
                progress: {
                    update_interval: 1000,
                    animation_duration: 300
                },
                notifications: {
                    position: 'top-right',
                    duration: 5000
                }
            }
        };
    }

    /**
     * 설정 파일 로드
     */
    async loadConfig() {
        try {
            console.log('🔧 설정 파일 로드 중...', this.configPath);
            
            const response = await fetch(this.configPath);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const yamlText = await response.text();
            this.config = this.parseYAML(yamlText);
            
            // 기본값과 병합
            this.config = this.mergeWithDefaults(this.config);
            
            console.log('✅ 설정 파일 로드 완료', this.config);
            return this.config;
            
        } catch (error) {
            console.warn('⚠️  설정 파일 로드 실패, 기본 설정 사용:', error.message);
            this.config = this.defaultConfig;
            return this.config;
        }
    }

    /**
     * 간단한 YAML 파서 (기본적인 YAML 문법만 지원)
     */
    parseYAML(yamlText) {
        const result = {};
        const lines = yamlText.split('\n');
        const stack = [{ obj: result, indent: -1 }];

        for (const line of lines) {
            const trimmed = line.trim();
            
            // 빈 줄이나 주석 무시
            if (!trimmed || trimmed.startsWith('#')) {
                continue;
            }

            const indent = line.length - line.trimStart().length;
            const colonIndex = trimmed.indexOf(':');
            
            if (colonIndex === -1) {
                continue;
            }

            const key = trimmed.substring(0, colonIndex).trim();
            const valueStr = trimmed.substring(colonIndex + 1).trim();

            // 현재 들여쓰기에 맞는 부모 찾기
            while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
                stack.pop();
            }

            const parent = stack[stack.length - 1].obj;

            if (valueStr === '' || valueStr === '{}' || valueStr === '[]') {
                // 객체나 배열의 시작
                if (valueStr === '[]' || (lines[lines.indexOf(line) + 1] && 
                    lines[lines.indexOf(line) + 1].trim().startsWith('-'))) {
                    parent[key] = [];
                } else {
                    parent[key] = {};
                    stack.push({ obj: parent[key], indent });
                }
            } else if (trimmed.startsWith('- ')) {
                // 배열 항목
                const arrayValue = trimmed.substring(2).trim();
                const parsedValue = this.parseValue(arrayValue);
                if (Array.isArray(parent)) {
                    parent.push(parsedValue);
                }
            } else {
                // 일반 값
                parent[key] = this.parseValue(valueStr);
            }
        }

        return result;
    }

    /**
     * 값 파싱 (문자열, 숫자, 불린)
     */
    parseValue(valueStr) {
        // 문자열 (따옴표 제거)
        if ((valueStr.startsWith('"') && valueStr.endsWith('"')) ||
            (valueStr.startsWith("'") && valueStr.endsWith("'"))) {
            return valueStr.slice(1, -1);
        }

        // 불린
        if (valueStr === 'true') return true;
        if (valueStr === 'false') return false;
        if (valueStr === 'null') return null;

        // 숫자
        if (!isNaN(valueStr) && !isNaN(parseFloat(valueStr))) {
            return valueStr.includes('.') ? parseFloat(valueStr) : parseInt(valueStr, 10);
        }

        // 기본적으로 문자열
        return valueStr;
    }

    /**
     * 기본 설정과 병합
     */
    mergeWithDefaults(config) {
        const merged = JSON.parse(JSON.stringify(this.defaultConfig));
        return this.deepMerge(merged, config);
    }

    /**
     * 깊은 병합
     */
    deepMerge(target, source) {
        for (const key in source) {
            if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
                if (!target[key]) target[key] = {};
                this.deepMerge(target[key], source[key]);
            } else {
                target[key] = source[key];
            }
        }
        return target;
    }

    /**
     * 설정값 가져오기
     */
    get(path, defaultValue = null) {
        if (!this.config) {
            console.warn('설정이 아직 로드되지 않았습니다. loadConfig()를 먼저 호출하세요.');
            return defaultValue;
        }

        const keys = path.split('.');
        let current = this.config;

        for (const key of keys) {
            if (current && typeof current === 'object' && key in current) {
                current = current[key];
            } else {
                return defaultValue;
            }
        }

        return current;
    }

    /**
     * 백엔드 URL 생성
     */
    getBackendURL() {
        if (!this.config) return 'http://localhost:8080';
        
        const { protocol, host, port } = this.config.backend;
        return `${protocol}://${host}:${port}`;
    }

    /**
     * API Base URL 생성
     */
    getAPIBaseURL() {
        const baseURL = this.getBackendURL();
        const apiPath = this.get('backend.api_path', '/api');
        return `${baseURL}${apiPath}`;
    }

    /**
     * 프론트엔드 포트 가져오기
     */
    getFrontendPort() {
        return this.get('frontend.port', 3000);
    }

    /**
     * 백엔드 포트 가져오기
     */
    getBackendPort() {
        return this.get('backend.port', 8080);
    }

    /**
     * 환경 변수 오버라이드 적용
     */
    applyEnvironmentOverrides() {
        if (!this.config) return;

        // URL 파라미터에서 설정 오버라이드
        const urlParams = new URLSearchParams(window.location.search);
        
        if (urlParams.has('backend_port')) {
            this.config.backend.port = parseInt(urlParams.get('backend_port'), 10);
        }
        
        if (urlParams.has('backend_host')) {
            this.config.backend.host = urlParams.get('backend_host');
        }

        // localStorage에서 사용자 설정 로드
        const userConfig = localStorage.getItem('frontend_config');
        if (userConfig) {
            try {
                const parsed = JSON.parse(userConfig);
                this.config = this.deepMerge(this.config, parsed);
            } catch (error) {
                console.warn('사용자 설정 파싱 실패:', error);
            }
        }
    }

    /**
     * 사용자 설정 저장
     */
    saveUserConfig(partialConfig) {
        try {
            let userConfig = {};
            const existing = localStorage.getItem('frontend_config');
            if (existing) {
                userConfig = JSON.parse(existing);
            }

            userConfig = this.deepMerge(userConfig, partialConfig);
            localStorage.setItem('frontend_config', JSON.stringify(userConfig));
            
            // 현재 설정에도 적용
            this.config = this.deepMerge(this.config, partialConfig);
            
            console.log('✅ 사용자 설정 저장 완료');
        } catch (error) {
            console.error('❌ 사용자 설정 저장 실패:', error);
        }
    }

    /**
     * 설정 리셋
     */
    resetConfig() {
        localStorage.removeItem('frontend_config');
        this.config = JSON.parse(JSON.stringify(this.defaultConfig));
        console.log('🔄 설정이 기본값으로 리셋되었습니다.');
    }
}

// 싱글톤 인스턴스
export const configManager = new ConfigManager();