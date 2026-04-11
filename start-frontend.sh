#!/bin/bash

# Google Drive 중복 파일 관리 - 프론트엔드 개발 서버 시작 스크립트

echo "🚀 Google Drive 중복 파일 관리 - 프론트엔드 서버 시작"
echo ""

# Python 버전 확인
if command -v python3 &> /dev/null; then
    echo "✅ Python3 발견"
    python3 dev-server.py
elif command -v python &> /dev/null; then
    echo "✅ Python 발견"
    python dev-server.py
else
    echo "❌ Python을 찾을 수 없습니다"
    echo "Python 3.6 이상을 설치해주세요"
    exit 1
fi