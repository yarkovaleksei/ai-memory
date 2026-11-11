#!/usr/bin/env bash
# Сверка вложений релиза GitHub vX.Y.Z по метаданным API (ADR-030): у каждого
# ожидаемого файла state=uploaded и digest SHA-256 совпадает с эталоном.
# Эталон пакетов Linux — локальные файлы bin/release/<версия>/, пакетов macOS
# и Windows — их SHA256SUMS-<система>.txt из того же релиза, который
# посчитал CI. Сами пакеты обратно не скачиваются.
#
#   bash scripts/release-verify.sh X.Y.Z
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
ver="${1:?версия X.Y.Z}"
tag="v$ver"
local_dir="bin/release/$ver"

repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
id="$(gh release view "$tag" --json databaseId -q .databaseId)"
draft="$(gh release view "$tag" --json isDraft -q .isDraft)"
# имя → «digest state size id»
declare -A meta
while IFS=$'\t' read -r name digest state size aid; do
  meta["$name"]="${digest#sha256:} $state $size $aid"
done < <(gh api "repos/$repo/releases/$id" --jq '.assets[] | [.name, (.digest // ""), .state, (.size|tostring), (.id|tostring)] | @tsv')

# Полный состав релиза (README, «Установка»): имя → ожидаемый sha256, пусто —
# эталона нет, проверяются только наличие, state и digest.
declare -A want
for name in "ai-memory_${ver}_amd64.deb" "ai-memory-${ver}.x86_64.rpm" "ai-memory-${ver}-x86_64.pkg.tar.zst" \
  "ai-memory-${ver}-macos-universal.dmg" "ai-memory-${ver}-windows-amd64-setup.exe" \
  SHA256SUMS-linux.txt SHA256SUMS-macos.txt SHA256SUMS-windows.txt; do
  want["$name"]=""
done
add_sums() { # содержимое SHA256SUMS → эталон
  # sha256sum из Git Bash на Windows пишет имя в двоичном режиме: «*имя».
  while read -r sum name; do
    name="${name#\*}" name="${name%$'\r'}"
    [[ -n "$name" ]] && want["$name"]="$sum"
  done
  return 0
}
# Linux — по локальным файлам, из которых собран релиз.
if [[ -f "$local_dir/SHA256SUMS-linux.txt" ]]; then
  for f in "$local_dir"/*; do want["$(basename -- "$f")"]="$(sha256sum -- "$f" | cut -d' ' -f1)"; done
else
  echo "Нет $local_dir: пакеты Linux сверяю только по наличию и digest в API" >&2
fi
# macOS и Windows — по суммам, которые выложил CI.
for os in macos windows; do
  sums="SHA256SUMS-$os.txt"
  if [[ -n "${meta[$sums]:-}" ]]; then
    aid="${meta[$sums]##* }"
    add_sums < <(gh api -H "Accept: application/octet-stream" "repos/$repo/releases/assets/$aid")
  fi
done

bad=0
printf '%-48s %s\n' "Файл" "Результат"
for name in $(printf '%s\n' "${!want[@]}" | sort); do
  m="${meta[$name]:-}"
  if [[ -z "$m" ]]; then res="НЕТ В РЕЛИЗЕ"; bad=1
  else
    read -r digest state _ _ <<< "$m"
    if [[ "$state" != uploaded ]]; then res="state=$state"; bad=1
    elif [[ -z "$digest" ]]; then res="нет digest в API"; bad=1
    elif [[ -n "${want[$name]}" && "${want[$name]}" != "$digest" ]]; then res="DIGEST НЕ СОВПАДАЕТ"; bad=1
    else res="ok"; fi
  fi
  printf '%-48s %s\n' "$name" "$res"
done
for name in "${!meta[@]}"; do
  [[ -v "want[$name]" ]] || { printf '%-48s %s\n' "$name" "лишний файл"; bad=1; }
done
echo "Релиз $tag: $([[ $draft == true ]] && echo черновик || echo опубликован)"
exit "$bad"
