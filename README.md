# Google Drive 중복 파일 검사기

구글 드라이브의 모든 파일을 분석하여 중복되는 파일을 찾아주는 웹 서비스입니다.

## 주요 기능

- 📁 **Google Drive 전체 스캔**: 모든 파일을 자동으로 검색하고 분석
- 🔍 **정확한 중복 검사**: 파일 크기와 SHA-256 해시를 기반으로 한 정밀한 중복 검출
- 🌐 **직관적인 웹 인터페이스**: 실시간 진행 상황과 결과 표시
- 🔗 **Google Drive 연동**: 중복 파일에 대한 직접 링크 및 경로 정보 제공
- 📊 **페이지네이션**: 대용량 데이터를 효율적으로 처리 (페이지당 20개 그룹)
- 🗑️ **일괄 파일 삭제**: 정규식 패턴을 사용한 안전한 대량 파일 삭제
- ⚡ **재시작 기능**: 중단된 작업을 체크포인트부터 재개
- 🔒 **안전한 인증**: OAuth 2.0 기반 Google Drive 접근

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
go mod tidy

# 서버 실행
go run .
```

### 3. 웹 브라우저에서 접속

```
http://localhost:8080
```

## 사용 방법

### 기본 사용법
1. 웹 브라우저에서 `http://localhost:8080` 접속
2. "중복 파일 검사 시작" 버튼 클릭
3. 처음 실행 시 Google 계정 인증:
   - 터미널에 표시되는 URL을 브라우저에서 열기
   - Google 계정 로그인 후 권한 허용 (Drive 전체 접근)
   - 인증 코드를 터미널에 입력
4. 실시간으로 진행 상황 모니터링
5. 검사 완료 후 페이지네이션된 중복 파일 목록 확인
6. 파일명 클릭으로 Google Drive에서 직접 열기

### 고급 기능
- **작업 재개**: 중단된 검사를 "작업 재개" 버튼으로 계속 진행
- **경로 조회**: "경로 조회" 버튼으로 개별 파일의 폴더 위치 확인
- **일괄 삭제**: 특정 폴더의 파일들을 정규식 패턴으로 검색 및 삭제
- **데이터 초기화**: 모든 검사 결과 및 데이터베이스 초기화

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

- **Backend**: Go 1.21+ (Goroutines를 활용한 동시성 처리)
- **Database**: SQLite3 (로컬 파일 데이터베이스)
- **Frontend**: HTML5, CSS3, Vanilla JavaScript (ES6+)
- **API**: Google Drive API v3 (REST)
- **인증**: OAuth 2.0 (Google Cloud Platform)
- **웹 서버**: Go 표준 라이브러리 net/http
- **파일 해싱**: SHA-256 암호화 해시

## 릴리즈 정보

### v1.0 (현재 버전)
- ✅ Google Drive 전체 파일 스캔 및 중복 검사
- ✅ 실시간 진행 상황 모니터링 및 웹 인터페이스
- ✅ 파일 경로 표시 및 Google Drive 연동
- ✅ 페이지네이션을 통한 대용량 데이터 처리
- ✅ 정규식 기반 일괄 파일 삭제 기능
- ✅ 작업 재시작 및 체크포인트 시스템
- ✅ API 오류 복구 및 재시도 로직
- ✅ SQLite 데이터베이스 자동 마이그레이션

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

## 파일 구조

```
go-drive-duplicates/
├── main.go              # 웹 서버 및 HTTP 핸들러
├── drive.go            # Google Drive API 연동 및 비즈니스 로직
├── templates/
│   └── index.html      # HTML 웹 인터페이스
├── static/
│   ├── css/
│   │   └── style.css   # 스타일시트
│   └── js/
│       └── app.js      # JavaScript 클라이언트 로직
├── credentials.json    # Google OAuth 클라이언트 설정 (사용자 생성)
├── token.json         # OAuth 토큰 (자동 생성)
├── drive_duplicates.db # SQLite 데이터베이스 (자동 생성)
├── go.mod             # Go 모듈 정의
├── README.md          # 사용 설명서
└── Dev_History.md     # 개발 히스토리
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