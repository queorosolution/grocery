import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { errorCodes } from "@/lib/errors.gen";
import { colomboDay } from "@/lib/time";
import { bootstrap, calls, me, mockApi, renderScreen } from "@/test/support";
import { DriverScreen } from "./DriverScreen";
import type { Delivery, DeliveryGroup } from "./types";

vi.mock("@/lib/api", async (orig) => ({ ...(await orig<typeof import("@/lib/api")>()), api: vi.fn() }));

const delivery = (over: Partial<Delivery> = {}): Delivery => ({
  id: "o1", code: "KS-7Q2M9X", status: "OUT_FOR_DELIVERY", version: 7,
  customer: { name: "Nimal Perera", phone: "+94 77 123 4567" },
  address: { label: "Home", line1: "12 Temple Rd", landmark: "Near the bakery", lat: 6.9, lng: 79.8 },
  totalCents: 125000, itemCount: 4, ...over,
});
const slot = { startsAt: "2026-10-29T03:00:00Z", endsAt: "2026-10-29T04:30:00Z" };
const list = (...groups: DeliveryGroup[]) => ({ data: groups });
const err = (code: keyof typeof errorCodes) => new ApiError(errorCodes[code].http, code, errorCodes[code].message);

function setup(extra: Record<string, () => unknown> = {}, groups: DeliveryGroup[] = [{ slot, orders: [delivery()] }]) {
  const fake = mockApi({
    "GET /bootstrap": () => bootstrap(me({ roles: [{ role: "SHOP_DRIVER", shopId: "s1", shopSlug: "krishna-super", shopName: "Krishna Super" }] })),
    "GET /driver/deliveries": () => list(...groups),
    "GET /driver/stats": () => ({ delivered: 3, failed: 1 }),
    ...extra,
  });
  renderScreen(<DriverScreen />);
  return fake;
}

const openCode = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "Enter code" }));
  return screen.findByRole("dialog", { name: "Delivery code" });
};
const typeCode = async (dialog: HTMLElement, code: string) => {
  const input = within(dialog).getByLabelText("4-digit code");
  await userEvent.clear(input);
  await userEvent.type(input, code);
  return input;
};

describe("driver list", () => {
  it("asks for today in Colombo and groups drop-offs by slot", async () => {
    const fake = setup({}, [{ slot, orders: [delivery()] }, { slot: null, orders: [delivery({ id: "o2", code: "KS-AAAAAA", customer: { name: "Kamal", phone: "0712345678" } })] }]);
    expect(await screen.findByText("Nimal Perera")).toBeInTheDocument();
    expect(calls(fake, "GET /driver/deliveries")[0]![1].query).toEqual({ day: colomboDay(0) });
    expect(screen.getByRole("heading", { name: /8:30.*10:00/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "On the way, no slot" })).toBeInTheDocument();
    expect(screen.getByText("KS-7Q2M9X · 4 items · Rs 1,250.00")).toBeInTheDocument();
  });

  it("gives a call link with only digits and a map link from the pin", async () => {
    setup();
    expect(await screen.findByRole("link", { name: "Call Nimal Perera" })).toHaveAttribute("href", "tel:+94771234567");
    expect(screen.getByRole("link", { name: "Open map" })).toHaveAttribute("href", "https://www.google.com/maps/search/?api=1&query=6.9,79.8");
  });

  it("leaves the map button out when there is no pin and the code buttons out when not on the way", async () => {
    setup({}, [{ slot: null, orders: [delivery({ status: "DELIVERED", address: { label: "Home", line1: "12 Temple Rd", landmark: null } })] }]);
    expect(await screen.findByText("Delivered", { selector: "span" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open map" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Enter code" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Couldn't deliver" })).not.toBeInTheDocument();
  });

  it("shows an empty message and the day and week numbers", async () => {
    const fake = setup({}, []);
    expect(await screen.findByText("No deliveries for today")).toBeInTheDocument();
    await waitFor(() => expect(screen.getAllByText("3")).toHaveLength(2));
    const asked = calls(fake, "GET /driver/stats").map(([, o]) => o.query);
    expect(asked).toContainEqual({ from: colomboDay(0), to: colomboDay(0) });
    expect(asked).toContainEqual({ from: colomboDay(-6), to: colomboDay(0) });
  });
});

describe("code entry", () => {
  it("keeps the button off until 4 digits and ignores other characters", async () => {
    setup();
    const dialog = await openCode();
    const input = await typeCode(dialog, "48a2");
    expect(input).toHaveValue("482");
    expect(within(dialog).getByRole("button", { name: "Confirm delivery" })).toBeDisabled();
    await userEvent.type(input, "1");
    expect(within(dialog).getByRole("button", { name: "Confirm delivery" })).toBeEnabled();
  });

  it("sends the version and code, then closes and asks for the list again", async () => {
    let done = false;
    const fake = setup({ "POST /driver/deliveries/o1/complete": () => { done = true; return {}; } });
    const dialog = await openCode();
    await typeCode(dialog, "4821");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
    await waitFor(() => expect(done).toBe(true));
    expect(calls(fake, "POST /driver/deliveries/o1/complete")[0]![1].body).toEqual({ version: 7, code: "4821" });
    await waitFor(() => expect(calls(fake, "GET /driver/deliveries").length).toBeGreaterThan(1));
    expect(await screen.findByText("Delivery saved.")).toBeInTheDocument();
  });

  it("counts the tries left after each wrong code and clears the box", async () => {
    setup({ "POST /driver/deliveries/o1/complete": () => err("DELIVERY_CODE_INVALID") });
    const dialog = await openCode();
    for (const left of ["4 tries left", "3 tries left", "2 tries left"]) {
      const input = await typeCode(dialog, "1111");
      await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
      expect(await within(dialog).findByText(new RegExp(`Wrong code. ${left}`))).toBeInTheDocument();
      await waitFor(() => expect(input).toHaveValue(""));
    }
    await typeCode(dialog, "1111");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
    expect(await within(dialog).findByText("Wrong code. 1 try left.")).toBeInTheDocument();
  });

  it("locks after the fifth wrong code and remembers it when the sheet is opened again", async () => {
    setup({ "POST /driver/deliveries/o1/complete": () => err("DELIVERY_CODE_INVALID") });
    const dialog = await openCode();
    for (let i = 0; i < 5; i++) {
      const input = await typeCode(dialog, "1111");
      await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
      await waitFor(() => expect(input).toHaveValue(""));
    }
    expect(await within(dialog).findByText(/No tries left/)).toBeInTheDocument();
    expect(within(dialog).getByLabelText("4-digit code")).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Confirm delivery" })).toBeDisabled();
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Enter code" }));
    expect(await screen.findByText(/No tries left/)).toBeInTheDocument();
  });

  it("shows the lock when the server says so, even on the first try", async () => {
    setup({ "POST /driver/deliveries/o1/complete": () => err("DELIVERY_CODE_LOCKED") });
    const dialog = await openCode();
    await typeCode(dialog, "4821");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
    expect(await within(dialog).findByText(/No tries left/)).toBeInTheDocument();
    expect(within(dialog).getByLabelText("4-digit code")).toBeDisabled();
  });

  it("shows the server's words for other errors and refreshes the list when the order changed", async () => {
    const fake = setup({ "POST /driver/deliveries/o1/complete": () => err("STALE_ORDER") });
    const dialog = await openCode();
    await typeCode(dialog, "4821");
    await userEvent.click(within(dialog).getByRole("button", { name: "Confirm delivery" }));
    expect(await within(dialog).findByText("This order just changed. Refreshing.")).toBeInTheDocument();
    await waitFor(() => expect(calls(fake, "GET /driver/deliveries").length).toBeGreaterThan(1));
    expect(within(dialog).queryByText(/Wrong code/)).not.toBeInTheDocument();
  });
});

describe("couldn't deliver", () => {
  it("needs a reason of 3 characters, then sends it with the version", async () => {
    let sent = false;
    const fake = setup({ "POST /driver/deliveries/o1/fail": () => { sent = true; return {}; } });
    await userEvent.click(await screen.findByRole("button", { name: "Couldn't deliver" }));
    const dialog = await screen.findByRole("dialog", { name: "Couldn't deliver" });
    const send = within(dialog).getByRole("button", { name: "Mark as not delivered" });
    await userEvent.type(within(dialog).getByLabelText("What went wrong?"), "  ab ");
    expect(send).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText("What went wrong?"), "c");
    await userEvent.click(send);
    await waitFor(() => expect(sent).toBe(true));
    expect(calls(fake, "POST /driver/deliveries/o1/fail")[0]![1].body).toEqual({ version: 7, reason: "ab c" });
    expect(await screen.findByText("Marked as not delivered.")).toBeInTheDocument();
  });

  it("shows an error from the server and keeps the note", async () => {
    setup({ "POST /driver/deliveries/o1/fail": () => err("INVALID_TRANSITION") });
    await userEvent.click(await screen.findByRole("button", { name: "Couldn't deliver" }));
    const dialog = await screen.findByRole("dialog", { name: "Couldn't deliver" });
    await userEvent.type(within(dialog).getByLabelText("What went wrong?"), "Gate locked");
    await userEvent.click(within(dialog).getByRole("button", { name: "Mark as not delivered" }));
    expect(await within(dialog).findByText(errorCodes.INVALID_TRANSITION.message)).toBeInTheDocument();
    expect(within(dialog).getByLabelText("What went wrong?")).toHaveValue("Gate locked");
  });
});
