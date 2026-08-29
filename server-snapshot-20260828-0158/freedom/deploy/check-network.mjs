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

  // 1. 查看公网 IP
  console.log("\n=== 公网 IP ===");
  await sshExec(conn, "curl -s ifconfig.me");
  console.log("");

  // 2. 查看内网 IP
  console.log("\n=== 内网 IP ===");
  await sshExec(conn, "ip addr show | grep 'inet ' | grep -v 127.0.0.1");

  // 3. 查看 iptables 规则
  console.log("\n=== iptables 规则 ===");
  await sshExec(conn, "iptables -L -n 2>/dev/null || echo 'no iptables'");

  // 4. 查看 firewalld
  console.log("\n=== firewalld ===");
  await sshExec(conn, "firewall-cmd --list-all 2>/dev/null || echo 'no firewalld'");

  // 5. 查看 80 端口监听情况
  console.log("\n=== 80 端口监听 ===");
  await sshExec(conn, "ss -tlnp | grep ':80'");

  // 6. 用公网 IP 从服务器内部访问
  console.log("\n=== 用公网 IP 访问 ===");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://43.248.3.138/ 2>&1 || echo 'failed'");
  await sshExec(conn, "curl -s http://43.248.3.138/ 2>&1 | head -3 || echo 'failed'");

  // 7. 查看 Nginx access log
  console.log("\n=== 最近 access log ===");
  await sshExec(conn, "tail -10 /var/log/nginx/access.log");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
