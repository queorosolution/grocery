import clsx from "clsx";
import type { HTMLAttributes } from "react";

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  /** Raises on hover (desktop only). Use with a click handler or a link inside. */
  interactive?: boolean;
}

export function Card({ interactive, className, ...rest }: CardProps) {
  return (
    <div
      className={clsx(
        "rounded-card border border-line bg-bg shadow-card",
        interactive && "transition-shadow md:hover:shadow-raise",
        className,
      )}
      {...rest}
    />
  );
}
