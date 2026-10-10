# Setup

```bash
cd skills/public/picturebook-illustrate/scripts
npm install
```

Requires Node.js 18+. Dependency: `@napi-rs/canvas`.

Dry-run:

```bash
PICTUREBOOK_DRY_RUN=1
# or
node draw.mjs --prompt "test" --output /tmp/dry.png --aspect-ratio 2:3 --dry-run
```

Smoke:

```bash
node draw.mjs --prompt "橘色的海水，小纸船" --output /tmp/sea.png --aspect-ratio 2:3 --seed 7
```
