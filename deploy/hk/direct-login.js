// 在 app 容器内直连后端 8080 测试 admin 登录，绕过前端代理。
const body = JSON.stringify({ username: "admin", password: "freedom" });
fetch("http://127.0.0.1:8080/api/admin/login", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body,
})
  .then(async (r) => {
    const text = await r.text();
    console.log("DIRECT_STATUS", r.status);
    console.log("DIRECT_BODY", text);
  })
  .catch((e) => {
    console.log("DIRECT_ERR", String(e));
  });
