# CLAUDE.md

이 파일은 Claude Code (claude.ai/code)가 이 저장소에서 작업할 때 지침을 제공합니다.

## 개발 규칙

### 커밋 메시지 규칙
- **모든 커밋 메시지는 한국어로 작성**
- 제목: 간결한 변경사항 요약 (50자 이내)
- 본문: 상세한 변경 내용과 이유 설명

### 로그 메시지 규칙
- 사용자 대면 로그는 한국어로 작성
- 이모지를 활용하여 로그 종류 구분 (✅, ❌, ⚠️, 🔍 등)

### 커밋 시 개발 히스토리 기록 규칙
- **커밋 전에 반드시 문서 저장소(`../go-drive-duplicates-docs/History/`)에 개발 히스토리 파일 작성**
- 파일명 형식: `YYYY-MM-DD_Dev_History.md`
- 같은 날짜에 여러 작업이 있으면 하나의 파일에 섹션을 추가
- 내용: 배경, 변경 내용, 수정 파일 목록
- 히스토리 파일은 문서 저장소에서 별도 커밋

### 코드 품질 규칙
- **SOLID 원칙 준수**: 단일 책임, 개방-폐쇄, 리스코프 치환, 인터페이스 분리, 의존성 역전
- **클린 아키텍처 준수**: 계층화된 구조, 의존성 규칙, 비즈니스 로직과 프레임워크 분리

### 삭제 정책
- **파일 삭제는 기본적으로 휴지통 이동** (`TrashFile` 사용)
- `DeleteFile`(완전 삭제)은 사용자가 명시적으로 선택한 경우에만 사용
- UI 문구도 "삭제" 대신 "휴지통으로 이동"으로 표기

## 명령어

### 백엔드
```bash
go mod tidy                                    # 의존성 설치
go run cmd/server/main.go                      # 서버 실행
go build -o go-drive-duplicates cmd/server/main.go  # 빌드
go fmt ./...                                   # 포맷팅
go vet ./...                                   # 검증
```

### 프론트엔드
```bash
python3 start-frontend-config.py               # 설정 기반 실행
python3 -m http.server 3000 --directory static  # 직접 실행
```

### 데이터베이스
```bash
# SQLite DB는 자동 생성/마이그레이션
rm drive_duplicates.db                          # 초기화
```

## 아키텍처

현재 버전: **v1.4** (2026년 3월)

### 프로젝트 구조
```
/cmd/server/main.go          # 엔트리포인트
/internal/
├── domain/                  # 도메인 계층 (엔티티, 리포지토리/서비스 인터페이스)
├── usecases/                # 유스케이스 계층 (비즈니스 로직)
├── interfaces/controllers/  # HTTP 컨트롤러
└── infrastructure/          # 인프라 (SQLite, Google Drive 어댑터, 설정, DI)
/static/                     # 프론트엔드 (어드민 대시보드 + 독립 페이지들)
/config/app.yaml             # 서버 설정
```

> 기술 문서는 별도 저장소로 분리: `../go-drive-duplicates-docs/`

### 주요 유스케이스
| 유스케이스 | 파일 | 기능 |
|-----------|------|------|
| FileScanningUseCase | `file_scanning.go` | 파일 스캔, 체크포인트 재개 |
| DuplicateFindingUseCase | `duplicate_finding.go` | 해시 기반 중복 검출 |
| FolderComparisonUseCase | `folder_comparison.go` | 폴더 비교, 단일 폴더 중복, 파일 이동 |
| FileCleanupUseCase | `file_cleanup.go` | 파일 휴지통 이동, 패턴 삭제 |
| FolderAnalysisUseCase | `folder_analysis.go` | 폴더 분석, 빈 폴더 정리 |
| FileExplorerUseCase | `file_explorer_usecase.go` | 듀얼 패널 탐색기 |
| FolderDownloadUseCase | `folder_download.go` | 폴더 ZIP 다운로드 |

### 프론트엔드 페이지
| 페이지 | 파일 | JS |
|--------|------|----|
| 어드민 대시보드 | `index.html` | `dashboard.js`, `duplicate-groups.js` 등 |
| 폴더 비교 | `compare.html` | `compare.js` |
| 폴더 분석 | `folder-analysis.html` | `folder-analysis.js` |
| 폴더 다운로드 | `folder-download.html` | `folder-download.js` |
| 파일 탐색기 | `explorer.html` | `explorer-main.js`, `explorer-api.js` |
| 일괄 아카이브 | `batch-archive.html` | `batch-archive.js` |

### UI 레이아웃
어드민 스타일: 왼쪽 사이드바(다크) + 상단바 + 메인 콘텐츠. 각 페이지는 iframe으로 로드.
- 디자인 상세: `../go-drive-duplicates-docs/DESIGN_Admin_UI_Guide.md`

### 데이터베이스 (SQLite)
자동 마이그레이션, WAL 모드, 64MB 캐시. 주요 테이블:
- `files`, `duplicate_groups`, `duplicate_group_files`, `progress`, `comparison_results`, `comparison_duplicate_files`

### 설정
- **서버**: `config/app.yaml` (포트, Google Drive 인증, 해시 알고리즘, 워커 수)
- **프론트엔드**: `static/config/frontend.yaml` (포트, 백엔드 URL)
- 환경 변수로 오버라이드 가능 (`SERVER_PORT`, `DATABASE_PATH` 등)
## 문서 참조

기술 문서는 별도 저장소 `../go-drive-duplicates-docs/`에서 관리합니다.

| 분류 | 위치 | 설명 |
|------|------|------|
| 문서 인덱스 | `README.md` | 전체 문서 목록 |
| 기능 문서 | `FEATURE_*.md` | 각 기능별 API, 데이터 구조, 워크플로우 |
| 디자인 문서 | `DESIGN_*.md` | UI 색상, 레이아웃, 컴포넌트 |
| 가이드 문서 | `GUIDE_*.md` | 설정, 마이그레이션, 코드 품질 |
| 계획 문서 | `PLAN_*.md` | 미구현/진행 중 기능 계획 |
| 개발 히스토리 | `History/` | 날짜별 개발 기록 |
