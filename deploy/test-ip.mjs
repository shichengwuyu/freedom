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

  // 用 IP 作为 Host 头访问
  console.log("\n=== 用 IP 作为 Host 访问 ===");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' -H 'Host: 43.248.3.138' http://127.0.0.1/");
  await sshExec(conn, "curl -s -H 'Host: 43.248.3.138' http://127.0.0.1/ | head -5");

  // 不带 Host 头访问
  console.log("\n=== 不带 Host 头访问 ===");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1/");
  await sshExec(conn, "curl -s http://127.0.0.1/ | head -5");

  // 看 PM2 前端是否在运行
  console.log("\n=== PM2 状态 ===");
  await sshExec(conn, "pm2 status");

  // 看前端端口是否在监听
  console.log("\n=== 端口监听 ===");
  await sshExec(conn, "ss -tlnp | grep -E '3000|8080|80'");

  // 直接访问前端
  console.log("\n=== 直接访问前端 3000 ===");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1:3000/");
  await sshExec(conn, "curl -s http://127.0.0.1:3000/ | head -3");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
