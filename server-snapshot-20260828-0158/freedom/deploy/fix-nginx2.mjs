import { Client } from "ssh2";

const SERVER = { host: "43.248.3.138", port: 10233, username: "root", password: "hu0aDCULerjL" };

function sshExec(conn, cmd, timeout = 60000) {
  return new Promise((resolve, reject) => {
    console.log(`  [SSH] ${cmd.substring(0, 120)}`);
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

  // 1. 备份并修改 nginx.conf，去掉默认 server 块
  console.log("\n>>> 备份 nginx.conf...");
  await sshExec(conn, "cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.bak");

  // 用 sed 删除 default_server 块（从 "server {" 到匹配的 "}"）
  console.log("\n>>> 修改 nginx.conf，去掉默认 server 块...");
  await sshExec(conn, `python3 -c "
import re
with open('/etc/nginx/nginx.conf', 'r') as f:
    content = f.read()

# 移除 default_server 块
pattern = r'    server \{\n        listen\s+80 default_server.*?\n    \}\n'
content = re.sub(pattern, '', content, flags=re.DOTALL)

with open('/etc/nginx/nginx.conf', 'w') as f:
    f.write(content)
print('Done')
"`);

  // 2. 验证配置
  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  // 3. 重启 nginx
  console.log("\n>>> 重启 nginx...");
  await sshExec(conn, "systemctl restart nginx");

  // 4. 验证
  console.log("\n>>> 验证访问...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1/");
  await sshExec(conn, "curl -s http://127.0.0.1/ | head -5");

  // 5. 检查 PM2 服务是否正常
  console.log("\n>>> PM2 状态...");
  await sshExec(conn, "pm2 status");

  conn.end();
  console.log("\n修复完成！");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
