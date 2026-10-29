"use client";

import { useState } from "react";
import { Button, Field, Input, Modal, Select } from "@/components/ui";
import { productStatusValues, unitValues } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import type { Category, Product } from "../types";
import { useSaveProduct } from "./api";

const BARCODE = /^\d{6,14}$/;

interface Props {
  product: Product | null;
  categories: Category[];
  open: boolean;
  onClose: () => void;
}

const orNull = (s: string) => (s.trim() === "" ? null : s.trim());

/** A master product. A blank box clears an optional field. The image must be an https link on our image host (the API checks). */
export function ProductDialog({ product, categories, open, onClose }: Props) {
  const save = useSaveProduct(product?.id ?? null);
  const [name, setName] = useState(product?.name ?? "");
  const [barcode, setBarcode] = useState(product?.barcode ?? "");
  const [brand, setBrand] = useState(product?.brand ?? "");
  const [size, setSize] = useState(product?.sizeLabel ?? "");
  const [unit, setUnit] = useState<Product["unit"]>(product?.unit ?? "PCS");
  const [weight, setWeight] = useState(product?.weightG === null || product?.weightG === undefined ? "" : String(product.weightG));
  const [category, setCategory] = useState(product?.categoryId ?? "");
  const [image, setImage] = useState(product?.imageUrl ?? "");
  const [keywords, setKeywords] = useState(product?.keywords ?? "");
  const [status, setStatus] = useState<Product["status"]>(product?.status ?? "ACTIVE");
  const w = weight.trim() === "" ? null : parseInt10(weight);
  const valid = name.trim().length >= 2 && (barcode === "" || BARCODE.test(barcode)) && (weight.trim() === "" || w !== null);
  const close = () => { save.reset(); onClose(); };

  return (
    <Modal open={open} onClose={close} title={product ? t("admin.catalog.editProduct") : t("admin.catalog.newProduct")}
      footer={<Button className="flex-1" loading={save.isPending} disabled={!valid}
        onClick={() => save.mutate({ name: name.trim(), barcode: orNull(barcode), brand: orNull(brand), sizeLabel: orNull(size), unit, weightG: w, categoryId: category || null, imageUrl: orNull(image), keywords: keywords.trim(), status }, { onSuccess: close })}>{t("common.save")}</Button>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label={t("admin.shops.name")} className="col-span-2" error={fieldText(save.error, "name")}>{(p) => <Input {...p} value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />}</Field>
        <Field label={t("admin.connector.col.barcode")} hint={t("admin.catalog.barcodeHint")} error={fieldText(save.error, "barcode")}>{(p) => <Input {...p} inputMode="numeric" value={barcode} maxLength={14} onChange={(e) => setBarcode(e.target.value)} />}</Field>
        <Field label={t("admin.connector.col.brand")}>{(p) => <Input {...p} value={brand} onChange={(e) => setBrand(e.target.value)} />}</Field>
        <Field label={t("admin.catalog.size")}>{(p) => <Input {...p} value={size} placeholder="5kg" onChange={(e) => setSize(e.target.value)} />}</Field>
        <Field label={t("admin.connector.col.unit")}>{(p) => <Select {...p} value={unit} onChange={(e) => setUnit(e.target.value as Product["unit"])}>{unitValues.map((u) => <option key={u} value={u}>{u}</option>)}</Select>}</Field>
        <NumberField label={t("admin.connector.col.weightG")} value={weight} onChange={setWeight} />
        <Field label={t("admin.catalog.category")}>
          {(p) => <Select {...p} value={category} onChange={(e) => setCategory(e.target.value)}><option value="">{t("admin.catalog.noCategory")}</option>{categories.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</Select>}
        </Field>
        <Field label={t("admin.catalog.image")} className="col-span-2" error={fieldText(save.error, "imageUrl")}>{(p) => <Input {...p} type="url" value={image} onChange={(e) => setImage(e.target.value)} />}</Field>
        <Field label={t("admin.catalog.keywords")} className="col-span-2">{(p) => <Input {...p} value={keywords} onChange={(e) => setKeywords(e.target.value)} />}</Field>
        <Field label={t("admin.shops.status")}>{(p) => <Select {...p} value={status} onChange={(e) => setStatus(e.target.value as Product["status"])}>{productStatusValues.map((s) => <option key={s} value={s}>{t(`admin.catalog.status_${s}`)}</option>)}</Select>}</Field>
        <div className="col-span-2"><SaveNotice error={save.error} saved={false} /></div>
      </div>
    </Modal>
  );
}
