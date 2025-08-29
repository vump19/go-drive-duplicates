# Google Drive 중복 파일 검사기 - Clean Architecture

구글 드라이브의 모든 파일을 분석하여 중복되는 파일을 찾아주는 REST API 서버입니다.  
SOLID 원칙과 클린 아키텍처 패턴을 적용하여 확장 가능하고 유지보수하기 쉽게 설계되었습니다.

## 🏗️ 아키텍처 특징

- 🏛️ **Clean Architecture**: 도메인 중심의 계층화된 아키텍처
- 🔧 **SOLID 원칙**: 단일 책임, 개방/폐쇄, 리스코프 치환, 인터페이스 분리, 의존성 역전 원칙 적용
- 🔌 **의존성 주입**: 느슨한 결합과 테스트 용이성
- 📦 **도메인 주도 설계**: 비즈니스 로직의 명확한 분리
- 🛡️ **미들웨어 패턴**: 로깅, 에러 처리, 보안, CORS 지원

## 주요 기능

- 📁 **Google Drive 전체 스캔**: 모든 파일을 자동으로 검색하고 분석  
- 🔍 **정확한 중복 검사**: 파일 크기와 SHA-256 해시를 기반으로 한 정밀한 중복 검출
- 🌐 **RESTful API**: 표준 HTTP 메서드와 JSON 응답
- 🔗 **Google Drive 연동**: 중복 파일에 대한 직접 링크 및 경로 정보 제공
- 📊 **페이지네이션**: 대용량 데이터를 효율적으로 처리
- 🗑️ **스마트 삭제**: 중복 그룹 단위 삭제 및 정규식 패턴 기반 일괄 삭제
- ⚡ **상태 관리**: 진행 상황 추적 및 작업 재개 기능
- 🔒 **안전한 인증**: OAuth 2.0 기반 Google Drive 접근

## 🖥️ 포트 설정

이 애플리케이션은 프론트엔드와 백엔드가 분리된 구조입니다:

- **백엔드 서버**: 포트 **8080** (Go 서버 - API 및 정적 파일 서빙)
- **프론트엔드 개발 서버**: 포트 **3000** (모던 UI 개발용)

### 🚀 빠른 시작

1. **백엔드 서버 실행**:
   ```bash
   go run cmd/server/main.go
   # 또는
   ./go-drive-duplicates
   ```
   → http://localhost:8080 에서 실행

2. **프론트엔드 개발 서버 실행** (새로운 모던 UI):
   ```bash
   # Linux/Mac
   ./start-frontend.sh
   
   # Windows
   start-frontend.bat
   
   # 또는 직접 실행
   python3 dev-server.py
   ```
   → http://localhost:3000 에서 실행

### 🌐 접속 URL

| 서비스 | URL | 설명 |
|--------|-----|------|
| 모던 UI (권장) | http://localhost:3000/index_port3000.html | ES6 모듈 기반 새로운 UI |
| 기존 UI | http://localhost:8080/static/index.html | 레거시 UI (참고용) |
| API 문서 | http://localhost:8080/health | 서버 상태 확인 |

## 설치 및 실행

### 1. Google Drive API 설정

1. [Google Cloud Console](https://console.cloud.google.com/)에 접속
2. 새 프로젝트 생성 또는 기존 프로젝트 선택
3. Google Drive API 활성화:
   - API 및 서비스 > 라이브러리로 이동
   - "Google Drive API" 검색 후 활성화
4. OAuth 동의 화면 설정:
   - API 및 서비스 > OAuth 동의 화면으로 이동
   - 사용자 유형: "외부" 선택 (개인 사용)
   - 앱 이름: "Drive Duplicates Finder"
   - 사용자 지원 이메일: 본인 이메일
   - 범위 추가: Google Drive API (../auth/drive) - 전체 접근 권한
5. OAuth 2.0 클라이언트 ID 생성:
   - API 및 서비스 > 사용자 인증 정보로 이동
   - "사용자 인증 정보 만들기" > "OAuth 클라이언트 ID" 선택
   - 애플리케이션 유형: "데스크톱 애플리케이션"
   - 이름: "Drive Duplicates Finder"
6. 생성된 클라이언트 ID의 JSON 파일을 다운로드
7. 다운로드한 파일을 프로젝트 루트에 `credentials.json`으로 저장

**credentials.json 파일 예시:**
```json
{
  "installed": {
    "client_id": "your-client-id.googleusercontent.com",
    "project_id": "your-project-id",
    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
    "token_uri": "https://oauth2.googleapis.com/token",
    "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
    "client_secret": "your-client-secret",
    "redirect_uris": ["urn:ietf:wg:oauth:2.0:oob","http://localhost"]
  }
}
```

### 2. 프로그램 실행

```bash
# 의존성 설치
make deps

# 서버 빌드 및 실행 (기본 설정)
make run

# 또는 직접 빌드 후 실행
make build
./server -config config/app.json

# YAML 설정 사용
make run-yaml

# 개발 환경으로 실행
make dev

# 프로덕션 환경으로 실행  
make prod
```

### 3. API 문서 확인

```
http://localhost:8080
```

서버 홈페이지에서 사용 가능한 모든 REST API 엔드포인트를 확인할 수 있습니다.

## 사용 방법

### REST API 사용법

#### 1. Health Check
```bash
# 서버 상태 확인
curl http://localhost:8080/health

# 데이터베이스 연결 확인
curl http://localhost:8080/health/db

# Google Drive 연결 확인  
curl http://localhost:8080/health/storage
```

#### 2. 파일 스캔
```bash
# 전체 파일 스캔 시작
curl -X POST http://localhost:8080/api/files/scan

# 스캔 진행 상황 확인
curl http://localhost:8080/api/files/scan/progress

# 특정 폴더 스캔
curl -X POST http://localhost:8080/api/files/scan/folder \
  -H "Content-Type: application/json" \
  -d '{"folderId": "folder-id-here"}'
```

#### 3. 중복 파일 검색
```bash
# 중복 파일 검색 시작
curl -X POST http://localhost:8080/api/duplicates/find

# 중복 그룹 목록 조회 (페이지네이션)
curl "http://localhost:8080/api/duplicates/groups?page=1&limit=10"

# 특정 중복 그룹 조회
curl "http://localhost:8080/api/duplicates/group?id=1"

# 파일 경로 조회 (실시간 Google Drive API)
curl "http://localhost:8080/api/duplicates/file/path?fileId=your-file-id"

# 중복 그룹 삭제
curl -X DELETE "http://localhost:8080/api/duplicates/group/delete?id=1"
```

#### 4. 폴더 비교
```bash
# 두 폴더 비교
curl -X POST http://localhost:8080/api/compare/folders \
  -H "Content-Type: application/json" \
  -d '{"sourceFolderId": "source-id", "targetFolderId": "target-id"}'

# 비교 진행 상황 확인
curl http://localhost:8080/api/compare/progress
```

#### 5. 파일 정리
```bash
# 특정 파일들 삭제
curl -X POST http://localhost:8080/api/cleanup/files \
  -H "Content-Type: application/json" \
  -d '{"fileIds": ["file1-id", "file2-id"]}'

# 빈 폴더 정리
curl -X POST http://localhost:8080/api/cleanup/folders

# 정리 진행 상황 확인
curl http://localhost:8080/api/cleanup/progress
```

### 초기 인증 설정

처음 실행 시 Google 계정 인증이 필요합니다:
1. 서버 실행 후 파일 스캔 API 호출
2. 터미널에 표시되는 OAuth URL을 브라우저에서 열기
3. Google 계정 로그인 후 권한 허용
4. 인증 코드를 터미널에 입력
5. `token.json` 파일이 자동 생성되어 이후 자동 인증

## 작동 원리

1. **파일 목록 수집**: Google Drive API를 통해 모든 파일의 메타데이터 수집
2. **크기별 그룹화**: 동일한 크기의 파일들을 먼저 그룹화
3. **해시 계산**: 동일한 크기의 파일들에 대해 SHA-256 해시 계산
4. **중복 검출**: 동일한 해시를 가진 파일들을 중복으로 판정
5. **결과 표시**: 웹 페이지에서 중복 파일 그룹 표시

## 보안 및 개인정보

- 🔒 **로컬 저장**: 모든 인증 정보와 데이터는 로컬 컴퓨터에만 저장
- 📁 **임시 다운로드**: 파일 내용은 해시 계산 시에만 임시로 다운로드
- 🛡️ **OAuth 2.0**: Google의 표준 인증 프로토콜 사용
- ⚠️ **삭제 권한**: 일괄 삭제 기능을 위해 Drive 전체 접근 권한 필요
- 📊 **투명성**: 모든 작업 로그가 터미널에 실시간 표시

## 기술 스택

### Backend Architecture
- **Language**: Go 1.21+ (Goroutines를 활용한 동시성 처리)
- **Architecture**: Clean Architecture + SOLID Principles
- **Dependency Injection**: 컨테이너 기반 의존성 관리
- **Web Framework**: Go 표준 라이브러리 net/http + 커스텀 미들웨어

### Data Layer
- **Database**: SQLite3 (로컬 파일 데이터베이스)
- **ORM**: sqlx (SQL 확장 라이브러리)
- **Migration**: 자동 데이터베이스 스키마 마이그레이션

### External APIs
- **Storage**: Google Drive API v3 (REST)
- **Auth**: OAuth 2.0 (Google Cloud Platform)
- **File Processing**: SHA-256 해시 알고리즘

### Configuration & Deployment
- **Config**: JSON/YAML 설정 파일 지원
- **Environment**: 환경별 설정 분리 (dev/prod/test)
- **Build**: Makefile 기반 빌드 시스템
- **Logging**: 구조화된 로깅 및 미들웨어

## 릴리즈 정보

### v1.0 (현재 버전)
- ✅ Google Drive 전체 파일 스캔 및 중복 검사
- ✅ 실시간 진행 상황 모니터링 및 웹 인터페이스
- ✅ **파일 경로 표시**: 실시간 Google Drive 경로 조회 및 표시
- ✅ **페이지네이션**: 백엔드/프론트엔드 통합 페이지네이션 시스템
- ✅ **모달 기반 상세 뷰**: 중복 그룹 상세 정보 및 파일별 경로 보기
- ✅ 정규식 기반 일괄 파일 삭제 기능
- ✅ 작업 재시작 및 체크포인트 시스템
- ✅ API 오류 복구 및 재시도 로직
- ✅ SQLite 데이터베이스 자동 마이그레이션
- ✅ **외래 키 제약 조건 해결**: 안전한 중복 그룹 삭제

## 개발 히스토리

상세한 개발 과정과 기술적 개선사항은 [Dev_History.md](Dev_History.md)를 참고하세요.

## 라이선스

이 프로젝트는 MIT 라이선스 하에 배포됩니다.

## 기여

버그 리포트나 기능 제안은 GitHub Issues를 통해 제출해 주세요.

---
*최종 업데이트: 2025년 8월 15일*  
*버전: 1.0*  
*개발: Claude AI Assistant*

## 프로젝트 구조

```
go-drive-duplicates/
├── cmd/                        # 애플리케이션 엔트리포인트
│   ├── server/main.go         # REST API 서버 메인
│   └── migrate/main.go        # 데이터베이스 마이그레이션 도구
├── internal/                   # 프라이빗 애플리케이션 코드
│   ├── domain/                # 도메인 계층 (비즈니스 로직)
│   │   ├── entities/          # 도메인 엔티티
│   │   ├── repositories/      # 리포지토리 인터페이스
│   │   └── services/          # 도메인 서비스 인터페이스
│   ├── usecases/              # 유스케이스 계층 (애플리케이션 로직)
│   ├── interfaces/            # 인터페이스 계층 (컨트롤러, 프레젠터)
│   │   ├── controllers/       # HTTP 컨트롤러
│   │   ├── middleware/        # HTTP 미들웨어
│   │   └── presenters/        # 응답 DTO 및 매퍼
│   └── infrastructure/        # 인프라스트럭처 계층
│       ├── config/           # 설정 관리 및 애플리케이션 부트스트랩
│       ├── repositories/     # 데이터베이스 구현체
│       ├── services/         # 외부 서비스 어댑터
│       └── database/         # 마이그레이션 스크립트
├── config/                    # 설정 파일
│   ├── app.json              # 기본 JSON 설정
│   ├── app.yaml              # 기본 YAML 설정
│   └── environments/         # 환경별 설정
│       ├── development.yaml
│       ├── production.yaml
│       └── testing.yaml
├── docs/                     # 문서
├── credentials.json          # Google OAuth 클라이언트 설정 (사용자 생성)
├── token.json               # OAuth 토큰 (자동 생성)
├── drive_duplicates.db      # SQLite 데이터베이스 (자동 생성)
├── server                   # 컴파일된 서버 바이너리
├── Makefile                 # 빌드 및 실행 스크립트
├── go.mod                   # Go 모듈 정의
└── README.md                # 프로젝트 설명서
```

### 아키텍처 다이어그램

```
┌─────────────────────────────────────────┐
│                Clients                   │
│            (HTTP/REST API)              │
└─────────────────────────────────────────┘
                    │
┌─────────────────────────────────────────┐
│           Interface Layer               │
│  ┌─────────────┐ ┌─────────────────────┐│
│  │Controllers │ │    Middleware       ││
│  └─────────────┘ └─────────────────────┘│
└─────────────────────────────────────────┘
                    │
┌─────────────────────────────────────────┐
│           UseCase Layer                 │
│  ┌─────────────────────────────────────┐│
│  │   Application Business Rules       ││
│  └─────────────────────────────────────┘│
└─────────────────────────────────────────┘
                    │
┌─────────────────────────────────────────┐
│            Domain Layer                 │
│  ┌─────────┐ ┌─────────────┐ ┌───────── ││
│  │Entities │ │Repositories │ │Services  ││
│  └─────────┘ └─────────────┘ └─────────┘│
└─────────────────────────────────────────┘
                    │
┌─────────────────────────────────────────┐
│         Infrastructure Layer            │
│  ┌─────────┐ ┌─────────────┐ ┌─────────┐│
│  │Database │ │Google Drive │ │Config   ││
│  └─────────┘ └─────────────┘ └─────────┘│
└─────────────────────────────────────────┘
```

## 주의사항 및 제한사항

### 성능 관련
- 📈 **검사 시간**: 파일 수가 많을수록 검사 시간이 길어짐 (수만 개 파일의 경우 수 시간 소요 가능)
- 💾 **메모리 사용**: 대량의 파일 메타데이터를 메모리에 로드하므로 충분한 RAM 필요
- 🌐 **네트워크**: 해시 계산을 위해 모든 파일을 다운로드하므로 네트워크 대역폭 소모

### API 제한
- 📊 **할당량**: Google Drive API 일일 할당량 (기본 1,000,000,000 단위/일)
- 🕐 **속도 제한**: 초당 100개 요청 제한으로 대량 파일 처리 시 시간 소요
- 🔄 **재시도**: API 오류 발생 시 자동 재시도되므로 완전 실패는 드물음

### 사용 환경
- 💻 **로컬 실행**: 웹 서버이지만 로컬 환경에서만 동작 (포트 8080)
- 🌐 **인터넷 필수**: Google Drive API 호출을 위해 지속적인 인터넷 연결 필요
- 🔑 **권한 관리**: 파일 삭제 기능 사용 시 Drive 전체 접근 권한 필요

## 문제 해결

- **인증 오류**: token.json 파일 삭제 후 재인증
- **API 할당량 초과**: 24시간 후 재시도 또는 Google Cloud Console에서 할당량 확인
- **메모리 부족**: 페이지네이션 크기를 줄이거나 시스템 메모리 증설
- **검사 중단**: "작업 재개" 버튼으로 중단 지점부터 계속 진행