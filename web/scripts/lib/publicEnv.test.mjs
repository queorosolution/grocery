import { describe, expect, it } from "vitest";
import { ALLOWED, unknownNames } from "./publicEnv.mjs";

describe("unknownNames (XSS-09)", () => {
  it("passes every name from docs/13 section 3", () => {
    expect(unknownNames([...ALLOWED].map((n) => `process.env.${n}`).join("\n"))).toEqual([]);
  });

  it("catches a new public name, once", () => {
    const text = "const k = process.env.NEXT_PUBLIC_" + "SECRET_KEY; k2 = NEXT_PUBLIC_" + "SECRET_KEY";
    expect(unknownNames(text)).toEqual(["NEXT_PUBLIC_" + "SECRET_KEY"]);
  });
});
