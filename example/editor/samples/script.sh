#!/usr/bin/env bash
# SourceView — shell quoting, substitutions and heredocs
set -eu

project="${PROJECT:-SourceView}"
root="$(pwd)"
count=$((1 << 3))

if [ -d "$root" ]; then
    printf '%s: %s files\n' "$project" "$count"
fi

for language in JSON XML HTML TOML YAML Shell; do
    printf 'Language: %s\n' "$language"
done

literal='A backslash stays literal: \'
escaped=$'Unicode: \u4F60\u597D\n'

cat <<MESSAGE
Hello, $USER!
Working directory: ${root}
MESSAGE

cat <<'SCRIPT'
# Quoted heredocs preserve $variables and "quotes".
echo "$project"
SCRIPT
