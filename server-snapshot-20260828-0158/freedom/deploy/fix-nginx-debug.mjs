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

  // 1. 看 nginx.conf 实际内容
  console.log("\n=== nginx.conf ===");
  await sshExec(conn, "cat /etc/nginx/nginx.conf");

  // 2. 看站点配置
  console.log("\n=== conf.d ===");
  await sshExec(conn, "cat /etc/nginx/conf.d/freedom.conf");

  // 3. 看 nginx 加载了哪些配置
  console.log("\n=== nginx -T (完整配置) ===");
  await sshExec(conn, "nginx -T 2>&1 | head -80");

  // 4. 看 nginx 进程启动时间
  console.log("\n=== nginx 进程 ===");
  await sshExec(conn, "ps aux | grep nginx");

  // 5. 看 nginx error log
  console.log("\n=== error log ===");
  await sshExec(conn, "tail -20 /var/log/nginx/error.log");

  // 6. 看是否有其他 server 块
  console.log("\n=== 所有 server_name ===");
  await sshExec(conn, "nginx -T 2>&1 | grep server_name");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
