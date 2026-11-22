import type { PropsWithChildren } from "react";
import { HelpHint } from "./help-hint";

type FieldProps = PropsWithChildren<{
  label: string;
  htmlFor?: string;
  required?: boolean;
  error?: string;
  help?: string;
  hint?: string;
}>;

export function Field({
  label,
  htmlFor,
  required,
  error,
  help,
  hint,
  children,
}: FieldProps) {
  const fieldLabel = (
    <label htmlFor={htmlFor} className="block text-form-label font-medium text-text-primary">
      {label}
      {required && <span className="ml-space-1 text-error">*</span>}
    </label>
  );
  return (
    <div className="space-y-1.5">
      {hint ? (
        <div className="flex items-center gap-1">
          {fieldLabel}
          <HelpHint label={`${label}说明`}>{hint}</HelpHint>
        </div>
      ) : fieldLabel}
      {children}
      {(!hint || error || help) && (
        <p className="min-h-[18px] text-helper text-text-tertiary">
          {error ? <span className="text-error">{error}</span> : help}
        </p>
      )}
    </div>
  );
}
