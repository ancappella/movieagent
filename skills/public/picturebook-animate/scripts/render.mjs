#!/usr/bin/env node
/**
 * LLM animation program → Playwright frame capture → ffmpeg MP4.
 *
 * Animation contract (animation.js):
 *   function draw(t, ctx, canvas, assets)  // required; t in seconds
 *   function seek(t)                       // optional
 *   async function setup({canvas,ctx,width,height,assets}) // optional
 *
 * Prints one JSON object on stdout when finished.
 */
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import ffmpegPath from "ffmpeg-static";
import { chromium } from "playwright";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const out = {
    code: "",
    codeFile: "",
    template: "",
    asset: "",
    output: "",
    duration: 4,
    fps: 24,
    width: 512,
    height: 768,
    dryRun: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    const next = () => argv[++i];
    switch (a) {
      case "--code":
        out.code = next();
        break;
      case "--code-file":
        out.codeFile = next();
        break;
      case "--template":
        out.template = next();
        break;
      case "--asset":
        out.asset = next();
        break;
      case "--output":
        out.output = next();
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

function fail(msg) {
  process.stdout.write(JSON.stringify({ ok: false, error: msg }) + "\n");
  process.exit(1);
}

function ok(payload) {
  process.stdout.write(JSON.stringify({ ok: true, ...payload }) + "\n");
}

function writeStubMp4(file) {
  const buf = Buffer.from([
    0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d, 0x00, 0x00, 0x00, 0x01,
    0x69, 0x73, 0x6f, 0x6d, 0x61, 0x76, 0x63, 0x31, 0x00, 0x00, 0x00, 0x08, 0x66, 0x72, 0x65, 0x65,
    0x00, 0x00, 0x00, 0x08, 0x6d, 0x64, 0x61, 0x74,
  ]);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, buf);
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

function resolveAnimationSource(args) {
  if (args.codeFile) {
    const p = path.resolve(args.codeFile);
    if (!fs.existsSync(p)) fail(`code file not found: ${p}`);
    return fs.readFileSync(p, "utf8");
  }
  if (String(args.code || "").trim()) {
    return String(args.code);
  }
  const name = String(args.template || "character_spin").trim() || "character_spin";
  const tpl = path.join(__dirname, "templates", `${name}.js`);
  if (!fs.existsSync(tpl)) {
    fail(`unknown template: ${name}`);
  }
  return fs.readFileSync(tpl, "utf8");
}

function wrapAnimationSource(src, durationSec) {
  // Inject duration constant for templates; keep user code intact.
  return `var __DURATION_SEC = ${Number(durationSec) || 4};\n${src}\n`;
}

async function main() {
  const args = parseArgs(process.argv);
  if (!args.output) {
    fail(
      "usage: render.mjs --output out.mp4 [--code-file anim.js | --code '...'] [--template character_spin] [--asset img.png] [--duration 4] [--fps 24]",
    );
  }

  const duration = Math.min(30, Math.max(1, Number(args.duration) || 4));
  const fps = Math.min(60, Math.max(8, Math.round(Number(args.fps) || 24)));
  const width = even(args.width > 0 ? args.width : 512);
  const height = even(args.height > 0 ? args.height : 768);
  const frames = Math.max(1, Math.round(duration * fps));

  fs.mkdirSync(path.dirname(args.output), { recursive: true });

  if (args.dryRun) {
    writeStubMp4(args.output);
    ok({
      path: args.output,
      duration_sec: duration,
      fps,
      frames,
      width,
      height,
      bytes: fs.statSync(args.output).size,
      dry_run: true,
      renderer: "playwright+ffmpeg",
      skill: "picturebook-animate",
      script: path.join(__dirname, "render.mjs"),
    });
    return;
  }

  const source = wrapAnimationSource(resolveAnimationSource(args), duration);
  const work = fs.mkdtempSync(path.join(os.tmpdir(), "pb-render-"));
  const framesDir = path.join(work, "frames");
  fs.mkdirSync(framesDir, { recursive: true });

  try {
    fs.copyFileSync(path.join(__dirname, "harness.html"), path.join(work, "harness.html"));
    fs.writeFileSync(path.join(work, "animation.js"), source, "utf8");

    let assetURL = "";
    if (args.asset && fs.existsSync(args.asset)) {
      const ext = path.extname(args.asset) || ".png";
      const assetPath = path.join(work, `asset${ext}`);
      fs.copyFileSync(args.asset, assetPath);
      assetURL = pathToFileURL(assetPath).href;
    }

    const pageURL =
      pathToFileURL(path.join(work, "harness.html")).href +
      `?w=${width}&h=${height}&asset=${encodeURIComponent(assetURL)}`;

    const browser = await chromium.launch({
      headless: true,
      args: ["--disable-dev-shm-usage"],
    });
    try {
      const page = await browser.newPage({
        viewport: { width, height },
        deviceScaleFactor: 1,
      });
      await page.goto(pageURL, { waitUntil: "domcontentloaded", timeout: 60_000 });
      await page.waitForFunction(
        () => window.__MA && (window.__MA.ready === true || window.__MA.error),
        null,
        { timeout: 60_000 },
      );
      const bootErr = await page.evaluate(() => window.__MA.error);
      if (bootErr) {
        fail(`animation boot failed: ${bootErr}`);
      }

      for (let i = 0; i < frames; i++) {
        const t = frames === 1 ? 0 : (i / (frames - 1)) * duration;
        await page.evaluate((sec) => window.__maFrame(sec), t);
        const framePath = path.join(framesDir, `frame_${String(i).padStart(5, "0")}.png`);
        await page.locator("#c").screenshot({ path: framePath, type: "png" });
      }
    } finally {
      await browser.close();
    }

    await runFfmpeg([
      "-y",
      "-framerate",
      String(fps),
      "-i",
      path.join(framesDir, "frame_%05d.png"),
      "-c:v",
      "libx264",
      "-pix_fmt",
      "yuv420p",
      "-movflags",
      "+faststart",
      args.output,
    ]);
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }

  const st = fs.statSync(args.output);
  ok({
    path: args.output,
    duration_sec: duration,
    fps,
    frames,
    width,
    height,
    bytes: st.size,
    renderer: "playwright+ffmpeg",
    has_code: Boolean(String(args.code || args.codeFile || "").trim()),
    template: args.template || "",
    skill: "picturebook-animate",
    script: path.join(__dirname, "render.mjs"),
  });
}

main().catch((err) => fail(err?.message || String(err)));
