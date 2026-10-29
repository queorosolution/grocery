import type { Metadata } from "next";
import { ConsoleGate } from "@/features/auth/guards";
import { Header } from "@/features/shell/Header";

export const metadata: Metadata = { robots: { index: false, follow: false } };

// Signed-in team members only. The checks here are for the screen; the API checks every call again.
export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Header />
      <ConsoleGate>{children}</ConsoleGate>
    </>
  );
}
