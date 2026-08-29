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

const NGINX_CONF = `user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log;
pid /run/nginx.pid;

include /usr/share/nginx/modules/*.conf;

events {
    worker_connections 1024;
}

http {
    log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
                      '$status $body_bytes_sent "$http_referer" '
                      '"$http_user_agent" "$http_x_forwarded_for"';

    access_log  /var/log/nginx/access.log  main;

    sendfile            on;
    tcp_nopush          on;
    tcp_nodelay         on;
    keepalive_timeout   65;
    types_hash_max_size 2048;

    include             /etc/nginx/mime.types;
    default_type        application/octet-stream;

    include /etc/nginx/conf.d/*.conf;
}
`;

const SITE_CONF = `server {
    listen 80 default_server;
    listen [::]:80 default_server;
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

  // 1. 检查当前 nginx.conf
  console.log("\n>>> 当前 nginx.conf...");
  await sshExec(conn, "grep -c 'default_server' /etc/nginx/nginx.conf");
  await sshExec(conn, "grep 'server_name' /etc/nginx/nginx.conf");

  // 2. 用 node 写入文件（更可靠）
  console.log("\n>>> 写入 nginx.conf...");
  const writeNginx = `node -e "require('fs').writeFileSync('/etc/nginx/nginx.conf', \`${NGINX_CONF.replace(/`/g, '\\`')}\`)"`;
  await sshExec(conn, writeNginx);

  console.log("\n>>> 写入站点配置...");
  const writeSite = `node -e "require('fs').writeFileSync('/etc/nginx/conf.d/freedom.conf', \`${SITE_CONF.replace(/`/g, '\\`')}\`)"`;
  await sshExec(conn, writeSite);

  // 3. 验证
  console.log("\n>>> 验证 nginx.conf...");
  await sshExec(conn, "grep -c 'default_server' /etc/nginx/nginx.conf || echo '0 matches in main conf'");
  await sshExec(conn, "grep 'default_server' /etc/nginx/conf.d/freedom.conf");

  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  console.log("\n>>> 重启 nginx...");
  await sshExec(conn, "systemctl restart nginx");

  console.log("\n>>> 验证访问...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' http://127.0.0.1/");
  await sshExec(conn, "curl -s http://127.0.0.1/ | head -3");

  conn.end();
  console.log("\n完成！");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
