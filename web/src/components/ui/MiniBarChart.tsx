import { t } from "@/lib/i18n";

export interface Bar {
  label: string;
  value: number;
}

interface MiniBarChartProps {
  bars: readonly Bar[];
  /** Names the chart for screen readers. */
  title: string;
  /** How a value is written in the table that stands in for the picture. */
  format?: (value: number) => string;
}

const WIDTH = 100;
const HEIGHT = 40;

/**
 * Small bar chart in plain SVG, scaled to its box. The values are also in a visually hidden table,
 * so the numbers are not only in the picture. No chart library (docs/08 section 1).
 */
export function MiniBarChart({ bars, title, format = String }: MiniBarChartProps) {
  const max = Math.max(1, ...bars.map((b) => b.value));
  const slot = WIDTH / Math.max(1, bars.length);
  return (
    <figure>
      <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={title} preserveAspectRatio="none" className="h-32 w-full">
        {bars.map((b, i) => {
          const h = (b.value / max) * (HEIGHT - 2);
          return <rect key={`${b.label}-${i}`} x={i * slot + slot * 0.15} y={HEIGHT - h} width={slot * 0.7} height={h} rx={0.8} className="fill-accent" />;
        })}
      </svg>
      <div aria-hidden="true" className="mt-1 flex justify-between text-xs text-muted">
        <span>{bars[0]?.label}</span>
        <span>{bars.at(-1)?.label}</span>
      </div>
      <table className="sr-only">
        <caption>{title}</caption>
        <thead><tr><th scope="col">{t("chart.day")}</th><th scope="col">{t("chart.value")}</th></tr></thead>
        <tbody>{bars.map((b, i) => <tr key={`${b.label}-${i}`}><th scope="row">{b.label}</th><td>{format(b.value)}</td></tr>)}</tbody>
      </table>
    </figure>
  );
}
