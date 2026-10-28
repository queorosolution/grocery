import clsx from "clsx";
import type { HTMLAttributes } from "react";

/** Page width: 16px gutter on phones, 1120px max on desktop. */
export function Page({ className, ...rest }: HTMLAttributes<HTMLElement>) {
  return <main className={clsx("mx-auto max-w-[1120px] px-4 py-4 md:py-6", className)} {...rest} />;
}
