#!/usr/bin/env bash
# 校验 .harness 自身完整性
# 退出码：0 通过，非 0 失败
set -euo pipefail

command -v yq >/dev/null 2>&1 || { echo "❌ 需要 yq (mikefarah/yq v4+)。安装：brew install yq / apt install yq"; exit 2; }

HARNESS_ROOT="${HARNESS_ROOT:-.harness}"
fail=0

# 1. manifest 存在
if [ ! -f "$HARNESS_ROOT/manifest.yaml" ]; then
  echo "❌ Missing $HARNESS_ROOT/manifest.yaml"; fail=1
fi

# 2. 必备目录
for d in rules knowledge playbooks specs plans retro agents hooks scripts; do
  [ -d "$HARNESS_ROOT/$d" ] || { echo "❌ Missing dir $HARNESS_ROOT/$d"; fail=1; }
done

# 3. manifest loadOrder 中每个 rules 文件存在
while read -r f; do
  [ -z "$f" ] && continue
  [ -f "$HARNESS_ROOT/$f" ] || { echo "❌ loadOrder references missing $f"; fail=1; }
done < <(yq e '.cognition.loadOrder[]' "$HARNESS_ROOT/manifest.yaml" 2>/dev/null)

# 4. rules 文件命名 NN-kebab-case
for f in "$HARNESS_ROOT/rules"/*.md; do
  [ -f "$f" ] || continue
  name=$(basename "$f")
  if ! [[ "$name" =~ ^[0-9]{2}-[a-z0-9-]+\.md$ ]]; then
    echo "❌ rules naming violates NN-kebab-case: $name"; fail=1
  fi
done

# 5. inherits 路径有效（如有）
inherits=$(yq e '.inherits[0].path // ""' "$HARNESS_ROOT/manifest.yaml")
if [ -n "$inherits" ] && [ ! -d "$HARNESS_ROOT/$inherits" ]; then
  echo "❌ inherits path not found: $inherits"; fail=1
fi

# 6. 桥接层 GENERATED 区段同步状态（hash 校验）
# （由 sync-bridges --check 子命令做；这里仅占位）

if [ $fail -eq 0 ]; then
  echo "✅ Harness valid"
else
  echo "❌ Harness validation failed"
  exit 1
fi
