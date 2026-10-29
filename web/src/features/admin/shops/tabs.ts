import type { ComponentType } from "react";
import type { MessageKey } from "@/lib/i18n";
import { ConnectorTab } from "../connector/ConnectorTab";
import type { ShopDetail } from "../types";
import { FeesTab } from "./FeesTab";
import { HoursTab } from "./HoursTab";
import { PaymentTab } from "./PaymentTab";
import { ProfileTab } from "./ProfileTab";
import { ShopProductsTab } from "./ShopProductsTab";
import { SettingsTab } from "./SettingsTab";
import { SlotsTab } from "./SlotsTab";

/** The tabs of the shop setup page, in order. `?tab=<id>` in the address picks one. */
export const SHOP_TABS: readonly { id: string; label: MessageKey; Tab: ComponentType<{ detail: ShopDetail }> }[] = [
  { id: "profile", label: "admin.shop.tab.profile", Tab: ProfileTab },
  { id: "settings", label: "admin.shop.tab.settings", Tab: SettingsTab },
  { id: "hours", label: "admin.shop.tab.hours", Tab: HoursTab },
  { id: "slots", label: "admin.shop.tab.slots", Tab: SlotsTab },
  { id: "fees", label: "admin.shop.tab.fees", Tab: FeesTab },
  { id: "payment", label: "admin.shop.tab.payment", Tab: PaymentTab },
  { id: "products", label: "admin.shop.tab.products", Tab: ShopProductsTab },
  { id: "connector", label: "admin.shop.tab.connector", Tab: ConnectorTab },
];
