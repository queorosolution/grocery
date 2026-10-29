import { afterEach, describe, expect, it } from "vitest";
import { loadScript } from "./loadScript";

const tags = (src: string) => document.querySelectorAll(`script[src="${src}"]`);

afterEach(() => document.head.replaceChildren());

describe("loadScript", () => {
  it("adds the tag once however many callers ask", async () => {
    const src = "https://example.test/once.js";
    const a = loadScript(src);
    const b = loadScript(src);
    expect(tags(src)).toHaveLength(1);
    tags(src)[0]!.dispatchEvent(new Event("load"));
    await Promise.all([a, b]);
  });

  it("lets a failed script be tried again", async () => {
    const src = "https://example.test/flaky.js";
    const first = loadScript(src);
    tags(src)[0]!.dispatchEvent(new Event("error"));
    await expect(first).rejects.toThrow();
    expect(tags(src)).toHaveLength(0);
    const second = loadScript(src);
    expect(tags(src)).toHaveLength(1);
    tags(src)[0]!.dispatchEvent(new Event("load"));
    await second;
  });
});
