import { describe, expect, it, vi } from "vitest";
import { submitPayHere } from "./payhere";

function fakeDocument() {
  const form = { method: "", action: "", style: { display: "" }, children: [] as { name: string; value: string; type: string }[], appendChild(c: never) { this.children.push(c); }, submit: vi.fn() };
  const appended: unknown[] = [];
  const doc = {
    createElement: (tag: string) => (tag === "form" ? form : { name: "", value: "", type: "" }),
    body: { appendChild: (n: unknown) => appended.push(n) },
  } as unknown as Document;
  return { doc, form, appended };
}

describe("submitPayHere", () => {
  it("posts a hidden form with every field to the PayHere checkout", () => {
    const { doc, form, appended } = fakeDocument();
    submitPayHere("https://sandbox.payhere.lk/pay/checkout", { merchant_id: "1", hash: "abc" }, doc);
    expect(form.method).toBe("post");
    expect(form.action).toBe("https://sandbox.payhere.lk/pay/checkout");
    expect(form.children.map((c) => [c.type, c.name, c.value])).toEqual([["hidden", "merchant_id", "1"], ["hidden", "hash", "abc"]]);
    expect(appended).toEqual([form]);
    expect(form.submit).toHaveBeenCalledOnce();
  });

  it("refuses any other address before a form exists (XSS-05)", () => {
    for (const bad of ["https://evil.example/pay/checkout", "javascript:alert(1)", "https://www.payhere.lk/pay/checkout/../x", "http://www.payhere.lk/pay/checkout", ""]) {
      const { doc, form, appended } = fakeDocument();
      expect(() => submitPayHere(bad, { a: "b" }, doc)).toThrow();
      expect(appended).toHaveLength(0);
      expect(form.submit).not.toHaveBeenCalled();
    }
  });
});
