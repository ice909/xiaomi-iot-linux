#!/usr/bin/env python3
"""Render the approved SVG masters with librsvg; package PNGs without resizing.

Run from any directory: python3 scripts/export-icons.py
Requires rsvg-convert. Uses only Python's standard library.
ICNS PNG chunk mapping follows Pillow's IcnsImagePlugin:
https://github.com/python-pillow/Pillow/blob/main/src/PIL/IcnsImagePlugin.py
"""
from pathlib import Path
import shutil
import struct
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ICONS = ROOT / "resources/icons"
SIZES = (16, 24, 32, 48, 64, 128, 256, 512, 1024)


def render(source, destination, size):
    destination.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["rsvg-convert", "--width", str(size), "--height", str(size),
                    "--output", str(destination), str(source)], check=True)


def main():
    if not shutil.which("rsvg-convert"):
        raise SystemExit("Install librsvg / rsvg-convert before exporting icons.")
    master = ROOT / "resources/xiaomi-iot-setup.svg"
    small = ICONS / "xiaomi-iot-setup-small.svg"
    for size in SIZES:
        render(small if size in (16, 32) else master,
               ICONS / "png" / f"{size}.png", size)

    # Modern Windows ICO: preserve each independently rendered optical size.
    sizes = [s for s in SIZES if s <= 256]
    offset = 6 + 16 * len(sizes)
    directory, pixels = bytearray(), bytearray()
    for size in sizes:
        data = (ICONS / "png" / f"{size}.png").read_bytes()
        directory.extend(struct.pack("<BBBBHHII", size % 256, size % 256,
                                     0, 0, 1, 32, len(data), offset))
        pixels.extend(data)
        offset += len(data)
    (ICONS / "xiaomi-iot-setup.ico").write_bytes(
        struct.pack("<HHH", 0, 1, len(sizes)) + directory + pixels)

    # Retina variants use the cut for their logical size, not their pixel size.
    iconset = ICONS / "xiaomi-iot-setup.iconset"
    chunks = bytearray()
    entries = (("icp4", 16, 1), ("ic11", 16, 2),
               ("icp5", 32, 1), ("ic12", 32, 2),
               ("ic07", 128, 1), ("ic13", 128, 2),
               ("ic08", 256, 1), ("ic14", 256, 2),
               ("ic09", 512, 1), ("ic10", 512, 2))
    for tag, logical, scale in entries:
        suffix = "@2x" if scale == 2 else ""
        path = iconset / f"icon_{logical}x{logical}{suffix}.png"
        render(small if logical <= 32 else master, path, logical * scale)
        data = path.read_bytes()
        chunks.extend(struct.pack(">4sI", tag.encode("ascii"), len(data) + 8))
        chunks.extend(data)
    (ICONS / "xiaomi-iot-setup.icns").write_bytes(
        struct.pack(">4sI", b"icns", len(chunks) + 8) + chunks)
    print("Exported 9 PNG sizes, 7-size ICO, 10-slot ICNS and editable iconset.")


if __name__ == "__main__":
    main()
