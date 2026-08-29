const fs = require('fs');

// Fix video.ts
let data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', 'utf8');
data = data.replace(/\n\.trim\(\);$/, '\n`.trim();');
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', data, 'utf8');
console.log('Fixed video.ts');

// Fix image.ts
data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', 'utf8');
data = data.replace(/\n\.trim\(\);$/, '\n`.trim();');
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', data, 'utf8');
console.log('Fixed image.ts');

// Verify
for (const file of ['video.ts', 'image.ts']) {
  const fpath = 'F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/' + file;
  const d = fs.readFileSync(fpath, 'utf8');
  const lines = d.split('\n');
  console.log('\n' + file + ' (last 5 lines):');
  lines.slice(-5).forEach((l, i) => console.log('  ' + (lines.length - 5 + i) + ': ' + l));
  // Count backticks
  let bt = 0;
  for (const c of d) if (c === '`') bt++;
  console.log('  Total backticks:', bt, '(should be even)');
}
