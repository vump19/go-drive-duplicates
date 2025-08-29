# 코드 품질 개선 작업 요구사항 문서

## 1. 프로젝트 개요

### 1.1 현재 상태
- **코드베이스 규모**: 총 4,661줄 (main.go: 1,073줄, drive.go: 3,588줄)
- **아키텍처**: 단일 바이너리, 모놀리식 구조
- **주요 기능**: Google Drive 중복 파일 검색, 폴더 비교, 파일 삭제

### 1.2 목표
SOLID 원칙과 클린 아키텍처를 준수하는 유지보수 가능한 코드로 리팩토링

## 2. SOLID 원칙 위반 사항 및 개선 계획

### 2.1 Single Responsibility Principle (SRP) 위반

#### 2.1.1 문제점
- **DriveService 구조체**: 74개 함수로 과도한 책임 담당
  - Google Drive API 통합
  - 데이터베이스 작업
  - 파일 해시 계산
  - 중복 파일 검색
  - 폴더 비교
  - 파일 삭제 및 정리
  - 진행 상태 관리

- **HTTP 핸들러**: 요청 검증, 비즈니스 로직, 응답 생성 모두 담당

#### 2.1.2 개선 방안
```
DriveService 분해:
├── FileService (파일 관련 작업)
├── HashService (해시 계산)
├── DuplicateService (중복 검색)
├── ComparisonService (폴더 비교)
├── CleanupService (파일 정리)
└── ProgressService (진행 상태 관리)

HTTP Layer 분리:
├── Controllers (HTTP 요청/응답 처리)
├── Services (비즈니스 로직)
└── Repositories (데이터 접근)
```

### 2.2 Open/Closed Principle (OCP) 위반

#### 2.2.1 문제점
- 하드코딩된 해시 알고리즘 (SHA256)
- 고정된 데이터베이스 (SQLite)
- 단일 스토리지 제공자 (Google Drive)

#### 2.2.2 개선 방안
```go
// 해시 계산 전략 패턴
type HashCalculator interface {
    Calculate(data io.Reader) (string, error)
}

// 스토리지 제공자 인터페이스
type StorageProvider interface {
    ListFiles(ctx context.Context) ([]*File, error)
    DeleteFile(ctx context.Context, fileID string) error
}

// 데이터베이스 추상화
type Repository interface {
    SaveFiles(files []*File) error
    LoadFiles() ([]*File, error)
}
```

### 2.3 Interface Segregation Principle (ISP) 위반

#### 2.3.1 문제점
- 거대한 DriveService로 인한 불필요한 의존성

#### 2.3.2 개선 방안
```go
// 기능별 인터페이스 분리
type FileScanner interface {
    ScanFiles(ctx context.Context) error
}

type DuplicateFinder interface {
    FindDuplicates() ([]*DuplicateGroup, error)
}

type FolderComparator interface {
    Compare(sourceID, targetID string) (*ComparisonResult, error)
}

type FileDeleter interface {
    DeleteFiles(fileIDs []string) error
}
```

### 2.4 Dependency Inversion Principle (DIP) 위반

#### 2.4.1 문제점
- 고수준 모듈이 저수준 모듈에 직접 의존
- 전역 변수를 통한 강한 결합

#### 2.4.2 개선 방안
```go
// 의존성 주입 컨테이너
type Container struct {
    fileService       FileService
    hashService       HashService
    duplicateService  DuplicateService
    comparisonService ComparisonService
    cleanupService    CleanupService
}

// 핸들러에 의존성 주입
type Handler struct {
    fileService      FileService
    duplicateService DuplicateService
}
```

## 3. 클린 아키텍처 설계

### 3.1 레이어 구조
```
┌─────────────────────────────────────┐
│         Web/Handlers (외부)          │  ← 프레임워크, 웹 핸들러
├─────────────────────────────────────┤
│      Controllers (인터페이스)        │  ← HTTP 요청/응답 변환
├─────────────────────────────────────┤
│       Use Cases (비즈니스)           │  ← 애플리케이션 비즈니스 로직
├─────────────────────────────────────┤
│       Entities (엔티티)              │  ← 도메인 모델, 비즈니스 규칙
├─────────────────────────────────────┤
│   Repositories (데이터 접근)         │  ← 데이터 저장소 인터페이스
└─────────────────────────────────────┘
│   Infrastructure (인프라)            │  ← DB, 외부 API 구현체
```

### 3.2 패키지 구조
```
/cmd
  /server          # 메인 애플리케이션
/internal
  /domain
    /entities      # 도메인 엔티티
    /repositories  # 리포지토리 인터페이스
    /services      # 도메인 서비스
  /usecases        # 유스케이스 (비즈니스 로직)
  /interfaces
    /controllers   # HTTP 컨트롤러
    /presenters    # 응답 포맷터
  /infrastructure
    /database      # 데이터베이스 구현
    /drive         # Google Drive API 구현
    /web           # HTTP 서버 설정
/pkg
  /hash           # 해시 계산 유틸리티
  /config         # 설정 관리
```

## 4. 상세 작업 계획

### 4.1 Phase 1: 도메인 모델 정의 ✅ **완료**
- [x] 핵심 엔티티 정의 (File, DuplicateGroup, ComparisonResult, Progress, FileStatistics)
- [x] 도메인 서비스 인터페이스 정의 (StorageProvider, HashCalculator, DuplicateFinder 등)
- [x] 리포지토리 인터페이스 정의 (FileRepository, DuplicateRepository, ComparisonRepository 등)

**완료 내역 (2024년 8월)**:
- 5개 핵심 엔티티 구현: 비즈니스 규칙과 도메인 로직 포함
- 6개 도메인 서비스 인터페이스: 확장 가능한 전략 패턴 적용
- 4개 리포지토리 인터페이스: 데이터 접근 추상화 완료

### 4.2 Phase 2: 유스케이스 계층 구현 ✅ **완료**
- [x] FileScanning 유스케이스 (전체/폴더별 스캔, 병렬 처리)
- [x] DuplicateFinding 유스케이스 (해시 기반 중복 검출, 통계 생성)
- [x] FolderComparison 유스케이스 (폴더 비교, 삭제 권장)
- [x] FileCleanup 유스케이스 (안전 삭제, 패턴 매칭, 폴더 정리)

**완료 내역 (2024년 8월)**:
- 4개 주요 유스케이스 구현: 복잡한 비즈니스 로직 캡슐화
- 워커 풀 기반 병렬 처리: 성능 최적화 및 확장성 확보
- 진행 상황 추적: 실시간 모니터링 및 재개 가능한 작업
- 에러 처리 및 복구: 견고한 예외 상황 대응

### 4.3 Phase 3: 인터페이스 계층 구현 ✅ **완료**
- [x] HTTP 컨트롤러 분리 (FileController, DuplicateController, ComparisonController, CleanupController)
- [x] 요청/응답 DTO 정의 (포괄적인 요청/응답 구조체 및 매퍼 함수)
- [x] 미들웨어 구현 (로깅, 에러 처리, CORS, 보안 헤더, 속도 제한)

**완료 내역 (2024년 8월)**:
- 4개 전문화된 컨트롤러: 관심사의 명확한 분리
- 포괄적인 DTO 시스템: 타입 안전한 API 계약
- 강력한 미들웨어 체인: 로깅, 보안, 에러 처리

### 4.4 Phase 4: 인프라스트럭처 계층 구현 ✅ **완료**
- [x] SQLite 리포지토리 구현 (FileRepository, DuplicateGroupRepository, ProgressRepository, ComparisonResultRepository)
- [x] Google Drive API 어댑터 구현 (인증, 파일 작업, 메타데이터, 검색)
- [x] 해시 계산 서비스 구현 (MD5, SHA1, SHA256 지원, 워커 풀, 스트리밍)

**완료 내역 (2024년 8월)**:
- 완전한 SQLite 영속성 계층: 트랜잭션, 배치 작업, 최적화된 인덱스
- 견고한 Google Drive 통합: 재시도 로직, 속도 제한, 에러 처리
- 효율적인 해시 서비스: 병렬 처리, 메모리 최적화, 알고리즘 선택

### 4.5 Phase 5: 의존성 주입 및 와이어링 ✅ **완료**
- [x] 의존성 주입 컨테이너 구현 (모든 종속성의 중앙 집중식 관리)
- [x] 설정 관리 시스템 구현 (JSON 기반, 환경 변수 오버라이드, 검증)
- [x] 애플리케이션 부트스트랩 구현 (그레이스풀 시작/종료, 헬스 체크)

**완료 내역 (2024년 8월)**:
- 완전한 의존성 주입: 느슨한 결합, 테스트 가능성
- 유연한 설정 시스템: 환경별 구성, 기본값, 검증
- 생산 준비 애플리케이션: 메인 엔트리 포인트, 미들웨어, 라우팅

### 4.6 Phase 6: 테스트 및 검증
- [ ] 단위 테스트 작성
- [ ] 통합 테스트 작성
- [ ] 성능 테스트 및 최적화

## 5. 기술적 고려사항

### 5.1 성능 고려사항
- 기존 성능 저하 없이 리팩토링
- 메모리 사용량 최적화
- 동시성 처리 개선

### 5.2 하위 호환성
- 기존 API 엔드포인트 유지
- 데이터베이스 스키마 마이그레이션 계획
- 사용자 인터페이스 변경 최소화

### 5.3 점진적 마이그레이션
- 기존 코드와 새 코드 공존 가능한 구조
- 단계적 교체 전략
- 롤백 계획

## 6. 품질 메트릭

### 6.1 코드 품질 지표
- 순환 복잡도 감소 (함수당 10 이하)
- 코드 중복도 감소 (5% 이하)
- 테스트 커버리지 (80% 이상)

### 6.2 설계 품질 지표
- 패키지 간 결합도 감소
- 응집도 향상
- 인터페이스 분리도 개선

### 6.3 유지보수성 지표
- 함수 길이 감소 (50줄 이하)
- 파일 크기 제한 (500줄 이하)
- 의존성 계층 준수

## 7. 일정 및 우선순위

### 7.1 1주차: 설계 및 기반 작업 ✅ **완료**
- ✅ 도메인 모델 정의
- ✅ 패키지 구조 설계  
- ✅ 인터페이스 정의

### 7.2 2주차: 핵심 유스케이스 구현 ✅ **완료**
- ✅ 파일 스캐닝 유스케이스
- ✅ 중복 검색 유스케이스
- ✅ 폴더 비교 유스케이스  
- ✅ 파일 정리 유스케이스

### 7.3 3주차: 인터페이스 및 인프라 구현 ✅ **완료**
- ✅ HTTP 컨트롤러 분리
- ✅ 데이터베이스 리포지토리 구현
- ✅ Google Drive API 어댑터 구현
- ✅ 해시 계산 서비스 구현

### 7.4 4주차: 통합 및 테스트 ✅ **완료 (구현 부분)**
- ✅ 의존성 주입 구현
- ✅ 설정 관리 및 애플리케이션 부트스트랩
- [ ] 테스트 작성 및 검증 (다음 단계)

## 8. 위험 요소 및 대응 방안

### 8.1 성능 저하 위험
- **위험**: 추상화 레이어 추가로 인한 성능 저하
- **대응**: 벤치마크 테스트 및 프로파일링

### 8.2 복잡성 증가 위험
- **위험**: 과도한 추상화로 인한 복잡성 증가
- **대응**: 점진적 리팩토링 및 단순성 우선

### 8.3 호환성 문제 위험
- **위험**: 기존 기능 동작 변경
- **대응**: 철저한 회귀 테스트

## 9. 성공 기준

### 9.1 기능적 성공 기준
- 모든 기존 기능 정상 동작
- 새로운 스토리지 제공자 추가 가능
- 새로운 해시 알고리즘 추가 가능

### 9.2 비기능적 성공 기준
- 코드 가독성 및 이해도 향상
- 테스트 작성 용이성 개선
- 기능 확장 시간 단축

### 9.3 팀 생산성 기준
- 새로운 기능 개발 속도 향상
- 버그 수정 시간 단축
- 코드 리뷰 효율성 개선