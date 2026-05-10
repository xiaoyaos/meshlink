#!/usr/bin/env bash
# MeshLink — build desktop icon assets from SVG source

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ICON_DIR="${ROOT_DIR}/assets/icon"
SRC="${ICON_DIR}/meshlink-icon.svg"
PNG_1024="${ICON_DIR}/meshlink-icon-1024.png"
ICONSET="${ICON_DIR}/MeshLink.iconset"

if [[ ! -f "${SRC}" ]]; then
  echo "Missing icon source: ${SRC}" >&2
  exit 1
fi

if ! command -v rsvg-convert >/dev/null 2>&1; then
  echo "Missing rsvg-convert; install librsvg before building icons." >&2
  exit 1
fi

if ! python3 - <<'PY' >/dev/null 2>&1
from PIL import Image
PY
then
  echo "Missing Python Pillow; cannot build desktop icon formats." >&2
  exit 1
fi

mkdir -p "${ICON_DIR}" "${ICONSET}"

rsvg-convert -w 1024 -h 1024 "${SRC}" -o "${PNG_1024}"

python3 - "${PNG_1024}" "${ICON_DIR}" "${ICONSET}" <<'PY'
import sys
from pathlib import Path
from PIL import Image

src = Path(sys.argv[1])
icon_dir = Path(sys.argv[2])
iconset = Path(sys.argv[3])
base = Image.open(src).convert("RGBA")

iconset_sizes = {
    "icon_16x16.png": 16,
    "icon_16x16@2x.png": 32,
    "icon_32x32.png": 32,
    "icon_32x32@2x.png": 64,
    "icon_128x128.png": 128,
    "icon_128x128@2x.png": 256,
    "icon_256x256.png": 256,
    "icon_256x256@2x.png": 512,
    "icon_512x512.png": 512,
    "icon_512x512@2x.png": 1024,
}
for name, size in iconset_sizes.items():
    base.resize((size, size), Image.Resampling.LANCZOS).save(iconset / name)

sizes = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
base.save(icon_dir / "meshlink-icon.ico", sizes=sizes)
base.save(icon_dir / "meshlink-icon.icns", sizes=[
    (16, 16), (32, 32), (64, 64), (128, 128), (256, 256), (512, 512), (1024, 1024),
])
PY

rm -rf "${ICONSET}"
echo "Icon assets ready in ${ICON_DIR}"
