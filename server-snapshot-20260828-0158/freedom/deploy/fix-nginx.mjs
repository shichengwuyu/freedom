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

  // 1. 查看当前 nginx 配置
  console.log("\n>>> 查看 nginx 主配置...");
  await sshExec(conn, "cat /etc/nginx/nginx.conf");

  // 2. 查看 conf.d 目录
  console.log("\n>>> 查看 conf.d 目录...");
  await sshExec(conn, "ls -la /etc/nginx/conf.d/");

  // 3. 查看我们的配置文件
  console.log("\n>>> 查看站点配置...");
  await sshExec(conn, "cat /etc/nginx/conf.d/freedom.conf");

  // 4. 检查 nginx 测试
  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  // 5. 查看 nginx 实际加载的配置
  console.log("\n>>> 查看 nginx 进程...");
  await sshExec(conn, "ps aux | grep nginx");

  conn.end();
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
