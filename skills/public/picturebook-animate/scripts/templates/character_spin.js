/**
 * Fallback template: fake Y-axis spin of assets.image (or a placeholder doll).
 * Contract: draw(t, ctx, canvas, assets) where t is seconds.
 */
let _duration = 4;

function setup({ assets }) {
  // duration injected by render.mjs as global __DURATION_SEC
  if (typeof __DURATION_SEC === "number" && __DURATION_SEC > 0) {
    _duration = __DURATION_SEC;
  }
  if (!assets.image) {
    // Placeholder when no asset is provided.
    const c = document.createElement("canvas");
    c.width = 256;
    c.height = 360;
    const g = c.getContext("2d");
    g.fillStyle = "#f4f1ea";
    g.fillRect(0, 0, c.width, c.height);
    g.fillStyle = "#e8d5c4";
    g.beginPath();
    g.ellipse(128, 210, 54, 70, 0, 0, Math.PI * 2);
    g.fill();
    g.fillStyle = "#ffe6d8";
    g.beginPath();
    g.arc(128, 120, 52, 0, Math.PI * 2);
    g.fill();
    g.fillStyle = "#333";
    g.beginPath();
    g.arc(110, 115, 5, 0, Math.PI * 2);
    g.arc(146, 115, 5, 0, Math.PI * 2);
    g.fill();
    assets.image = c;
  }
}

function seek(_t) {}

function draw(t, ctx, canvas, assets) {
  const w = canvas.width;
  const h = canvas.height;
  const img = assets.image;
  const u = _duration > 0 ? (t % _duration) / _duration : 0;
  const angle = u * Math.PI * 2;
  const sx = Math.cos(angle);
  const scaleX = Math.sign(sx || 1) * Math.max(0.08, Math.abs(sx));

  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, w, h);

  const iw = img.width || img.naturalWidth || 256;
  const ih = img.height || img.naturalHeight || 360;
  const fit = Math.min((w * 0.72) / iw, (h * 0.82) / ih);
  const dw = iw * fit;
  const dh = ih * fit;
  const cx = w / 2;
  const cy = h * 0.55;

  ctx.save();
  ctx.translate(cx, cy + dh * 0.42);
  ctx.scale(Math.abs(scaleX) * 1.05, 1);
  ctx.fillStyle = "rgba(0,0,0,0.10)";
  ctx.beginPath();
  ctx.ellipse(0, 0, dw * 0.28, Math.max(4, dh * 0.03), 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();

  ctx.save();
  ctx.translate(cx, cy);
  ctx.scale(scaleX, 1 - Math.abs(sx) * 0.04);
  ctx.drawImage(img, -dw / 2, -dh / 2, dw, dh);
  ctx.restore();
}
