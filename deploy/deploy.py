#!/usr/bin/env python3
"""
一键部署脚本 - 从 Windows 自动部署 infinite-canvas 到 CentOS 服务器
用法: python deploy/deploy.py
"""

import os
import sys
import subprocess
import time
import tarfile
import io

try:
    import paramiko
except ImportError:
    print("正在安装 paramiko...")
    subprocess.check_call([sys.executable, "-m", "pip", "install", "paramiko", "-q"])
    import paramiko

# ==================== 配置 ====================
SERVER_IP = "43.248.3.138"
SERVER_PORT = 10233
SERVER_USER = "root"
SERVER_PASSWORD = "hu0aDCULerjL"
DOMAIN = "xiaoyxiao.xyz"
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DEPLOY_DIR = os.path.join(PROJECT_ROOT, "deploy", "build")
REMOTE_PROJECT_DIR = "/opt/infinite-canvas"


def run_cmd(cmd, cwd=None, env=None, check=True):
    """运行本地命令"""
    print(f"  $ {cmd}")
    result = subprocess.run(
        cmd, shell=True, cwd=cwd, env=env,
        capture_output=True, text=True
    )
    if result.stdout:
        for line in result.stdout.strip().split("\n"):
            if line.strip():
                print(f"    {line}")
    if result.returncode != 0 and check:
        print(f"  [ERROR] 命令失败 (exit code {result.returncode})")
        if result.stderr:
            print(f"  {result.stderr.strip()}")
        sys.exit(1)
    return result


def ssh_exec(ssh, cmd, timeout=300):
    """在远程服务器执行命令"""
    print(f"  [SSH] {cmd[:80]}{'...' if len(cmd) > 80 else ''}")
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", errors="replace")
    err = stderr.read().decode("utf-8", errors="replace")
    if out.strip():
        for line in out.strip().split("\n"):
            print(f"    {line}")
    if err.strip():
        for line in err.strip().split("\n"):
            print(f"    [ERR] {line}")
    return stdout.channel.recv_exit_status(), out, err


def ssh_upload(ssh, local_path, remote_path):
    """上传文件到服务器"""
    sftp = ssh.open_sftp()
    print(f"  [上传] {local_path} -> {remote_path}")
    sftp.put(local_path, remote_path)
    sftp.close()


def main():
    print("=" * 50)
    print("  infinite-canvas 一键部署工具")
    print(f"  服务器: {SERVER_IP}:{SERVER_PORT}")
    print(f"  域名: {DOMAIN}")
    print("=" * 50)

    # ========== 步骤 1: 构建前端 ==========
    print("\n[1/6] 构建前端 Next.js...")
    web_dir = os.path.join(PROJECT_ROOT, "web")
    run_cmd("npm run build", cwd=web_dir)
    print("  OK 前端构建完成")

    # ========== 步骤 2: 交叉编译后端 ==========
    print("\n[2/6] 交叉编译后端 Go (linux/amd64)...")
    build_env = os.environ.copy()
    build_env["CGO_ENABLED"] = "0"
    build_env["GOOS"] = "linux"
    build_env["GOARCH"] = "amd64"

    # 清理并创建部署目录
    if os.path.exists(DEPLOY_DIR):
        import shutil
        shutil.rmtree(DEPLOY_DIR)
    os.makedirs(DEPLOY_DIR, exist_ok=True)

    server_bin = os.path.join(DEPLOY_DIR, "server")
    run_cmd(f"go build -o {server_bin} main.go", cwd=PROJECT_ROOT, env=build_env)
    print("  OK 后端编译完成")

    # ========== 步骤 3: 组装部署文件 ==========
    print("\n[3/6] 组装部署文件...")

    # 复制前端 standalone
    import shutil
    web_deploy = os.path.join(DEPLOY_DIR, "web")
    os.makedirs(web_deploy, exist_ok=True)

    standalone = os.path.join(PROJECT_ROOT, "web", ".next", "standalone")
    if os.path.exists(standalone):
        shutil.copytree(standalone, web_deploy, dirs_exist_ok=True)
        print("  OK standalone 已复制")

    static_src = os.path.join(PROJECT_ROOT, "web", ".next", "static")
    static_dst = os.path.join(DEPLOY_DIR, "web", ".next", "static")
    if os.path.exists(static_src):
        os.makedirs(static_dst, exist_ok=True)
        shutil.copytree(static_src, static_dst, dirs_exist_ok=True)
        print("  OK static 已复制")

    public_src = os.path.join(PROJECT_ROOT, "web", "public")
    if os.path.exists(public_src):
        shutil.copytree(public_src, os.path.join(DEPLOY_DIR, "web", "public"), dirs_exist_ok=True)
        print("  OK public 已复制")

    # 复制数据目录
    data_src = os.path.join(PROJECT_ROOT, "data")
    data_dst = os.path.join(DEPLOY_DIR, "data")
    os.makedirs(data_dst, exist_ok=True)
    if os.path.exists(data_src):
        shutil.copytree(data_src, data_dst, dirs_exist_ok=True)
        print("  OK 数据目录已复制")

    # 生成 .env
    env_content = f"""ADMIN_USERNAME=admin
ADMIN_PASSWORD=infinite-canvas
JWT_SECRET=infinite-canvas-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=https://{DOMAIN}
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/infinite-canvas.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
"""
    with open(os.path.join(DEPLOY_DIR, ".env"), "w", encoding="utf-8") as f:
        f.write(env_content)
    print("  OK .env 已生成")

    # ========== 步骤 4: 打包 ==========
    print("\n[4/6] 打包压缩...")
    archive_path = os.path.join(PROJECT_ROOT, "deploy", "infinite-canvas-deploy.tar.gz")
    with tarfile.open(archive_path, "w:gz") as tar:
        tar.add(DEPLOY_DIR, arcname=".")
    size_mb = os.path.getsize(archive_path) / (1024 * 1024)
    print(f"  OK 打包完成 ({size_mb:.1f} MB)")

    # ========== 步骤 5: SSH 连接并上传 ==========
    print(f"\n[5/6] 连接服务器 {SERVER_IP}:{SERVER_PORT}...")
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    try:
        ssh.connect(
            hostname=SERVER_IP,
            port=SERVER_PORT,
            username=SERVER_USER,
            password=SERVER_PASSWORD,
            timeout=30
        )
        print("  OK SSH 连接成功")
    except Exception as e:
        print(f"  [ERROR] SSH 连接失败: {e}")
        sys.exit(1)

    # 上传压缩包
    remote_archive = f"/tmp/infinite-canvas-deploy.tar.gz"
    ssh_upload(ssh, archive_path, remote_archive)
    print("  OK 上传完成")

    # ========== 步骤 6: 远程部署 ==========
    print("\n[6/6] 远程部署...")

    # 安装依赖
    print("\n  --- 安装系统依赖 ---")
    ssh_exec(ssh, "yum install -y epel-release 2>/dev/null; yum install -y curl wget git nginx tar 2>/dev/null", timeout=120)

    # 安装 Node.js 18
    print("\n  --- 安装 Node.js 18 ---")
    ssh_exec(ssh, """
if ! command -v node &>/dev/null || [[ $(node --version 2>/dev/null | cut -d'v' -f2 | cut -d'.' -f1) -lt 18 ]]; then
    curl -fsSL https://rpm.nodesource.com/setup_18.x | bash - 2>/dev/null
    yum install -y nodejs 2>/dev/null
fi
node --version
""", timeout=120)

    # 安装 PM2
    print("\n  --- 安装 PM2 ---")
    ssh_exec(ssh, "npm install -g pm2 2>/dev/null; pm2 --version", timeout=60)

    # 解压部署包
    print("\n  --- 解压部署包 ---")
    ssh_exec(ssh, f"""
mkdir -p {REMOTE_PROJECT_DIR}
rm -rf {REMOTE_PROJECT_DIR}/web {REMOTE_PROJECT_DIR}/server {REMOTE_PROJECT_DIR}/data {REMOTE_PROJECT_DIR}/.env
tar -xzf {remote_archive} -C {REMOTE_PROJECT_DIR}
chmod +x {REMOTE_PROJECT_DIR}/server
ls -la {REMOTE_PROJECT_DIR}/
""", timeout=30)

    # 停止旧服务
    print("\n  --- 停止旧服务 ---")
    ssh_exec(ssh, "pm2 delete all 2>/dev/null; echo done")

    # 启动后端
    print("\n  --- 启动后端 ---")
    ssh_exec(ssh, f"cd {REMOTE_PROJECT_DIR} && pm2 start ./server --name backend --cwd {REMOTE_PROJECT_DIR} --max-memory-restart 512M", timeout=30)
    time.sleep(3)

    # 启动前端
    print("\n  --- 启动前端 ---")
    ssh_exec(ssh, f"pm2 start node --name frontend --cwd {REMOTE_PROJECT_DIR}/web -- server.js", timeout=30)
    time.sleep(3)

    # 查看 PM2 状态
    print("\n  --- PM2 状态 ---")
    ssh_exec(ssh, "pm2 status")

    # 保存 PM2 配置（开机自启）
    print("\n  --- 配置开机自启 ---")
    ssh_exec(ssh, "pm2 save; pm2 startup systemd -u root --hp /root 2>/dev/null; echo done")

    # 配置 Nginx
    print("\n  --- 配置 Nginx ---")
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

    # 写入 Nginx 配置
    ssh_exec(ssh, f"cat > /etc/nginx/conf.d/infinite-canvas.conf << 'NGINXEOF'\n{nginx_conf}\nNGINXEOF")
    ssh_exec(ssh, "rm -f /etc/nginx/conf.d/default.conf 2>/dev/null; nginx -t && systemctl restart nginx && systemctl enable nginx", timeout=30)

    # 开放防火墙
    print("\n  --- 开放防火墙 ---")
    ssh_exec(ssh, "firewall-cmd --permanent --add-service=http 2>/dev/null; firewall-cmd --permanent --add-service=https 2>/dev/null; firewall-cmd --reload 2>/dev/null; echo done")

    # 验证服务
    print("\n  --- 验证服务 ---")
    ssh_exec(ssh, "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/ || echo 'frontend not ready'")
    ssh_exec(ssh, "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/settings/public || echo 'backend not ready'")

    ssh.close()

    # ========== 完成 ==========
    print("\n" + "=" * 50)
    print("  部署完成！")
    print("=" * 50)
    print(f"\n  访问地址: http://{DOMAIN}")
    print(f"  管理员账号: admin / infinite-canvas")
    print(f"\n  后续操作：")
    print(f"  1. 在阿里云域名控制台添加 A 记录: {DOMAIN} -> {SERVER_IP}")
    print(f"  2. 建议配置 HTTPS (certbot)")
    print(f"  3. 建议修改 JWT_SECRET 为随机字符串")
    print(f"  4. 定期备份 {REMOTE_PROJECT_DIR}/data/infinite-canvas.db")
    print()


if __name__ == "__main__":
    main()
