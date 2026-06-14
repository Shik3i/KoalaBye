import path from "node:path";
import { fileURLToPath } from "node:url";
import sharp from "sharp";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(scriptDir, "..", "..");
const source = path.join(projectRoot, "web", "static", "img", "logo.svg");
const outputDir = path.join(projectRoot, "web", "static", "img");

const targets = [
  ["favicon-16x16.png", 16],
  ["favicon-32x32.png", 32],
  ["apple-touch-icon.png", 180],
  ["android-chrome-192x192.png", 192],
  ["android-chrome-512x512.png", 512],
];

await Promise.all(
  targets.map(([name, size]) =>
    sharp(source, { density: 512 })
      .resize(size, size, { fit: "contain" })
      .png({ compressionLevel: 9, palette: true })
      .toFile(path.join(outputDir, name)),
  ),
);

console.log(`Generated ${targets.length} favicon assets from ${source}`);
