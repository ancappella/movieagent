"""Default watercolor + colored-pencil children's picture-book style prompts."""

from __future__ import annotations

STYLE_POSITIVE = (
    "children's picture book illustration, watercolor and colored pencil, "
    "soft hand-drawn lines, gentle washes, pastel palette, whimsical storybook art, "
    "warm soft lighting, clean composition, kid-friendly, high quality illustration"
)

STYLE_NEGATIVE = (
    "photorealistic, photo, 3d render, cgi, harsh contrast, text, watermark, logo, "
    "signature, blurry, low quality, deformed hands, extra fingers, scary, gore, nsfw"
)

# Optional trigger words for common SD1.5 children's / pencil LoRAs (override via --lora-trigger).
DEFAULT_LORA_TRIGGER = "picturebook pencil watercolor"


def build_prompt(user_prompt: str, *, lora_trigger: str = "", strength_hint: bool = True) -> str:
    user = (user_prompt or "").strip()
    parts: list[str] = []
    trigger = (lora_trigger or "").strip()
    if trigger:
        parts.append(trigger)
    if user:
        parts.append(user)
    parts.append(STYLE_POSITIVE)
    if strength_hint and "watercolor" not in user.lower():
        parts.append("delicate colored pencil shading")
    return ", ".join(parts)


def build_negative(user_negative: str = "") -> str:
    extra = (user_negative or "").strip()
    if not extra:
        return STYLE_NEGATIVE
    return f"{STYLE_NEGATIVE}, {extra}"
