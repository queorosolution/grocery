"use client";

import { LocateFixed } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { COLOMBO, type LatLng } from "@/lib/geo";
import { env } from "@/lib/env";
import { t } from "@/lib/i18n";
import { loadScript } from "@/lib/loadScript";
import { Button } from "./Button";
import { Field } from "./Field";
import { Input } from "./Input";

// Only what we use of Google Maps; the full type package is not worth a dependency.
interface Pos {
  lat(): number;
  lng(): number;
}
interface MapsApi {
  Map: new (el: HTMLElement, o: object) => { addListener(e: string, f: (ev: { latLng: Pos }) => void): void; panTo(p: LatLng): void };
  Marker: new (o: object) => { setPosition(p: LatLng): void; setVisible(v: boolean): void; addListener(e: string, f: () => void): void; getPosition(): Pos };
}

const MAPS_SRC = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(env.mapsKey)}`;
const mapsApi = () => (window as unknown as { google?: { maps?: MapsApi } }).google?.maps;

interface MapPickerProps {
  value: LatLng | null;
  onChange: (p: LatLng) => void;
  error?: string;
}

/**
 * Pin picker. The map loads only when this mounts. Without a Maps key (local work) or when the
 * script cannot load, the two number boxes still let the person set the pin.
 */
export function MapPicker({ value, onChange, error }: MapPickerProps) {
  const box = useRef<HTMLDivElement>(null);
  const move = useRef<((p: LatLng) => void) | null>(null);
  const change = useRef(onChange);
  const [ready, setReady] = useState(false);
  const [locating, setLocating] = useState(false);
  const [note, setNote] = useState("");

  useEffect(() => {
    change.current = onChange;
  });

  useEffect(() => {
    if (!env.mapsKey) return;
    let gone = false;
    loadScript(MAPS_SRC).then(() => {
      const maps = mapsApi();
      if (gone || !maps || !box.current) return;
      const start = value ?? COLOMBO;
      const map = new maps.Map(box.current, { center: start, zoom: value ? 16 : 11, disableDefaultUI: true, zoomControl: true, gestureHandling: "greedy" });
      const pin = new maps.Marker({ map, position: start, draggable: true, visible: !!value });
      const show = (p: LatLng) => {
        pin.setPosition(p);
        pin.setVisible(true);
        map.panTo(p);
      };
      const set = (p: LatLng) => {
        show(p);
        change.current(p);
      };
      map.addListener("click", (e) => set({ lat: e.latLng.lat(), lng: e.latLng.lng() }));
      pin.addListener("dragend", () => set({ lat: pin.getPosition().lat(), lng: pin.getPosition().lng() }));
      move.current = show;
      setReady(true);
    }, () => setNote(t("map.failed")));
    return () => {
      gone = true;
    };
    // The map is built once; later value changes go through move.current.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (value) move.current?.(value);
  }, [value]);

  const locate = () => {
    if (!navigator.geolocation) return setNote(t("map.noLocation"));
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (p) => {
        setLocating(false);
        setNote("");
        onChange({ lat: p.coords.latitude, lng: p.coords.longitude });
      },
      () => {
        setLocating(false);
        setNote(t("map.noLocation"));
      },
      { enableHighAccuracy: true, timeout: 10_000 },
    );
  };

  return (
    <div className="flex flex-col gap-2">
      {env.mapsKey && !note && (
        <div ref={box} className="h-56 w-full rounded-input border border-line bg-surface" role="application" aria-label={t("map.label")}>
          {!ready && <p className="p-3 text-sm text-muted">{t("map.loading")}</p>}
        </div>
      )}
      <Button variant="secondary" loading={locating} onClick={locate}>
        <LocateFixed size={18} aria-hidden="true" />
        {t("map.useMine")}
      </Button>
      {(!env.mapsKey || note) && <PinBoxes value={value} onChange={onChange} error={error} />}
      {note && <p className="text-sm text-muted">{note}</p>}
      {env.mapsKey && !note && error && <p className="text-sm text-danger">{error}</p>}
    </div>
  );
}

/** Two number boxes. They keep what was typed and report only when both are numbers. */
function PinBoxes({ value, onChange, error }: MapPickerProps) {
  const [lat, setLat] = useState(value ? String(value.lat) : "");
  const [lng, setLng] = useState(value ? String(value.lng) : "");
  // A pin chosen another way (my location) fills the boxes. Typing "6." must not turn into "6",
  // so the boxes are only overwritten when the pin really differs from what they say.
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    if (value && (Number(lat) !== value.lat || Number(lng) !== value.lng)) {
      setLat(String(value.lat));
      setLng(String(value.lng));
    }
  }
  const report = (a: string, b: string) => {
    const p = { lat: Number(a), lng: Number(b) };
    if (a.trim() && b.trim() && Number.isFinite(p.lat) && Number.isFinite(p.lng)) onChange(p);
  };
  return (
    <div className="grid grid-cols-2 gap-2">
      <Field label={t("map.lat")} error={error}>
        {(p) => <Input {...p} inputMode="decimal" value={lat} onChange={(e) => { setLat(e.target.value); report(e.target.value, lng); }} />}
      </Field>
      <Field label={t("map.lng")}>
        {(p) => <Input {...p} inputMode="decimal" value={lng} onChange={(e) => { setLng(e.target.value); report(lat, e.target.value); }} />}
      </Field>
    </div>
  );
}
