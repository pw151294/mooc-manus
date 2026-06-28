#!/usr/bin/env bash
# 从 rules/ 拼装 .cursorrules
set -euo pipefail

HARNESS_ROOT="${HARNESS_ROOT:-.harness}"
OUTPUT="${OUTPUT:-.cursorrules}"

{
  echo "# Auto-generated from $HARNESS_ROOT/rules/ - DO NOT EDIT MANUALLY"
  echo "# Last updated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo ""
  # 父仓继承（如 inherits）
  inherits=$(yq e '.inherits[0].path // ""' "$HARNESS_ROOT/manifest.yaml")
  if [ -n "$inherits" ]; then
    echo "## Inherited from $inherits"
    parent_manifest="$HARNESS_ROOT/$inherits/manifest.yaml"
    if [ -f "$parent_manifest" ]; then
      yq e '.cognition.loadOrder[]' "$parent_manifest" | while read -r f; do
        [ -z "$f" ] && continue
        echo ""
        echo "### From parent: $f"
        cat "$HARNESS_ROOT/$inherits/$f"
      done
    fi
  fi
  echo ""
  echo "## Local rules"
  yq e '.cognition.loadOrder[]' "$HARNESS_ROOT/manifest.yaml" | while read -r f; do
    [ -z "$f" ] && continue
    echo ""
    echo "### $f"
    cat "$HARNESS_ROOT/$f"
  done
} > "$OUTPUT"

echo "✅ Regenerated $OUTPUT"
