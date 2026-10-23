#!/bin/bash
# Runs an amd64 binary on an emulated baseline x86-64 CPU (qemu64: no AVX2, BMI2, MOVBE), to catch
# statically linked C code (glibc, SQLite) that was built for newer CPUs, see #2092.
#
# Fails without qemu-x86_64 in CI (CI is set by GitHub Actions), skips with a warning elsewhere.
set -euo pipefail

binary="$1"
if ! command -v qemu-x86_64 >/dev/null; then
  if [ -n "${CI:-}" ]; then
    echo "ERROR: qemu-x86_64 not found, cannot check $binary on a baseline x86-64 CPU. On Ubuntu, run: apt install qemu-user"
    exit 1
  fi
  echo "WARNING: qemu-x86_64 not found, skipping baseline x86-64 CPU check of $binary"
  exit 0
fi
if ! qemu-x86_64 -cpu qemu64 "$binary" --help >/dev/null; then
  echo "ERROR: $binary does not run on a baseline x86-64 CPU (qemu64), it likely contains x86-64-v3 instructions (see #2092)"
  exit 1
fi
echo "$binary runs on a baseline x86-64 CPU (qemu64)"
