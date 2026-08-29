import { Client } from "ssh2";

const SERVER = { host: "43.248.3.138", port: 10233, username: "root", password: "hu0aDCULerjL" };

function sshExec(conn, cmd, timeout = 300000) {
  return new Promise((resolve, reject) => {
    console.log(`  [SSH] ${cmd.substring(0, 120)}${cmd.length > 120 ? "..." : ""}`);
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

async function main() {
  console.log("连接服务器...");
  const conn = await new Promise((resolve, reject) => {
    const c = new Client();
    c.on("ready", () => { console.log("  OK SSH 连接成功"); resolve(c); });
    c.on("error", (err) => { console.log(`  [ERROR] ${err.message}`); reject(err); });
    c.connect(SERVER);
  });

  // 1. 安装 Node.js 18（用官方二进制包，不依赖 nodesource）
  console.log("\n>>> 1. 安装 Node.js 18...");
  await sshExec(conn, `
cd /tmp
curl -fsSL https://nodejs.org/dist/v18.20.4/node-v18.20.4-linux-x64.tar.xz -o node18.tar.xz
tar -xf node18.tar.xz -C /usr/local --strip-components=1
rm -f node18.tar.xz
node --version
npm --version
`);

  // 2. 安装 PM2
  console.log("\n>>> 2. 安装 PM2...");
  await sshExec(conn, "npm install -g pm2; pm2 --version");

  // 3. 检查部署文件
  console.log("\n>>> 3. 检查部署文件...");
  await sshExec(conn, "ls -la /opt/freedom/ && ls -la /opt/freedom/web/ && cat /opt/freedom/.env");

  // 4. 停止旧服务
  console.log("\n>>> 4. 停止旧服务...");
  await sshExec(conn, "pm2 delete all 2>/dev/null; echo done");

  // 5. 启动后端
  console.log("\n>>> 5. 启动后端...");
  await sshExec(conn, "cd /opt/freedom && pm2 start ./server --name backend --cwd /opt/freedom --max-memory-restart 512M");

  // 6. 启动前端
  console.log("\n>>> 6. 启动前端...");
  await sshExec(conn, "pm2 start node --name frontend --cwd /opt/freedom/web -- server.js");

  // 7. 等待并检查
  console.log("\n>>> 7. 等待服务启动...");
  await new Promise(r => setTimeout(r, 8000));

  console.log("\n>>> 8. PM2 状态...");
  await sshExec(conn, "pm2 status");

  // 9. 查看日志
  console.log("\n>>> 9. 后端日志...");
  await sshExec(conn, "pm2 logs backend --lines 20 --nostream");

  console.log("\n>>> 10. 前端日志...");
  await sshExec(conn, "pm2 logs frontend --lines 20 --nostream");

  // 11. 验证
  console.log("\n>>> 11. 验证服务...");
  await sshExec(conn, "curl -s -o /dev/null -w 'Frontend HTTP: %{http_code}\\n' http://127.0.0.1:3000/ 2>/dev/null || echo 'Frontend not ready'");
  await sshExec(conn, "curl -s -o /dev/null -w 'Backend HTTP: %{http_code}\\n' http://127.0.0.1:8080/api/settings/public 2>/dev/null || echo 'Backend not ready'");

  // 12. 保存 PM2
  console.log("\n>>> 12. 保存 PM2 配置...");
  await sshExec(conn, "pm2 save; pm2 startup systemd -u root --hp /root 2>/dev/null; echo done");

  conn.end();
  console.log("\n修复完成！");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
