const { Client } = require("ssh2");
const fs = require("fs");
const { execSync } = require("child_process");
const path = require("path");

const SERVER = { host: "149.88.78.8", port: 22, username: "root", password: "BpgAZt1TrQVc" };
const REMOTE_DIR = "/opt/freedom";
const PROJECT_ROOT = path.resolve(__dirname, "..");
const BUILD_DIR = path.join(PROJECT_ROOT, "deploy/build");
const ARCHIVE_PATH = path.join(PROJECT_ROOT, "deploy", "freedom-deploy-hk.tar.gz");

function step(msg) { console.log(`\n>>> ${msg}`); }
function ok(msg) { console.log(`  OK ${msg}`); }
function err(msg) { console.log(`  ERR ${msg}`); }

function sshExec(conn, cmd, label) {
  return new Promise((resolve, reject) => {
    if (label) console.log(`  [SSH] ${label}`);
    conn.exec(cmd, (err, stream) => {
      if (err) return reject(err);
      let out = "", errOut = "";
      stream.on("data", d => { process.stdout.write(d); out += d.toString(); });
      stream.stderr.on("data", d => { errOut += d.toString(); });
      stream.on("close", code => {
        if (code !== 0 && code !== null) {
          reject(new Error(`Exit code ${code}: ${errOut.trim()}`));
        } else {
          resolve(out);
        }
      });
      stream.on("error", reject);
    });
  });
}

async function main() {
  console.log("==================================================");
  console.log("  freedom 部署到香港服务器");
  console.log(`  服务器: ${SERVER.host}:${SERVER.port}`);
  console.log(`  目录: ${REMOTE_DIR}`);
  console.log(`  时间: ${new Date().toISOString()}`);
  console.log("==================================================");

  // ===== 1. 检查构建产物 =====
  step("1/5 检查构建产物");

  const serverBin = path.join(BUILD_DIR, "server");
  if (!fs.existsSync(serverBin)) {
    err("后端二进制不存在: " + serverBin);
    console.log("  请先运行: CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY=https://goproxy.cn,direct go build -o deploy/build/server main.go");
    process.exit(1);
  }
  ok(`后端二进制 ${fs.statSync(serverBin).size / 1024 / 1024}MB`);

  const standaloneWeb = path.join(BUILD_DIR, "web", ".next", "standalone");
  if (!fs.existsSync(standaloneWeb)) {
    err("前端standalone构建不存在: " + standaloneWeb);
    console.log("  请先运行: cd web && npm run build");
    process.exit(1);
  }
  ok("前端standalone构建存在");

  const staticDir = path.join(PROJECT_ROOT, "web", ".next", "static");
  if (fs.existsSync(staticDir)) ok("前端static构建存在");
  else err("前端static构建缺失");

  // ===== 2. 准备部署目录 =====
  step("2/5 准备部署目录");

  // 确保standalone目录结构完整
  const standaloneRoot = path.join(standaloneWeb, "web");
  if (!fs.existsSync(path.join(standaloneRoot, "server.js"))) {
    err("standalone中缺少server.js，构建可能不完整");
    process.exit(1);
  }
  ok("standalone结构完整");

  // 复制static到standalone（如果standalone没有）
  if (fs.existsSync(staticDir)) {
    const standaloneStatic = path.join(standaloneRoot, ".next", "static");
    if (!fs.existsSync(standaloneStatic)) {
      const standaloneNext = path.dirname(standaloneStatic);
      fs.mkdirSync(standaloneNext, { recursive: true });
      const { execSync: exec } = require("child_process");
      try {
        exec(`xcopy /E /I /Y "${staticDir}/*" "${standaloneStatic}/"`, { windowsVerbatimArguments: true });
        ok("static已复制到standalone");
      } catch(e) {
        console.log(`  复制static失败: ${e.message}`);
      }
    }
  }

  // 复制public
  const publicSrc = path.join(PROJECT_ROOT, "web", "public");
  const publicDst = path.join(standaloneRoot, "public");
  if (fs.existsSync(publicSrc)) {
    fs.mkdirSync(publicDst, { recursive: true });
    if (fs.readdirSync(publicSrc).length > 0) {
      const { execSync: exec } = require("child_process");
      try {
        exec(`xcopy /E /I /Y "${publicSrc}/*" "${publicDst}/"`, { windowsVerbatimArguments: true });
        ok("public已复制");
      } catch(e) {}
    }
  }

  // 复制data
  const dataSrc = path.join(PROJECT_ROOT, "data");
  const dataDst = path.join(BUILD_DIR, "data");
  fs.mkdirSync(dataDst, { recursive: true });
  if (fs.existsSync(dataSrc) && fs.readdirSync(dataSrc).length > 0) {
    if (fs.readdirSync(dataDst).length === 0) {
      const { execSync: exec } = require("child_process");
      try {
        exec(`xcopy /E /I /Y "${dataSrc}/*" "${dataDst}/"`, { windowsVerbatimArguments: true });
        ok("data已复制");
      } catch(e) {}
    }
  }

  // 生成.env
  const envContent = `ADMIN_USERNAME=admin
ADMIN_PASSWORD=freedom
JWT_SECRET=freedom-change-me-in-production
JWT_EXPIRE_HOURS=168
PORT=8080
PUBLIC_BASE_URL=http://149.88.78.8
API_BASE_URL=http://127.0.0.1:8080
STORAGE_DRIVER=mysql
DATABASE_DSN=data/freedom.db
LICENSE_PURCHASE_URL=https://pay.ldxp.cn/shop/35TCHF9A
`;
  fs.writeFileSync(path.join(BUILD_DIR, ".env"), envContent);
  ok(".env已生成");

  // ===== 3. 打包 =====
  step("3/5 打包压缩");
  try {
    execSync(`tar -czf "${ARCHIVE_PATH}" .`, { cwd: BUILD_DIR, shell: process.env.ComSpec || "cmd.exe" });
    const sizeMB = (fs.statSync(ARCHIVE_PATH).size / 1024 / 1024).toFixed(1);
    ok(`打包完成 (${sizeMB} MB)`);
  } catch(e) {
    err("打包失败: " + e.message);
    process.exit(1);
  }

  // ===== 4. 上传到服务器 =====
  step(`4/5 上传到 ${SERVER.host}`);

  const conn = await new Promise((resolve, reject) => {
    const c = new Client();
    c.on("ready", () => { console.log("  OK SSH连接成功"); resolve(c); });
    c.on("error", err => { console.log(`  [ERROR] SSH连接失败: ${err.message}`); reject(err); });
    c.connect(SERVER);
  });

  // 上传
  console.log("  [上传] freedom-deploy-hk.tar.gz -> /tmp/");
  await new Promise((resolve, reject) => {
    conn.sftp((err, sftp) => {
      if (err) return reject(err);
      const readStream = fs.createReadStream(ARCHIVE_PATH);
      const writeStream = sftp.createWriteStream("/tmp/freedom-deploy-hk.tar.gz");
      writeStream.on("close", () => { sftp.end(); resolve(); });
      writeStream.on("error", reject);
      readStream.pipe(writeStream);
    });
  });
  ok("上传完成");

  // ===== 5. 远程部署 =====
  step("5/5 远程部署");

  // 安装依赖
  console.log("\n  --- 检查Node.js ---");
  await sshExec(conn, "node --version 2>/dev/null || echo 'node not found'");

  console.log("\n  --- 安装PM2 ---");
  await sshExec(conn, "npm install -g pm2 2>/dev/null; pm2 --version 2>/dev/null || echo 'pm2 install failed'");

  console.log("\n  --- 解压部署包 ---");
  await sshExec(conn, `mkdir -p ${REMOTE_DIR}; rm -rf ${REMOTE_DIR}/web ${REMOTE_DIR}/server ${REMOTE_DIR}/data ${REMOTE_DIR}/.env; tar -xzf /tmp/freedom-deploy-hk.tar.gz -C ${REMOTE_DIR}; chmod +x ${REMOTE_DIR}/server; ls -la ${REMOTE_DIR}/`);

  console.log("\n  --- 停止旧服务 ---");
  await sshExec(conn, "pm2 delete all 2>/dev/null; echo 'old services stopped'");

  console.log("\n  --- 启动后端 ---");
  await sshExec(conn, `pm2 start ${REMOTE_DIR}/server --name backend --cwd ${REMOTE_DIR} --max-memory-restart 512M`);

  console.log("\n  --- 启动前端 ---");
  await sshExec(conn, `pm2 start node --name frontend --cwd ${REMOTE_DIR}/web -- server.js`);

  console.log("\n  --- 等待服务启动 ---");
  await new Promise(r => setTimeout(r, 5000));

  console.log("\n  --- PM2 状态 ---");
  const status = await sshExec(conn, "pm2 status", "pm2 status");
  console.log(status);

  console.log("\n  --- 保存PM2配置 ---");
  await sshExec(conn, "pm2 save 2>/dev/null; pm2 startup systemd -u root --hp /root 2>/dev/null; echo done", "pm2 save");

  // 测试服务
  console.log("\n  --- 验证服务 ---");
  await sshExec(conn, "sleep 3 && curl -s -o /dev/null -w 'Frontend: %{http_code}\\n' http://127.0.0.1:3000/ 2>/dev/null || echo 'Frontend not ready'");
  await sshExec(conn, "curl -s -o /dev/null -w 'Backend: %{http_code}\\n' http://127.0.0.1:8080/api/health 2>/dev/null || echo 'Backend not ready'");

  conn.end();

  console.log("\n==================================================");
  console.log("  部署完成！");
  console.log("==================================================");
  console.log(`\n  访问地址: http://${SERVER.host}`);
  console.log(`  管理员账号: admin / freedom`);
  console.log("\n  常用命令:");
  console.log(`  ssh -p ${SERVER.port} ${SERVER.username}@${SERVER.host} 'pm2 status'`);
  console.log(`  ssh -p ${SERVER.port} ${SERVER.username}@${SERVER.host} 'pm2 logs'`);
  console.log(`  ssh -p ${SERVER.port} ${SERVER.username}@${SERVER.host} 'pm2 restart all'`);
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
