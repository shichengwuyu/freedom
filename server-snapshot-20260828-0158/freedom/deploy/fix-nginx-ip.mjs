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

const SITE_CONF = `server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _ xiaoyxiao.xyz www.xiaoyxiao.xyz;

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

  // 写入配置（加了 _ 匹配所有请求）
  console.log("\n>>> 写入站点配置（添加 _ 通配）...");
  const writeSite = `node -e "require('fs').writeFileSync('/etc/nginx/conf.d/freedom.conf', \`${SITE_CONF.replace(/`/g, '\\`')}\`)"`;
  await sshExec(conn, writeSite);

  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  console.log("\n>>> 重启 nginx...");
  await sshExec(conn, "systemctl restart nginx");

  console.log("\n>>> 验证...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1/");
  await sshExec(conn, "curl -s http://127.0.0.1/ | grep '<title>'");

  conn.end();
  console.log("\n完成！");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
