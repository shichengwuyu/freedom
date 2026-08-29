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

// 改用 8088 端口（避开 NAT 网关的 80 端口拦截）
const SITE_CONF = `server {
    listen 8088 default_server;
    listen [::]:8088 default_server;
    server_name _;

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
}
`;

async function main() {
  console.log("连接服务器...");
  const conn = await new Promise((resolve, reject) => {
    const c = new Client();
    c.on("ready", () => { console.log("  OK"); resolve(c); });
    c.on("error", (err) => { console.log(`  [ERROR] ${err.message}`); reject(err); });
    c.connect(SERVER);
  });

  // 1. 写入新配置（用 8088 端口）
  console.log("\n>>> 写入站点配置（端口 8088）...");
  const writeSite = `node -e "require('fs').writeFileSync('/etc/nginx/conf.d/freedom.conf', \`${SITE_CONF.replace(/`/g, '\\`')}\`)"`;
  await sshExec(conn, writeSite);

  // 2. 验证
  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  // 3. 重启
  console.log("\n>>> 重启 nginx...");
  await sshExec(conn, "systemctl restart nginx");

  // 4. 验证 8088 端口
  console.log("\n>>> 验证 8088 端口...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1:8088/");
  await sshExec(conn, "curl -s http://127.0.0.1:8088/ | grep '<title>'");

  // 5. 也试试从公网 IP 的 8088 端口访问
  console.log("\n>>> 从公网 IP 8088 端口访问...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://43.248.3.138:8088/ 2>&1 || echo 'failed'");
  await sshExec(conn, "curl -s http://43.248.3.138:8088/ 2>&1 | head -3 || echo 'failed'");

  conn.end();
  console.log("\n完成！");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
