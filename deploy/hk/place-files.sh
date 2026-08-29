#!/bin/bash
# 把暂存的改动文件放到源码树对应路径，落地前先备份原文件。
set -e
cd /root/freedom

STAMP=$(date +%Y%m%d-%H%M%S)
BK="/root/freedom-backup-$STAMP"
mkdir -p "$BK"
echo "备份目录: $BK"

place() {
  local src="$1"; local dst="$2"
  if [ ! -f "$src" ]; then echo "STAGE_MISSING $src"; exit 1; fi
  if [ -f "$dst" ]; then
    # 备份时保留相对路径结构
    local rel="${dst#/root/freedom/}"
    mkdir -p "$BK/$(dirname "$rel")"
    cp -a "$dst" "$BK/$rel"
  fi
  cp -a "$src" "$dst"
  echo "PLACED $dst"
}

place "/root/stage/novel-page.tsx"              "/root/freedom/web/src/app/(user)/novel/page.tsx"
place "/root/stage/canvas-node.tsx"             "/root/freedom/web/src/app/(user)/canvas/components/canvas-node.tsx"
place "/root/stage/canvas-assistant-panel.tsx"  "/root/freedom/web/src/app/(user)/canvas/components/canvas-assistant-panel.tsx"
place "/root/stage/canvas-client-page.tsx"      "/root/freedom/web/src/app/(user)/canvas/[id]/canvas-client-page.tsx"

echo "=== 校验落地内容（关键标记）==="
grep -q "activeChannelId: effectiveConfig.textChannelId" "/root/freedom/web/src/app/(user)/novel/page.tsx" && echo "novel: 渠道修复 OK" || echo "novel: 渠道修复 MISSING"
grep -q "onViewText" "/root/freedom/web/src/app/(user)/canvas/components/canvas-node.tsx" && echo "canvas-node: onViewText OK" || echo "canvas-node: onViewText MISSING"
grep -q "MessageActions" "/root/freedom/web/src/app/(user)/canvas/components/canvas-assistant-panel.tsx" && echo "assistant-panel: 复制按钮 OK" || echo "assistant-panel: 复制按钮 MISSING"
grep -q "textDetailNodeId" "/root/freedom/web/src/app/(user)/canvas/[id]/canvas-client-page.tsx" && echo "client-page: 文本详情弹窗 OK" || echo "client-page: 文本详情弹窗 MISSING"
grep -q "当前模型渠道被上游限流" "/root/freedom/handler/ai.go" && echo "ai.go: 429 文案 OK" || echo "ai.go: 429 文案 MISSING"
echo "PLACE_DONE"
