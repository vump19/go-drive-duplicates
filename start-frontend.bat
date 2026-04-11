@echo off
REM Google Drive 중복 파일 관리 - 프론트엔드 개발 서버 시작 스크립트 (Windows)

echo 🚀 Google Drive 중복 파일 관리 - 프론트엔드 서버 시작
echo.

REM Python 버전 확인
python --version >nul 2>&1
if %ERRORLEVEL% == 0 (
    echo ✅ Python 발견
    python dev-server.py
) else (
    echo ❌ Python을 찾을 수 없습니다
    echo Python 3.6 이상을 설치해주세요
    pause
    exit /b 1
)