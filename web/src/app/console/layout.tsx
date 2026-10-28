import type { Metadata } from "next";
import { Header } from "@/features/shell/Header";

export const metadata: Metadata = { robots: { index: false, follow: false } };

// The sign-in and role guard arrive with the auth module.
export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Header />
      {children}
    </>
  );
}
