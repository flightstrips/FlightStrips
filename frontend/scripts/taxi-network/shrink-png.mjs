// Decode an 8-bit RGBA PNG, report colour usage, and re-encode it as a compact grayscale PNG.
// usage: node shrink-png.mjs in.png out.png [bits=8|4]
import fs from "node:fs";
import zlib from "node:zlib";

const [, , inPath, outPath, bitsArg] = process.argv;
const bits = Number(bitsArg || 8);
const buf = fs.readFileSync(inPath);
const W = buf.readUInt32BE(16), H = buf.readUInt32BE(20);
if (buf[24] !== 8 || buf[25] !== 6 || buf[28] !== 0) throw new Error("expected 8-bit RGBA, non-interlaced");
const idat = [];
for (let o = 8; o < buf.length;) {
  const len = buf.readUInt32BE(o), type = buf.toString("ascii", o + 4, o + 8);
  if (type === "IDAT") idat.push(buf.subarray(o + 8, o + 8 + len));
  o += 12 + len;
}
const raw = zlib.inflateSync(Buffer.concat(idat));
const bpp = 4, stride = W * bpp, px = Buffer.alloc(H * stride);
for (let y = 0; y < H; y++) {
  const f = raw[y * (stride + 1)], src = raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1));
  const row = px.subarray(y * stride, (y + 1) * stride), prev = y ? px.subarray((y - 1) * stride, y * stride) : null;
  for (let x = 0; x < stride; x++) {
    const a = x >= bpp ? row[x - bpp] : 0, b = prev ? prev[x] : 0, c = prev && x >= bpp ? prev[x - bpp] : 0;
    let v = src[x];
    if (f === 1) v += a; else if (f === 2) v += b; else if (f === 3) v += (a + b) >> 1;
    else if (f === 4) { const p = a + b - c, pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c); v += pa <= pb && pa <= pc ? a : pb <= pc ? b : c; }
    row[x] = v & 255;
  }
}
let maxSat = 0, minA = 255;
for (let i = 0; i < px.length; i += 4) {
  const r = px[i], g = px[i + 1], b = px[i + 2];
  maxSat = Math.max(maxSat, Math.max(r, g, b) - Math.min(r, g, b)); minA = Math.min(minA, px[i + 3]);
}
console.log(`${W}x${H} max colour spread ${maxSat}, min alpha ${minA}`);

// grayscale, composited over white where transparent
const levels = (1 << bits) - 1, rowBytes = Math.ceil(W * bits / 8);
const out = Buffer.alloc(H * (rowBytes + 1));
for (let y = 0; y < H; y++) {
  out[y * (rowBytes + 1)] = 0;
  for (let x = 0; x < W; x++) {
    const i = (y * W + x) * 4, a = px[i + 3] / 255;
    const l = (0.299 * px[i] + 0.587 * px[i + 1] + 0.114 * px[i + 2]) * a + 255 * (1 - a);
    const q = Math.round(l / 255 * levels);
    const o = y * (rowBytes + 1) + 1;
    if (bits === 8) out[o + x] = q;
    else out[o + (x >> 1)] |= x & 1 ? q : q << 4;
  }
}
const chunk = (type, data) => {
  const len = Buffer.alloc(4); len.writeUInt32BE(data.length);
  const td = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4); crc.writeUInt32BE(zlib.crc32(td) >>> 0);
  return Buffer.concat([len, td, crc]);
};
const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(W, 0); ihdr.writeUInt32BE(H, 4); ihdr[8] = bits; ihdr[9] = 0; ihdr[10] = 0; ihdr[11] = 0; ihdr[12] = 0;
const png = Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", zlib.deflateSync(out, { level: 9 })), chunk("IEND", Buffer.alloc(0))]);
fs.writeFileSync(outPath, png);
console.log(`wrote ${outPath}: ${(png.length / 1024).toFixed(0)} KB (${bits}-bit grayscale)`);
