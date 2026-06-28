#!/usr/bin/env bash
# 一键安装 hooks 并校验 manifest
set -euo pipefail

command -v yq >/dev/null 2>&1 || { echo "❌ 需要 yq (mikefarah/yq v4+)。安装：brew install yq / apt install yq"; exit 2; }

HARNESS_ROOT="${HARNESS_ROOT:-.harness}"

# 设置 git hooks 路径
git config core.hooksPath "$HARNESS_ROOT/hooks"
echo "✅ git core.hooksPath = $HARNESS_ROOT/hooks"

# 确保 hooks 可执行
chmod +x "$HARNESS_ROOT/hooks/"*

# 跑一次 validate
"$HARNESS_ROOT/scripts/validate-harness.sh"

# 跑一次 sync-bridges（初始烘焙）
"$HARNESS_ROOT/scripts/sync-bridges.sh"

echo "✅ Harness bootstrap complete"
