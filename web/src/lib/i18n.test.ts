import { describe, expect, it } from "vitest";
import { t } from "./i18n";

describe("t", () => {
  it("returns the text for a key", () => {
    expect(t("common.cancel")).toBe("Cancel");
  });

  it("fills {placeholders}", () => {
    expect(t("price.save", { amount: "Rs 65" })).toBe("Save Rs 65");
  });

  it("leaves an unknown placeholder visible rather than printing 'undefined'", () => {
    expect(t("price.save")).toBe("Save {amount}");
  });
});
