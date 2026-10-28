import clsx from "clsx";
import type { InputHTMLAttributes, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";

// Shared look for text-like controls. 44px high, 16px text (stops iOS zooming on focus).
export const controlClass = clsx(
  "w-full rounded-input border border-line bg-bg px-3 text-base text-ink placeholder:text-muted",
  "aria-[invalid=true]:border-danger disabled:bg-surface disabled:text-muted",
);

export function Input({ className, ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={clsx(controlClass, "h-11", className)} {...rest} />;
}

export function Textarea({ className, rows = 3, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={rows} className={clsx(controlClass, "py-2", className)} {...rest} />;
}

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={clsx(controlClass, "h-11", className)} {...rest}>
      {children}
    </select>
  );
}
