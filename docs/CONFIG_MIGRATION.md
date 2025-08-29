# Configuration Migration Guide

## JSON to YAML Migration

이 프로젝트의 설정 시스템이 JSON에서 YAML로 마이그레이션되었습니다. 기존 JSON 설정 파일은 계속 지원되지만, YAML을 사용하는 것을 권장합니다.

## 지원 형식

- **YAML**: `.yaml`, `.yml` (권장)
- **JSON**: `.json` (호환성 유지)

## 기본 설정 파일 변경

```bash
# 이전
./server -config config/app.json

# 현재 (기본값)
./server -config config/app.yaml
```

## 설정 구조 비교

### JSON 형식 (기존)
```json
{
  "server": {
    "host": "localhost",
    "port": 8080,
    "readTimeout": "30s"
  },
  "googleDrive": {
    "credentialsPath": "./credentials.json",
    "maxRetries": 3
  }
}
```

### YAML 형식 (신규)
```yaml
# 주석 지원
server:
  host: localhost
  port: 8080
  read_timeout: 30s

google_drive:
  credentials_path: ./credentials.json
  max_retries: 3
```

## 주요 변경사항

### 1. 필드명 변경 (snake_case)
- `readTimeout` → `read_timeout`
- `writeTimeout` → `write_timeout` 
- `googleDrive` → `google_drive`
- `maxRetries` → `max_retries`
- `workerCount` → `worker_count`

### 2. 새로운 필드 추가
```yaml
environment: development  # 환경 구분용
```

### 3. 주석 지원
YAML에서는 `#`을 사용한 주석이 가능합니다.

## 환경별 설정 파일

### 개발 환경
```bash
./server -config config/environments/development.yaml
```

### 프로덕션 환경
```bash
./server -config config/environments/production.yaml
```

### 테스트 환경
```bash
./server -config config/environments/testing.yaml
```

## 마이그레이션 단계

### 1. 기존 JSON 설정 확인
```bash
# 현재 JSON 설정으로 실행 (호환성 확인)
./server -config config/app.json
```

### 2. YAML 설정으로 전환
```bash
# 새로운 YAML 설정으로 실행
./server -config config/app.yaml
```

### 3. 환경별 설정 활용
```bash
# 개발 환경
ENV=development ./server -config config/environments/development.yaml

# 프로덕션 환경  
ENV=production ./server -config config/environments/production.yaml
```

## 호환성 매트릭스

| 기능 | JSON | YAML | 환경 변수 |
|------|------|------|-----------|
| 기본 설정 | ✅ | ✅ | ✅ |
| 주석 | ❌ | ✅ | ❌ |
| 환경별 설정 | ⚠️ | ✅ | ✅ |
| 검증 | ✅ | ✅ | ✅ |

## 설정 검증

### 설정 파일 유효성 검사
```bash
# YAML 문법 검사
./server -config config/app.yaml -help

# JSON 호환성 검사
./server -config config/app.json -help
```

### 환경 변수 우선순위
1. 명령행 인수 (`-config`)
2. 환경 변수 (`SERVER_PORT=8080`)
3. 설정 파일 (`app.yaml`)
4. 기본값

## 권장사항

1. **신규 프로젝트**: YAML 사용
2. **기존 프로젝트**: 단계적 마이그레이션
3. **운영 환경**: 환경별 설정 파일 사용
4. **개발 환경**: 주석을 활용한 문서화

## 문제 해결

### YAML 파싱 오류
```bash
# 들여쓰기 확인 (스페이스 사용)
# 탭 문자 사용 금지
```

### JSON 호환성 오류
```bash
# 기존 JSON 필드명 확인
# camelCase → snake_case 변환 필요
```

### 환경 변수 설정
```bash
# .env 파일 활용
export SERVER_HOST=0.0.0.0
export SERVER_PORT=9090
```