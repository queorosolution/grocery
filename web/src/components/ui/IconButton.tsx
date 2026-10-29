import clsx from "clsx";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label" | "children"> {
  /** Required: an icon alone tells a screen reader nothing. */
  "aria-label": string;
  icon: ReactNode;
  variant?: "ghost" | "secondary";
}

export function IconButton({ icon, variant = "ghost", className, type = "button", ...rest }: IconButtonProps) {
  return (
    <button
      type={type}
      className={clsx(
        "inline-flex size-11 shrink-0 items-center justify-center rounded-pill text-ink transition-colors",
        "disabled:cursor-not-allowed disabled:text-muted",
        variant === "secondary" ? "border border-line bg-bg hover:bg-surface" : "hover:bg-surface",
        className,
      )}
      {...rest}
    >
      <span aria-hidden="true" className="inline-flex">{icon}</span>
    </button>
  );
}
