const fs = require('fs');

// Fix video.ts
let data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', 'utf8');
data = data.replace(/String\.raw`([\s\S]*?)`\s*\.\s*trim\(\)/, (match, content) => {
  return 'String.raw`' + content.replace(/`/g, '\\`') + '`\\n.trim();';
});
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', data, 'utf8');
console.log('Fixed video.ts');

// Fix image.ts
data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', 'utf8');
data = data.replace(/String\.raw`([\s\S]*?)`\s*\.\s*trim\(\)/, (match, content) => {
  return 'String.raw`' + content.replace(/`/g, '\\`') + '`\\n.trim();';
});
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', data, 'utf8');
console.log('Fixed image.ts');

// Verify
for (const file of ['video.ts', 'image.ts']) {
  const fpath = 'F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/' + file;
  const d = fs.readFileSync(fpath, 'utf8');
  const lines = d.split('\n');
  let count = 0;
  for (const line of lines) {
    count += (line.match(/[^\\]`/g) || []).length;
  }
  console.log(file + ': remaining unescaped backticks =', count);
}
