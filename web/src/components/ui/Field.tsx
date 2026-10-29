import clsx from "clsx";
import { useId, type ReactNode } from "react";

/** What an input needs to be tied to its label, hint and error. */
export interface FieldControlProps {
  id: string;
  "aria-describedby": string | undefined;
  "aria-invalid": true | undefined;
}

interface FieldProps {
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  children: (control: FieldControlProps) => ReactNode;
}

/** Label + hint + error around any input: <Field label="Name">{(p) => <Input {...p} />}</Field>. */
export function Field({ label, hint, error, className, children }: FieldProps) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [errorId, hintId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={clsx("flex flex-col gap-1", className)}>
      <label htmlFor={id} className="text-sm font-semibold">{label}</label>
      {children({ id, "aria-describedby": describedBy, "aria-invalid": error ? true : undefined })}
      {hint && <p id={hintId} className="text-sm text-muted">{hint}</p>}
      {error && <p id={errorId} className="text-sm text-danger">{error}</p>}
    </div>
  );
}
