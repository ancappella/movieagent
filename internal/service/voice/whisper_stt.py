#!/usr/bin/env python3
"""Local Whisper STT used by movieagent. No LLM / TTS."""

from __future__ import annotations

import argparse
import json
import platform
import sys
import wave
from pathlib import Path

import numpy as np

TARGET_SR = 16000


def _mlx_repo(name: str) -> str:
    key = (name or "tiny").strip().lower()
    mapping = {
        "tiny": "mlx-community/whisper-tiny",
        "base": "mlx-community/whisper-base",
        "small": "mlx-community/whisper-small",
        "medium": "mlx-community/whisper-medium-mlx",
        "large": "mlx-community/whisper-large-v3-mlx",
        "large-v3": "mlx-community/whisper-large-v3-mlx",
        "turbo": "mlx-community/whisper-large-v3-turbo",
    }
    if key in mapping:
        return mapping[key]
    if "/" in name:
        return name
    return mapping["tiny"]


def load_wav(path: str) -> np.ndarray:
    with wave.open(path, "rb") as wf:
        channels = wf.getnchannels()
        width = wf.getsampwidth()
        sr = wf.getframerate()
        n = wf.getnframes()
        raw = wf.readframes(n)
    if width == 2:
        audio = np.frombuffer(raw, dtype=np.int16).astype(np.float32) / 32768.0
    elif width == 4:
        audio = np.frombuffer(raw, dtype=np.int32).astype(np.float32) / 2147483648.0
    else:
        raise RuntimeError(f"unsupported sample width {width}")
    if channels > 1:
        audio = audio.reshape(-1, channels).mean(axis=1)
    if sr != TARGET_SR and audio.size:
        n_out = max(1, int(round(audio.size * TARGET_SR / sr)))
        x_old = np.linspace(0.0, 1.0, audio.size, endpoint=False)
        x_new = np.linspace(0.0, 1.0, n_out, endpoint=False)
        audio = np.interp(x_new, x_old, audio).astype(np.float32)
    return np.ascontiguousarray(audio, dtype=np.float32)


def transcribe(path: str, model: str, language: str) -> str:
    audio = load_wav(path)
    use_mlx = platform.system() == "Darwin" and platform.machine() == "arm64"
    if use_mlx:
        import mlx_whisper

        out = mlx_whisper.transcribe(
            audio,
            path_or_hf_repo=_mlx_repo(model),
            language=language or None,
            temperature=0.0,
            verbose=False,
        )
        return str(out.get("text") or "").strip()

    from faster_whisper import WhisperModel

    fw = WhisperModel(model or "tiny", device="auto", compute_type="int8")
    segments, _info = fw.transcribe(path, language=language or None)
    return "".join(seg.text for seg in segments).strip()


def main() -> int:
    parser = argparse.ArgumentParser(description="movieagent local Whisper STT")
    parser.add_argument("audio")
    parser.add_argument("--model", default="tiny")
    parser.add_argument("--language", default="zh")
    args = parser.parse_args()
    try:
        text = transcribe(args.audio, args.model, args.language)
    except Exception as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False), file=sys.stdout)
        return 1
    print(json.dumps({"ok": True, "text": text}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
