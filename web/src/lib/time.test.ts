import { describe, expect, it } from "vitest";
import { formatDateTime, formatDay, formatSlot, formatTime } from "./time";

// 2026-11-02T03:00:00Z is 08:30 in Colombo (UTC+5:30), whatever zone the test machine uses.
const START = "2026-11-02T03:00:00Z";
const END = "2026-11-02T04:30:00Z";

describe("time", () => {
  it("shows Sri Lanka time", () => {
    expect(formatTime(START)).toMatch(/8:30/);
    expect(formatTime(END)).toMatch(/10:00/);
  });

  it("shows the Colombo date even when UTC is still the day before", () => {
    // 22:00Z on the 1st is 03:30 on the 2nd in Colombo.
    expect(formatDay("2026-11-01T22:00:00Z")).toMatch(/2/);
    expect(formatDay("2026-11-01T22:00:00Z")).toMatch(/Nov/);
  });

  it("formats a slot and a full date", () => {
    const slot = formatSlot(START, END);
    expect(slot).toMatch(/Nov/);
    expect(slot).toMatch(/8:30/);
    expect(slot).toMatch(/10:00/);
    expect(formatDateTime(START)).toMatch(/2026/);
  });

  it("throws on a bad date instead of showing 'Invalid Date'", () => {
    expect(() => formatTime("not a date")).toThrow(RangeError);
  });
});
