#!/usr/bin/env bash
# Enumerate the running kernel and every kernel selected by mkinitcpio presets.
set -euo pipefail
preset_dir=${1:-/etc/mkinitcpio.d}
# Reuse mkinitcpio's image-version parser instead of guessing from filenames.
source /usr/lib/initcpio/functions
declare -A seen=()
emit() {
    [[ "$1" =~ ^[0-9][a-zA-Z0-9._+-]*$ ]] || { echo "无效内核版本：$1" >&2; return 1; }
    if [[ -z ${seen[$1]:-} ]]; then printf '%s\n' "$1"; seen[$1]=1; fi
}
emit "$(uname -r)"
for preset in "$preset_dir"/*.preset; do
    [[ -f "$preset" ]] || continue
    images=$(set +u; unset ALL_kver; PRESETS=(); source "$preset"
        for name in "${PRESETS[@]}"; do
            [[ "$name" =~ ^[a-zA-Z0-9_]+$ ]] || exit 1
            key=${name}_kver
            image=${!key:-${ALL_kver:-}}
            [[ -n "$image" ]] || { echo "缺少内核配置：$preset/$name" >&2; exit 1; }
            printf '%s\n' "$image"
        done)
    while IFS= read -r image; do
        [[ -n "$image" ]] || continue
        if [[ "$image" == /* ]]; then
            version=$(kver "$image") || { echo "无法读取待启动内核：$image" >&2; exit 1; }
            emit "$version"
        else emit "$image"
        fi
    done <<< "$images"
done
