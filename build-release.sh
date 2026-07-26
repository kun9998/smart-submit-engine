#!/usr/bin/env bash
# 发版构建（推荐）
# 说明：本项目依赖 go.starlark.net，其 syntax↔resolve 使用跨包 //go:linkname 且不能互相 import，
# 与 garble 的 linkname 处理不兼容（会报 linkname 或 import cycle）。请用本脚本普通编译发版。
set -euo pipefail

cd "$(dirname "$0")"

export CGO_ENABLED=0

go build \
	-trimpath \
	-ldflags="-s -w" \
	-buildvcs=false \
	-o tj \
	.

echo "OK: $(pwd)/tj"
file tj
