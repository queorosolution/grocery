import type { Permission, Role } from "@/lib/enums.gen";
import { t } from "@/lib/i18n";
import { rolePermissions } from "@/lib/perms.gen";
import type { Me } from "./types";

/** Every role that works in the console. A customer has no membership row at all. */
export const consoleRoles: readonly Role[] = ["SUPER_ADMIN", "SHOP_OWNER", "SHOP_STAFF", "SHOP_DRIVER", "PLATFORM_DRIVER"];

export function hasRole(me: Me | null, ...roles: Role[]): boolean {
  return !!me && me.roles.some((r) => roles.includes(r.role));
}

export interface ConsoleLink {
  href: string;
  title: string;
  hint: string;
}

/** What `/console` offers a person: one link per place they work. */
export function consoleLinks(me: Me): ConsoleLink[] {
  const links: ConsoleLink[] = [];
  if (hasRole(me, "SUPER_ADMIN")) links.push({ href: "/console/admin", title: t("console.admin"), hint: t("console.adminHint") });
  for (const r of me.roles) {
    if ((r.role === "SHOP_OWNER" || r.role === "SHOP_STAFF") && r.shopId) {
      links.push({ href: `/console/shop/${r.shopId}/orders`, title: r.shopName ?? t("console.shop"), hint: t("console.shopHint") });
    }
  }
  if (hasRole(me, "SHOP_DRIVER", "PLATFORM_DRIVER")) links.push({ href: "/console/driver", title: t("console.driver"), hint: t("console.driverHint") });
  return links;
}

/**
 * What this person may do in one shop. A hint for what to show: the API decides on every call.
 * A super admin may do everything of the SUPER_ADMIN list; staff get product edits only when the
 * owner switched it on (`canEditProducts`).
 */
export function shopPermissions(me: Me | null, shopId: string | null): ReadonlySet<Permission> {
  const out = new Set<Permission>();
  if (!me || !shopId) return out;
  for (const r of me.roles) {
    if (r.role === "SUPER_ADMIN") rolePermissions.SUPER_ADMIN.forEach((p) => out.add(p));
    else if (r.shopId === shopId) {
      rolePermissions[r.role].forEach((p) => out.add(p));
      if (r.canEditProducts) out.add("product.edit");
    }
  }
  return out;
}
