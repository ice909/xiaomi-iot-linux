#!/usr/bin/env bash
# Refuse to shadow an updated distro driver with an older vendored baseline.
set -euo pipefail
release="${1:?kernel release required}"
[[ "$release" =~ ^[0-9][a-zA-Z0-9._+-]*$ ]] || exit 1
original=(/lib/modules/"$release"/kernel/drivers/platform/x86/bitland-mifs-wmi.ko*)
if [[ ${#original[@]} != 1 || ! -f "${original[0]}" ]]; then
    echo "Cannot locate the original Bitland driver for $release." >&2
    exit 1
fi
baseline=9A432DB17D1453AA4DD476C
if [[ "$(modinfo -F srcversion "${original[0]}")" != "$baseline" ]]; then
    echo "Bitland driver for $release differs from the reviewed baseline; refresh the IoT patch before building." >&2
    exit 1
fi
