/* 插件图标占位（SVG → PNG）说明
 *
 * 由于不想引入额外二进制资源，本目录提供两份方案任选其一，都满足 MV3 manifest 引用：
 *
 * 方案 A（推荐，零依赖）：运行一次 generate-icons.js 生成 16/32/48/128 PNG
 *   - 需要系统有 Node.js + 全局可用的 sharp 或 canvas
 *   - 或者直接用 PowerShell 自带的 SVG 转 PNG （见 generate-icons.powershell.ps1）
 *
 * 方案 B：直接把 4 张你自己的 logo.png 丢到 /icons 目录下，命名为：
 *   icon16.png  icon32.png  icon48.png  icon128.png
 *
 * 临时方案：开发期可以把 manifest.json 的 icons/action.default_icon 两行先注释掉再加载扩展，
 *          等最终上线再补 PNG。
 */

const fs = require('fs');
const path = require('path');

const OUT_DIR = path.join(__dirname, 'icons');
if (!fs.existsSync(OUT_DIR)) fs.mkdirSync(OUT_DIR, { recursive: true });

const SVG_SRC = `
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#6366f1"/>
      <stop offset="100%" stop-color="#8b5cf6"/>
    </linearGradient>
  </defs>
  <rect x="4" y="4" width="120" height="120" rx="26" fill="url(#g)"/>
  <g transform="translate(28,36)" fill="#fff" font-family="-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif" font-weight="700">
    <text x="4" y="30" font-size="34">☁️</text>
    <text x="2" y="68" font-size="28" fill="#fff">Cookie</text>
  </g>
</svg>`;

// 只写入 SVG 源文件；实际 PNG 请按上文说明生成
fs.writeFileSync(path.join(OUT_DIR, 'source.svg'), SVG_SRC.trim(), 'utf8');

// 如果宿主环境有 sharp / canvas，自动尝试生成（失败则跳过）
async function tryGenerateWithSharpOrCanvas() {
  try {
    const sharp = require('sharp');
    const sizes = [16, 32, 48, 128];
    for (const size of sizes) {
      await sharp(Buffer.from(SVG_SRC.trim()))
        .resize(size, size)
        .png()
        .toFile(path.join(OUT_DIR, `icon${size}.png`));
    }
    console.log('[OK] vendor-browser-extension/icons/*.png 已用 sharp 生成');
    return true;
  } catch (_) { /* 没装 sharp，fallback 下面 canvas */ }
  try {
    const { createCanvas, loadImage } = require('canvas');
    const sizes = [16, 32, 48, 128];
    // canvas 不能直接解析 SVG，改用纯色渐变 + 文字简易绘制
    for (const size of sizes) {
      const canvas = createCanvas(size, size);
      const ctx = canvas.getContext('2d');
      const grd = ctx.createLinearGradient(0, 0, size, size);
      grd.addColorStop(0, '#6366f1'); grd.addColorStop(1, '#8b5cf6');
      ctx.fillStyle = grd;
      roundRect(ctx, 0, 0, size, size, Math.max(2, Math.floor(size * 0.22)));
      ctx.fill();
      ctx.fillStyle = '#ffffff';
      ctx.font = `${Math.floor(size * 0.78)}px -apple-system, Segoe UI Emoji, Apple Color Emoji, sans-serif`;
      ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
      ctx.fillText('☁️', size / 2, size / 2 + 1);
      const out = fs.createWriteStream(path.join(OUT_DIR, `icon${size}.png`));
      const stream = canvas.createPNGStream();
      await new Promise((resolve, reject) => {
        stream.pipe(out);
        out.on('finish', resolve);
        out.on('error', reject);
      });
    }
    console.log('[OK] vendor-browser-extension/icons/*.png 已用 node-canvas 生成');
    return true;
  } catch (e) {
    console.warn('[SKIP] sharp / node-canvas 均不可用，跳过 PNG 生成。请按 README 说明放置 4 张 iconXX.png。', e.message);
    return false;
  }
}

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y,     x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x,     y + h, r);
  ctx.arcTo(x,     y + h, x,     y,     r);
  ctx.arcTo(x,     y,     x + w, y,     r);
  ctx.closePath();
}

tryGenerateWithSharpOrCanvas().catch(() => process.exit(0));
