#!/usr/bin/env node
/**
 * Canvas 2D children's picture-book illustrator (no SD / LoRA).
 * Interprets a text prompt into a stylized watercolor/pencil scene.
 * Prints one JSON object on stdout when finished.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createCanvas } from "@napi-rs/canvas";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const ASPECT = {
  "1:1": [512, 512],
  "3:4": [576, 768],
  "4:3": [768, 576],
  "16:9": [768, 432],
  "9:16": [432, 768],
  "2:3": [512, 768],
  "21:9": [896, 384],
};

function parseArgs(argv) {
  const out = {
    prompt: "",
    output: "",
    aspect: "2:3",
    width: 0,
    height: 0,
    seed: 0,
    dryRun: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    const next = () => argv[++i];
    switch (a) {
      case "--prompt":
        out.prompt = next();
        break;
      case "--output":
        out.output = next();
        break;
      case "--aspect-ratio":
        out.aspect = next();
        break;
      case "--width":
        out.width = Number(next());
        break;
      case "--height":
        out.height = Number(next());
        break;
      case "--seed":
        out.seed = Number(next());
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

function fail(msg) {
  process.stdout.write(JSON.stringify({ ok: false, error: msg }) + "\n");
  process.exit(1);
}

function ok(payload) {
  process.stdout.write(JSON.stringify({ ok: true, ...payload }) + "\n");
}

function mulberry32(a) {
  return function rand() {
    let t = (a += 0x6d2b79f5);
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function hashSeed(s) {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function even(n) {
  n = Math.max(2, Math.round(n));
  return n % 2 === 0 ? n : n + 1;
}

function detectTheme(prompt) {
  const p = String(prompt || "").toLowerCase();
  const has = (...keys) => keys.some((k) => p.includes(k));
  if (has("海", "sea", "ocean", "wave", "海水", "沙滩", "beach")) return "sea";
  if (has("狐", "fox")) return "fox";
  if (has("兔", "rabbit", "bunny")) return "rabbit";
  if (has("花", "flower", "rose", "bloom")) return "flowers";
  if (has("猫", "cat", "kitty")) return "cat";
  if (has("森", "forest", "wood", "树")) return "forest";
  if (has("女孩", "girl", "人物", "child", "小孩", "角色")) return "girl";
  if (has("山", "mountain", "日落", "sunset", "太阳")) return "sunset";
  return "meadow";
}

function detectPalette(prompt, theme) {
  const p = String(prompt || "");
  if (/橘|橙|orange|柿/.test(p)) {
    return { sky: "#ffe8c8", mid: "#ff9a4a", deep: "#e85d1a", accent: "#fff3d6", ink: "#5a3418" };
  }
  if (/蓝|blue|青/.test(p)) {
    return { sky: "#d9eefc", mid: "#6eb6e8", deep: "#2f6fa8", accent: "#f7fbff", ink: "#1e3a5f" };
  }
  if (/粉|pink|玫瑰/.test(p)) {
    return { sky: "#ffe8f0", mid: "#f4a0b8", deep: "#d45a7a", accent: "#fff7fa", ink: "#5a2034" };
  }
  const byTheme = {
    sea: { sky: "#cfe9f8", mid: "#4e9fd0", deep: "#1f5f8a", accent: "#f3fbff", ink: "#16324a" },
    fox: { sky: "#ffe9c9", mid: "#e8893a", deep: "#b24a14", accent: "#fff6e8", ink: "#4a2810" },
    rabbit: { sky: "#f3e9ff", mid: "#c9a6e8", deep: "#7a4eaa", accent: "#fbf7ff", ink: "#3a2450" },
    flowers: { sky: "#fff4e8", mid: "#f2a3b5", deep: "#6aa86a", accent: "#fffaf4", ink: "#3f2a20" },
    cat: { sky: "#eef2f6", mid: "#9aa3ad", deep: "#2c2c2c", accent: "#ffffff", ink: "#1a1a1a" },
    forest: { sky: "#e5f0d8", mid: "#6f9b4c", deep: "#2f5a28", accent: "#f7fbea", ink: "#1f3318" },
    girl: { sky: "#f7f3ee", mid: "#e8d5c4", deep: "#8a6a55", accent: "#ffffff", ink: "#3a2a22" },
    sunset: { sky: "#ffd2a8", mid: "#ff7a45", deep: "#7a3b8a", accent: "#fff0d8", ink: "#3a1e28" },
    meadow: { sky: "#e8f4ff", mid: "#8fd08a", deep: "#4a8f4a", accent: "#fffdf6", ink: "#2a3a20" },
  };
  return byTheme[theme] || byTheme.meadow;
}

function paper(ctx, w, h, rand, base = "#fbf7f0") {
  ctx.fillStyle = base;
  ctx.fillRect(0, 0, w, h);
  const img = ctx.getImageData(0, 0, w, h);
  const d = img.data;
  for (let i = 0; i < d.length; i += 4) {
    const n = (rand() - 0.5) * 14;
    d[i] = Math.min(255, Math.max(0, d[i] + n));
    d[i + 1] = Math.min(255, Math.max(0, d[i + 1] + n));
    d[i + 2] = Math.min(255, Math.max(0, d[i + 2] + n));
  }
  ctx.putImageData(img, 0, 0);
}

function wash(ctx, x, y, r, color, alpha = 0.35) {
  const g = ctx.createRadialGradient(x, y, r * 0.1, x, y, r);
  g.addColorStop(0, color);
  g.addColorStop(1, "rgba(255,255,255,0)");
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.fillStyle = g;
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();
}

function pencilStroke(ctx, points, color, width = 1.4) {
  if (points.length < 2) return;
  ctx.save();
  ctx.strokeStyle = color;
  ctx.lineWidth = width;
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.globalAlpha = 0.75;
  ctx.beginPath();
  ctx.moveTo(points[0][0], points[0][1]);
  for (let i = 1; i < points.length; i++) ctx.lineTo(points[i][0], points[i][1]);
  ctx.stroke();
  ctx.restore();
}

function blob(ctx, cx, cy, rx, ry, color, alpha = 0.55) {
  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.ellipse(cx, cy, rx, ry, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();
}

function drawSea(ctx, w, h, pal, rand) {
  const horizon = h * 0.38;
  // sky
  const sky = ctx.createLinearGradient(0, 0, 0, horizon);
  sky.addColorStop(0, pal.accent);
  sky.addColorStop(1, pal.sky);
  ctx.fillStyle = sky;
  ctx.fillRect(0, 0, w, horizon);
  // sun
  wash(ctx, w * 0.72, horizon * 0.55, w * 0.18, "#ffe08a", 0.55);
  blob(ctx, w * 0.72, horizon * 0.55, w * 0.07, w * 0.07, "#ffd36a", 0.85);
  // sea bands
  const sea = ctx.createLinearGradient(0, horizon, 0, h);
  sea.addColorStop(0, pal.mid);
  sea.addColorStop(0.55, pal.deep);
  sea.addColorStop(1, "#0f3a55");
  ctx.fillStyle = sea;
  ctx.fillRect(0, horizon, w, h - horizon);
  for (let i = 0; i < 7; i++) {
    const y = horizon + (h - horizon) * (0.12 + i * 0.12);
    const amp = 6 + rand() * 10;
    const pts = [];
    for (let x = 0; x <= w; x += 14) {
      pts.push([x, y + Math.sin(x * 0.03 + i) * amp + (rand() - 0.5) * 2]);
    }
    pencilStroke(ctx, pts, pal.accent, 1.2);
  }
  // foam
  for (let i = 0; i < 18; i++) {
    blob(ctx, rand() * w, horizon + (h - horizon) * (0.55 + rand() * 0.4), 3 + rand() * 6, 2 + rand() * 3, "#ffffff", 0.35);
  }
  // shore
  blob(ctx, w * 0.5, h * 0.92, w * 0.55, h * 0.08, "#f0d7b0", 0.55);
}

function drawForest(ctx, w, h, pal, rand) {
  const sky = ctx.createLinearGradient(0, 0, 0, h * 0.55);
  sky.addColorStop(0, pal.sky);
  sky.addColorStop(1, pal.accent);
  ctx.fillStyle = sky;
  ctx.fillRect(0, 0, w, h);
  blob(ctx, w * 0.5, h * 0.85, w * 0.7, h * 0.25, pal.mid, 0.35);
  for (let i = 0; i < 9; i++) {
    const x = (i + 0.4) * (w / 9) + (rand() - 0.5) * 10;
    const trunkH = h * (0.22 + rand() * 0.18);
    const base = h * 0.78;
    ctx.fillStyle = "#6b4226";
    ctx.fillRect(x - 4, base - trunkH, 8, trunkH);
    wash(ctx, x, base - trunkH, 28 + rand() * 24, pal.deep, 0.45);
    wash(ctx, x + 8, base - trunkH + 10, 22, pal.mid, 0.4);
  }
}

function drawMeadow(ctx, w, h, pal, rand) {
  const sky = ctx.createLinearGradient(0, 0, 0, h * 0.55);
  sky.addColorStop(0, "#dff0ff");
  sky.addColorStop(1, pal.sky);
  ctx.fillStyle = sky;
  ctx.fillRect(0, 0, w, h);
  const ground = ctx.createLinearGradient(0, h * 0.5, 0, h);
  ground.addColorStop(0, "#b7e39a");
  ground.addColorStop(1, pal.deep);
  ctx.fillStyle = ground;
  ctx.fillRect(0, h * 0.55, w, h * 0.45);
  for (let i = 0; i < 40; i++) {
    const x = rand() * w;
    const y = h * (0.58 + rand() * 0.35);
    pencilStroke(ctx, [[x, y], [x + (rand() - 0.5) * 6, y - 10 - rand() * 12]], pal.ink, 1);
  }
}

function drawSunset(ctx, w, h, pal, rand) {
  const g = ctx.createLinearGradient(0, 0, 0, h);
  g.addColorStop(0, "#3a1d5c");
  g.addColorStop(0.45, pal.mid);
  g.addColorStop(0.7, pal.sky);
  g.addColorStop(1, "#2a1830");
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, w, h);
  wash(ctx, w * 0.5, h * 0.42, w * 0.28, "#ffb36a", 0.65);
  blob(ctx, w * 0.5, h * 0.42, w * 0.1, w * 0.1, "#ffd27a", 0.9);
  for (let i = 0; i < 5; i++) {
    blob(ctx, rand() * w, h * (0.2 + rand() * 0.25), 40 + rand() * 50, 16 + rand() * 12, "#ffffff", 0.12);
  }
}

function drawFlowersBg(ctx, w, h, pal, rand) {
  drawMeadow(ctx, w, h, pal, rand);
  const colors = ["#f4a0b8", "#f6d35a", "#8ec5f0", "#e85d7a", "#c9a6e8"];
  for (let i = 0; i < 12; i++) {
    const x = w * (0.1 + rand() * 0.8);
    const y = h * (0.45 + rand() * 0.4);
    const c = colors[i % colors.length];
    for (let p = 0; p < 5; p++) {
      const a = (p / 5) * Math.PI * 2;
      blob(ctx, x + Math.cos(a) * 10, y + Math.sin(a) * 10, 8, 10, c, 0.7);
    }
    blob(ctx, x, y, 5, 5, "#ffe08a", 0.9);
    pencilStroke(ctx, [[x, y + 8], [x + (rand() - 0.5) * 4, y + 40]], "#3f6b3a", 1.5);
  }
}

function chibiBody(ctx, x, y, scale, body, face, ink, rand) {
  // body
  blob(ctx, x, y + 18 * scale, 22 * scale, 28 * scale, body, 0.95);
  // head
  blob(ctx, x, y - 18 * scale, 26 * scale, 24 * scale, face, 0.98);
  // ears / cheeks / eyes
  blob(ctx, x - 10 * scale, y - 10 * scale, 4 * scale, 3 * scale, "#f5a0b0", 0.55);
  blob(ctx, x + 10 * scale, y - 10 * scale, 4 * scale, 3 * scale, "#f5a0b0", 0.55);
  ctx.fillStyle = ink;
  ctx.beginPath();
  ctx.arc(x - 8 * scale, y - 18 * scale, 2.2 * scale, 0, Math.PI * 2);
  ctx.arc(x + 8 * scale, y - 18 * scale, 2.2 * scale, 0, Math.PI * 2);
  ctx.fill();
  // smile
  pencilStroke(
    ctx,
    [
      [x - 6 * scale, y - 10 * scale],
      [x, y - 7 * scale],
      [x + 6 * scale, y - 10 * scale],
    ],
    ink,
    1.4,
  );
  // outline wobble
  const pts = [];
  for (let i = 0; i <= 16; i++) {
    const a = (i / 16) * Math.PI * 2;
    const rr = 26 * scale + (rand() - 0.5) * 2;
    pts.push([x + Math.cos(a) * rr, y - 18 * scale + Math.sin(a) * 24 * scale]);
  }
  pencilStroke(ctx, pts, ink, 1.1);
}

function drawFox(ctx, w, h, pal, rand) {
  drawMeadow(ctx, w, h, pal, rand);
  const x = w * 0.5;
  const y = h * 0.58;
  const s = Math.min(w, h) / 220;
  blob(ctx, x, y, 34 * s, 28 * s, pal.mid, 0.95);
  blob(ctx, x, y - 34 * s, 28 * s, 24 * s, pal.mid, 0.98);
  // ears
  ctx.fillStyle = pal.mid;
  ctx.beginPath();
  ctx.moveTo(x - 18 * s, y - 48 * s);
  ctx.lineTo(x - 6 * s, y - 30 * s);
  ctx.lineTo(x - 22 * s, y - 28 * s);
  ctx.fill();
  ctx.beginPath();
  ctx.moveTo(x + 18 * s, y - 48 * s);
  ctx.lineTo(x + 6 * s, y - 30 * s);
  ctx.lineTo(x + 22 * s, y - 28 * s);
  ctx.fill();
  blob(ctx, x, y + 6 * s, 12 * s, 10 * s, "#fff6e8", 0.95);
  ctx.fillStyle = pal.ink;
  ctx.beginPath();
  ctx.arc(x - 8 * s, y - 34 * s, 2.4 * s, 0, Math.PI * 2);
  ctx.arc(x + 8 * s, y - 34 * s, 2.4 * s, 0, Math.PI * 2);
  ctx.fill();
  // tail
  blob(ctx, x + 40 * s, y + 10 * s, 28 * s, 12 * s, pal.deep, 0.9);
  blob(ctx, x + 58 * s, y + 6 * s, 10 * s, 8 * s, "#fff6e8", 0.95);
}

function drawRabbit(ctx, w, h, pal, rand) {
  drawMeadow(ctx, w, h, pal, rand);
  const x = w * 0.5;
  const y = h * 0.6;
  const s = Math.min(w, h) / 220;
  blob(ctx, x, y, 26 * s, 30 * s, "#f7f2ff", 0.96);
  blob(ctx, x, y - 32 * s, 24 * s, 22 * s, "#f7f2ff", 0.98);
  blob(ctx, x - 10 * s, y - 60 * s, 7 * s, 22 * s, "#f7f2ff", 0.95);
  blob(ctx, x + 10 * s, y - 60 * s, 7 * s, 22 * s, "#f7f2ff", 0.95);
  blob(ctx, x - 10 * s, y - 60 * s, 3 * s, 12 * s, "#f5a0b0", 0.55);
  blob(ctx, x + 10 * s, y - 60 * s, 3 * s, 12 * s, "#f5a0b0", 0.55);
  ctx.fillStyle = pal.ink;
  ctx.beginPath();
  ctx.arc(x - 7 * s, y - 34 * s, 2.2 * s, 0, Math.PI * 2);
  ctx.arc(x + 7 * s, y - 34 * s, 2.2 * s, 0, Math.PI * 2);
  ctx.fill();
}

function drawCat(ctx, w, h, pal, rand) {
  drawMeadow(ctx, w, h, { ...pal, mid: "#cfd6dd", deep: "#6b7280" }, rand);
  const x = w * 0.5;
  const y = h * 0.58;
  const s = Math.min(w, h) / 220;
  blob(ctx, x, y, 28 * s, 26 * s, "#f4f4f4", 0.96);
  blob(ctx, x, y - 30 * s, 24 * s, 22 * s, "#f4f4f4", 0.98);
  ctx.fillStyle = "#f4f4f4";
  ctx.beginPath();
  ctx.moveTo(x - 16 * s, y - 42 * s);
  ctx.lineTo(x - 4 * s, y - 28 * s);
  ctx.lineTo(x - 20 * s, y - 26 * s);
  ctx.fill();
  ctx.beginPath();
  ctx.moveTo(x + 16 * s, y - 42 * s);
  ctx.lineTo(x + 4 * s, y - 28 * s);
  ctx.lineTo(x + 20 * s, y - 26 * s);
  ctx.fill();
  ctx.fillStyle = pal.ink;
  ctx.beginPath();
  ctx.arc(x - 7 * s, y - 32 * s, 2.3 * s, 0, Math.PI * 2);
  ctx.arc(x + 7 * s, y - 32 * s, 2.3 * s, 0, Math.PI * 2);
  ctx.fill();
  blob(ctx, x + 36 * s, y + 8 * s, 22 * s, 8 * s, "#f4f4f4", 0.9);
}

function drawGirl(ctx, w, h, pal, rand) {
  drawMeadow(ctx, w, h, pal, rand);
  const x = w * 0.5;
  const y = h * 0.55;
  const s = Math.min(w, h) / 200;
  chibiBody(ctx, x, y, s, "#f2ebe3", "#ffe6d8", pal.ink, rand);
  // hair bun
  blob(ctx, x + 14 * s, y - 34 * s, 10 * s, 9 * s, "#c9b8a8", 0.95);
  blob(ctx, x, y - 40 * s, 28 * s, 12 * s, "#c9b8a8", 0.85);
}

function drawBoatOnSea(ctx, w, h, pal, rand) {
  drawSea(ctx, w, h, pal, rand);
  const x = w * 0.35;
  const y = h * 0.62;
  ctx.fillStyle = "#d9a066";
  ctx.beginPath();
  ctx.moveTo(x - 28, y);
  ctx.lineTo(x + 28, y);
  ctx.lineTo(x + 18, y + 14);
  ctx.lineTo(x - 18, y + 14);
  ctx.closePath();
  ctx.fill();
  pencilStroke(ctx, [[x, y], [x, y - 34]], pal.ink, 2);
  ctx.fillStyle = "#fff7e8";
  ctx.beginPath();
  ctx.moveTo(x, y - 34);
  ctx.lineTo(x + 22, y - 16);
  ctx.lineTo(x, y - 10);
  ctx.closePath();
  ctx.fill();
}

function renderScene(ctx, w, h, theme, pal, rand, prompt) {
  paper(ctx, w, h, rand, pal.accent);
  switch (theme) {
    case "sea":
      if (/船|boat|纸船/.test(prompt)) drawBoatOnSea(ctx, w, h, pal, rand);
      else drawSea(ctx, w, h, pal, rand);
      break;
    case "fox":
      drawFox(ctx, w, h, pal, rand);
      break;
    case "rabbit":
      drawRabbit(ctx, w, h, pal, rand);
      break;
    case "flowers":
      drawFlowersBg(ctx, w, h, pal, rand);
      break;
    case "cat":
      drawCat(ctx, w, h, pal, rand);
      break;
    case "forest":
      drawForest(ctx, w, h, pal, rand);
      chibiBody(ctx, w * 0.5, h * 0.62, Math.min(w, h) / 240, "#e7c6a0", "#ffe6d8", pal.ink, rand);
      break;
    case "girl":
      drawGirl(ctx, w, h, pal, rand);
      break;
    case "sunset":
      drawSunset(ctx, w, h, pal, rand);
      break;
    default:
      drawMeadow(ctx, w, h, pal, rand);
      chibiBody(ctx, w * 0.5, h * 0.6, Math.min(w, h) / 230, "#e8d5c4", "#ffe6d8", pal.ink, rand);
      break;
  }
  // soft vignette
  const v = ctx.createRadialGradient(w / 2, h / 2, Math.min(w, h) * 0.35, w / 2, h / 2, Math.max(w, h) * 0.75);
  v.addColorStop(0, "rgba(255,255,255,0)");
  v.addColorStop(1, "rgba(80,50,30,0.12)");
  ctx.fillStyle = v;
  ctx.fillRect(0, 0, w, h);
}

function writeStubPng(file) {
  // 1x1 PNG
  const buf = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
    "base64",
  );
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, buf);
}

async function main() {
  const args = parseArgs(process.argv);
  if (!args.prompt || !args.output) {
    fail("usage: draw.mjs --prompt TEXT --output out.png [--aspect-ratio 2:3] [--seed N]");
  }

  let w;
  let h;
  if (args.width > 0 && args.height > 0) {
    w = even(args.width);
    h = even(args.height);
  } else {
    const ar = ASPECT[args.aspect] || ASPECT["2:3"];
    w = even(ar[0]);
    h = even(ar[1]);
  }
  const aspect = args.aspect || "2:3";
  const seed = args.seed > 0 ? args.seed >>> 0 : hashSeed(args.prompt + "|" + aspect);
  const rand = mulberry32(seed);
  const theme = detectTheme(args.prompt);
  const pal = detectPalette(args.prompt, theme);

  fs.mkdirSync(path.dirname(args.output), { recursive: true });
  if (args.dryRun) {
    writeStubPng(args.output);
    ok({
      path: args.output,
      prompt: args.prompt,
      theme,
      aspect_ratio: aspect,
      width: w,
      height: h,
      seed,
      bytes: fs.statSync(args.output).size,
      model: "canvas-2d",
      device: "cpu",
      dry_run: true,
      skill: "picturebook-illustrate",
      script: path.join(__dirname, "draw.mjs"),
    });
    return;
  }

  const canvas = createCanvas(w, h);
  const ctx = canvas.getContext("2d");
  renderScene(ctx, w, h, theme, pal, rand, args.prompt);
  const png = canvas.toBuffer("image/png");
  fs.writeFileSync(args.output, png);

  ok({
    path: args.output,
    prompt: args.prompt,
    theme,
    aspect_ratio: aspect,
    width: w,
    height: h,
    seed,
    bytes: png.length,
    model: "canvas-2d",
    device: "cpu",
    skill: "picturebook-illustrate",
    script: path.join(__dirname, "draw.mjs"),
  });
}

main().catch((err) => fail(err?.message || String(err)));
