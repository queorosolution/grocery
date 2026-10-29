"use client";

import { useState } from "react";
import { Button, Field, Input, Modal, Select } from "@/components/ui";
import { t } from "@/lib/i18n";
import { NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import type { Category } from "../types";
import { useSaveCategory } from "./api";

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

interface Props {
  /** The category being changed, or null to add one. */
  category: Category | null;
  all: Category[];
  open: boolean;
  onClose: () => void;
}

/** Two levels only: a parent must be a top-level category, and a category with children cannot get a parent. */
export function CategoryDialog({ category, all, open, onClose }: Props) {
  const save = useSaveCategory(category?.id ?? null);
  const [name, setName] = useState(category?.name ?? "");
  const [slug, setSlug] = useState(category?.slug ?? "");
  const [parent, setParent] = useState(category?.parentId ?? "");
  const [sort, setSort] = useState(String(category?.sort ?? 0));
  const [weight, setWeight] = useState(String(category?.defaultWeightG ?? 500));
  const s = parseInt10(sort);
  const w = parseInt10(weight);
  const valid = name.trim().length >= 2 && SLUG.test(slug) && s !== null && w !== null && w >= 1 && w <= 100000;
  const hasChildren = !!category && all.some((c) => c.parentId === category.id);
  const close = () => { save.reset(); onClose(); };

  return (
    <Modal open={open} onClose={close} title={category ? t("admin.catalog.editCategory") : t("admin.catalog.newCategory")}
      footer={<Button className="flex-1" loading={save.isPending} disabled={!valid}
        onClick={() => s !== null && w !== null && save.mutate({ name: name.trim(), slug, parentId: parent || null, sort: s, defaultWeightG: w }, { onSuccess: close })}>{t("common.save")}</Button>}>
      <div className="flex flex-col gap-3">
        <Field label={t("admin.shops.name")} error={fieldText(save.error, "name")}>{(p) => <Input {...p} value={name} maxLength={80} onChange={(e) => setName(e.target.value)} />}</Field>
        <Field label={t("admin.catalog.slug")} error={fieldText(save.error, "slug")}>{(p) => <Input {...p} value={slug} maxLength={60} autoCapitalize="none" onChange={(e) => setSlug(e.target.value.toLowerCase())} />}</Field>
        <Field label={t("admin.catalog.parent")} error={fieldText(save.error, "parentId")}>
          {(p) => (
            <Select {...p} value={parent} disabled={hasChildren} onChange={(e) => setParent(e.target.value)}>
              <option value="">{t("admin.catalog.topLevel")}</option>
              {all.filter((c) => c.parentId === null && c.id !== category?.id).map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </Select>
          )}
        </Field>
        <NumberField label={t("admin.catalog.sort")} value={sort} onChange={setSort} />
        <NumberField label={t("admin.catalog.defaultWeight")} hint={t("admin.catalog.grams")} value={weight} onChange={setWeight} error={fieldText(save.error, "defaultWeightG")} />
        <SaveNotice error={save.error} saved={false} />
      </div>
    </Modal>
  );
}
