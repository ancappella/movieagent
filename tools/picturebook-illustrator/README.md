# Moved

Picture-book illustrator now lives as a standard skill:

```text
skills/public/picturebook-illustrate/
├── SKILL.md
├── scripts/          # generate.py, aspect.py, style.py, .venv
├── references/
└── assets/           # models/lora, …
```

Update env paths accordingly, e.g.:

```bash
export PICTUREBOOK_PYTHON="$PWD/skills/public/picturebook-illustrate/scripts/.venv/bin/python"
```
