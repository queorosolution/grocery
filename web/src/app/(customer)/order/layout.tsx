import type { Metadata } from "next";

// Private pages: no search results, no link previews.
export const metadata: Metadata = { robots: { index: false, follow: false } };

export default function OrderLayout({ children }: { children: React.ReactNode }) {
  return children;
}
