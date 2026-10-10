#!/usr/bin/env python3
from aspect import ASPECT_PRESETS, normalize_aspect, resolve_size


def test_presets_match_table():
    assert resolve_size(aspect_ratio="1:1")[:2] == (512, 512)
    assert resolve_size(aspect_ratio="3:4")[:2] == (576, 768)
    assert resolve_size(aspect_ratio="4:3")[:2] == (768, 576)
    assert resolve_size(aspect_ratio="16:9")[:2] == (768, 432)
    assert resolve_size(aspect_ratio="9:16")[:2] == (432, 768)
    assert resolve_size(aspect_ratio="2:3")[:2] == (512, 768)
    assert resolve_size(aspect_ratio="21:9")[:2] == (896, 384)


def test_aliases_and_override():
    assert normalize_aspect("story") == "9:16"
    assert normalize_aspect("cover") == "3:4"
    w, h, label = resolve_size(width=640, height=480)
    assert (w, h) == (640, 480)
    assert label


def test_all_keys_documented():
    assert set(ASPECT_PRESETS) == {"1:1", "3:4", "4:3", "16:9", "9:16", "2:3", "21:9"}


if __name__ == "__main__":
    test_presets_match_table()
    test_aliases_and_override()
    test_all_keys_documented()
    print("ok")
