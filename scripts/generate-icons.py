#!/usr/bin/env python3
"""Export app icons from SVG artwork. Requires Node.js with sharp, and Pillow.

Run from any directory: python3 scripts/generate-icons.py
Generated assets are checked in; ordinary app builds do not need these tools.
NODE and SHARP_MODULE may point to an existing Node.js runtime and sharp module.
"""

from io import BytesIO
from pathlib import Path
import os
import subprocess

from PIL import Image, ImageFilter


ROOT = Path(__file__).resolve().parents[1]
PUBLIC = ROOT / "frontend/public"
DESKTOP = ROOT / "build/desktop"


def render(svg: str, size: int) -> Image.Image:
    # sharp uses librsvg so gradients and SVG opacity survive rasterization.
    png = subprocess.check_output([
        os.environ.get("NODE", "node"), "-e",
        "const sharp = require(process.env.SHARP_MODULE || 'sharp');"
        "const fs = require('node:fs');"
        "sharp(fs.readFileSync(0), {density: 384})"
        ".resize(Number(process.argv[1])).png().toBuffer()"
        ".then(data => process.stdout.write(data))"
        ".catch(error => { console.error(error); process.exitCode = 1; });",
        str(size),
    ], input=svg.encode())
    return Image.open(BytesIO(png)).convert("RGBA")


def main() -> None:
    artwork = (PUBLIC / "app-icon.svg").read_text()
    tile = render(artwork, 1024)
    tile.save(DESKTOP / "app-icon.png")

    # macOS supplies no automatic mask: inset the rounded tile and its shadow.
    mac = Image.new("RGBA", (1024, 1024))
    inset = tile.resize((864, 864), Image.Resampling.LANCZOS)
    shadow = Image.new("RGBA", mac.size)
    shadow.paste((19, 12, 35, 70), (80, 96), inset.getchannel("A"))
    mac.alpha_composite(shadow.filter(ImageFilter.GaussianBlur(12)))
    mac.alpha_composite(inset, (80, 80))
    mac.save(DESKTOP / "app-icon.icns")

    # iOS applies its own corner mask and requires an opaque square image.
    square = artwork.replace('rx="56"', 'rx="0"').replace('rx="55"', 'rx="0"')
    ios = render(square, 1024).convert("RGB")
    ios.save(ROOT / "ios/WRDAISSH/Assets.xcassets/AppIcon.appiconset/AppIcon.png")
    ios.resize((180, 180), Image.Resampling.LANCZOS).save(PUBLIC / "apple-touch-icon.png")

    # Use simplified artwork at browser-tab sizes to preserve legibility.
    favicon = render((PUBLIC / "favicon.svg").read_text(), 256)
    favicon.save(PUBLIC / "favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])
    tile.save(DESKTOP / "app-icon.ico", sizes=[
        (16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256),
    ])
    print("Generated desktop, iOS, and browser icons.")


if __name__ == "__main__":
    main()
