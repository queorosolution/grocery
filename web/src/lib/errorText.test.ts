import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { errorText, fieldErrors } from "./errorText";

describe("errorText", () => {
  it("uses the API's message and a plain sentence for anything else", () => {
    expect(errorText(new ApiError(429, "RATE_LIMITED", "Too many tries."))).toBe("Too many tries.");
    expect(errorText(new Error("boom: stack details"))).toBe("Something went wrong. Please try again.");
  });
});

describe("fieldErrors", () => {
  const err = new ApiError(400, "VALIDATION_ERROR", "Check the highlighted fields.", { phone: "invalid", line1: "too_short" });

  it("maps field.code first, then the field, then the API message", () => {
    expect(fieldErrors(err, { "phone.invalid": "Bad number", line1: "Too short" })).toEqual({ phone: "Bad number", line1: "Too short" });
    expect(fieldErrors(err, {})).toEqual({ phone: "Check the highlighted fields.", line1: "Check the highlighted fields." });
  });

  it("is empty for errors without fields", () => {
    expect(fieldErrors(new Error("x"), {})).toEqual({});
    expect(fieldErrors(undefined, {})).toEqual({});
  });
});
