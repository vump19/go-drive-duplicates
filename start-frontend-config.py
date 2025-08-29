#!/usr/bin/env python3
"""
프론트엔드 개발 서버 시작 스크립트
설정 파일(frontend.yaml)에서 포트를 읽어서 서버를 시작합니다.
"""

import http.server
import socketserver
import os
import sys
import yaml
import json
from pathlib import Path

def load_config():
    """YAML 설정 파일 로드"""
    config_path = Path("static/config/frontend.yaml")
    
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
        print(f"✅ 설정 파일 로드 완료: {config_path}")
        return config
    except FileNotFoundError:
        print(f"⚠️  설정 파일을 찾을 수 없습니다: {config_path}")
        print("기본 설정을 사용합니다.")
        return {
            'frontend': {
                'port': 3000,
                'host': 'localhost'
            },
            'backend': {
                'port': 8080,
                'host': 'localhost'
            }
        }
    except yaml.YAMLError as e:
        print(f"❌ YAML 파싱 오류: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"❌ 설정 파일 로드 오류: {e}")
        sys.exit(1)

def check_backend_connection(config):
    """백엔드 서버 연결 확인"""
    import socket
    
    backend_host = config['backend']['host']
    backend_port = config['backend']['port']
    
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.settimeout(2)
            result = s.connect_ex((backend_host, backend_port))
            if result == 0:
                print(f"✅ 백엔드 서버 연결 확인: {backend_host}:{backend_port}")
                return True
            else:
                print(f"⚠️  백엔드 서버에 연결할 수 없습니다: {backend_host}:{backend_port}")
                return False
    except Exception as e:
        print(f"⚠️  백엔드 연결 확인 실패: {e}")
        return False

class CustomHTTPRequestHandler(http.server.SimpleHTTPRequestHandler):
    """커스텀 HTTP 요청 핸들러"""
    
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory="static", **kwargs)
    
    def end_headers(self):
        # CORS 헤더 추가
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type, Authorization')
        self.send_header('Cache-Control', 'no-cache, no-store, must-revalidate')
        super().end_headers()
    
    def do_OPTIONS(self):
        """OPTIONS 요청 처리 (CORS preflight)"""
        self.send_response(200)
        self.end_headers()
    
    def log_message(self, format, *args):
        """로그 메시지 커스터마이징"""
        print(f"🌐 [{self.address_string()}] {format % args}")

def main():
    """메인 함수"""
    print("=" * 50)
    print("🚀 Google Drive Duplicates - Frontend Server")
    print("=" * 50)
    
    # 설정 로드
    config = load_config()
    
    # 포트 설정
    port = config['frontend']['port']
    host = config['frontend'].get('host', 'localhost')
    
    print(f"📋 프론트엔드 설정:")
    print(f"   - 호스트: {host}")
    print(f"   - 포트: {port}")
    print(f"   - 정적 파일 디렉터리: ./static")
    
    print(f"📋 백엔드 설정:")
    print(f"   - 호스트: {config['backend']['host']}")
    print(f"   - 포트: {config['backend']['port']}")
    
    # 백엔드 연결 확인
    backend_running = check_backend_connection(config)
    
    if not backend_running:
        print("\n⚠️  백엔드 서버가 실행되지 않은 것 같습니다.")
        print("백엔드를 먼저 시작하세요:")
        print(f"   go run cmd/server/main.go")
        print("\n계속하시겠습니까? (y/N): ", end="")
        
        response = input().lower().strip()
        if response != 'y' and response != 'yes':
            print("❌ 서버 시작을 취소합니다.")
            sys.exit(1)
    
    # 정적 파일 디렉터리 확인
    if not os.path.exists("static"):
        print("❌ 'static' 디렉터리를 찾을 수 없습니다.")
        print("올바른 프로젝트 루트 디렉터리에서 실행해주세요.")
        sys.exit(1)
    
    # 서버 시작
    try:
        with socketserver.TCPServer((host, port), CustomHTTPRequestHandler) as httpd:
            print(f"\n🎉 프론트엔드 서버가 시작되었습니다!")
            print(f"   URL: http://{host}:{port}")
            print(f"   백엔드 API: {config['backend']['protocol']}://{config['backend']['host']}:{config['backend']['port']}")
            print("\n✋ 서버를 중지하려면 Ctrl+C를 누르세요.\n")
            
            httpd.serve_forever()
            
    except KeyboardInterrupt:
        print("\n\n🛑 서버를 중지합니다...")
    except PermissionError:
        print(f"❌ 포트 {port}에 대한 권한이 없습니다.")
        print("다른 포트를 사용하거나 관리자 권한으로 실행해주세요.")
    except OSError as e:
        if "Address already in use" in str(e):
            print(f"❌ 포트 {port}가 이미 사용 중입니다.")
            print("다른 포트를 사용하거나 해당 포트를 사용하는 프로세스를 중지해주세요.")
        else:
            print(f"❌ 서버 시작 오류: {e}")
    except Exception as e:
        print(f"❌ 예상치 못한 오류: {e}")

if __name__ == "__main__":
    main()