"use client";

import { Dialog, type DialogProps } from "./Dialog";

/** Bottom sheet on phones, side panel on desktop (cart, filters). */
export function Sheet(props: DialogProps) {
  return <Dialog shape="sheet" {...props} />;
}
