# Setup

```bash
cd skills/public/picturebook-animate/scripts
npm install
npx playwright install chromium
```

Requires Node.js 18+. Dependencies:

- `playwright` — headless Chromium for per-frame `draw(t)` screenshots
- `ffmpeg-static` — bundled ffmpeg for H.264 MP4

Dry-run (CI / no encode):

```bash
PICTUREBOOK_ANIMATE_DRY_RUN=1
```

Manual smoke (fallback template, no LLM code):

```bash
node render.mjs --template character_spin --asset /path/to.png --output /tmp/out.mp4 --duration 2 --fps 24
```

With LLM-written animation program:

```bash
node render.mjs --code-file ./animation.js --asset /path/to.png --output /tmp/out.mp4 --duration 4 --fps 24
```
