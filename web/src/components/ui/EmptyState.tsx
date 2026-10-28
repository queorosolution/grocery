import clsx from "clsx";
import type { ReactNode } from "react";

interface EmptyStateProps {
  title: string;
  body?: string;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, body, icon, action, className }: EmptyStateProps) {
  return (
    <div className={clsx("flex flex-col items-center gap-2 px-4 py-10 text-center", className)}>
      {icon && <span aria-hidden="true" className="text-muted">{icon}</span>}
      <h2 className="text-lg font-semibold">{title}</h2>
      {body && <p className="max-w-sm text-muted">{body}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
