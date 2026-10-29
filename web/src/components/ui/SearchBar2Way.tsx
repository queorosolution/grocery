"use client";

import { Search } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { t } from "@/lib/i18n";

export interface SearchShop {
  slug: string;
  name: string;
}

interface SearchBar2WayProps {
  shops: readonly SearchShop[];
  /** Text and shop to start from (the search page repeats the last search). */
  initialQuery?: string;
  initialShop?: string | null;
  onSearch: (query: string, shopSlug: string | null) => void;
  /** Every edit, for a page that searches while the person types (it debounces by itself). */
  onChange?: (query: string, shopSlug: string | null) => void;
}

const MIN_CHARS = 2;

/** The pill: [All shops ▾] | [What do you need?] [search]. One shop, or every shop near you. */
export function SearchBar2Way({ shops, initialQuery = "", initialShop = null, onSearch, onChange }: SearchBar2WayProps) {
  const [query, setQuery] = useState(initialQuery);
  const [shop, setShop] = useState(initialShop ?? "");
  const id = useId();
  const ready = query.trim().length >= MIN_CHARS;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (ready) onSearch(query.trim(), shop || null);
  };

  return (
    <form role="search" onSubmit={submit}
      className="flex h-12 items-center rounded-pill border border-line bg-bg pl-3 pr-1 shadow-card focus-within:border-ink">
      <label htmlFor={`${id}-shop`} className="sr-only">{t("search.shopLabel")}</label>
      <select id={`${id}-shop`} value={shop} onChange={(e) => { setShop(e.target.value); onChange?.(query, e.target.value || null); }}
        className="h-full max-w-[38%] shrink-0 truncate bg-transparent pr-1 text-sm font-semibold outline-none">
        <option value="">{t("search.allShops")}</option>
        {shops.map((s) => <option key={s.slug} value={s.slug}>{s.name}</option>)}
      </select>
      <span aria-hidden="true" className="mx-2 h-6 w-px shrink-0 bg-line" />
      <label htmlFor={`${id}-q`} className="sr-only">{t("search.queryLabel")}</label>
      <input id={`${id}-q`} type="search" enterKeyHint="search" autoComplete="off" maxLength={60}
        value={query} onChange={(e) => { setQuery(e.target.value); onChange?.(e.target.value, shop || null); }} placeholder={t("search.placeholder")}
        className="h-full min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted" />
      <button type="submit" disabled={!ready} aria-label={t("search.submit")}
        className="inline-flex size-10 shrink-0 items-center justify-center rounded-pill bg-accent text-on-accent disabled:bg-line disabled:text-muted">
        <Search size={18} aria-hidden="true" />
      </button>
    </form>
  );
}
