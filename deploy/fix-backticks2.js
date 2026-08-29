const fs = require('fs');

// Fix video.ts - remove the broken closing
let data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', 'utf8');
// Fix the broken ending
data = data.replace(/`\\n\.trim\(\);;/, '\n.trim();');
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/video.ts', data, 'utf8');
console.log('Fixed video.ts ending');

// Fix image.ts - remove the broken closing
data = fs.readFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', 'utf8');
data = data.replace(/`\\n\.trim\(\);;/, '\n.trim();');
fs.writeFileSync('F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/image.ts', data, 'utf8');
console.log('Fixed image.ts ending');

// Verify
for (const file of ['video.ts', 'image.ts']) {
  const fpath = 'F:/trae/wifi/infinite-canvas-main/web/src/app/(user)/canvas/agent/skills/' + file;
  const d = fs.readFileSync(fpath, 'utf8');
  const lines = d.split('\n');
  console.log('\n' + file + ' (last 3 lines):');
  lines.slice(-3).forEach((l, i) => console.log('  ' + (lines.length - 3 + i) + ': ' + l));
}
