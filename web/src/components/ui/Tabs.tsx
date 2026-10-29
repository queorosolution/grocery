"use client";

import clsx from "clsx";
import type { KeyboardEvent, ReactNode } from "react";

export interface TabItem {
  id: string;
  label: string;
}

const tabDomId = (prefix: string, id: string) => `${prefix}-tab-${id}`;
const panelDomId = (prefix: string, id: string) => `${prefix}-panel-${id}`;

interface TabsProps {
  tabs: readonly TabItem[];
  value: string;
  onChange: (id: string) => void;
  /** Names the list for screen readers, and makes the DOM ids unique on a page. */
  label: string;
  idPrefix: string;
  className?: string;
}

export function Tabs({ tabs, value, onChange, label, idPrefix, className }: TabsProps) {
  // Arrow keys move between tabs, as in the ARIA tabs pattern.
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = tabs.findIndex((tab) => tab.id === value);
    const next = e.key === "ArrowRight" ? i + 1 : e.key === "ArrowLeft" ? i - 1 : null;
    if (next === null || tabs.length === 0) return;
    e.preventDefault();
    const target = tabs[(next + tabs.length) % tabs.length];
    if (!target) return;
    onChange(target.id);
    document.getElementById(tabDomId(idPrefix, target.id))?.focus();
  };

  return (
    <div role="tablist" aria-label={label} onKeyDown={onKeyDown}
      className={clsx("flex overflow-x-auto border-b border-line [scrollbar-width:none]", className)}>
      {tabs.map((tab) => {
        const selected = tab.id === value;
        return (
          <button
            key={tab.id}
            id={tabDomId(idPrefix, tab.id)}
            type="button"
            role="tab"
            aria-selected={selected}
            aria-controls={panelDomId(idPrefix, tab.id)}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(tab.id)}
            className={clsx(
              "-mb-px h-11 shrink-0 whitespace-nowrap border-b-2 px-4 text-base",
              selected ? "border-ink font-semibold text-ink" : "border-transparent text-muted hover:text-ink",
            )}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}

export function TabPanel({ idPrefix, id, active, children }: { idPrefix: string; id: string; active: boolean; children: ReactNode }) {
  return (
    <div role="tabpanel" id={panelDomId(idPrefix, id)} aria-labelledby={tabDomId(idPrefix, id)} hidden={!active}>
      {active ? children : null}
    </div>
  );
}
