// 一次性脚本：压缩新增活动横幅 PNG → banner-event.jpg（宽 1440，质量 80）
import sharp from "sharp";
import path from "node:path";

const src = "C:/Users/yz02/.qoder/vibe_images/banner-event_1787043000.png";
const out = path.resolve("src/assets/covers/banner-event.jpg");
const info = await sharp(src).resize({ width: 1440 }).jpeg({ quality: 80 }).toFile(out);
console.log(`OK ${info.width}x${info.height} -> ${out}`);
