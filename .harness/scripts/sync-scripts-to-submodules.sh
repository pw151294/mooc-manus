#!/usr/bin/env bash
# 把总仓 .harness/scripts 与 hooks 同步到两子仓（保证三仓内容一致）
# 用法：
#   ./sync-scripts-to-submodules.sh           # 默认：直接同步（写）
#   ./sync-scripts-to-submodules.sh --check   # CI 模式：仅检查 diff，不写
set -euo pipefail

MODE="write"
case "${1:-}" in
  --check) MODE="check" ;;
  "") ;;
  *) echo "unknown arg: $1"; exit 2 ;;
esac

diff_count=0
for sub in mooc-manus mooc-manus-web; do
  if [ "$MODE" = "check" ]; then
    if ! diff -rq --exclude=validate-contracts.sh .harness/scripts/ "$sub/.harness/scripts/" >/dev/null; then
      echo "❌ scripts/ 在 $sub 不同步"; diff_count=$((diff_count+1))
    fi
    if ! diff -rq .harness/hooks/ "$sub/.harness/hooks/" >/dev/null; then
      echo "❌ hooks/ 在 $sub 不同步"; diff_count=$((diff_count+1))
    fi
  else
    rsync -a --delete --exclude validate-contracts.sh .harness/scripts/ "$sub/.harness/scripts/"
    rsync -a --delete .harness/hooks/ "$sub/.harness/hooks/"
    echo "✅ Synced to $sub"
  fi
done

if [ "$MODE" = "check" ]; then
  if [ $diff_count -ne 0 ]; then
    echo "❌ 有 $diff_count 处不同步，请在总仓跑 sync-scripts-to-submodules.sh 后重新提交"
    exit 1
  fi
  echo "✅ 三仓 scripts/hooks 一致"
else
  echo ""
  echo "Next: cd mooc-manus && git status; cd mooc-manus-web && git status"
fi
