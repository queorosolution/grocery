"use client";

import { LocateFixed, X } from "lucide-react";
import { Button, useToast } from "@/components/ui";
import { t } from "@/lib/i18n";
import { writePin } from "@/lib/pinStore";
import { usePin } from "./api";

/** "Use my location" for the shop list. Only a rough place (about 1 km) is kept, on this device. */
export function NearMe() {
  const pin = usePin();
  const toast = useToast();

  const locate = () => {
    if (!("geolocation" in navigator)) return toast.show(t("home.noLocation"), "danger");
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        if (!writePin({ lat: coords.latitude, lng: coords.longitude })) toast.show(t("home.outsideSriLanka"), "danger");
      },
      () => toast.show(t("home.noLocation"), "danger"),
      { maximumAge: 600_000, timeout: 10_000 },
    );
  };

  return pin ? (
    <Button variant="ghost" size="md" onClick={() => writePin(null)}>
      <X size={16} aria-hidden="true" /> {t("home.clearLocation")}
    </Button>
  ) : (
    <Button variant="secondary" onClick={locate}>
      <LocateFixed size={16} aria-hidden="true" /> {t("home.useLocation")}
    </Button>
  );
}
