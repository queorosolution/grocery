import { Field, Input } from "@/components/ui";

interface TextProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  hint?: string;
  error?: string;
}

/** Rupees typed as text ("1,250.50"). The caller turns it into whole cents with parseRupees. */
export const MoneyField = ({ label, value, onChange, hint, error }: TextProps) => (
  <Field label={label} hint={hint} error={error}>{(p) => <Input {...p} inputMode="decimal" value={value} onChange={(e) => onChange(e.target.value)} />}</Field>
);

/** A whole number typed as text. The caller checks it with parseInt10. */
export const NumberField = ({ label, value, onChange, hint, error }: TextProps) => (
  <Field label={label} hint={hint} error={error}>{(p) => <Input {...p} inputMode="numeric" value={value} onChange={(e) => onChange(e.target.value)} />}</Field>
);

/** A number with up to 2 decimals typed as text (km, kg). The caller checks it with parseDec2. */
export const DecimalField = ({ label, value, onChange, hint, error }: TextProps) => (
  <Field label={label} hint={hint} error={error}>{(p) => <Input {...p} inputMode="decimal" value={value} onChange={(e) => onChange(e.target.value)} />}</Field>
);
