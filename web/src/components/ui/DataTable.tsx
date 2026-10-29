"use client";

import clsx from "clsx";
import type { KeyboardEvent, ReactNode } from "react";
import { t } from "@/lib/i18n";
import { EmptyState } from "./EmptyState";
import { Skeleton } from "./Skeleton";

export interface Column<T> {
  key: string;
  header: string;
  cell: (row: T) => ReactNode;
  align?: "left" | "right";
  /** Hide this column on phones; the table scrolls sideways for the rest. */
  hideOnMobile?: boolean;
}

interface DataTableProps<T> {
  /** Names the table for screen readers. */
  caption: string;
  columns: readonly Column<T>[];
  rows: readonly T[];
  rowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  loading?: boolean;
  emptyTitle?: string;
  emptyBody?: string;
}

const LOADING_ROWS = 5;

export function DataTable<T>({ caption, columns, rows, rowKey, onRowClick, loading, emptyTitle, emptyBody }: DataTableProps<T>) {
  const cellClass = (c: Column<T>) => clsx("px-3 py-3", c.align === "right" && "text-right", c.hideOnMobile && "hidden md:table-cell");
  const keyDown = (row: T) => (e: KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onRowClick?.(row);
    }
  };

  return (
    <div className="overflow-x-auto rounded-card border border-line">
      <table className="w-full border-collapse text-left text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="bg-surface text-muted">
          <tr>
            {columns.map((c) => (
              <th key={c.key} scope="col" className={clsx(cellClass(c), "font-semibold")}>{c.header}</th>
            ))}
          </tr>
        </thead>
        <tbody aria-busy={loading || undefined}>
          {loading &&
            Array.from({ length: LOADING_ROWS }, (_, i) => (
              <tr key={i} className="border-t border-line">
                {columns.map((c) => (
                  <td key={c.key} className={cellClass(c)}><Skeleton className="h-4 w-full max-w-32" /></td>
                ))}
              </tr>
            ))}
          {!loading && rows.length === 0 && (
            <tr className="border-t border-line">
              <td colSpan={columns.length}>
                <EmptyState title={emptyTitle ?? t("table.empty")} body={emptyBody} />
              </td>
            </tr>
          )}
          {!loading &&
            rows.map((row) => (
              <tr
                key={rowKey(row)}
                className={clsx("border-t border-line", onRowClick && "cursor-pointer hover:bg-surface")}
                tabIndex={onRowClick ? 0 : undefined}
                onClick={onRowClick && (() => onRowClick(row))}
                onKeyDown={onRowClick && keyDown(row)}
              >
                {columns.map((c) => (
                  <td key={c.key} className={cellClass(c)}>{c.cell(row)}</td>
                ))}
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  );
}
