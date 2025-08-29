#!/usr/bin/env python3
"""
Google Drive 중복 파일 관리 - 개발용 정적 파일 서버
포트 3000에서 프론트엔드를 서빙하고 포트 8080의 백엔드와 연동
"""

import http.server
import socketserver
import os
import sys
import webbrowser
import threading
import time
from urllib.parse import urljoin

class CustomHTTPRequestHandler(http.server.SimpleHTTPRequestHandler):
    """CORS 헤더를 추가하는 커스텀 핸들러"""
    
    def end_headers(self):
        # CORS 헤더 추가
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type, Authorization')
        
        # 캐시 제어 (개발 환경)
        self.send_header('Cache-Control', 'no-cache, no-store, must-revalidate')
        self.send_header('Pragma', 'no-cache')
        self.send_header('Expires', '0')
        
        super().end_headers()
    
    def do_OPTIONS(self):
        """OPTIONS 요청 처리 (CORS preflight)"""
        self.send_response(200)
        self.end_headers()
    
    def log_message(self, format, *args):
        """로그 메시지 포맷 개선"""
        timestamp = time.strftime('%H:%M:%S', time.localtime())
        print(f"[{timestamp}] {format % args}")

def check_backend_server():
    """백엔드 서버 연결 확인"""
    import urllib.request
    import urllib.error
    
    try:
        with urllib.request.urlopen('http://localhost:8080/health', timeout=5) as response:
            if response.status == 200:
                return True
    except (urllib.error.URLError, urllib.error.HTTPError, ConnectionRefusedError):
        pass
    return False

def start_server():
    """개발 서버 시작"""
    PORT = 3000
    DIRECTORY = "static"
    
    # static 디렉토리로 이동
    if os.path.exists(DIRECTORY):
        os.chdir(DIRECTORY)
    else:
        print(f"❌ '{DIRECTORY}' 디렉토리를 찾을 수 없습니다.")
        sys.exit(1)
    
    print("🚀 Google Drive 중복 파일 관리 - 개발 서버 시작")
    print("=" * 50)
    
    # 백엔드 서버 상태 확인
    if check_backend_server():
        print("✅ 백엔드 서버 (포트 8080) 연결 확인됨")
    else:
        print("⚠️  백엔드 서버 (포트 8080)에 연결할 수 없습니다")
        print("   Go 서버가 실행 중인지 확인해주세요:")
        print("   $ go run cmd/server/main.go")
        print()
    
    # HTTP 서버 시작
    try:
        with socketserver.TCPServer(("", PORT), CustomHTTPRequestHandler) as httpd:
            print(f"🌐 프론트엔드 서버: http://localhost:{PORT}")
            print(f"📁 정적 파일 디렉토리: {os.getcwd()}")
            print(f"🔗 백엔드 API 서버: http://localhost:8080")
            print()
            print("📋 사용 가능한 URL:")
            print(f"   • 메인 애플리케이션: http://localhost:{PORT}/index_port3000.html")
            print(f"   • 기존 UI (참고용): http://localhost:8080/static/index.html")
            print()
            print("⌨️  개발자 도구 단축키 (브라우저):")
            print("   • Ctrl/Cmd + Shift + D: 디버그 모드 토글")
            print("   • Ctrl/Cmd + Shift + R: 상태 리셋")
            print("   • Ctrl/Cmd + Shift + P: 포트 정보 표시")
            print()
            print("🛑 서버를 중지하려면 Ctrl+C를 누르세요")
            print("=" * 50)
            
            # 3초 후 브라우저 자동 열기
            def open_browser():
                time.sleep(3)
                webbrowser.open(f'http://localhost:{PORT}/index_port3000.html')
            
            threading.Thread(target=open_browser, daemon=True).start()
            
            # 서버 실행
            httpd.serve_forever()
            
    except KeyboardInterrupt:
        print("\n🛑 서버가 중지되었습니다")
    except OSError as e:
        if e.errno == 48:  # Address already in use
            print(f"❌ 포트 {PORT}가 이미 사용 중입니다")
            print("   다른 프로세스를 종료하거나 다른 포트를 사용해주세요")
        else:
            print(f"❌ 서버 시작 실패: {e}")
        sys.exit(1)

if __name__ == "__main__":
    start_server()