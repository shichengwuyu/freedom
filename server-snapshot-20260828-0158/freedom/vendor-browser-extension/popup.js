// popup.js — 插件弹窗脚本（MV3）
// 极简：采集 Cookie → 复制到剪贴板 → 用户粘贴到项目里绑定
// 不需要配置 API Base / Token / 自动提交

const VENDOR_META = {
  updream: { icon: '🚀', name: 'UpDream 云端', loginPage: 'https://www.updream.cn/' },
  libtv:   { icon: '📺', name: 'LibTV 云端',   loginPage: 'https://www.liblib.tv/' },
  newwow:  { icon: '✨', name: 'NewWow 云端',  loginPage: 'https://neowow.cn/neo-tv' },
};

function send(cmd, payload = {}) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ cmd, ...payload }, (resp) => resolve(resp || { ok: false, error: '无响应' }));
  });
}

function renderVendorList(records) {
  const list = document.getElementById('vendorList');
  const order = ['updream', 'libtv', 'newwow'];
  list.innerHTML = order.map((type) => renderVendorCard(type, records?.[type])).join('');
  for (const type of order) {
    bindVendorCardEvents(type);
  }
}

function statusTag(record) {
  if (!record || !record.cookieString || record.count === 0) {
    return '<span class="status-tag status-none"><span class="dot"></span>未采集</span>';
  }
  if (record.verified) {
    const userText = record.nickname ? record.nickname : '登录成功';
    return `<span class="status-tag status-ok"><span class="dot"></span>已登录 · ${userText}</span>`;
  }
  const why = record.lastVerifyError ? ` · ${record.lastVerifyError}` : '';
  return `<span class="status-tag status-warn"><span class="dot"></span>未登录${why}</span>`;
}

function renderVendorCard(type, record) {
  const meta = VENDOR_META[type];
  const capturedAt = record?.capturedAt ? new Date(record.capturedAt).toLocaleString() : '—';
  const count = record?.count ?? 0;
  const cookieCountLabel = count ? `${count} 条 Cookie` : '0 条';
  const avatar = record?.avatar
    ? `<img class="avatar" src="${escapeAttr(record.avatar)}" alt="" onerror="this.style.display='none'" />`
    : '';
  const nickname = record?.nickname ? escapeHtml(record.nickname) : '';
  const expire = record?.expireAt ? new Date(record.expireAt).toLocaleString() : '—';

  return `
    <div class="vendor-card" data-type="${type}">
      <div class="vendor-icon">${meta.icon}</div>
      <div class="vendor-main">
        <div class="name">${meta.name} ${statusTag(record)}</div>
        <div class="meta">采集：${escapeHtml(capturedAt)} · ${cookieCountLabel} · 预估过期：${escapeHtml(expire)}</div>
        <div class="user">
          ${avatar}
          <span>${nickname ? nickname : '（还没有登录态）'}</span>
        </div>
      </div>
      <div class="vendor-actions">
        <button class="btn btn-sm btn-ghost" data-action="openLogin" title="在新标签页打开对应平台登录页">去登录</button>
        <button class="btn btn-sm" data-action="collect" title="立即按域采集该平台当前 Cookie">立即采集</button>
        <button class="btn btn-sm btn-primary" data-action="copy" title="复制 Cookie 到剪贴板，粘贴到无限画布项目里绑定" ${(record?.cookieString && (record?.count ?? 0) > 0) ? '' : 'disabled'}>复制 Cookie</button>
      </div>
    </div>
  `;
}

function bindVendorCardEvents(type) {
  const root = document.querySelector(`.vendor-card[data-type="${type}"]`);
  if (!root) return;
  const actions = {
    openLogin: async () => {
      await send('openLogin', { type });
      window.close();
    },
    collect: async (btn) => {
      btn.disabled = true; btn.innerHTML = '<span class="spinner"></span> 采集中...';
      const res = await send('collect', { type });
      await refreshStatus();
      if (!res.ok) alert('采集失败：' + (res.error || '未知错误'));
    },
    copy: async (btn) => {
      const res = await send('getStatus', { type });
      const cookie = res?.record?.cookieString;
      if (!cookie) { alert('当前还没有采集到 Cookie，先点「立即采集」'); return; }
      try {
        await navigator.clipboard.writeText(cookie);
        const orig = btn.textContent;
        btn.textContent = '✓ 已复制';
        setTimeout(() => (btn.textContent = orig), 1800);
      } catch (e) {
        alert('复制失败，手动选择下方文本：\n' + cookie);
      }
    },
  };
  root.querySelectorAll('[data-action]').forEach((btn) => {
    btn.addEventListener('click', () => actions[btn.dataset.action](btn));
  });
}

async function refreshStatus() {
  const res = await send('getStatus');
  renderVendorList(res?.records || {});
}

// ============ 连接设置 ============
async function loadSettings() {
  const res = await send('getSettings');
  const s = res?.settings || {};
  const baseEl = document.getElementById('settingBase');
  const tokenEl = document.getElementById('settingToken');
  if (baseEl && s.backendBase) baseEl.value = s.backendBase;
  if (tokenEl && s.backendToken) tokenEl.value = s.backendToken;
}

async function saveSettings() {
  const baseEl = document.getElementById('settingBase');
  const tokenEl = document.getElementById('settingToken');
  const hint = document.getElementById('settingsHint');
  const base = (baseEl?.value || '').trim().replace(/\/+$/, '');
  const token = (tokenEl?.value || '').trim();
  if (!base || !token) {
    if (hint) hint.textContent = 'Web 地址和 Token 都要填';
    return;
  }
  const res = await send('setSettings', { settings: { backendBase: base, backendToken: token } });
  if (hint) hint.textContent = res?.ok ? '✓ 已保存' : ('保存失败：' + (res?.error || ''));
  setTimeout(() => { if (hint) hint.textContent = ''; }, 2500);
}

// ============ 生成样本嗅探 ============
const SAMPLE_TYPES = ['updream', 'newwow'];

async function refreshSamples() {
  const res = await send('getSamples');
  const all = res?.samples || {};
  const list = document.getElementById('sampleList');
  if (!list) return;
  list.innerHTML = SAMPLE_TYPES.map((type) => {
    const arr = all[type] || [];
    const meta = VENDOR_META[type];
    const genCount = arr.filter((s) => isGenSample(s)).length;
    return `
      <div class="sample-row" data-type="${type}">
        <div class="sample-meta">
          <span class="sample-name">${meta?.icon || ''} ${escapeHtml(meta?.name || type)}</span>
          <span class="sample-count">${arr.length} 条（生成类 ${genCount}）</span>
        </div>
        <div class="sample-actions">
          <button class="btn btn-sm btn-primary" data-action="pushSample">推送</button>
          <button class="btn btn-sm btn-ghost" data-action="clearSample">清空</button>
        </div>
      </div>`;
  }).join('');
  SAMPLE_TYPES.forEach((type) => {
    const root = list.querySelector(`.sample-row[data-type="${type}"]`);
    if (!root) return;
    root.querySelector('[data-action="pushSample"]').addEventListener('click', () => pushSamples(type));
    root.querySelector('[data-action="clearSample"]').addEventListener('click', () => clearSamples(type));
  });
}

function isGenSample(s) {
  const hay = String(s.url || '').toLowerCase() + ' ' + String(s.requestBody || '').toLowerCase();
  const kws = ['image', 'generate', 'gen', 'draw', 'txt2img', 'img2img', 'creation', 'task', 'prompt', 'diffusion', 'paint', 'render', 'upscale', 'imagine'];
  return kws.some((k) => hay.includes(k));
}

async function pushSamples(type) {
  const res = await send('pushSamplesToBackend', { type });
  showPushResult(res);
}

async function clearSamples(type) {
  await send('clearSamples', { type });
  await refreshSamples();
}

function showPushResult(res) {
  const el = document.getElementById('pushResult');
  if (!el) return;
  if (!res || !res.ok) {
    el.className = 'push-result push-fail';
    el.textContent = '✗ ' + (res?.error || '推送失败');
    return;
  }
  el.className = 'push-result push-ok';
  let msg = `✓ 成功推送 ${res.pushed} 条`;
  if (res.failed > 0) msg += `，失败 ${res.failed} 条`;
  if (res.errors && res.errors.length) msg += '：' + res.errors.slice(0, 3).join('；');
  el.textContent = msg;
  setTimeout(() => { if (el) el.textContent = ''; }, 4000);
}

document.addEventListener('DOMContentLoaded', async () => {
  await refreshStatus();
  // 打开弹窗时自动采集一次
  try { await send('collectAll'); await refreshStatus(); } catch (_) {}
  // 连接设置
  try { await loadSettings(); } catch (_) {}
  const saveBtn = document.getElementById('saveSettings');
  if (saveBtn) saveBtn.addEventListener('click', () => saveSettings());
  const pushAll = document.getElementById('pushAllSamples');
  if (pushAll) pushAll.addEventListener('click', () => pushSamples());
  const clearAll = document.getElementById('clearAllSamples');
  if (clearAll) clearAll.addEventListener('click', () => clearSamples());
  // 样本
  try { await refreshSamples(); } catch (_) {}
});

function escapeHtml(str) {
  return String(str ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
function escapeAttr(str) { return escapeHtml(str); }
