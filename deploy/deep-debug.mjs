import { Client } from "ssh2";

const SERVER = { host: "43.248.3.138", port: 10233, username: "root", password: "hu0aDCULerjL" };

function sshExec(conn, cmd, timeout = 60000) {
  return new Promise((resolve, reject) => {
    console.log(`  [SSH] ${cmd}`);
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
    c.on("ready", () => { console.log("  OK"); resolve(c); });
    c.on("error", (err) => { console.log(`  [ERROR] ${err.message}`); reject(err); });
    c.connect(SERVER);
  });

  // 1. 杀掉所有 nginx 进程
  console.log("\n=== 杀掉所有 nginx ===");
  await sshExec(conn, "pkill -9 nginx; sleep 1; echo 'killed'");

  // 2. 确认 nginx.conf 内容
  console.log("\n=== nginx.conf 内容 ===");
  await sshExec(conn, "cat /etc/nginx/nginx.conf");

  // 3. 确认站点配置
  console.log("\n=== 站点配置 ===");
  await sshExec(conn, "cat /etc/nginx/conf.d/freedom.conf");

  // 4. 检查是否有其他 conf 文件
  console.log("\n=== conf.d 所有文件 ===");
  await sshExec(conn, "ls -la /etc/nginx/conf.d/");

  // 5. 检查 default.d
  console.log("\n=== default.d ===");
  await sshExec(conn, "ls -la /etc/nginx/default.d/");

  // 6. 检查 sites-enabled
  console.log("\n=== sites-enabled ===");
  await sshExec(conn, "ls -la /etc/nginx/sites-enabled/ 2>/dev/null || echo 'no sites-enabled'");

  // 7. 用 nginx -T 看完整加载的配置
  console.log("\n=== nginx -T 完整配置 ===");
  await sshExec(conn, "nginx -T 2>&1");

  // 8. 启动 nginx
  console.log("\n=== 启动 nginx ===");
  await sshExec(conn, "nginx");

  // 9. 确认进程
  console.log("\n=== 进程 ===");
  await sshExec(conn, "ps aux | grep nginx");

  // 10. 测试
  console.log("\n=== 测试 ===");
  await sshExec(conn, "curl -s http://127.0.0.1/ | head -3");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
