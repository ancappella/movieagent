#!/usr/bin/env python3
"""Local SD1.5 (+ optional picture-book LoRA) illustrator via HuggingFace diffusers.

Designed for ~16GB VRAM (fp16 + attention slicing). Also runs on Apple MPS / CPU.

Prints one JSON object on stdout (last line) for movieagent tool bridging:
  {"ok": true, "path": "...", "seed": 42, "model": "...", "lora": "...", "device": "..."}
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
from pathlib import Path

from aspect import DEFAULT_ASPECT, aspect_help_text, resolve_size
from style import DEFAULT_LORA_TRIGGER, build_negative, build_prompt

DEFAULT_MODEL = os.environ.get(
    "PICTUREBOOK_MODEL",
    "stable-diffusion-v1-5/stable-diffusion-v1-5",
)
DEFAULT_LORA = os.environ.get("PICTUREBOOK_LORA", "").strip()
DEFAULT_LORA_SCALE = float(os.environ.get("PICTUREBOOK_LORA_SCALE", "0.85"))


def _pick_device() -> str:
    forced = (os.environ.get("PICTUREBOOK_DEVICE") or "").strip().lower()
    if forced in {"cuda", "mps", "cpu"}:
        return forced
    try:
        import torch

        if torch.cuda.is_available():
            return "cuda"
        if getattr(torch.backends, "mps", None) and torch.backends.mps.is_available():
            return "mps"
    except Exception:
        pass
    return "cpu"


def _dtype_for(device: str):
    import torch

    # SD1.5 on Apple MPS often NaNs in float16 → blank/tiny PNGs; use fp32 there.
    if device == "cuda":
        return torch.float16
    return torch.float32


def _write_placeholder_png(path: Path, seed: int, width: int, height: int) -> None:
    from PIL import Image, ImageDraw

    path.parent.mkdir(parents=True, exist_ok=True)
    w, h = max(64, int(width)), max(64, int(height))
    img = Image.new("RGB", (w, h), (245, 232, 214))
    draw = ImageDraw.Draw(img)
    pad_x, pad_y = w // 6, h // 5
    draw.ellipse((pad_x, pad_y, w - pad_x, h - pad_y), fill=(186, 214, 186), outline=(120, 150, 120))
    draw.text((16, 16), f"dry-run {w}x{h} seed={seed}", fill=(90, 70, 50))
    img.save(path, format="PNG")


def generate(args: argparse.Namespace) -> dict:
    out = Path(args.output).expanduser().resolve()
    seed = int(args.seed) if args.seed is not None else int(time.time()) % (2**31 - 1)
    prompt = build_prompt(args.prompt, lora_trigger=args.lora_trigger or "")
    negative = build_negative(args.negative_prompt or "")
    device = _pick_device()
    model_id = args.model or DEFAULT_MODEL
    lora = (args.lora or DEFAULT_LORA or "").strip()
    lora_scale = float(args.lora_scale)
    width, height, aspect = resolve_size(
        aspect_ratio=getattr(args, "aspect_ratio", "") or "",
        width=int(getattr(args, "width", 0) or 0),
        height=int(getattr(args, "height", 0) or 0),
    )
    args.width = width
    args.height = height

    if args.dry_run:
        _write_placeholder_png(out, seed, width, height)
        return {
            "ok": True,
            "dry_run": True,
            "path": str(out),
            "seed": seed,
            "model": model_id,
            "lora": lora or None,
            "device": device,
            "prompt": prompt,
            "aspect_ratio": aspect,
            "width": width,
            "height": height,
        }

    import torch
    from diffusers import DPMSolverMultistepScheduler, StableDiffusionPipeline

    dtype = _dtype_for(device)
    pipe = StableDiffusionPipeline.from_pretrained(
        model_id,
        torch_dtype=dtype,
        safety_checker=None,
        requires_safety_checker=False,
    )
    pipe.scheduler = DPMSolverMultistepScheduler.from_config(pipe.scheduler.config)

    if lora:
        lora_path = Path(lora).expanduser()
        weight = str(lora_path if lora_path.exists() else lora)
        pipe.load_lora_weights(weight)
        try:
            pipe.fuse_lora(lora_scale=lora_scale)
        except TypeError:
            # older diffusers
            pipe.fuse_lora()

    if device == "cuda":
        pipe = pipe.to("cuda")
        pipe.enable_attention_slicing()
        try:
            pipe.enable_vae_slicing()
        except Exception:
            pass
    elif device == "mps":
        pipe = pipe.to("mps")
        pipe.enable_attention_slicing()
    else:
        pipe = pipe.to("cpu")
        pipe.enable_attention_slicing()

    generator = torch.Generator(device=device if device != "mps" else "cpu").manual_seed(seed)
    result = pipe(
        prompt=prompt,
        negative_prompt=negative,
        width=int(args.width),
        height=int(args.height),
        num_inference_steps=int(args.steps),
        guidance_scale=float(args.guidance),
        generator=generator,
    )
    image = result.images[0]
    out.parent.mkdir(parents=True, exist_ok=True)
    image.save(out, format="PNG")

    return {
        "ok": True,
        "dry_run": False,
        "path": str(out),
        "seed": seed,
        "model": model_id,
        "lora": lora or None,
        "lora_scale": lora_scale if lora else None,
        "device": device,
        "steps": args.steps,
        "guidance": args.guidance,
        "aspect_ratio": aspect,
        "width": width,
        "height": height,
        "prompt": prompt,
    }


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="SD1.5 watercolor + colored-pencil picture-book illustrator",
        epilog=aspect_help_text(),
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    p.add_argument("--prompt", required=True, help="Scene / character description")
    p.add_argument("--negative-prompt", default="", help="Extra negative prompt")
    p.add_argument("--output", required=True, help="Output PNG path")
    p.add_argument("--model", default=DEFAULT_MODEL, help="HF model id or local path")
    p.add_argument("--lora", default=DEFAULT_LORA, help="LoRA HF id, local dir, or .safetensors path")
    p.add_argument("--lora-scale", type=float, default=DEFAULT_LORA_SCALE)
    p.add_argument("--lora-trigger", default=os.environ.get("PICTUREBOOK_LORA_TRIGGER", DEFAULT_LORA_TRIGGER))
    p.add_argument(
        "--aspect-ratio",
        default=os.environ.get("PICTUREBOOK_ASPECT", DEFAULT_ASPECT),
        help="Preset ratio: 1:1 | 3:4 | 4:3 | 16:9 | 9:16 | 2:3 | 21:9 (default 2:3)",
    )
    p.add_argument(
        "--width",
        type=int,
        default=int(os.environ.get("PICTUREBOOK_WIDTH", "0") or 0),
        help="Override width px (must set with --height; overrides aspect-ratio)",
    )
    p.add_argument(
        "--height",
        type=int,
        default=int(os.environ.get("PICTUREBOOK_HEIGHT", "0") or 0),
        help="Override height px (must set with --width; overrides aspect-ratio)",
    )
    p.add_argument("--steps", type=int, default=int(os.environ.get("PICTUREBOOK_STEPS", "28")))
    p.add_argument("--guidance", type=float, default=float(os.environ.get("PICTUREBOOK_GUIDANCE", "7.0")))
    p.add_argument("--seed", type=int, default=None)
    p.add_argument("--dry-run", action="store_true", help="Skip model load; write placeholder PNG")
    return p.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        payload = generate(args)
    except Exception as exc:  # noqa: BLE001 — surface to movieagent JSON
        err = {"ok": False, "error": str(exc)}
        print(json.dumps(err, ensure_ascii=False), flush=True)
        return 1
    print(json.dumps(payload, ensure_ascii=False), flush=True)
    return 0 if payload.get("ok") else 1


if __name__ == "__main__":
    sys.exit(main())
