#!/usr/bin/env bash
# Copyright (c) 2025 AUTHORS All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
OUT_DIR=${1:-"${ROOT_DIR}/dist/vendor"}

cd "$ROOT_DIR"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

ldflags=()
if [ -n "${VERSION:-}" ]; then
  ldflags+=("-X" "main.version=${VERSION}")
fi
if [ -n "${COMMIT:-}" ]; then
  ldflags+=("-X" "main.commit=${COMMIT}")
fi
if [ -n "${BUILD_DATE:-}" ]; then
  ldflags+=("-X" "main.buildDate=${BUILD_DATE}")
fi

build_target() {
  local goos=$1
  local goarch=$2
  local triple=$3
  local ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi
  local out="${OUT_DIR}/${triple}/viberun"
  mkdir -p "$out"
  if [ ${#ldflags[@]} -gt 0 ]; then
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -ldflags "${ldflags[*]}" -o "${out}/viberun${ext}" ./cmd/viberun
  else
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -o "${out}/viberun${ext}" ./cmd/viberun
  fi
}

build_target linux amd64 x86_64-unknown-linux-musl
build_target linux arm64 aarch64-unknown-linux-musl
build_target darwin amd64 x86_64-apple-darwin
build_target darwin arm64 aarch64-apple-darwin
build_target windows amd64 x86_64-pc-windows-msvc
build_target windows arm64 aarch64-pc-windows-msvc
