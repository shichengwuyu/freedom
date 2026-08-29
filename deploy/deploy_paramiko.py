import paramiko
import os
import sys

# Server config
SERVER_IP = "43.248.3.138"
SERVER_PORT = 10233
SERVER_USER = "root"
SSH_KEY = r"C:\Users\liuxaio\.ssh\id_ed25519"
SSH_PASS = "hu0aDCULerjL"

# Local file
LOCAL_FILE = r"f:\trae\wifi\infinite-canvas-main\deploy\infinite-canvas-deploy.tar.gz"
REMOTE_FILE = f"/tmp/{os.path.basename(LOCAL_FILE)}"

# Project dir on server
PROJECT_DIR = "/opt/infinite-canvas"
DOMAIN = "xiaoyxiao.xyz"

def connect():
    transport = paramiko.Transport((SERVER_IP, SERVER_PORT))
    try:
        transport.connect(username=SERVER_USER, key_filename=SSH_KEY)
        print("[OK] Connected via SSH key")
    except Exception as e:
        print(f"[INFO] SSH key failed: {e}, trying password...")
        transport.connect(username=SERVER_USER, password=SSH_PASS)
        print("[OK] Connected via password")
    return transport

def upload(transport, local, remote):
    sftp = paramiko.SFTPClient.from_transport(transport)
    file_size = os.path.getsize(local)
    print(f"[UPLOAD] {local} ({file_size/1024/1024:.1f} MB) -> {remote}")
    
    uploaded = 0
    last_pct = 0
    
    def progress(transferred, total):
        nonlocal last_pct
        pct = int(transferred * 100 / total)
        if pct != last_pct and pct % 10 == 0:
            print(f"  {pct}% ({transferred/1024/1024:.1f}/{total/1024/1024:.1f} MB)")
            last_pct = pct
    
    sftp.put(local, remote, callback=progress)
    sftp.close()
    print(f"[OK] Upload complete")

def exec_cmd(transport, command, timeout=120):
    print(f"\n[EXEC] {command[:100]}{'...' if len(command) > 100 else ''}")
    channel = transport.open_session()
    channel.settimeout(timeout)
    channel.exec_command(command)
    
    output = []
    while True:
        try:
            data = channel.recv(4096).decode('utf-8')
            if not data:
                break
            output.append(data)
        except Exception:
            break
    
    # Read remaining stderr
    try:
        err_data = channel.recv_stderr(4096).decode('utf-8')
        if err_data:
            print(f"[STDERR] {err_data[:500]}")
    except Exception:
        pass
    
    result = ''.join(output)
    if result:
        print(result[:2000])
    
    exit_code = channel.recv_exit_status()
    print(f"[EXIT] code={exit_code}")
    channel.close()
    return exit_code == 0, result

def main():
    print("=" * 60)
    print("Freedom Platform Deployment")
    print("=" * 60)
    
    # Connect
    print("\n[STEP 1] Connecting to server...")
    transport = connect()
    
    # Upload
    print("\n[STEP 2] Uploading deploy package...")
    upload(transport, LOCAL_FILE, REMOTE_FILE)
    
    # Verify
    print("\n[STEP 3] Verifying upload...")
    ok, _ = exec_cmd(transport, f"ls -lh {REMOTE_FILE}")
    
    # Stop old services
    print("\n[STEP 4] Stopping old services...")
    exec_cmd(transport, "pm2 delete all 2>/dev/null || true")
    
    # Extract
    print("\n[STEP 5] Extracting package...")
    exec_cmd(transport, f"mkdir -p {PROJECT_DIR}")
    exec_cmd(transport, f"rm -rf {PROJECT_DIR}/web {PROJECT_DIR}/server {PROJECT_DIR}/data")
    exec_cmd(transport, f"tar -xzf {REMOTE_FILE} -C {PROJECT_DIR}")
    exec_cmd(transport, f"chmod +x {PROJECT_DIR}/server")
    
    # Start backend
    print("\n[STEP 6] Starting backend...")
    exec_cmd(transport, f"cd {PROJECT_DIR} && pm2 start ./server --name backend --cwd {PROJECT_DIR} --max-memory-restart 512M")
    
    # Start frontend
    print("\n[STEP 7] Starting frontend...")
    exec_cmd(transport, f"pm2 start node --name frontend --cwd {PROJECT_DIR}/web -- server.js")
    
    # Save PM2
    exec_cmd(transport, "pm2 save")
    
    # Wait and check
    print("\n[STEP 8] Waiting for services to start...")
    import time
    time.sleep(3)
    
    # Configure Nginx
    print("\n[STEP 9] Configuring Nginx...")
    nginx_conf = f"""server {{
    listen 80;
    server_name {DOMAIN} www.{DOMAIN};

    location /_next/static/ {{
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }}

    location /api/ {{
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 50m;
    }}

    location / {{
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        client_max_body_size 50m;
    }}
}}"""
    
    # Write nginx config
    sftp = paramiko.SFTPClient.from_transport(transport)
    with sftp.file('/etc/nginx/conf.d/infinite-canvas.conf', 'w') as f:
        f.write(nginx_conf)
    sftp.close()
    
    exec_cmd(transport, "rm -f /etc/nginx/conf.d/default.conf 2>/dev/null")
    exec_cmd(transport, "nginx -t")
    exec_cmd(transport, "systemctl restart nginx")
    exec_cmd(transport, "systemctl enable nginx")
    
    # Firewall
    exec_cmd(transport, "firewall-cmd --permanent --add-service=http 2>/dev/null || true")
    exec_cmd(transport, "firewall-cmd --permanent --add-service=https 2>/dev/null || true")
    exec_cmd(transport, "firewall-cmd --reload 2>/dev/null || true")
    
    # Check status
    print("\n[STEP 10] Checking service status...")
    exec_cmd(transport, "pm2 status")
    
    print("\n" + "=" * 60)
    print("  DEPLOYMENT COMPLETE!")
    print("=" * 60)
    print(f"  URL: http://{DOMAIN}")
    print(f"  Admin: admin / infinite-canvas")
    print(f"  Backend logs: pm2 logs backend")
    print(f"  Frontend logs: pm2 logs frontend")
    print("=" * 60)
    
    transport.close()

if __name__ == "__main__":
    main()