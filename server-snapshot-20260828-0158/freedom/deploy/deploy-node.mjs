import { Client } from "ssh2";
import { execSync } from "child_process";
import {
  existsSync, mkdirSync, rmSync, copyFileSync, statSync,
  writeFileSync, readdirSync, readFileSync
} from "fs";
import { join, dirname, basename } from "path";
import { createReadStream } from "fs";

import { fileURLToPath } from "url";
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const PROJECT_ROOT = join(__dirname, "..");

// ==================== 配置 ====================
const SERVER = { host: "43.248.3.138", port: 10233, username: "root", password: "hu0aDCULerjL" };
const DOMAIN = "xiaoyxiao.xyz";
const REMOTE_DIR = "/opt/freedom";

function step(msg) { console.log(`\n>>> ${msg}`); }
function ok(msg) { console.log(`  OK ${msg}`); }

function run(cmd, opts = {}) {
  console.log(`  $ ${cmd}`);
  const env = { ...process.env, ...opts.env };
  const r = execSync(cmd, {
    cwd: opts.cwd || PROJECT_ROOT,
    env,
    stdio: "pipe",
    shell: process.env.ComSpec || "cmd.exe"
  });
  const out = r.toString().trim();
  if (out) out.split("\n").forEach(l => console.log(`    ${l}`));
  return out;
}

function sshExec(conn, cmd, timeout = 300000) {
  return new Promise((resolve, reject) => {
    console.log(`  [SSH] ${cmd.substring(0, 100)}${cmd.length > 100 ? "..." : ""}`);
    conn.exec(cmd, { timeout }, (err, stream) => {
      if (err) return reject(err);
      let out = "", errOut = "";
      stream.on("data", d => { process.stdout.write(d); out += d.toString(); });
      stream.stderr.on("data", d => { errOut += d.toString(); });
      stream.on("close", (code) => resolve({ code, out, err: errOut }));
      stream.on("error", reject);
    });
  });
}

function sshUpload(conn, localPath, remotePath) {
  return new Promise((resolve, reject) => {
    console.log(`  [上传] ${basename(localPath)} -> ${remotePath}`);
    conn.sftp((err, sftp) => {
      if (err) return reject(err);
      const readStream = createReadStream(localPath);
      const writeStream = sftp.createWriteStream(remotePath);
      writeStream.on("close", () => { sftp.end(); resolve(); });
      writeStream.on("error", reject);
      readStream.pipe(writeStream);
    });
  });
}

// 递归复制目录
function copyDirRecursive(src, dst) {
  mkdirSync(dst, { recursive: true });
  const entries = readdirSync(src, { withFileTypes: true });
  for (const entry of entries) {
    const srcPath = join(src, entry.name);
    const dstPath = join(dst, entry.name);
    if (entry.isDirectory()) {
      copyDirRecursive(srcPath, dstPath);
    } else {
      copyFileSync(srcPath, dstPath);
    }
  }
}

async function main() {
  console.log("==================================================");
  console.log("  freedom 一键部署工具");
  console.log(`  服务器: ${SERVER.host}:${SERVER.port}`);
  console.log(`  域名: ${DOMAIN}`);
  console.log("==================================================");

  // ========== 1. 构建前端 ==========
  step("1/6 构建前端 Next.js...");
  run("npm run build", { cwd: join(PROJECT_ROOT, "web") });
  ok("前端构建完成");

  // ========== 2. 交叉编译后端 ==========
  step("2/6 交叉编译后端 Go (linux/amd64)...");
  const buildDir = join(PROJECT_ROOT, "deploy", "build");
  if (existsSync(buildDir)) rmSync(buildDir, { recursive: true, force: true });
  mkdirSync(buildDir, { recursive: true });

  const serverBin = join(buildDir, "server");
  run("go build -o server main.go", {
    cwd: PROJECT_ROOT,
    env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: "amd64" }
  });
  // go build 输出到 PROJECT_ROOT/server，复制到 buildDir
  const builtBin = join(PROJECT_ROOT, "server");
  if (existsSync(builtBin)) {
    copyFileSync(builtBin, serverBin);
    rmSync(builtBin, { force: true });
  }
  ok("后端编译完成");

  // ========== 3. 组装部署文件 ==========
  step("3/6 组装部署文件...");

  const webDeploy = join(buildDir, "web");
  mkdirSync(webDeploy, { recursive: true });

  // 前端 standalone
  const standalone = join(PROJECT_ROOT, "web", ".next", "standalone");
  if (existsSync(standalone)) {
    copyDirRecursive(standalone, webDeploy);
    ok("standalone 已复制");
  }

  // static
  const staticSrc = join(PROJECT_ROOT, "web", ".next", "static");
  const staticDst = join(buildDir, "web", ".next", "static");
  if (existsSync(staticSrc)) {
    copyDirRecursive(staticSrc, staticDst);
    ok("static 已复制");
  }

  // public
  const publicSrc = join(PROJECT_ROOT, "web", "public");
  if (existsSync(publicSrc)) {
    copyDirRecursive(publicSrc, join(buildDir, "web", "public"));
    ok("public 已复制");
  }

  // 数据目录
  const dataSrc = join(PROJECT_ROOT, "data");
  const dataDst = join(buildDir, "data");
  mkdirSync(dataDst, { recursive: true });
  if (existsSync(dataSrc)) {
    copyDirRecursive(dataSrc, dataDst);
    ok("数据目录已复制");
  }

  // .env
  const envContent = `ADMIN_USERNAME=admin
ADMIN_PASSWORD=freedom
JWT_SECRET=freedom-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=https://${DOMAIN}
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/freedom.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
`;
  writeFileSync(join(buildDir, ".env"), envContent);
  ok(".env 已生成");

  // ========== 4. 打包 ==========
  step("4/6 打包压缩...");
  const archivePath = join(PROJECT_ROOT, "deploy", "freedom-deploy.tar.gz");
  // 使用系统 tar 命令打包
  run(`tar -czf "${archivePath}" .`, { cwd: buildDir });
  const sizeMB = (statSync(archivePath).size / (1024 * 1024)).toFixed(1);
  ok(`打包完成 (${sizeMB} MB)`);

  // ========== 5. SSH 连接并上传 ==========
  step(`5/6 连接服务器 ${SERVER.host}:${SERVER.port}...`);

  const conn = await new Promise((resolve, reject) => {
    const c = new Client();
    c.on("ready", () => { console.log("  OK SSH 连接成功"); resolve(c); });
    c.on("error", (err) => { console.log(`  [ERROR] SSH 连接失败: ${err.message}`); reject(err); });
    c.connect(SERVER);
  });

  await sshUpload(conn, archivePath, "/tmp/freedom-deploy.tar.gz");
  ok("上传完成");

  // ========== 6. 远程部署 ==========
  step("6/6 远程部署...");

  // 安装依赖
  console.log("\n  --- 安装系统依赖 ---");
  await sshExec(conn, "yum install -y epel-release 2>/dev/null; yum install -y curl wget git nginx tar 2>/dev/null");

  // 安装 Node.js 18
  console.log("\n  --- 安装 Node.js 18 ---");
  await sshExec(conn, `if ! command -v node &>/dev/null || [[ \\$(node --version 2>/dev/null | cut -d'v' -f2 | cut -d'.' -f1) -lt 18 ]]; then curl -fsSL https://rpm.nodesource.com/setup_18.x | bash - 2>/dev/null; yum install -y nodejs 2>/dev/null; fi; node --version 2>/dev/null || echo 'node not found'`);

  // 安装 PM2
  console.log("\n  --- 安装 PM2 ---");
  await sshExec(conn, "npm install -g pm2 2>/dev/null; pm2 --version 2>/dev/null || echo 'pm2 not found'");

  // 解压
  console.log("\n  --- 解压部署包 ---");
  await sshExec(conn, `mkdir -p ${REMOTE_DIR}; rm -rf ${REMOTE_DIR}/web ${REMOTE_DIR}/server ${REMOTE_DIR}/data ${REMOTE_DIR}/.env; tar -xzf /tmp/freedom-deploy.tar.gz -C ${REMOTE_DIR}; chmod +x ${REMOTE_DIR}/server; ls -la ${REMOTE_DIR}/`);

  // 停止旧服务
  console.log("\n  --- 停止旧服务 ---");
  await sshExec(conn, "pm2 delete all 2>/dev/null; echo done");

  // 启动后端
  console.log("\n  --- 启动后端 ---");
  await sshExec(conn, `cd ${REMOTE_DIR} && pm2 start ./server --name backend --cwd ${REMOTE_DIR} --max-memory-restart 512M`);

  // 启动前端
  console.log("\n  --- 启动前端 ---");
  await sshExec(conn, `pm2 start node --name frontend --cwd ${REMOTE_DIR}/web -- server.js`);

  // 等待服务启动
  console.log("\n  --- 等待服务启动 ---");
  await new Promise(r => setTimeout(r, 5000));

  // 查看状态
  console.log("\n  --- PM2 状态 ---");
  await sshExec(conn, "pm2 status");

  // 保存 PM2
  console.log("\n  --- 配置开机自启 ---");
  await sshExec(conn, "pm2 save 2>/dev/null; pm2 startup systemd -u root --hp /root 2>/dev/null; echo done");

  // Nginx 配置
  console.log("\n  --- 配置 Nginx ---");
  const nginxConf = `server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};

    location /_next/static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 30d;
        add_header Cache-Control "public, immutable";
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 50m;
    }

    location / {
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
    }
}`;

  await sshExec(conn, `cat > /etc/nginx/conf.d/freedom.conf << 'NGINXEOF'\n${nginxConf}\nNGINXEOF`);
  await sshExec(conn, "rm -f /etc/nginx/conf.d/default.conf 2>/dev/null; nginx -t 2>&1 && systemctl restart nginx && systemctl enable nginx");

  // 防火墙
  console.log("\n  --- 开放防火墙 ---");
  await sshExec(conn, "firewall-cmd --permanent --add-service=http 2>/dev/null; firewall-cmd --permanent --add-service=https 2>/dev/null; firewall-cmd --reload 2>/dev/null; echo done");

  // 验证
  console.log("\n  --- 验证服务 ---");
  await sshExec(conn, "curl -s -o /dev/null -w 'Frontend HTTP: %{http_code}\\n' http://127.0.0.1:3000/ 2>/dev/null || echo 'Frontend not ready'");
  await sshExec(conn, "curl -s -o /dev/null -w 'Backend HTTP: %{http_code}\\n' http://127.0.0.1:8080/api/settings/public 2>/dev/null || echo 'Backend not ready'");

  conn.end();

  // ========== 完成 ==========
  console.log("\n==================================================");
  console.log("  部署完成！");
  console.log("==================================================");
  console.log(`\n  访问地址: http://${DOMAIN}`);
  console.log(`  管理员账号: admin / freedom`);
  console.log(`\n  后续操作：`);
  console.log(`  1. 在阿里云域名控制台添加 A 记录: ${DOMAIN} -> ${SERVER.host}`);
  console.log(`  2. 建议配置 HTTPS (certbot)`);
  console.log(`  3. 建议修改 JWT_SECRET 为随机字符串`);
  console.log(`  4. 定期备份 ${REMOTE_DIR}/data/freedom.db`);
  console.log();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
