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

  // 1. 查看当前 nginx.conf 是否还有 default_server
  console.log("\n>>> 当前 nginx.conf 内容...");
  await sshExec(conn, "cat /etc/nginx/nginx.conf");

  // 2. 查看 conf.d 下所有文件
  console.log("\n>>> conf.d 下所有文件...");
  await sshExec(conn, "ls -la /etc/nginx/conf.d/");
  await sshExec(conn, "cat /etc/nginx/conf.d/*.conf");

  // 3. 查看 default.d 目录
  console.log("\n>>> default.d 目录...");
  await sshExec(conn, "ls -la /etc/nginx/default.d/ 2>/dev/null; cat /etc/nginx/default.d/*.conf 2>/dev/null");

  // 4. 查看 sites-enabled（如果有）
  console.log("\n>>> sites-enabled...");
  await sshExec(conn, "ls -la /etc/nginx/sites-enabled/ 2>/dev/null; cat /etc/nginx/sites-enabled/* 2>/dev/null");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
