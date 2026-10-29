import { useMemo } from "react";
import { qrMatrix } from "@/lib/qr";

interface QrCodeProps {
  value: string;
  /** What the code opens, for a screen reader. */
  label: string;
  size?: number;
}

/** A QR code as one SVG path. Always black on white with the 4-module quiet zone scanners need. */
export function QrCode({ value, label, size = 176 }: QrCodeProps) {
  const { path, side } = useMemo(() => {
    const m = qrMatrix(value);
    const quiet = 4;
    const runs: string[] = [];
    m.forEach((row, y) => {
      for (let x = 0; x < row.length; x++) {
        if (!row[x]) continue;
        const start = x;
        while (row[x + 1]) x++;
        runs.push(`M${start + quiet} ${y + quiet}h${x - start + 1}v1h-${x - start + 1}z`);
      }
    });
    return { path: runs.join(""), side: m.length + quiet * 2 };
  }, [value]);
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${side} ${side}`} width={size} height={size} shapeRendering="crispEdges">
      <rect width={side} height={side} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  );
}
