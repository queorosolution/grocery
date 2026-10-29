import { describe, expect, it } from "vitest";
import { colomboDay } from "./time";

describe("colomboDay", () => {
  it("uses Colombo time (UTC+5:30), not the device's", () => {
    expect(colomboDay(0, new Date("2026-11-01T19:00:00Z"))).toBe("2026-11-02"); // 00:30 next day in Colombo
    expect(colomboDay(0, new Date("2026-11-01T18:00:00Z"))).toBe("2026-11-01");
  });
  it("counts days back", () => {
    expect(colomboDay(-6, new Date("2026-11-07T06:00:00Z"))).toBe("2026-11-01");
  });
});
