#!/usr/bin/env bash
# Garble 与 go.starlark.net 不兼容，见 build-release.sh。
# 若将来移除 Starlark 依赖，可再尝试 garble。
set -euo pipefail
echo "error: garble 与 go.starlark.net（syntax/resolve linkname）不兼容，请使用 ./build-release.sh" >&2
echo "  CGO_ENABLED=0 go build -trimpath -ldflags=\"-s -w\" -buildvcs=false -o tj ." >&2
exit 1
