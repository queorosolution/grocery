import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LatLng } from "@/lib/geo";
import { MapPicker } from "./MapPicker";

// No Maps key in tests, so this is the fallback with two number boxes.
function Harness({ onPin }: { onPin: (p: LatLng) => void }) {
  const [pin, setPin] = useState<LatLng | null>(null);
  return <MapPicker value={pin} onChange={(p) => { setPin(p); onPin(p); }} />;
}

afterEach(() => vi.unstubAllGlobals());

describe("MapPicker without a map key", () => {
  it("reports a pin only when both boxes hold numbers, and keeps what was typed", async () => {
    const onPin = vi.fn();
    render(<Harness onPin={onPin} />);
    const lat = screen.getByLabelText("Latitude");
    const lng = screen.getByLabelText("Longitude");
    await userEvent.type(lat, "6.");
    expect(onPin).not.toHaveBeenCalled();
    await userEvent.type(lng, "79.86");
    expect(onPin).toHaveBeenLastCalledWith({ lat: 6, lng: 79.86 });
    await userEvent.type(lat, "9"); // finishes 6.9; the box must still read "6.9"
    expect(lat).toHaveValue("6.9");
    expect(onPin).toHaveBeenLastCalledWith({ lat: 6.9, lng: 79.86 });
  });

  it("fills the boxes from the device location", async () => {
    vi.stubGlobal("navigator", {
      geolocation: { getCurrentPosition: (ok: (p: { coords: { latitude: number; longitude: number } }) => void) => ok({ coords: { latitude: 7.29, longitude: 80.63 } }) },
    });
    const onPin = vi.fn();
    render(<Harness onPin={onPin} />);
    await userEvent.click(screen.getByRole("button", { name: /Use my location/ }));
    expect(onPin).toHaveBeenCalledWith({ lat: 7.29, lng: 80.63 });
    expect(await screen.findByDisplayValue("7.29")).toBeInTheDocument();
  });

  it("says so when the location cannot be read", async () => {
    vi.stubGlobal("navigator", { geolocation: { getCurrentPosition: (_: unknown, fail: () => void) => fail() } });
    render(<Harness onPin={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: /Use my location/ }));
    expect(await screen.findByText(/couldn't get your location/)).toBeInTheDocument();
  });
});
