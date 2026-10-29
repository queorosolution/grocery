import type { MessageKey } from "@/lib/i18n";

/** The admin pages in the order they appear in the menu. Each part of P33 adds its own lines. */
export const SECTIONS: readonly { path: string; label: MessageKey }[] = [
  { path: "", label: "admin.section.dashboard" },
  { path: "shops", label: "admin.section.shops" },
  { path: "sync-issues", label: "admin.section.issues" },
];
