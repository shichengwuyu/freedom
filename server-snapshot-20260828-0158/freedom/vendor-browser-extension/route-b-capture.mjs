// route-b-capture.mjs — 路线 B：复用你已登录的 Edge 登录态，嗅探 NewWow / UpDream 的生图接口样本
//
// 前置条件（必须）：
//   1. 完全关闭 Edge（否则 User Data 目录被锁，无法复用登录态）
//   2. npm i playwright-core   （只用系统 Edge，不下载浏览器）
//
// 用法：
//   node route-b-capture.mjs newwow            # 抓 NewWow，样本存本地 JSON
//   node route-b-capture.mjs updream            # 抓 UpDream
//   node route-b-capture.mjs newwow --push      # 抓完直接推送到后端（需下面两个环境变量）
//
// 推送需要的环境变量（与插件「连接设置」一致）：
//   WB_BASE  = 无限画布 Web 地址，例如 http://localhost:3000
//   WB_TOKEN = 登录 Token
//
// 行为：打开对应官网（带着你的登录 Cookie）→ 你在页面上生成一张图 → 脚本自动抓取
//       这次真实请求/响应，存为样本（与插件 content.js 的 shouldCapture 判定一致）。

import { createRequire } from 'module';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';

// ESM 下 NODE_PATH 不生效，这里用 createRequire 解析 playwright-core：
// 顺序：NODE_PATH → 托管工作区绝对路径 → 脚本同级 node_modules。
const require = createRequire(import.meta.url);
function loadPlaywrightCore() {
  const candidates = [];
  if (process.env.NODE_PATH) {
    for (const p of process.env.NODE_PATH.split(/[;:]/).map((s) => s.trim()).filter(Boolean)) {
      candidates.push(path.join(p, 'playwright-core'));
    }
  }
  candidates.push(path.join(os.homedir(), '.workbuddy', 'binaries', 'node', 'workspace', 'node_modules', 'playwright-core'));
  candidates.push(path.join(path.dirname(new URL(import.meta.url).pathname), 'node_modules', 'playwright-core'));
  for (const c of candidates) {
    try { return require(c); } catch (_) {}
  }
  try { return require('playwright-core'); } catch (_) {}
  throw new Error('找不到 playwright-core，请先安装：npm i playwright-core');
}
const { chromium } = loadPlaywrightCore();

const TARGET = (process.argv[2] || 'newwow').toLowerCase();
const DO_PUSH = process.argv.includes('--push');

const VENDOR = {
  updream: { host: 'updream.cn', url: 'https://www.updream.cn/' },
  newwow: { host: 'neowow.cn', url: 'https://neowow.cn/neo-tv' },
};
const v = VENDOR[TARGET];
if (!v) {
  console.error('未知目标，请用 updream 或 newwow');
  process.exit(1);
}

// 与 content.js shouldCapture 完全一致的判定
const SENSITIVE = ['login', 'pass', 'auth', 'token', 'oauth', 'signin', 'sign-in', 'register', 'signup', 'sign-up', 'password', 'logout', 'captcha', 'verify'];
function shouldCapture(targetURL, method) {
  try {
    const u = new URL(targetURL);
    const host = u.hostname.toLowerCase();
    const okHost = host.endsWith(v.host);
    if (!okHost) return false;
    const m = String(method || '').toUpperCase();
    if (m !== 'POST' && m !== 'PUT' && m !== 'PATCH') return false;
    const path = (u.pathname + ' ' + u.search).toLowerCase();
    if (SENSITIVE.some((k) => path.includes(k))) return false;
    return true;
  } catch {
    return false;
  }
}

const EDGE_USER_DATA = path.join(
  process.env.LOCALAPPDATA || path.join(os.homedir(), 'AppData', 'Local'),
  'Microsoft', 'Edge', 'User Data'
);

const samples = [];
const seen = new Set();
const pendingRequests = new Map();
const MAX_REQ = 64 * 1024;
const MAX_RESP = 256 * 1024;

async function run() {
  if (!fs.existsSync(EDGE_USER_DATA)) {
    console.error('找不到 Edge User Data 目录：', EDGE_USER_DATA);
    process.exit(1);
  }

  let context;
  try {
    context = await chromium.launchPersistentContext(EDGE_USER_DATA, {
      channel: 'msedge',
      headless: false,
      args: ['--profile-directory=Default'],
    });
  } catch (e) {
    console.error('\n启动 Edge 失败 —— 多半是 Edge 还没完全关闭（User Data 被锁）。');
    console.error('请完全退出 Edge（任务管理器确认没有 msedge 进程）后再运行。');
    console.error('错误：', e && e.message ? e.message : String(e));
    process.exit(1);
  }

  const page = context.pages()[0] || (await context.newPage());

  if (process.env.WB_DEBUG) {
    page.on('request', (req) => {
      try {
        const h = new URL(req.url()).hostname.toLowerCase();
        if (h.endsWith(v.host) || h.includes('neowow') || h.includes('updream') || h.includes('liblib')) {
          console.log('[debug-req]', req.method(), req.url());
        }
      } catch {}
    });
  }

  // 双事件捕获：request 事件在请求发出瞬间触发（不受 SPA 路由跳转影响），
  // 先把请求部分存进 pendingRequests；response 事件补上响应部分。
  page.on('request', (req) => {
    if (!shouldCapture(req.url(), req.method())) {
      if (process.env.WB_DEBUG) {
        try {
          const h = new URL(req.url()).hostname.toLowerCase();
          if (h.endsWith(v.host) || h.includes(v.host.replace('.cn', ''))) {
            console.log('[debug-skip]', req.method(), req.url(), '→ shouldCapture=false');
          }
        } catch {}
      }
      return;
    }
    const key = req.method() + ':' + req.url();
    if (seen.has(key)) return;
    seen.add(key);
    try {
      pendingRequests.set(key, {
        url: req.url(),
        method: String(req.method()).toUpperCase(),
        requestHeaders: req.headers(),
        requestBody: (req.postData() || '').slice(0, MAX_REQ),
      });
      if (process.env.WB_DEBUG) console.log('[debug-req-capture]', req.method(), req.url());
    } catch (e) {
      console.error('[request-capture-error]', req.url(), e && e.message ? e.message : String(e));
    }
  });

  page.on('response', async (resp) => {
    const req = resp.request();
    if (!shouldCapture(req.url(), req.method())) return;
    const key = req.method() + ':' + req.url();
    const pending = pendingRequests.get(key);
    if (!pending) return;
    pendingRequests.delete(key);
    try {
      const headers = resp.headers();
      let bodyBuf = Buffer.alloc(0);
      try { bodyBuf = await resp.body(); } catch (e2) {
        if (process.env.WB_DEBUG) console.log('[debug-warn] response body read failed:', e2 && e2.message);
      }
      samples.push({
        vendorType: TARGET,
        url: pending.url,
        method: pending.method,
        requestHeaders: pending.requestHeaders,
        requestBody: pending.requestBody,
        responseStatus: resp.status(),
        responseHeaders: headers,
        responseBody: bodyBuf.toString('utf8').slice(0, MAX_RESP),
        contentType: headers['content-type'] || '',
      });
      console.log('✔ 捕获样本：', pending.url);
    } catch (e) {
      console.error('[response-capture-error]', pending.url, e && e.message ? e.message : String(e));
    }
  });

  console.log(`\n已用你的 Edge 登录态打开 ${v.url}`);
  console.log('请在页面上生成一张图片（线路 B 正在嗅探生图接口）…\n');

  try {
    await page.goto(v.url, { waitUntil: 'domcontentloaded', timeout: 30000 });
  } catch (e) {
    console.warn('导航提示：', e && e.message ? e.message : String(e));
  }

  // 等待你生成图片；抓到 3 条或 5 分钟超时即结束
  const deadline = Date.now() + 5 * 60 * 1000;
  while (Date.now() < deadline) {
    if (samples.length >= 3) break;
    await page.waitForTimeout(1000);
  }

  await context.close();

  // 把还没收到响应的请求也存为样本（至少保留请求形状，便于后端学习接口）
  for (const [key, p] of pendingRequests) {
    samples.push({
      vendorType: TARGET,
      url: p.url,
      method: p.method,
      requestHeaders: p.requestHeaders,
      requestBody: p.requestBody,
      responseStatus: 0,
      responseHeaders: {},
      responseBody: '',
      contentType: '',
    });
    console.log('⚠ 仅捕获到请求（未收到响应）：', p.url);
  }
  pendingRequests.clear();

  const outFile = path.join(process.cwd(), `route-b-samples-${TARGET}.json`);
  fs.writeFileSync(outFile, JSON.stringify(samples, null, 2), 'utf8');
  console.log(`\n共捕获 ${samples.length} 条样本，已保存：`, outFile);

  if (DO_PUSH) {
    const base = (process.env.WB_BASE || '').replace(/\/+$/, '');
    const token = process.env.WB_TOKEN || '';
    if (!base || !token) {
      console.error('推送失败：未设置 WB_BASE / WB_TOKEN 环境变量');
      return;
    }
    let pushed = 0, failed = 0;
    for (const s of samples) {
      const payload = {
        vendorType: s.vendorType,
        sample: {
          url: s.url,
          method: s.method,
          requestHeaders: s.requestHeaders || {},
          requestBody: s.requestBody || '',
          responseStatus: s.responseStatus || 0,
          responseHeaders: s.responseHeaders || {},
          responseBody: s.responseBody || '',
          contentType: s.contentType || '',
        },
      };
      try {
        const r = await fetch(base + '/api/v1/vendor/capture-sample', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
          body: JSON.stringify(payload),
        });
        const data = await r.json().catch(() => null);
        const code = data && typeof data.code === 'number' ? data.code : (r.ok ? 0 : 1);
        if (r.ok && code === 0) pushed++;
        else { failed++; console.error('推送失败：', (data && data.msg) || ('HTTP ' + r.status)); }
      } catch (e) {
        failed++; console.error('推送异常：', e && e.message ? e.message : String(e));
      }
    }
    console.log(`推送完成：成功 ${pushed} 条，失败 ${failed} 条`);
  } else if (samples.length) {
    console.log('（未推送，仅保存本地。要推送加 --push 并设置 WB_BASE / WB_TOKEN）');
  }
}

run().catch((e) => {
  console.error('运行出错：', e && e.message ? e.message : String(e));
  process.exit(1);
});
