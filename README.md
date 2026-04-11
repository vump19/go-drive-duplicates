# Google Drive 중복 파일 관리 시스템

Google Drive의 중복 파일을 찾고, 폴더를 비교/정리하며, 다양한 파일 관리 기능을 제공하는 웹 애플리케이션입니다.

## 주요 기능

| 기능 | 설명 | 문서 |
|------|------|------|
| 파일 스캔 | Google Drive 전체/폴더별 메타데이터 수집 | [상세](docs/FEATURE_File_Scanning.md) |
| 중복 검사 | SHA-256 해시 기반 중복 검출 및 그룹화 | [상세](docs/FEATURE_Duplicate_Finding.md) |
| 폴더 비교 | 두 폴더 간 중복 비교, 비중복 파일 이동 | [상세](docs/FEATURE_Folder_Comparison.md) |
| 폴더 내 중복 | 단일 폴더 내 중복 파일 검색 | [상세](docs/FEATURE_Single_Folder_Duplicate.md) |
| 폴더 분석 | 구조 분석, 빈 폴더 정리, 폴더명 검색 | [상세](docs/FEATURE_Folder_Analysis.md) |
| 파일 정리 | 휴지통 이동, 패턴 기반 일괄 정리 | [상세](docs/FEATURE_File_Cleanup.md) |
| 일괄 아카이브 | 여러 폴더를 ZIP 압축 후 원본 정리 | [상세](docs/FEATURE_Batch_Archive.md) |
| 파일 탐색기 | 듀얼 패널 탐색기 (복사/이동/삭제) | [상세](docs/PLAN_Dual_Panel_Explorer.md) |
| 폴더 다운로드 | Google Drive 폴더를 ZIP으로 다운로드 | - |
| 스마트 정리 | AI 기반 파일 정리 제안 | [상세](docs/PLAN_Smart_Organizer.md) |
| 텔레그램 알림 | 장시간 작업 완료/실패 알림 | - |

## 빠른 시작

### 1. Google Drive API 설정

1. [Google Cloud Console](https://console.cloud.google.com/)에서 프로젝트 생성
2. Google Drive API 활성화
3. OAuth 2.0 클라이언트 ID 생성 (데스크톱 애플리케이션)
4. JSON 파일을 `credentials.json`으로 프로젝트 루트에 저장

### 2. 실행

```bash
# 백엔드
go mod tidy
go run cmd/server/main.go

# 프론트엔드 (별도 터미널)
python3 -m http.server 3000 --directory static
```

### 3. 접속

- **메인 UI**: http://localhost:3000
- **서버 상태**: http://localhost:9090/health

포트 설정: 백엔드 `config/app.yaml`, 프론트엔드 `static/config/frontend.yaml`

## 프로젝트 구조

```
go-drive-duplicates/
├── cmd/server/main.go              # 엔트리포인트
├── internal/
│   ├── domain/                     # 도메인 계층 (엔티티, 인터페이스)
│   ├── usecases/                   # 유스케이스 계층 (비즈니스 로직)
│   ├── interfaces/controllers/     # HTTP 컨트롤러
│   └── infrastructure/             # 인프라 (DB, Google Drive, 설정)
├── static/                         # 프론트엔드 (어드민 대시보드 + 독립 페이지)
├── config/app.yaml                 # 서버 설정
├── docs/                           # 기술 문서
└── drive_duplicates.db             # SQLite DB (자동 생성)
```

## 기술 스택

| 영역 | 기술 |
|------|------|
| 백엔드 | Go 1.21+, Clean Architecture, 의존성 주입 |
| 데이터베이스 | SQLite3 (WAL 모드, 자동 마이그레이션) |
| 외부 API | Google Drive API v3, OAuth 2.0 |
| 프론트엔드 | Vanilla JavaScript, Font Awesome |

## 문제 해결

| 문제 | 해결 |
|------|------|
| 인증 오류 | `token.json` 삭제 후 재인증 |
| API 할당량 초과 | 24시간 후 재시도 |
| 검사 중단 | 체크포인트에서 자동 재개 |
| 파일 삭제 실패 | Google Drive scope 확인 (`drive.readonly` → `drive`) |

## 문서

- [기능/디자인/가이드 문서 인덱스](docs/README.md)
- [UI 디자인 가이드](docs/DESIGN_Admin_UI_Guide.md)
- [코드 품질 가이드](docs/GUIDE_Code_Quality.md)
- [설정 마이그레이션](docs/GUIDE_Config_Migration.md)
- [개발 히스토리](docs/History/)

## 릴리즈

| 버전 | 날짜 | 주요 변경 |
|------|------|----------|
| v1.4 | 2026-03 | 어드민 UI 레이아웃, 페이지 분리, 휴지통 이동 통일 |
| v1.3 | 2026-01 | 일괄 아카이브, 폴더 다운로드, 대용량 응답 개선 |
| v1.2 | 2025-12 | 삭제 방식 선택 (휴지통/완전삭제) |
| v1.1 | 2025-09 | 대량 중복 관리, 폴더 분석, 스마트 삭제 UI |
| v1.0 | 2025-08 | 파일 스캔, 중복 검사, 폴더 비교 |

---
*최종 업데이트: 2026년 3월 28일 | 버전: 1.4*
