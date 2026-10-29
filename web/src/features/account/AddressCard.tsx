import { Badge, Button, Card } from "@/components/ui";
import type { Address } from "@/features/address/types";
import { t } from "@/lib/i18n";

interface AddressCardProps {
  address: Address;
  busy: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onMakeDefault: () => void;
}

export function AddressCard({ address, busy, onEdit, onDelete, onMakeDefault }: AddressCardProps) {
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="font-semibold">{address.label}</p>
          <p className="text-muted">{address.line1}</p>
          {address.landmark && <p className="text-sm text-muted">{address.landmark}</p>}
        </div>
        {address.isDefault && <Badge tone="info">{t("address.default")}</Badge>}
      </div>
      <div className="mt-2 flex flex-wrap gap-1">
        <Button variant="ghost" onClick={onEdit}>{t("account.edit")}</Button>
        {!address.isDefault && <Button variant="ghost" loading={busy} onClick={onMakeDefault}>{t("address.makeDefaultShort")}</Button>}
        <Button variant="ghost" onClick={onDelete}>{t("address.remove")}</Button>
      </div>
    </Card>
  );
}
