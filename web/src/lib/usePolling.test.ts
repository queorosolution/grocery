import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePolling } from "./usePolling";

const setVisibility = (state: "visible" | "hidden") => {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => state });
  document.dispatchEvent(new Event("visibilitychange"));
};

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  setVisibility("visible");
  vi.useRealTimers();
});

describe("usePolling", () => {
  it("polls every 20 s while the thing is moving", () => {
    const { result } = renderHook(() => usePolling());
    expect(result.current(true)).toBe(20_000);
  });
  it("stops once the thing is final", () => {
    const { result } = renderHook(() => usePolling());
    expect(result.current(false)).toBe(false);
  });
  it("stops while the tab is hidden and starts again when it is back", () => {
    const { result } = renderHook(() => usePolling());
    act(() => setVisibility("hidden"));
    expect(result.current(true)).toBe(false);
    act(() => setVisibility("visible"));
    expect(result.current(true)).toBe(20_000);
  });
  it("gives up 30 minutes after the page opened", () => {
    const { result } = renderHook(() => usePolling());
    expect(result.current(true)).toBe(20_000);
    vi.advanceTimersByTime(30 * 60_000 + 1);
    expect(result.current(true)).toBe(false);
  });
});
