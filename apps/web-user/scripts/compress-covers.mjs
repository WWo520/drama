// 一次性脚本：将 ImageGen 产出的大 PNG 压成 JPG（封面宽 800 / 横幅宽 1440，质量 80）
// 用法：node scripts/compress-covers.mjs
import sharp from "sharp";
import { readdir, unlink } from "node:fs/promises";
import path from "node:path";

const dir = path.resolve("src/assets/covers");
const files = await readdir(dir);
for (const f of files) {
  if (!f.endsWith(".png")) continue;
  const src = path.join(dir, f);
  const out = path.join(dir, f.replace(/\.png$/, ".jpg"));
  const width = f.startsWith("banner") ? 1440 : 800;
  await sharp(src).resize({ width }).jpeg({ quality: 80 }).toFile(out);
  await unlink(src);
  console.log(`OK ${f} -> ${path.basename(out)}`);
}
