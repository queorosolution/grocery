import clsx from "clsx";

interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  disabled?: boolean;
  className?: string;
}

export function Switch({ checked, onChange, label, disabled, className }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      // The button is 44px tall for the tap target; the track inside is smaller.
      className={clsx("inline-flex h-11 w-14 shrink-0 items-center justify-center disabled:opacity-50", className)}
    >
      <span className={clsx("relative h-7 w-12 rounded-pill transition-colors", checked ? "bg-accent" : "bg-line")}>
        <span
          className={clsx(
            "absolute top-0.5 size-6 rounded-pill bg-white shadow-card transition-transform",
            checked ? "translate-x-[22px]" : "translate-x-0.5",
          )}
        />
      </span>
    </button>
  );
}
