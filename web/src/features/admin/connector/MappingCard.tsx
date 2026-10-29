"use client";

import { useState } from "react";
import { Banner, Button, Card, Checkbox, Field, Select } from "@/components/ui";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { NumberField } from "../fields";
import { fieldText, parseInt10, SaveNotice } from "../form";
import { COLUMN_KEYS, REQUIRED_COLUMNS, type ColumnKey, type Connector } from "../types";
import { useColumns, useSaveMapping, useTables } from "./connectorApi";

const label = (k: ColumnKey) => t(`admin.connector.col.${k}` as MessageKey);

/** Steps 2 and 3: pick the table, then say which column holds which field. The server checks every name against the source. */
export function MappingCard({ shopId, connector }: { shopId: string; connector: Connector }) {
  const tables = useTables(connector.id, true);
  const save = useSaveMapping(shopId, connector.id);
  const [table, setTable] = useState(connector.sourceTable ?? "");
  const [unit, setUnit] = useState(connector.moneyUnit);
  const [change, setChange] = useState(connector.changeColumn ?? "");
  const [every, setEvery] = useState(String(connector.intervalMin));
  const [auto, setAuto] = useState(connector.autoCreateProducts);
  const [cols, setCols] = useState(connector.columns);
  const columns = useColumns(connector.id, table);
  const names = columns.data?.data.map((c) => c.name) ?? [];
  const minutes = parseInt10(every);
  const valid = table !== "" && REQUIRED_COLUMNS.every((k) => !!cols[k]) && minutes !== null && minutes >= 10 && minutes <= 1440;

  const pick = (k: ColumnKey, v: string) => setCols((c) => {
    const next = { ...c };
    if (v) next[k] = v; else delete next[k];
    return next;
  });
  const submit = () => {
    if (!valid) return;
    save.mutate({ sourceTable: table, moneyUnit: unit, changeColumn: change || cols.updatedAt || "updatedAt", intervalMin: minutes, autoCreateProducts: auto, columns: cols });
  };

  return (
    <Card className="flex flex-col gap-3 p-4">
      <h2 className="text-lg font-semibold">{t("admin.connector.stepMap")}</h2>
      <Field label={t("admin.connector.table")} error={fieldText(save.error, "sourceTable")}>
        {(p) => (
          <Select {...p} value={table} disabled={tables.isPending} onChange={(e) => { setTable(e.target.value); setCols({}); }}>
            <option value="">{t("admin.connector.pickTable")}</option>
            {tables.data?.data.map((x) => <option key={`${x.schema}.${x.name}`} value={`${x.schema}.${x.name}`}>{x.schema}.{x.name}</option>)}
          </Select>
        )}
      </Field>
      {tables.isError && <Banner tone="danger">{errorText(tables.error)}</Banner>}
      {table !== "" && (
        <form className="grid gap-3 md:grid-cols-2" onSubmit={(e) => { e.preventDefault(); submit(); }}>
          {COLUMN_KEYS.map((k) => (
            <Field key={k} label={REQUIRED_COLUMNS.includes(k) ? `${label(k)} *` : label(k)} error={fieldText(save.error, `columns.${k}`)}>
              {(p) => (
                <Select {...p} value={cols[k] ?? ""} disabled={columns.isPending} onChange={(e) => pick(k, e.target.value)}>
                  <option value="">{t("admin.connector.noColumn")}</option>
                  {names.map((n) => <option key={n} value={n}>{n}</option>)}
                </Select>
              )}
            </Field>
          ))}
          <Field label={t("admin.connector.moneyUnit")} error={fieldText(save.error, "moneyUnit")}>
            {(p) => <Select {...p} value={unit} onChange={(e) => setUnit(e.target.value as typeof unit)}><option value="RUPEES">{t("admin.connector.rupees")}</option><option value="CENTS">{t("admin.connector.cents")}</option></Select>}
          </Field>
          <Field label={t("admin.connector.changeColumn")} hint={t("admin.connector.changeHint")} error={fieldText(save.error, "changeColumn")}>
            {(p) => <Select {...p} value={change} onChange={(e) => setChange(e.target.value)}><option value="">{t("admin.connector.sameAsUpdated")}</option>{names.map((n) => <option key={n} value={n}>{n}</option>)}</Select>}
          </Field>
          <NumberField label={t("admin.connector.interval")} hint={t("admin.connector.intervalHint")} value={every} onChange={setEvery} error={fieldText(save.error, "intervalMin")} />
          <Checkbox label={t("admin.connector.autoCreate")} checked={auto} onChange={setAuto} className="md:col-span-2" />
          <div className="flex flex-col gap-2 md:col-span-2">
            <SaveNotice error={save.error} saved={save.isSuccess} />
            <Button type="submit" loading={save.isPending} disabled={!valid} className="self-start">{t("admin.connector.saveMapping")}</Button>
          </div>
        </form>
      )}
    </Card>
  );
}
