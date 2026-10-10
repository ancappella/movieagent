/**
 * Fallback template: slight bob/sway of assets.image; background stays white/static.
 */
let _duration = 4;

function setup({ assets }) {
  if (typeof __DURATION_SEC === "number" && __DURATION_SEC > 0) {
    _duration = __DURATION_SEC;
  }
  if (!assets.image) {
    const c = document.createElement("canvas");
    c.width = 240;
    c.height = 320;
    const g = c.getContext("2d");
    g.fillStyle = "#eef6ff";
    g.fillRect(0, 0, c.width, c.height);
    g.fillStyle = "#8fd08a";
    g.fillRect(0, 200, 240, 120);
    g.fillStyle = "#ffe6d8";
    g.beginPath();
    g.arc(120, 130, 48, 0, Math.PI * 2);
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
  const breath = Math.sin(u * Math.PI * 2 * 2);
  const sway = Math.sin(u * Math.PI * 2);
  const dy = breath * Math.max(2, h * 0.008);
  const rot = (sway * 2.2 * Math.PI) / 180;

  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, w, h);

  const iw = img.width || img.naturalWidth || 240;
  const ih = img.height || img.naturalHeight || 320;
  const fit = Math.min((w * 0.75) / iw, (h * 0.85) / ih);
  const dw = iw * fit;
  const dh = ih * fit;
  const cx = w / 2;
  const cy = h * 0.52;

  ctx.save();
  ctx.translate(cx, cy + dh * 0.45);
  ctx.rotate(rot);
  ctx.scale(1, 1 + breath * 0.015);
  ctx.translate(0, dy - dh * 0.45);
  ctx.drawImage(img, -dw / 2, -dh / 2, dw, dh);
  ctx.restore();
}
