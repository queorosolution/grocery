import Link from "next/link";
import { UserMenu } from "@/features/auth/UserMenu";
import { env } from "@/lib/env";

/** Top bar: the platform name from config, never hard-coded (CLAUDE.md brand rule). */
export function Header() {
  return (
    <header className="sticky top-0 z-10 border-b border-line bg-bg">
      <div className="mx-auto flex h-14 max-w-[1120px] items-center justify-between px-4">
        <Link href="/" className="text-lg font-semibold text-accent">{env.platformName}</Link>
        <UserMenu />
      </div>
    </header>
  );
}
