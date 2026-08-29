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

// 新的 nginx.conf - 完全去掉默认 server 块
const NEW_NGINX_CONF = `# For more information on configuration, see:
#   * Official English Documentation: http://nginx.org/en/docs/
#   * Official Russian Documentation: http://nginx.org/ru/docs/

user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log;
pid /run/nginx.pid;

# Load dynamic modules. See /usr/share/doc/nginx/README.dynamic.
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

    # Load modular configuration files from the /etc/nginx/conf.d directory.
    include /etc/nginx/conf.d/*.conf;
}
`;

// 新的站点配置 - 添加 default_server
const NEW_SITE_CONF = `server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name xiaoyxiao.xyz www.xiaoyxiao.xyz;

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

  // 1. 写入新的 nginx.conf（无默认 server 块）
  console.log("\n>>> 重写 nginx.conf...");
  const escapedConf = NEW_NGINX_CONF.replace(/'/g, "'\\''");
  await sshExec(conn, `cat > /etc/nginx/nginx.conf << 'NGINXEOF'\n${NEW_NGINX_CONF}\nNGINXEOF`);

  // 2. 写入新的站点配置（带 default_server）
  console.log("\n>>> 重写站点配置...");
  await sshExec(conn, `cat > /etc/nginx/conf.d/freedom.conf << 'SITEEOF'\n${NEW_SITE_CONF}\nSITEEOF`);

  // 3. 验证
  console.log("\n>>> nginx -t...");
  await sshExec(conn, "nginx -t 2>&1");

  // 4. 重启
  console.log("\n>>> 重启 nginx...");
  await sshExec(conn, "systemctl restart nginx");

  // 5. 验证访问
  console.log("\n>>> 验证本地访问...");
  await sshExec(conn, "curl -s -o /dev/null -w 'HTTP %{http_code}\\n' -H 'Host: xiaoyxiao.xyz' http://127.0.0.1/");
  await sshExec(conn, "curl -s -H 'Host: xiaoyxiao.xyz' http://127.0.0.1/ | head -10");

  conn.end();
  console.log("\n修复完成！请刷新浏览器访问 http://xiaoyxiao.xyz");
}

main().catch(e => { console.error("\n[ERROR]", e.message); process.exit(1); });
