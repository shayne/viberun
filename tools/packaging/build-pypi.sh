#!/usr/bin/env bash
# Copyright (c) 2025 AUTHORS All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SRC_DIR="${ROOT_DIR}/packaging/pypi"
VENDOR_DIR="${ROOT_DIR}/dist/vendor"
STAGE_DIR="${ROOT_DIR}/dist/pypi-src"
OUT_DIR="${ROOT_DIR}/dist/pypi"

: "${VERSION:?VERSION is required}"
if [ ! -d "$VENDOR_DIR" ]; then
  echo "missing vendor dir: $VENDOR_DIR (run tools/packaging/build-vendor.sh first)" >&2
  exit 1
fi
PACKAGE_VERSION="${VERSION#v}"
export PACKAGE_VERSION

rm -rf "$STAGE_DIR" "$OUT_DIR"
mkdir -p "$STAGE_DIR" "$OUT_DIR"

cp -R "${SRC_DIR}/"* "$STAGE_DIR/"
cp "${ROOT_DIR}/README.md" "$STAGE_DIR/README.md"
cp "${ROOT_DIR}/LICENSE" "$STAGE_DIR/"
mkdir -p "$STAGE_DIR/viberun"
cp -R "$VENDOR_DIR" "$STAGE_DIR/viberun/vendor"

export STAGE_DIR

python3 - <<'PY'
import pathlib
import re
import os

stage = pathlib.Path(os.environ['STAGE_DIR'])
pyproject = stage / 'pyproject.toml'
text = pyproject.read_text()
text = re.sub(r'version = "[^"]+"', f'version = "{os.environ["PACKAGE_VERSION"]}"', text)
pyproject.write_text(text)
PY

if command -v uv >/dev/null 2>&1; then
  uv run --with build -- python -m build --outdir "$OUT_DIR" "$STAGE_DIR"
else
  VENV_DIR="${ROOT_DIR}/dist/pypi-venv"
  python3 -m venv "$VENV_DIR"
  # shellcheck disable=SC1090
  source "$VENV_DIR/bin/activate"
  python3 -m pip install build
  python3 -m build --outdir "$OUT_DIR" "$STAGE_DIR"
fi
