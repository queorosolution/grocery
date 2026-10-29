import clsx from "clsx";
import type { ButtonHTMLAttributes } from "react";
import { Spinner } from "./Spinner";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: "md" | "lg";
  loading?: boolean;
  /** Fill the row. Handy for sticky bars on phones. */
  block?: boolean;
}

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent text-on-accent hover:brightness-95 disabled:bg-line disabled:text-muted",
  secondary: "border border-ink bg-bg text-ink hover:bg-surface disabled:border-line disabled:text-muted",
  ghost: "bg-transparent text-ink underline-offset-2 hover:bg-surface",
  danger: "bg-danger text-white hover:brightness-95 disabled:bg-line disabled:text-muted",
};

// md is 44px high: the minimum tap target on a phone.
const sizes = { md: "h-11 px-4", lg: "h-12 px-6" } as const;

export function Button({
  variant = "primary", size = "md", loading = false, block = false, className, children, disabled, type = "button", ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={clsx(
        "inline-flex items-center justify-center gap-2 rounded-input text-base font-semibold",
        "transition-colors disabled:cursor-not-allowed",
        variants[variant], sizes[size], block && "w-full", className,
      )}
      {...rest}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}
