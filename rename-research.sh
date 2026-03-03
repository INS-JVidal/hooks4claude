#!/usr/bin/env bash
set -euo pipefail

# rename-research.sh — Add YYYYMMDD-HHMM_ datetime prefix to research files
# based on filesystem mtime. Uses git mv to preserve history.
#
# Usage:
#   bash rename-research.sh            # dry-run (prints mapping)
#   bash rename-research.sh --execute  # apply renames + update cross-refs

RESEARCH_DIR="research"
EXECUTE=false

if [[ "${1:-}" == "--execute" ]]; then
    EXECUTE=true
fi

# Verify we're at the repo root
if [[ ! -d "$RESEARCH_DIR" ]]; then
    echo "Error: $RESEARCH_DIR/ not found. Run from repo root." >&2
    exit 1
fi

# Build rename mapping: old_basename → new_basename
declare -A RENAME_MAP
rename_count=0

for filepath in "$RESEARCH_DIR"/*.md; do
    basename="$(basename "$filepath")"

    # Skip README.md
    [[ "$basename" == "README.md" ]] && continue

    # Skip files already prefixed with YYYYMMDD-HHMM_
    if [[ "$basename" =~ ^[0-9]{8}-[0-9]{4}_ ]]; then
        echo "skip (already prefixed): $basename"
        continue
    fi

    # Extract mtime as epoch, format as YYYYMMDD-HHMM
    epoch="$(stat -c '%Y' "$filepath")"
    prefix="$(date -d "@$epoch" '+%Y%m%d-%H%M')"

    new_basename="${prefix}_${basename}"
    RENAME_MAP["$basename"]="$new_basename"
    ((rename_count++))
done

# Early exit if nothing to rename
if [[ $rename_count -eq 0 ]]; then
    echo ""
    echo "Nothing to rename (all files already prefixed or skipped)."
    exit 0
fi

# Print mapping table
echo ""
echo "=== Rename Mapping ==="
echo ""
printf "%-50s → %s\n" "OLD" "NEW"
printf "%-50s   %s\n" "---" "---"
for old in $(echo "${!RENAME_MAP[@]}" | tr ' ' '\n' | sort); do
    printf "%-50s → %s\n" "$old" "${RENAME_MAP[$old]}"
done
echo ""
echo "Total: ${#RENAME_MAP[@]} files"
echo ""

if [[ "$EXECUTE" == false ]]; then
    echo "(dry-run mode — pass --execute to apply)"
    exit 0
fi

echo "=== Applying renames ==="

# Step 1: git mv all files
for old in $(echo "${!RENAME_MAP[@]}" | tr ' ' '\n' | sort); do
    new="${RENAME_MAP[$old]}"
    echo "  git mv $RESEARCH_DIR/$old → $RESEARCH_DIR/$new"
    git mv "$RESEARCH_DIR/$old" "$RESEARCH_DIR/$new"
done

# Step 2: Update cross-references in all markdown files
echo ""
echo "=== Updating cross-references ==="

# Collect all .md files that might contain references
mapfile -t md_files < <(find . -name '*.md' \
    -not -path './.git/*' \
    -not -path './claude-hooks-monitor/*' \
    -not -path './hooks-store/*' \
    -not -path './hooks-mcp/*' \
    -not -path './.claude/*')

for old in $(echo "${!RENAME_MAP[@]}" | tr ' ' '\n' | sort); do
    new="${RENAME_MAP[$old]}"
    for md in "${md_files[@]}"; do
        # Replace both relative refs (within research/) and path refs (research/filename)
        if grep -q "$old" "$md" 2>/dev/null; then
            sed -i "s|$old|$new|g" "$md"
            echo "  updated ref in $md: $old → $new"
        fi
    done
done

echo ""
echo "=== Done ==="
echo "Run 'git status' and 'git diff --staged' to review changes."
