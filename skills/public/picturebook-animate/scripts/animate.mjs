#!/usr/bin/env node
/**
 * Canvas 2D still-image motion → H.264 MP4 (ffmpeg-static).
 * Prints one JSON object on stdout when finished.
 *
 * Presets:
 * - ken_burns / reveal_wash / float_particles: whole-frame camera/fx
 * - character_idle: one subject bob/sway, background stays still
 */
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createCanvas, loadImage } from "@napi-rs/canvas";
import ffmpegPath from "ffmpeg-static";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const out = {
    input: "",
    output: "",
    preset: "character_idle",
    duration: 4,
    fps: 24,
    width: 0,
    height: 0,
    dryRun: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    const next = () => argv[++i];
    switch (a) {
      case "--input":
        out.input = next();
        break;
      case "--output":
        out.output = next();
        break;
      case "--preset":
        out.preset = next();
        break;
      case "--duration":
        out.duration = Number(next());
        break;
      case "--fps":
        out.fps = Number(next());
        break;
      case "--width":
        out.width = Number(next());
        break;
      case "--height":
        out.height = Number(next());
        break;
      case "--dry-run":
        out.dryRun = true;
        break;
      default:
        break;
    }
  }
  return out;
}

function even(n) {
  n = Math.max(2, Math.round(n));
  return n % 2 === 0 ? n : n + 1;
}

function easeInOut(t) {
  return t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
}

function fail(msg) {
  process.stdout.write(JSON.stringify({ ok: false, error: msg }) + "\n");
  process.exit(1);
}

function ok(payload) {
  process.stdout.write(JSON.stringify({ ok: true, ...payload }) + "\n");
}

/** Minimal valid-ish MP4 for dry-run / CI without rendering. */
function writeStubMp4(file) {
  const buf = Buffer.from([
    0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d, 0x00, 0x00, 0x00, 0x01,
    0x69, 0x73, 0x6f, 0x6d, 0x61, 0x76, 0x63, 0x31, 0x00, 0x00, 0x00, 0x08, 0x66, 0x72, 0x65, 0x65,
    0x00, 0x00, 0x00, 0x08, 0x6d, 0x64, 0x61, 0x74,
  ]);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, buf);
}

function drawKenBurns(ctx, img, w, h, t) {
  const e = easeInOut(t);
  const scale = 1.08 + 0.12 * e;
  const sw = w * scale;
  const sh = h * scale;
  const x = (w - sw) * (0.15 + 0.7 * e);
  const y = (h - sh) * (0.35 - 0.2 * e);
  ctx.fillStyle = "#0b1220";
  ctx.fillRect(0, 0, w, h);
  ctx.drawImage(img, x, y, sw, sh);
}

function drawRevealWash(ctx, img, w, h, t) {
  const e = easeInOut(Math.min(1, t * 1.15));
  ctx.fillStyle = "#f4efe6";
  ctx.fillRect(0, 0, w, h);
  ctx.save();
  ctx.beginPath();
  ctx.rect(0, 0, w, h * e);
  ctx.clip();
  ctx.drawImage(img, 0, 0, w, h);
  ctx.restore();
  if (e < 1) {
    const band = Math.max(8, h * 0.04);
    const gy = h * e - band / 2;
    const g = ctx.createLinearGradient(0, gy, 0, gy + band);
    g.addColorStop(0, "rgba(244,239,230,0)");
    g.addColorStop(1, "rgba(244,239,230,0.85)");
    ctx.fillStyle = g;
    ctx.fillRect(0, gy, w, band);
  }
}

function drawFloatParticles(ctx, img, w, h, t) {
  const e = easeInOut(t);
  const scale = 1.04 + 0.06 * e;
  const sw = w * scale;
  const sh = h * scale;
  const x = (w - sw) / 2 + Math.sin(t * Math.PI * 2) * w * 0.015;
  const y = (h - sh) / 2 + Math.cos(t * Math.PI * 2) * h * 0.01;
  ctx.fillStyle = "#071018";
  ctx.fillRect(0, 0, w, h);
  ctx.drawImage(img, x, y, sw, sh);
  ctx.fillStyle = "rgba(255,255,255,0.35)";
  const n = 28;
  for (let i = 0; i < n; i++) {
    const seed = (i * 97) % 1000;
    const px = ((seed * 17 + t * (40 + (i % 7) * 9)) % (w + 40)) - 20;
    const py = h * (0.55 + ((seed % 40) / 100)) - t * h * 0.08 * ((i % 5) + 1) * 0.15;
    const r = 1 + (i % 3);
    ctx.beginPath();
    ctx.arc(px, ((py % (h + 20)) + h + 20) % (h + 20) - 10, r, 0, Math.PI * 2);
    ctx.fill();
  }
}

const SIMPLE_PRESETS = {
  ken_burns: drawKenBurns,
  reveal_wash: drawRevealWash,
  float_particles: drawFloatParticles,
};

function sampleBgColor(data, w, h) {
  const pts = [
    [2, 2],
    [w - 3, 2],
    [2, h - 3],
    [w - 3, h - 3],
    [(w / 2) | 0, 2],
    [(w / 2) | 0, h - 3],
    [2, (h / 2) | 0],
    [w - 3, (h / 2) | 0],
  ];
  let r = 0;
  let g = 0;
  let b = 0;
  for (const [x, y] of pts) {
    const i = (y * w + x) * 4;
    r += data[i];
    g += data[i + 1];
    b += data[i + 2];
  }
  const n = pts.length;
  return { r: r / n, g: g / n, b: b / n };
}

function colorDist(r, g, b, bg) {
  const dr = r - bg.r;
  const dg = g - bg.g;
  const db = b - bg.b;
  return Math.sqrt(dr * dr + dg * dg + db * db);
}

function estimateThreshold(data, w, h, bg) {
  // Adaptive: lighter paper backgrounds need a higher cut; busy scenes lower.
  const lum = 0.2126 * bg.r + 0.7152 * bg.g + 0.0722 * bg.b;
  let base = lum > 180 ? 42 : lum > 100 ? 36 : 30;
  // Sample mid-frame variance to avoid wiping soft watercolor edges.
  let far = 0;
  let count = 0;
  for (let y = (h * 0.25) | 0; y < h * 0.75; y += 8) {
    for (let x = (w * 0.25) | 0; x < w * 0.75; x += 8) {
      const i = (y * w + x) * 4;
      far += colorDist(data[i], data[i + 1], data[i + 2], bg);
      count++;
    }
  }
  const meanFar = count ? far / count : 40;
  return Math.max(24, Math.min(58, (base + meanFar * 0.35) / 1.6));
}

function dilate(mask, w, h, radius = 1) {
  const out = new Uint8Array(mask.length);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      let on = 0;
      for (let dy = -radius; dy <= radius && !on; dy++) {
        for (let dx = -radius; dx <= radius; dx++) {
          const nx = x + dx;
          const ny = y + dy;
          if (nx < 0 || ny < 0 || nx >= w || ny >= h) continue;
          if (mask[ny * w + nx]) {
            on = 1;
            break;
          }
        }
      }
      out[y * w + x] = on;
    }
  }
  return out;
}

function erode(mask, w, h, radius = 1) {
  const out = new Uint8Array(mask.length);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      let on = 1;
      for (let dy = -radius; dy <= radius && on; dy++) {
        for (let dx = -radius; dx <= radius; dx++) {
          const nx = x + dx;
          const ny = y + dy;
          if (nx < 0 || ny < 0 || nx >= w || ny >= h || !mask[ny * w + nx]) {
            on = 0;
            break;
          }
        }
      }
      out[y * w + x] = on;
    }
  }
  return out;
}

function largestCenteredComponent(mask, w, h) {
  const visited = new Uint8Array(w * h);
  const cx0 = w / 2;
  const cy0 = h / 2;
  let best = null;
  const minArea = Math.max(64, ((w * h) / 100) | 0);

  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const start = y * w + x;
      if (!mask[start] || visited[start]) continue;
      const stack = [start];
      visited[start] = 1;
      const pixels = [];
      let sx = 0;
      let sy = 0;
      let minX = x;
      let maxX = x;
      let minY = y;
      let maxY = y;
      while (stack.length) {
        const p = stack.pop();
        pixels.push(p);
        const px = p % w;
        const py = (p / w) | 0;
        sx += px;
        sy += py;
        if (px < minX) minX = px;
        if (px > maxX) maxX = px;
        if (py < minY) minY = py;
        if (py > maxY) maxY = py;
        const neigh = [p + 1, p - 1, p + w, p - w];
        const bounds = [
          px + 1 < w,
          px - 1 >= 0,
          py + 1 < h,
          py - 1 >= 0,
        ];
        for (let k = 0; k < 4; k++) {
          if (!bounds[k]) continue;
          const ni = neigh[k];
          if (!mask[ni] || visited[ni]) continue;
          visited[ni] = 1;
          stack.push(ni);
        }
      }
      const area = pixels.length;
      if (area < minArea) continue;
      const cx = sx / area;
      const cy = sy / area;
      const dist = (cx - cx0) * (cx - cx0) + (cy - cy0) * (cy - cy0);
      const score = area * 1.15 - dist;
      if (!best || score > best.score) {
        best = { pixels, cx, cy, area, score, minX, maxX, minY, maxY };
      }
    }
  }
  return best;
}

function inpaintBackground(bgData, hole, w, h, bg, passes = 4) {
  const src = bgData.data;
  for (let pass = 0; pass < passes; pass++) {
    const copy = Buffer.from(src);
    for (let y = 1; y < h - 1; y++) {
      for (let x = 1; x < w - 1; x++) {
        const i = y * w + x;
        if (!hole[i]) continue;
        let r = 0;
        let g = 0;
        let b = 0;
        let n = 0;
        for (let dy = -1; dy <= 1; dy++) {
          for (let dx = -1; dx <= 1; dx++) {
            if (dx === 0 && dy === 0) continue;
            const ni = (y + dy) * w + (x + dx);
            if (hole[ni] && pass === 0) continue;
            const o = ni * 4;
            r += copy[o];
            g += copy[o + 1];
            b += copy[o + 2];
            n++;
          }
        }
        const o = i * 4;
        if (n === 0) {
          src[o] = bg.r;
          src[o + 1] = bg.g;
          src[o + 2] = bg.b;
          src[o + 3] = 255;
        } else {
          src[o] = (r / n) | 0;
          src[o + 1] = (g / n) | 0;
          src[o + 2] = (b / n) | 0;
          src[o + 3] = 255;
        }
      }
    }
  }
}

/** Split one dominant subject from a relatively flat background. */
function splitSubjectLayers(img, w, h) {
  const src = createCanvas(w, h);
  const sctx = src.getContext("2d");
  sctx.drawImage(img, 0, 0, w, h);
  const imageData = sctx.getImageData(0, 0, w, h);
  const data = imageData.data;
  const bg = sampleBgColor(data, w, h);
  const threshold = estimateThreshold(data, w, h, bg);

  let mask = new Uint8Array(w * h);
  for (let i = 0; i < w * h; i++) {
    const o = i * 4;
    mask[i] = colorDist(data[o], data[o + 1], data[o + 2], bg) > threshold ? 1 : 0;
  }
  mask = dilate(erode(mask, w, h, 1), w, h, 1);

  let comp = largestCenteredComponent(mask, w, h);
  if (!comp) {
    // Fallback: treat center third as subject so we still produce motion.
    const x0 = (w * 0.28) | 0;
    const x1 = (w * 0.72) | 0;
    const y0 = (h * 0.18) | 0;
    const y1 = (h * 0.92) | 0;
    const pixels = [];
    for (let y = y0; y < y1; y++) {
      for (let x = x0; x < x1; x++) pixels.push(y * w + x);
    }
    comp = {
      pixels,
      cx: (x0 + x1) / 2,
      cy: (y0 + y1) / 2,
      minX: x0,
      maxX: x1 - 1,
      minY: y0,
      maxY: y1 - 1,
      area: pixels.length,
      score: 0,
    };
  }

  let charMask = new Uint8Array(w * h);
  for (const p of comp.pixels) charMask[p] = 1;
  charMask = dilate(charMask, w, h, 2);

  const bgCanvas = createCanvas(w, h);
  const bgCtx = bgCanvas.getContext("2d");
  const bgImage = bgCtx.createImageData(w, h);
  for (let i = 0; i < w * h; i++) {
    const o = i * 4;
    if (charMask[i]) {
      bgImage.data[o] = bg.r;
      bgImage.data[o + 1] = bg.g;
      bgImage.data[o + 2] = bg.b;
      bgImage.data[o + 3] = 255;
    } else {
      bgImage.data[o] = data[o];
      bgImage.data[o + 1] = data[o + 1];
      bgImage.data[o + 2] = data[o + 2];
      bgImage.data[o + 3] = 255;
    }
  }
  inpaintBackground(bgImage, charMask, w, h, bg, 5);
  bgCtx.putImageData(bgImage, 0, 0);

  const fgCanvas = createCanvas(w, h);
  const fgCtx = fgCanvas.getContext("2d");
  const fgImage = fgCtx.createImageData(w, h);
  for (let i = 0; i < w * h; i++) {
    const o = i * 4;
    if (charMask[i]) {
      fgImage.data[o] = data[o];
      fgImage.data[o + 1] = data[o + 1];
      fgImage.data[o + 2] = data[o + 2];
      fgImage.data[o + 3] = 255;
    } else {
      fgImage.data[o + 3] = 0;
    }
  }
  fgCtx.putImageData(fgImage, 0, 0);

  return {
    bgCanvas,
    fgCanvas,
    pivotX: (comp.minX + comp.maxX) / 2,
    // Pivot near feet so sway feels grounded.
    pivotY: Math.min(h - 2, comp.maxY - (comp.maxY - comp.minY) * 0.05),
    bbox: {
      minX: comp.minX,
      maxX: comp.maxX,
      minY: comp.minY,
      maxY: comp.maxY,
    },
    threshold,
  };
}

function drawCharacterIdle(ctx, layers, w, h, t) {
  // 2 breath cycles + gentle sway over the clip; background never moves.
  const breath = Math.sin(t * Math.PI * 2 * 2);
  const sway = Math.sin(t * Math.PI * 2);
  const dy = breath * Math.max(2, h * 0.006);
  const rot = (sway * 1.6 * Math.PI) / 180;
  const scaleY = 1 + breath * 0.012;

  ctx.clearRect(0, 0, w, h);
  ctx.drawImage(layers.bgCanvas, 0, 0, w, h);
  ctx.save();
  ctx.translate(layers.pivotX, layers.pivotY);
  ctx.rotate(rot);
  ctx.scale(1, scaleY);
  ctx.translate(-layers.pivotX, -layers.pivotY + dy);
  ctx.drawImage(layers.fgCanvas, 0, 0, w, h);
  ctx.restore();
}

/** Fake Y-axis spin via horizontal scale; background stays still. */
function drawCharacterSpin(ctx, layers, w, h, t) {
  // One full turn over the clip (ease slightly at ends).
  const e = easeInOut(t);
  const angle = e * Math.PI * 2;
  const sx = Math.cos(angle);
  // Keep a tiny width so the silhouette never fully disappears.
  const scaleX = Math.sign(sx || 1) * Math.max(0.08, Math.abs(sx));
  const foreshorten = 1 - Math.abs(sx) * 0.04;
  const bw = Math.max(8, (layers.bbox.maxX - layers.bbox.minX) * 0.35);
  const bh = Math.max(4, (layers.bbox.maxY - layers.bbox.minY) * 0.04);

  ctx.clearRect(0, 0, w, h);
  ctx.drawImage(layers.bgCanvas, 0, 0, w, h);

  // Soft ground shadow that squashes with the spin.
  ctx.save();
  ctx.translate(layers.pivotX, layers.pivotY + 2);
  ctx.scale(Math.abs(scaleX) * 1.05, 1);
  ctx.fillStyle = "rgba(0,0,0,0.10)";
  ctx.beginPath();
  ctx.ellipse(0, 0, bw, bh, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  ctx.save();
  ctx.translate(layers.pivotX, layers.pivotY);
  ctx.scale(scaleX, foreshorten);
  ctx.translate(-layers.pivotX, -layers.pivotY);
  ctx.drawImage(layers.fgCanvas, 0, 0, w, h);
  ctx.restore();
}

function runFfmpeg(args) {
  return new Promise((resolve, reject) => {
    if (!ffmpegPath) {
      reject(new Error("ffmpeg-static binary missing"));
      return;
    }
    const child = spawn(ffmpegPath, args, { stdio: ["ignore", "pipe", "pipe"] });
    let stderr = "";
    child.stderr.on("data", (d) => {
      stderr += d.toString();
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) resolve();
      else reject(new Error(stderr.trim().split("\n").slice(-8).join("\n") || `ffmpeg exit ${code}`));
    });
  });
}

const LAYERED_PRESETS = new Set(["character_idle", "character_spin"]);
const ALL_PRESETS = [...Object.keys(SIMPLE_PRESETS), ...LAYERED_PRESETS];

async function main() {
  const args = parseArgs(process.argv);
  if (!args.input || !args.output) {
    fail(
      `usage: animate.mjs --input in.png --output out.mp4 [--preset ${ALL_PRESETS.join("|")}] [--duration 4] [--fps 24]`,
    );
  }
  if (!fs.existsSync(args.input)) {
    fail(`input not found: ${args.input}`);
  }
  const preset = String(args.preset || "character_idle").trim();
  if (!ALL_PRESETS.includes(preset)) {
    fail(`unknown preset: ${preset} (want ${ALL_PRESETS.join("|")})`);
  }
  const duration = Math.min(30, Math.max(1, Number(args.duration) || 4));
  const fps = Math.min(60, Math.max(8, Math.round(Number(args.fps) || 24)));
  const frames = Math.max(1, Math.round(duration * fps));

  fs.mkdirSync(path.dirname(args.output), { recursive: true });

  if (args.dryRun) {
    writeStubMp4(args.output);
    ok({
      path: args.output,
      preset,
      duration_sec: duration,
      fps,
      frames,
      width: args.width || 512,
      height: args.height || 768,
      bytes: fs.statSync(args.output).size,
      dry_run: true,
      skill: "picturebook-animate",
      script: path.join(__dirname, "animate.mjs"),
    });
    return;
  }

  const img = await loadImage(args.input);
  let w = args.width > 0 ? args.width : img.width;
  let h = args.height > 0 ? args.height : img.height;
  w = even(w);
  h = even(h);

  const canvas = createCanvas(w, h);
  const ctx = canvas.getContext("2d");
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "pb-anim-"));

  let layers = null;
  let meta = {};
  if (LAYERED_PRESETS.has(preset)) {
    layers = splitSubjectLayers(img, w, h);
    meta = {
      subject_bbox: layers.bbox,
      subject_threshold: layers.threshold,
    };
  }
  const simpleDraw = SIMPLE_PRESETS[preset];

  try {
    for (let i = 0; i < frames; i++) {
      const t = frames === 1 ? 0 : i / (frames - 1);
      if (preset === "character_idle") {
        drawCharacterIdle(ctx, layers, w, h, t);
      } else if (preset === "character_spin") {
        drawCharacterSpin(ctx, layers, w, h, t);
      } else {
        simpleDraw(ctx, img, w, h, t);
      }
      const framePath = path.join(tmp, `frame_${String(i).padStart(5, "0")}.png`);
      fs.writeFileSync(framePath, canvas.toBuffer("image/png"));
    }
    await runFfmpeg([
      "-y",
      "-framerate",
      String(fps),
      "-i",
      path.join(tmp, "frame_%05d.png"),
      "-c:v",
      "libx264",
      "-pix_fmt",
      "yuv420p",
      "-movflags",
      "+faststart",
      args.output,
    ]);
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }

  const st = fs.statSync(args.output);
  ok({
    path: args.output,
    preset,
    duration_sec: duration,
    fps,
    frames,
    width: w,
    height: h,
    bytes: st.size,
    skill: "picturebook-animate",
    script: path.join(__dirname, "animate.mjs"),
    ...meta,
  });
}

main().catch((err) => fail(err?.message || String(err)));
