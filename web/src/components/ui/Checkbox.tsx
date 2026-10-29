import clsx from "clsx";
import type { InputHTMLAttributes } from "react";

interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "onChange"> {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}

export function Checkbox({ label, checked, onChange, className, ...rest }: CheckboxProps) {
  return (
    <label className={clsx("flex min-h-11 cursor-pointer items-center gap-3 text-base", className)}>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="size-5 shrink-0 rounded-[4px] border border-line accent-[var(--c-accent)] aria-[invalid=true]:border-danger"
        {...rest}
      />
      <span>{label}</span>
    </label>
  );
}
