#!/usr/bin/env python3
"""Rasterize PsyNote favicon PNGs + ICO from the brand mark."""

from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = Path(__file__).resolve().parents[1] / "public"
C1 = (143, 171, 255)  # #8fabff
C2 = (93, 119, 214)  # #5d77d6
INK = (10, 10, 11, 255)

FONTS = (
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/Library/Fonts/Arial Bold.ttf",
    "/System/Library/Fonts/SFNS.ttf",
    "/System/Library/Fonts/HelveticaNeue.ttc",
)


def lerp(a: int, b: int, t: float) -> int:
    return int(a + (b - a) * t)


def gradient(size: int) -> Image.Image:
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    px = img.load()
    den = max(size - 1, 1) * 2
    for y in range(size):
        for x in range(size):
            t = (x + y) / den
            px[x, y] = (
                lerp(C1[0], C2[0], t),
                lerp(C1[1], C2[1], t),
                lerp(C1[2], C2[2], t),
                255,
            )
    return img


def apply_round(img: Image.Image, radius: int) -> Image.Image:
    mask = Image.new("L", img.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, img.size[0] - 1, img.size[1] - 1), radius=radius, fill=255
    )
    out = Image.new("RGBA", img.size, (0, 0, 0, 0))
    out.paste(img, (0, 0), mask)
    return out


def add_sheen(img: Image.Image) -> Image.Image:
    """Very light top-left wash — no mid-line 'wave'."""
    if img.size[0] < 96:
        return img
    overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
    px = overlay.load()
    w, h = img.size
    for y in range(h):
        for x in range(w):
            t = 1.0 - (x * 0.45 + y) / (w + h)
            if t <= 0:
                continue
            px[x, y] = (255, 255, 255, int(18 * t * t))
    return Image.alpha_composite(img, overlay)


def font_for(size: int) -> ImageFont.FreeTypeFont:
    px = max(10, int(size * 0.58))
    for path in FONTS:
        p = Path(path)
        if not p.exists():
            continue
        try:
            return ImageFont.truetype(path, px)
        except OSError:
            continue
    return ImageFont.load_default()


def draw_p(img: Image.Image) -> Image.Image:
    size = img.size[0]
    draw = ImageDraw.Draw(img)
    font = font_for(size)
    # Optical center: P sits a hair left/up vs geometric center.
    text = "P"
    bbox = draw.textbbox((0, 0), text, font=font)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    x = (size - tw) / 2 - bbox[0] - size * 0.02
    y = (size - th) / 2 - bbox[1] - size * 0.03
    draw.text((x, y), text, font=font, fill=INK)
    return img


def make(size: int, rounded: bool) -> Image.Image:
    img = gradient(size)
    img = add_sheen(img)
    img = draw_p(img)
    if rounded:
        img = apply_round(img, radius=max(2, round(size * 0.22)))
    if size <= 32:
        img = img.filter(ImageFilter.UnsharpMask(radius=0.6, percent=120, threshold=2))
    return img


def save_ico(path: Path) -> None:
    tiles = [make(s, rounded=True) for s in (16, 32)]
    tiles[-1].save(path, format="ICO", sizes=[(16, 16), (32, 32)], append_images=tiles[:-1])


def main() -> None:
    ROOT.mkdir(parents=True, exist_ok=True)
    make(180, rounded=False).save(ROOT / "apple-touch-icon.png", "PNG")
    make(192, rounded=False).save(ROOT / "icon-192.png", "PNG")
    make(512, rounded=False).save(ROOT / "icon-512.png", "PNG")
    save_ico(ROOT / "favicon.ico")
    print("wrote", ROOT)


if __name__ == "__main__":
    main()
