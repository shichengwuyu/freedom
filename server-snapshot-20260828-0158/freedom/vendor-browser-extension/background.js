// background.js (MV3 Service Worker)
// 极简版：只负责采集 Cookie + 状态查询 + 打开登录页
// 用户自己复制 Cookie → 粘贴到无限画布项目里绑定

const VENDOR_META = {
  updream: {
    type: 'updream',
    displayName: 'UpDream 云端',
    cookieDomains: ['.updream.cn', 'updream.cn', 'www.updream.cn'],
    necessaryCookieKeys: ['token', 'session', 'jwt', 'SESSION', 'passport', 'user_id', 'uid', 'auth'],
    loginPage: 'https://www.updream.cn/',
    userInfoEndpoint: 'https://www.updream.cn/api/user/info',
  },
  libtv: {
    type: 'libtv',
    displayName: 'LibTV 云端',
    cookieDomains: ['.liblib.tv', 'liblib.tv', 'www.liblib.tv', '.liblibai.cloud', '.liblib.art', 'api2.liblib.art'],
    necessaryCookieKeys: ['SESSION', 'token', 'jwt', 'libtv_session', 'passport', 'access_key'],
    loginPage: 'https://www.liblib.tv/',
    userInfoEndpoint: 'https://api2.liblib.art/api/www/activity/userInfo',
    userInfoMethod: 'POST',
  },
  newwow: {
    type: 'newwow',
    displayName: 'NewWow 云端',
    cookieDomains: ['.neowow.cn', 'neowow.cn', 'www.neowow.cn', 'hub.neowow.cn'],
    necessaryCookieKeys: ['token', 'session', 'jwt', 'SESSION', 'passport', 'neo_token', 'uid'],
    loginPage: 'https://neowow.cn/neo-tv',
    userInfoEndpoint: 'https://neowow.cn/api/user/info',
  },
};

const STORAGE_KEYS = {
  CAPTURES: 'vendor_captures_v1',
  SAMPLES: 'vendor_samples_v1',
  SETTINGS: 'vendor_settings_v1',
};

// 每个供应商本地保留的最新样本上限（超出丢弃最旧的）
const SAMPLE_LIMIT_PER_VENDOR = 300;

// ============ 工具：从 chrome.storage.local 读写 ============
async function readStorage(key, fallback) {
  try {
    const obj = await chrome.storage.local.get(key);
    return obj[key] ?? fallback;
  } catch (e) {
    console.warn('[vendor-cookie] readStorage failed:', key, e);
    return fallback;
  }
}
async function writeStorage(key, value) {
  await chrome.storage.local.set({ [key]: value });
}

// 简单字符串哈希（样本去重用）
function hashStr(s) {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h << 5) - h + s.charCodeAt(i);
    h |= 0;
  }
  return String(h);
}

// ============ 工具：按供应商拉取该域下所有 Cookie ============
async function collectCookiesForVendor(type) {
  const meta = VENDOR_META[type];
  if (!meta) throw new Error('未知供应商：' + type);
  const domainCookies = new Map();
  for (const domain of meta.cookieDomains) {
    try {
      const raw = await chrome.cookies.getAll({ domain });
      for (const c of raw) {
        const existing = domainCookies.get(c.name);
        if (!existing || (c.expirationDate || 0) > (existing.expirationDate || 0)) {
          domainCookies.set(c.name, c);
        }
      }
    } catch (e) {
      console.warn('[vendor-cookie] collect domain failed:', domain, e);
    }
  }
  const items = [];
  const cookieNameValueMap = new Map();
  domainCookies.forEach((c, name) => {
    items.push(`${name}=${c.value}`);
    cookieNameValueMap.set(name, c.value);
  });
  const cookieString = items.join('; ');
  const matchedNecessary = meta.necessaryCookieKeys.some((k) =>
    [...domainCookies.keys()].some((name) => name.toLowerCase() === k.toLowerCase() || name.toLowerCase().includes(k.toLowerCase()))
  );
  return {
    type,
    cookieString,
    count: domainCookies.size,
    matchedNecessary,
    capturedAt: Date.now(),
    cookieNameValueMap,
    estimatedExpireAt: [...domainCookies.values()]
      .map((c) => c.expirationDate)
      .filter(Boolean)
      .reduce((a, b) => Math.min(a, b), Infinity),
  };
}

// ============ 工具：校验 Cookie（fetch userInfoEndpoint） ============
// 插件端 userinfo 接口是猜测的，容易 404；用 lenient 软通过：必要 Key 命中 + count≥5 就算已登录
async function verifyCookies(type, cookieString, lenientContext = null) {
  const meta = VENDOR_META[type];
  try {
    const method = (meta.userInfoMethod || 'GET').toUpperCase();
    const resp = await fetch(meta.userInfoEndpoint, {
      method,
      headers: {
        Cookie: cookieString,
        Accept: 'application/json, text/plain, */*',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 vendor-cookie-helper/1.0',
      },
      credentials: 'omit',
      redirect: 'manual',
    });
    if (resp.status === 0 || resp.status >= 400 || resp.type === 'opaqueredirect') {
      return tryLenientPass(lenientContext, `HTTP ${resp.status || 'redirect/no-cors'}`);
    }
    let data = null;
    const text = await resp.text();
    try { data = JSON.parse(text); } catch (_) {}
    const info = extractUserInfo(data || {});
    if (!info.vendorUserId && !info.nickname) {
      return tryLenientPass(lenientContext, '响应缺少用户信息字段');
    }
    return { ok: true, soft: false, ...info, raw: data };
  } catch (e) {
    return tryLenientPass(lenientContext, e.message || String(e));
  }
}

function tryLenientPass(ctx, strictError) {
  if (!ctx || !ctx.allowLenient) return { ok: false, error: strictError };
  if (!ctx.matchedNecessary || ctx.count < 5) {
    return { ok: false, error: strictError };
  }
  const nickname = guessNicknameFromCookie(ctx) || '';
  const vendorUserId = guessUserIdFromCookie(ctx) || '';
  return {
    ok: true,
    soft: true,
    nickname,
    avatar: '',
    vendorUserId,
    strictError,
  };
}
function guessNicknameFromCookie(ctx) {
  const map = ctx.cookieNameValueMap || new Map();
  for (const [k, v] of map.entries()) {
    const lk = String(k).toLowerCase();
    if (['nickname', 'nick_name', 'username', 'user_name', 'display_name', 'displayname', 'name'].includes(lk)) {
      return decodeURIComponentSafe(v);
    }
  }
  return '';
}
function guessUserIdFromCookie(ctx) {
  const map = ctx.cookieNameValueMap || new Map();
  for (const [k, v] of map.entries()) {
    const lk = String(k).toLowerCase();
    if (['userid', 'user_id', 'uid', 'id', 'openid', 'member_id', 'account_id'].includes(lk)) {
      return decodeURIComponentSafe(v);
    }
  }
  return '';
}
function decodeURIComponentSafe(s) {
  try { return decodeURIComponent(String(s || '')); } catch (_) { return String(s || ''); }
}

function extractUserInfo(obj) {
  const nested = obj.data || obj.user || obj.result || obj;
  const name = nested.nickname || nested.nickName || nested.username || nested.name || nested.displayName || nested.userName || '';
  const avatar = nested.avatar || nested.avatarUrl || nested.headUrl || nested.photo || nested.headimgurl || '';
  const uid = nested.id || nested.userId || nested.user_id || nested.uid || nested.userid || nested.openid || '';
  return {
    nickname: String(name || '').trim().slice(0, 50),
    avatar: String(avatar || '').trim().slice(0, 500),
    vendorUserId: String(uid || '').trim().slice(0, 100),
  };
}

// ============ 主流程：全量采集 + 校验 + 存 storage ============
async function fullCaptureForVendor(type) {
  const collected = await collectCookiesForVendor(type);
  let verify = { ok: false, error: '未命中必要 Cookie key' };
  if (collected.matchedNecessary && collected.cookieString) {
    const lenientContext = {
      allowLenient: true,
      matchedNecessary: collected.matchedNecessary,
      count: collected.count,
      cookieNameValueMap: collected.cookieNameValueMap,
    };
    verify = await verifyCookies(type, collected.cookieString, lenientContext);
  }
  delete collected.cookieNameValueMap;
  const softMode = Boolean(verify.ok && verify.soft);
  const record = {
    ...collected,
    verified: verify.ok,
    soft: softMode,
    nickname: verify.ok ? verify.nickname : '',
    avatar: verify.ok ? verify.avatar : '',
    vendorUserId: verify.ok ? verify.vendorUserId : '',
    lastVerifyError: verify.ok ? '' : verify.error,
    strictVerifyError: verify.strictError || '',
    expireAt: isFinite(collected.estimatedExpireAt) ? new Date(collected.estimatedExpireAt * 1000).toISOString() : '',
  };
  const captures = await readStorage(STORAGE_KEYS.CAPTURES, {});
  captures[type] = record;
  await writeStorage(STORAGE_KEYS.CAPTURES, captures);
  return record;
}

async function fullCaptureAll() {
  const result = {};
  for (const type of Object.keys(VENDOR_META)) {
    result[type] = await fullCaptureForVendor(type);
  }
  return result;
}

// ============ 样本存储 / 推送 / 绑定 ============

// storeSample：把一条嗅探样本存到 chrome.storage.local（按供应商分组、按 url+method+body 去重）
async function storeSample(type, sample) {
  const all = await readStorage(STORAGE_KEYS.SAMPLES, {});
  const list = Array.isArray(all[type]) ? all[type] : [];
  const key = (sample.url || '') + '|' + (sample.method || '') + '|' + (sample.requestBody || '').slice(0, 512);
  const h = hashStr(key);
  const existing = list.find((s) => s._h === h);
  if (existing) {
    existing.responseStatus = sample.responseStatus || existing.responseStatus;
    existing.responseBody = sample.responseBody || existing.responseBody;
    existing.capturedAt = Date.now();
  } else {
    list.unshift({ ...sample, _h: h, capturedAt: Date.now() });
  }
  all[type] = list.slice(0, SAMPLE_LIMIT_PER_VENDOR);
  await writeStorage(STORAGE_KEYS.SAMPLES, all);
  return { ok: true, stored: true, total: list.length };
}

// pushSamplesToBackend：把本地样本经「无限画布 Web 地址」代理 POST 到后端 capture-sample
// 走 Web 代理（/api/* 由 Next.js 转发到 Go 后端），同源、无 CORS 问题。
async function pushSamplesToBackend(type) {
  const settings = await readStorage(STORAGE_KEYS.SETTINGS, {});
  const base = (settings.backendBase || '').replace(/\/+$/, '');
  const token = settings.backendToken || '';
  if (!base || !token) {
    return { ok: false, error: '请先在弹窗「连接设置」里填写无限画布 Web 地址和登录 Token' };
  }
  const all = await readStorage(STORAGE_KEYS.SAMPLES, {});
  const types = type ? [type] : Object.keys(all);
  let pushed = 0;
  let failed = 0;
  const errors = [];
  for (const t of types) {
    if (!VENDOR_META[t]) continue;
    for (const s of all[t] || []) {
      const payload = {
        vendorType: t,
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
        const resp = await fetch(base + '/api/v1/vendor/capture-sample', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
          body: JSON.stringify(payload),
        });
        // 后端业务失败也返回 HTTP 200（{code:1,msg}），必须解析 body 里的 code 判断成功，
        // 否则会把「失败」误判成「推送成功」。
        const data = await resp.json().catch(() => null);
        const code = data && typeof data.code === 'number' ? data.code : (resp.ok ? 0 : 1);
        if (resp.ok && code === 0) {
          pushed++;
        } else {
          failed++;
          const msg = (data && data.msg) || ('HTTP ' + resp.status);
          errors.push((s.url || '').slice(0, 80) + ' → ' + String(msg).slice(0, 80));
        }
      } catch (e) {
        failed++;
        errors.push((s.url || '').slice(0, 80) + ' → ' + (e && e.message ? e.message : String(e)));
      }
    }
  }
  return { ok: true, pushed, failed, errors: errors.slice(0, 10) };
}

// submitToProject：把当前平台的 Cookie 直接绑定到用户无限画布账户（修复气泡「绑定」按钮）
async function submitToProject(type) {
  const settings = await readStorage(STORAGE_KEYS.SETTINGS, {});
  const base = (settings.backendBase || '').replace(/\/+$/, '');
  const token = settings.backendToken || '';
  if (!base || !token) {
    return { ok: false, error: '请先在弹窗「连接设置」里填写无限画布 Web 地址和登录 Token' };
  }
  const collected = await collectCookiesForVendor(type);
  if (!collected.cookieString || collected.count === 0) {
    return { ok: false, error: '还没有采集到该平台的登录 Cookie，请先在官网完成登录' };
  }
  try {
    const resp = await fetch(base + '/api/v1/vendor/bind-cookie', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
      body: JSON.stringify({ vendorType: type, cookieString: collected.cookieString }),
    });
    const data = await resp.json().catch(() => ({}));
    if (resp.ok && data && data.code === 0) return { ok: true };
    return { ok: false, error: (data && data.msg) || '绑定失败 HTTP ' + resp.status };
  } catch (e) {
    return { ok: false, error: e && e.message ? e.message : String(e) };
  }
}

// ============ 命令中心 ============
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  (async () => {
    try {
      const cmd = msg && msg.cmd;
      switch (cmd) {
        case 'collect': {
          const type = msg.type;
          if (!VENDOR_META[type]) return sendResponse({ ok: false, error: '未知 type：' + type });
          const rec = await fullCaptureForVendor(type);
          return sendResponse({ ok: true, record: rec });
        }
        case 'collectAll': {
          const all = await fullCaptureAll();
          return sendResponse({ ok: true, records: all });
        }
        case 'getStatus': {
          const type = msg.type;
          const captures = await readStorage(STORAGE_KEYS.CAPTURES, {});
          if (type) return sendResponse({ ok: true, record: captures[type] || null });
          return sendResponse({ ok: true, records: captures });
        }
        case 'openLogin': {
          const type = msg.type;
          const meta = VENDOR_META[type];
          if (!meta) return sendResponse({ ok: false, error: '未知 type' });
          await chrome.tabs.create({ url: meta.loginPage, active: true });
          return sendResponse({ ok: true });
        }
        // ---- P1 新增：样本嗅探 + 设置 + 绑定 ----
        case 'captureSample': {
          const { type, sample } = msg;
          if (!VENDOR_META[type] || !sample) return sendResponse({ ok: false, error: '参数缺失' });
          const r = await storeSample(type, sample);
          return sendResponse(r);
        }
        case 'getSamples': {
          const type = msg.type;
          const all = await readStorage(STORAGE_KEYS.SAMPLES, {});
          if (type) return sendResponse({ ok: true, samples: all[type] || [] });
          return sendResponse({ ok: true, samples: all });
        }
        case 'clearSamples': {
          const type = msg.type;
          const all = await readStorage(STORAGE_KEYS.SAMPLES, {});
          if (type) delete all[type]; else for (const k in all) delete all[k];
          await writeStorage(STORAGE_KEYS.SAMPLES, all);
          return sendResponse({ ok: true });
        }
        case 'pushSamplesToBackend': {
          const { type } = msg;
          const r = await pushSamplesToBackend(type);
          return sendResponse(r);
        }
        case 'submitToProject': {
          const { type } = msg;
          if (!VENDOR_META[type]) return sendResponse({ ok: false, error: '未知 type' });
          const r = await submitToProject(type);
          return sendResponse(r);
        }
        case 'getSettings': {
          const s = await readStorage(STORAGE_KEYS.SETTINGS, {});
          return sendResponse({ ok: true, settings: s });
        }
        case 'setSettings': {
          const s = msg.settings || {};
          const cur = await readStorage(STORAGE_KEYS.SETTINGS, {});
          const next = Object.assign({}, cur, s);
          await writeStorage(STORAGE_KEYS.SETTINGS, next);
          return sendResponse({ ok: true, settings: next });
        }
        default:
          return sendResponse({ ok: false, error: '未知命令：' + cmd });
      }
    } catch (e) {
      console.error('[vendor-cookie] onMessage catch:', e);
      sendResponse({ ok: false, error: e.message || String(e) });
    }
  })();
  return true;
});

// ============ 启动时采集 + 每 5 分钟轮询 ============
chrome.runtime.onInstalled.addListener(() => {
  chrome.alarms.create('vendor-cookie-poll', { periodInMinutes: 5 });
  fullCaptureAll().catch((e) => console.warn('[vendor-cookie] install init capture failed:', e));
});
chrome.runtime.onStartup.addListener(() => {
  fullCaptureAll().catch((e) => console.warn('[vendor-cookie] startup init capture failed:', e));
});
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === 'vendor-cookie-poll') {
    fullCaptureAll().catch((e) => console.warn('[vendor-cookie] poll failed:', e));
  }
});

// ============ 页面导航到 3 家平台 → 3 秒后自动采集 ============
chrome.webRequest.onCompleted.addListener(
  (details) => {
    if (details.tabId < 0) return;
    setTimeout(() => {
      const match = matchUrlToVendor(details.url);
      if (match) fullCaptureForVendor(match).catch(() => {});
    }, 3000);
  },
  { urls: ['*://*.updream.cn/*', '*://*.liblib.tv/*', '*://*.liblib.art/*', '*://*.neowow.cn/*'], types: ['xmlhttprequest', 'main_frame', 'sub_frame'] },
);

function matchUrlToVendor(url) {
  try {
    const u = new URL(url);
    const host = u.hostname;
    if (host.includes('updream.cn')) return 'updream';
    if (host.includes('liblib.tv') || host.includes('liblibai.cloud') || host.includes('liblib.art')) return 'libtv';
    if (host.includes('neowow.cn')) return 'newwow';
  } catch (_) {}
  return '';
}
