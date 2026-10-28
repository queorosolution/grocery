import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, setAuthLostHandler } from "./api";

const json = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  setAuthLostHandler(null);
});

describe("api", () => {
  it("sends cookies, the CSRF header and JSON", async () => {
    fetchMock.mockResolvedValue(json(200, { ok: true }));
    await api("/bills", { method: "POST", body: { a: 1 }, idempotencyKey: "k1", query: { q: "x y", skip: undefined } });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://localhost:8080/v1/bills?q=x+y");
    expect(init.credentials).toBe("include");
    expect(init.method).toBe("POST");
    expect(init.body).toBe('{"a":1}');
    expect(init.headers).toMatchObject({ "X-Requested-With": "gp-web", "Content-Type": "application/json", "Idempotency-Key": "k1" });
  });

  it("returns undefined for 204", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(api("/auth/logout", { method: "POST" })).resolves.toBeUndefined();
  });

  it("maps the error body to ApiError with the client message", async () => {
    fetchMock.mockResolvedValue(json(409, { error: { code: "SLOT_FULL", message: "server text", fields: { slot: "full" }, requestId: "req_1" } }));
    const err = await api("/bills").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "SLOT_FULL", fields: { slot: "full" }, requestId: "req_1" });
    expect((err as ApiError).message).not.toBe("server text");
  });

  it("gives a safe UNKNOWN error for a body that is not ours", async () => {
    fetchMock.mockResolvedValue(new Response("<html>bad gateway</html>", { status: 502 }));
    await expect(api("/home")).rejects.toMatchObject({ status: 502, code: "UNKNOWN" });
  });

  it("gives NETWORK when the request cannot be sent", async () => {
    fetchMock.mockRejectedValue(new TypeError("failed to fetch"));
    await expect(api("/home")).rejects.toMatchObject({ status: 0, code: "NETWORK" });
  });

  it("refreshes once on 401 and retries", async () => {
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: "UNAUTHENTICATED" } }))
      .mockResolvedValueOnce(json(200, {}))
      .mockResolvedValueOnce(json(200, { me: 1 }));
    await expect(api("/me")).resolves.toEqual({ me: 1 });
    expect(fetchMock.mock.calls.map((c) => (c[0] as string).replace("http://localhost:8080/v1", ""))).toEqual(["/me", "/auth/refresh", "/me"]);
  });

  it("after a second 401 it tells the app the session is gone", async () => {
    const lost = vi.fn();
    setAuthLostHandler(lost);
    fetchMock
      .mockResolvedValueOnce(json(401, { error: { code: "UNAUTHENTICATED" } }))
      .mockResolvedValueOnce(json(200, {}))
      .mockResolvedValueOnce(json(401, { error: { code: "UNAUTHENTICATED" } }));
    await expect(api("/me")).rejects.toMatchObject({ status: 401, code: "UNAUTHENTICATED" });
    expect(lost).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(3); // call, refresh, one retry. No loop.
  });

  it("tells the app too when the refresh itself fails", async () => {
    const lost = vi.fn();
    setAuthLostHandler(lost);
    fetchMock.mockResolvedValue(json(401, { error: { code: "UNAUTHENTICATED" } }));
    await expect(api("/me")).rejects.toMatchObject({ status: 401 });
    expect(lost).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not refresh when the refresh call itself says 401", async () => {
    fetchMock.mockResolvedValue(json(401, { error: { code: "UNAUTHENTICATED" } }));
    await expect(api("/auth/refresh", { method: "POST" })).rejects.toBeInstanceOf(ApiError);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("shares one refresh between calls that fail together", async () => {
    let refreshes = 0;
    fetchMock.mockImplementation((url: string) => {
      if (url.endsWith("/auth/refresh")) {
        refreshes++;
        return Promise.resolve(json(200, {}));
      }
      return Promise.resolve(refreshes === 0 ? json(401, {}) : json(200, { ok: true }));
    });
    await Promise.all([api("/a"), api("/b"), api("/c")]);
    expect(refreshes).toBe(1);
  });
});
