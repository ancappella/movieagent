"""Aspect-ratio presets for SD1.5 (multiples of 64, ~16GB-friendly)."""

from __future__ import annotations

# aspect → (width, height, 用途说明)
ASPECT_PRESETS: dict[str, tuple[int, int, str]] = {
    "1:1": (512, 512, "头像、图标、方形海报"),
    "3:4": (576, 768, "竖版人像、手机壁纸、书籍封面"),
    "4:3": (768, 576, "传统照片、PPT"),
    "16:9": (768, 432, "横版视频封面、桌面壁纸"),
    "9:16": (432, 768, "短视频、Story"),
    "2:3": (512, 768, "摄影、明信片、绘本插页"),
    "21:9": (896, 384, "电影宽银幕"),
}

DEFAULT_ASPECT = "2:3"

# Aliases / common typos
_ALIASES = {
    "1x1": "1:1",
    "square": "1:1",
    "3x4": "3:4",
    "portrait": "3:4",
    "book": "3:4",
    "cover": "3:4",
    "4x3": "4:3",
    "landscape": "4:3",
    "16x9": "16:9",
    "widescreen": "16:9",
    "9x16": "9:16",
    "story": "9:16",
    "reels": "9:16",
    "2x3": "2:3",
    "postcard": "2:3",
    "21x9": "21:9",
    "cinematic": "21:9",
    "ultrawide": "21:9",
}


def normalize_aspect(raw: str) -> str:
    key = (raw or "").strip().lower().replace(" ", "")
    if not key:
        return DEFAULT_ASPECT
    key = key.replace("：", ":").replace("×", "x").replace("*", "x")
    if key in ASPECT_PRESETS:
        return key
    if key in _ALIASES:
        return _ALIASES[key]
    # accept "3/4"
    if "/" in key:
        alt = key.replace("/", ":")
        if alt in ASPECT_PRESETS:
            return alt
    raise ValueError(
        f"unsupported aspect_ratio {raw!r}; "
        f"choose one of: {', '.join(ASPECT_PRESETS)}"
    )


def resolve_size(
    *,
    aspect_ratio: str = "",
    width: int = 0,
    height: int = 0,
) -> tuple[int, int, str]:
    """Return (width, height, aspect_label).

    Explicit width+height both >0 win over aspect_ratio.
    Otherwise aspect_ratio (default 2:3) selects a preset.
    """
    if width > 0 and height > 0:
        label = aspect_ratio.strip() or f"{width}:{height}"
        return int(width), int(height), label

    if (width > 0) ^ (height > 0):
        raise ValueError("width and height must both be set, or use aspect_ratio alone")

    label = normalize_aspect(aspect_ratio or DEFAULT_ASPECT)
    w, h, _use = ASPECT_PRESETS[label]
    return w, h, label


def aspect_help_text() -> str:
    lines = ["Supported aspect_ratio presets:"]
    for k, (w, h, use) in ASPECT_PRESETS.items():
        lines.append(f"  {k:5} → {w}x{h}  ({use})")
    return "\n".join(lines)
