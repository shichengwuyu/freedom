// content.js — 注入到 updream / liblib / newwow 三个官网页面
// 职责：
//   1. 页面加载完成后 → 给 background 发命令 collect 一下当前域的 cookie
//   2. 根据后台返回的登录状态，在页面右上角渲染一个小气泡：
//      - 已登录+已验证 → 显示"已捕获你的账户 XXX · 一键绑定到无限画布"按钮
//      - 未登录 → 显示"登录完成后会自动检测 Cookie"
//   3. 气泡按钮点击 → 直接调到后台 submitToProject 把 cookie 发回项目（用户不需要打开插件弹窗）

(function () {
  // 只允许挂一次（SPA 路由切换可能导致 content 被重复注入的防御）
  if (window.__FREEDOM_VENDOR_BUBBLE_MOUNTED__) return;
  window.__FREEDOM_VENDOR_BUBBLE_MOUNTED__ = true;

  const META_BY_HOST = [
    { test: (h) => h.includes('updream.cn'), type: 'updream', name: 'UpDream 云端', icon: '🚀' },
    { test: (h) => h.includes('liblib.tv') || h.includes('liblibai.cloud') || h.includes('liblib.art'), type: 'libtv', name: 'LibTV 云端', icon: '📺' },
    { test: (h) => h.includes('neowow.cn'), type: 'newwow', name: 'NewWow 云端', icon: '✨' },
  ];
  const host = (location.hostname || '').toLowerCase();
  const meta = META_BY_HOST.find((m) => m.test(host));
  if (!meta) return; // 不是目标站点，不注入

  // ========== 跟 background 通信 ==========
  let extensionDead = false; // 标记插件上下文是否已失效（更新/重载后不可恢复，需刷新页面）

  function send(cmd, payload = {}) {
    if (extensionDead) return Promise.resolve({ ok: false, error: '插件已失效，请刷新本页后重试' });
    return new Promise((resolve) => {
      try {
        chrome.runtime.sendMessage({ cmd, ...payload }, (resp) => {
          // 检测 runtime.lastError（包括 context invalidated）
          if (chrome.runtime.lastError) {
            const msg = chrome.runtime.lastError.message || '';
            if (msg.includes('invalidated') || msg.includes('context')) {
              extensionDead = true;
            }
            resolve({ ok: false, error: msg || '插件通信异常' });
            return;
          }
          resolve(resp || { ok: false, error: '无响应' });
        });
      } catch (e) {
        const msg = String(e);
        if (msg.includes('invalidated') || msg.includes('context')) {
          extensionDead = true;
        }
        resolve({ ok: false, error: msg });
      }
    });
  }

  /** 检查错误是否为「插件上下文失效」，是则把气泡 UI 切为不可用状态 */
  function checkExtensionDead(error) {
    if (!error) return false;
    const msg = String(error).toLowerCase();
    if (msg.includes('invalidated') || msg.includes('context')) {
      extensionDead = true;
      const statusEl = document.getElementById('fv-status');
      const statusText = document.getElementById('fv-status-text');
      const metaEl = document.getElementById('fv-meta');
      const bindBtn = document.getElementById('fv-btn-bind');
      const refreshBtn = document.getElementById('fv-btn-refresh');
      if (statusEl) statusEl.className = 'fv-status fv-warn';
      if (statusText) statusText.textContent = '插件已失效';
      if (metaEl) metaEl.textContent = '插件刚被更新或重载，请按 F5 刷新本页后重新打开';
      if (bindBtn) { bindBtn.disabled = true; bindBtn.textContent = '需刷新页面'; }
      if (refreshBtn) { refreshBtn.disabled = true; }
      return true;
    }
    return false;
  }

  // ========== 嗅探生成类请求样本（仅 UpDream / NewWow 需要；LibTV 走开放平台不需要）==========
  // 插件在用户官网浏览器里实际生成一次，把这次真实请求/响应抓下来回传后端，
  // 后端据此学习内部接口形状，之后用该用户自己的 Cookie 在后端重放。
  const CAPTURE_TYPES = new Set(['updream', 'newwow']);
  const capturedHashes = new Set(); // 本次会话内去重，避免重复上报同一条请求
  const pendingCaptures = new Map(); // 请求已发出但未收到响应时的兜底缓存（页面跳转/请求中止时仍能保留请求形状）

  function shouldCapture(targetURL, method) {
    try {
      const u = new URL(targetURL, location.href);
      const host = u.hostname.toLowerCase();
      // 仅捕获目标供应商域下的请求
      const okHost = host.endsWith('updream.cn') || host.endsWith('neowow.cn');
      if (!okHost) return false;
      const m = String(method || '').toUpperCase();
      if (m !== 'POST' && m !== 'PUT' && m !== 'PATCH') return false;
      // 排除明显敏感接口（登录 / 鉴权 / 改密码 / 验证码），避免误抓凭据
      const path = (u.pathname + ' ' + u.search).toLowerCase();
      const sensitive = ['login', 'pass', 'auth', 'token', 'oauth', 'signin', 'sign-in', 'register', 'signup', 'sign-up', 'password', 'logout', 'captcha', 'verify'];
      if (sensitive.some((k) => path.includes(k))) return false;
      return true;
    } catch (_) {
      return false;
    }
  }

  function headersToObj(headers) {
    const obj = {};
    if (headers && typeof headers.forEach === 'function') {
      headers.forEach((v, k) => { obj[k] = v; });
    } else if (headers && typeof headers === 'object') {
      for (const k in headers) obj[k] = headers[k];
    }
    return obj;
  }

  function truncateStr(s, max) {
    s = String(s == null ? '' : s);
    return s.length > max ? s.slice(0, max) : s;
  }

  function hashStr(s) {
    let h = 0;
    for (let i = 0; i < s.length; i++) {
      h = (h << 5) - h + s.charCodeAt(i);
      h |= 0;
    }
    return String(h);
  }

  function buildSample(targetURL, method, headersObj, bodyStr) {
    return {
      url: targetURL,
      method: String(method || 'POST').toUpperCase(),
      requestHeaders: headersObj || {},
      requestBody: truncateStr(bodyStr || '', 64 * 1024),
      responseStatus: 0,
      responseHeaders: {},
      responseBody: '',
      contentType: (headersObj && (headersObj['content-type'] || headersObj['Content-Type'])) || '',
    };
  }

  // 把样本发往 background 存本地 + 后续推送后端；会话内按 url+method+body 去重
  function pushSample(sample) {
    const key = sample.url + '|' + sample.method + '|' + (sample.requestBody || '').slice(0, 512);
    const h = hashStr(key);
    if (capturedHashes.has(h)) return;
    capturedHashes.add(h);
    pendingCaptures.delete(key); // 已收到完整样本，移出兜底缓存
    send('captureSample', { type: meta.type, sample }).catch(() => {});
  }

  // 与 pushSample 去重口径一致的 key（用于 pending 缓存的读写）
  function sampleKey(sample) {
    return sample.url + '|' + sample.method + '|' + (sample.requestBody || '').slice(0, 512);
  }

  function installInterceptors() {
    if (window.__FREEDOM_VENDOR_INTERCEPTED__) return;
    window.__FREEDOM_VENDOR_INTERCEPTED__ = true;

    // ---- 拦截 fetch ----
    const origFetch = window.fetch ? window.fetch.bind(window) : null;
    if (origFetch) {
      window.fetch = function (input, init) {
        const url = typeof input === 'string' ? input : (input && input.url) || '';
        const method = (init && init.method) || (input && input.method) || 'GET';
        let headersObj = {};
        if (init && init.headers) headersObj = headersToObj(init.headers);
        else if (input && input.headers) headersObj = headersToObj(input.headers);
        const bodyStr = typeof (init && init.body) === 'string'
          ? init.body
          : (init && init.body && typeof init.body.toString === 'function' && init.body.toString() !== '[object Object]')
            ? init.body.toString()
            : '';
        const capture = shouldCapture(url, method);
        const reqSample = capture ? buildSample(url, method, headersObj, bodyStr) : null;
        if (capture) pendingCaptures.set(sampleKey(reqSample), reqSample);
        return origFetch(input, init).then(async (resp) => {
          if (capture && resp) {
            const sample = buildSample(url, method, headersObj, bodyStr);
            sample.responseStatus = resp.status;
            const rh = {};
            if (resp.headers && typeof resp.headers.forEach === 'function') {
              resp.headers.forEach((v, k) => { rh[k] = v; });
            }
            sample.responseHeaders = rh;
            try {
              const txt = await resp.clone().text();
              sample.responseBody = truncateStr(txt, 64 * 1024);
            } catch (_) { /* 响应体不可读（opaque）就留空 */ }
            pushSample(sample);
          }
          return resp;
        }, (err) => {
          if (reqSample) pushSample(reqSample); // 兜底：请求已发出但响应没拿到（如页面跳转导致中止）
          throw err;
        });
      };
    }

    // ---- 拦截 XMLHttpRequest ----
    const origOpen = XMLHttpRequest.prototype.open;
    const origSend = XMLHttpRequest.prototype.send;
    const origSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;
    XMLHttpRequest.prototype.open = function (method, url) {
      this.__fv_method = method;
      this.__fv_url = url;
      this.__fv_headers = {};
      return origOpen.apply(this, arguments);
    };
    XMLHttpRequest.prototype.setRequestHeader = function (name, value) {
      if (this.__fv_headers) this.__fv_headers[name] = value;
      return origSetRequestHeader.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function (body) {
      const self = this;
      const url = self.__fv_url;
      const method = self.__fv_method;
      const headersObj = self.__fv_headers || {};
      const capture = shouldCapture(url, method);
      const bodyStr = typeof body === 'string'
        ? body
        : (body && typeof body.toString === 'function' && body.toString() !== '[object Object]')
          ? body.toString()
          : '';
      if (capture) {
        const reqSample = buildSample(url, method, headersObj, bodyStr);
        pendingCaptures.set(sampleKey(reqSample), reqSample);
        self.addEventListener('loadend', function () {
          const sample = buildSample(url, method, headersObj, bodyStr);
          sample.responseStatus = self.status;
          const rh = {};
          try {
            const all = (self.getAllResponseHeaders() || '').split('\r\n');
            for (const line of all) {
              const idx = line.indexOf(':');
              if (idx > 0) rh[line.slice(0, idx).trim().toLowerCase()] = line.slice(idx + 1).trim();
            }
          } catch (_) { /* ignore */ }
          sample.responseHeaders = rh;
          try { sample.responseBody = truncateStr(self.responseText || '', 64 * 1024); } catch (_) { /* ignore */ }
          pushSample(sample); // 内部会移出 pending
        });
      }
      return origSend.apply(this, arguments);
    };
  }

  // 页面卸载/跳转时兜底：把还没收到响应的请求也发往 background（至少保留请求形状）
  function flushPendingCaptures() {
    for (const [, s] of pendingCaptures) pushSample(s);
    pendingCaptures.clear();
  }
  if (!window.__FREEDOM_VENDOR_FLUSH_BOUND__) {
    window.__FREEDOM_VENDOR_FLUSH_BOUND__ = true;
    window.addEventListener('pagehide', flushPendingCaptures);
    window.addEventListener('beforeunload', flushPendingCaptures);
  }

  // ========== 渲染气泡 ==========
  function renderBubble() {
    if (document.getElementById('freedom-vendor-bubble-root')) return;
    const root = document.createElement('div');
    root.id = 'freedom-vendor-bubble-root';
    root.innerHTML = `
      <div id="freedom-vendor-bubble" role="dialog" aria-live="polite">
        <div class="fv-header">
          <div class="fv-icon">🎨</div>
          <div>
            <div class="fv-title">无限画布供应商助手</div>
            <div class="fv-meta" id="fv-meta">正在采集 ${meta.name} 的登录态…</div>
          </div>
          <button class="fv-close" id="fv-close" aria-label="关闭">×</button>
        </div>
        <div class="fv-status fv-none" id="fv-status"><span class="fv-dot"></span><span id="fv-status-text">检测中</span></div>
        <div class="fv-actions">
          <button class="fv-btn" id="fv-btn-refresh">重新检测</button>
          <button class="fv-btn fv-btn-primary" id="fv-btn-bind" disabled>绑定到我的无限画布</button>
        </div>
      </div>
    `;
    document.documentElement.appendChild(root);

    // 首次 show 动画
    requestAnimationFrame(() => {
      setTimeout(() => root.querySelector('#freedom-vendor-bubble').classList.add('is-show'), 50);
    });

    // 关闭按钮：直接移除，本会话不再出现
    document.getElementById('fv-close').addEventListener('click', () => {
      const b = document.getElementById('freedom-vendor-bubble');
      b.classList.remove('is-show');
      setTimeout(() => { const r = document.getElementById('freedom-vendor-bubble-root'); if (r) r.remove(); }, 220);
    });

    document.getElementById('fv-btn-refresh').addEventListener('click', () => detectAndRender(true));
    document.getElementById('fv-btn-bind').addEventListener('click', (e) => {
      const btn = e.currentTarget;
      btn.disabled = true;
      const orig = btn.textContent;
      btn.textContent = '提交中…';
      (async () => {
        const res = await send('submitToProject', { type: meta.type });
        btn.disabled = false;
        btn.textContent = orig;
        if (checkExtensionDead(res.error)) return;
        if (res.ok) {
          btn.classList.remove('fv-btn-primary');
          btn.textContent = '✓ 已绑定成功';
          const statusEl = document.getElementById('fv-status');
          statusEl.className = 'fv-status fv-ok';
          document.getElementById('fv-status-text').textContent = 'Cookie 已同步到无限画布账户';
        } else {
          alert('绑定失败：\n' + (res.error || '未知错误'));
        }
      })().catch(() => { btn.disabled = false; btn.textContent = orig; });
    });
  }

  function setStatus(level, statusText, metaText, canBind) {
    const statusEl = document.getElementById('fv-status');
    if (statusEl) {
      statusEl.className = 'fv-status ' + (level === 'ok' ? 'fv-ok' : level === 'warn' ? 'fv-warn' : 'fv-none');
      const t = document.getElementById('fv-status-text');
      if (t) t.textContent = statusText;
    }
    const metaEl = document.getElementById('fv-meta');
    if (metaEl) metaEl.textContent = metaText;
    const bindBtn = document.getElementById('fv-btn-bind');
    if (bindBtn) bindBtn.disabled = !canBind;
  }

  async function detectAndRender(force) {
    renderBubble();
    // 触发采集（autoSubmit=false，因为我们允许用户点按钮再提交）
    const res = await send('collect', { type: meta.type, autoSubmit: false });
    if (checkExtensionDead(res.error)) return;
    const rec = res?.record;
    if (!rec || !rec?.cookieString) {
      setStatus('none', '尚未检测到登录 Cookie', '请先在本页完成登录，登录后点「重新检测」', false);
      return;
    }
    const hasCookie = Boolean(rec.cookieString && (rec.count ?? 0) > 0);
    // canBind：只要有 Cookie 就能点绑定 → 后端 BindVendorByCookie 会做真正的验真
    const canBind = hasCookie;
    if (rec.verified) {
      const userText = rec.nickname
        ? `${meta.name} 账户「${escapeHtml(rec.nickname)}」`
        : `${meta.name} 登录态`;
      const expire = rec.expireAt ? new Date(rec.expireAt).toLocaleString() : '会话级别';
      const softHint = rec.soft ? ' · 交由项目后端做最终验真' : '';
      setStatus('ok', rec.soft ? '已捕获登录态（软）' : '已捕获登录态', `${userText}${softHint} · 预估过期：${expire}`, canBind);
    } else {
      const why = rec.lastVerifyError ? ` 原因：${escapeHtml(rec.lastVerifyError)}` : '';
      // 有 cookie 但 verified=false（连 lenient 条件都不满足）：仍让用户可以点绑定（后端最后把关）
      setStatus('warn', hasCookie ? '检测到 Cookie 但插件端未判定登录态' : '尚未检测到登录 Cookie', `命中必要 Key：${rec.matchedNecessary ? '是' : '否'} · 共 ${rec.count} 条${why}${hasCookie ? '（仍可点绑定 → 后端严格验真）' : ''}`, canBind);
    }
  }

  // 仅在 UpDream / NewWow 站点挂请求嗅探（LibTV 走开放平台无需嗅探）
  if (CAPTURE_TYPES.has(meta.type)) installInterceptors();

  // 页面稳定后 1.5s 第一次检测；页面后台轮询也会触发，这里只负责 UI 展示
  window.addEventListener('load', () => setTimeout(() => detectAndRender(false), 1500), { once: true });

  // ========== 工具函数 ==========
  function escapeHtml(str) {
    return String(str ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }
})();
