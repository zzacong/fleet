import sharp from "sharp";

const W = 1200;
const H = 630;

// Convoy V mark paths, scaled 3.5x and centered at (600, 205).
const S = 3.5;
const CX = 600;
const CY = 205;
const tx = CX - 32 * S;
const ty = CY - 32 * S;

const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
  <rect width="${W}" height="${H}" fill="#0C1116"/>
  <g transform="translate(${tx},${ty}) scale(${S})">
    <g fill="#F1F5F9" stroke-linejoin="round">
      <path d="M32 6 L42 24 L36 24 L32 17 L28 24 L22 24 Z"/>
      <path d="M16 34 L26 52 L20 52 L16 45 L12 52 L6 52 Z"/>
      <path d="M48 34 L58 52 L52 52 L48 45 L44 52 L38 52 Z"/>
    </g>
    <circle cx="32" cy="52" r="3.5" fill="#FF4D00"/>
  </g>
  <text x="600" y="448" text-anchor="middle" font-family="Helvetica, Arial, sans-serif" font-size="92" font-weight="800" letter-spacing="18" fill="#F1F5F9">FLEET</text>
  <text x="600" y="512" text-anchor="middle" font-family="Helvetica, Arial, sans-serif" font-size="34" fill="#94A3B8">Manage agent skills across AI coding agents.</text>
</svg>`;

await sharp(Buffer.from(svg)).png().toFile("public/og-image.png");
console.log("wrote public/og-image.png");
