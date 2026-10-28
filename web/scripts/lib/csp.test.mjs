import { describe, expect, it } from "vitest";
import { buildCsp, hasInlineHandlers, hashOf, inlineScripts, pagePatterns, parseRedirects, sampleUrl } from "./csp.mjs";

describe("inlineScripts", () => {
  it("finds inline scripts and skips files and data blocks", () => {
    const html = `<script src="/a.js"></script><script>self.x=1</script>
      <script type="application/ld+json">{"a":1}</script><script async src="/b.js"></script>
      <script type="module">import "/c.js"</script><script></script>`;
    expect(inlineScripts(html)).toEqual(["self.x=1", 'import "/c.js"']);
  });
});

describe("hashOf", () => {
  it("is the CSP sha256 form", () => {
    // sha256("alert(1)") in base64
    expect(hashOf("alert(1)")).toBe("'sha256-bhHHL3z2vDgxUt0W3dWQOrprscmda2Y5pLsLg4GF+pI='");
  });
});

describe("hasInlineHandlers", () => {
  it("flags onclick= in markup but not in script text", () => {
    expect(hasInlineHandlers('<button onclick="x()">a</button>')).toBe(true);
    expect(hasInlineHandlers("<script>el.onclick = f</script><p>ok</p>")).toBe(false);
  });
});

describe("buildCsp", () => {
  const csp = buildCsp({
    hashes: ["'sha256-AAA='"],
    apiBase: "https://api.example.test/v1",
    cartApiBase: "https://cart.example.test/v1",
    imgBase: "https://img.example.test",
  });
  const directive = (name) => csp.split("; ").find((d) => d.startsWith(`${name} `)) ?? "";

  it("allows own scripts by hash and never unsafe-inline or eval", () => {
    expect(directive("script-src")).toContain("'self' 'sha256-AAA='");
    expect(directive("script-src")).not.toMatch(/unsafe-(inline|eval)/);
  });

  it("allows this build's own hosts", () => {
    expect(directive("connect-src")).toContain("https://api.example.test");
    expect(directive("connect-src")).toContain("https://cart.example.test");
    expect(directive("img-src")).toContain("https://img.example.test");
  });

  it("locks framing, base, objects and payment forms", () => {
    expect(csp).toContain("frame-ancestors 'none'");
    expect(csp).toContain("object-src 'none'");
    expect(csp).toContain("base-uri 'self'");
    expect(directive("form-action")).toBe("form-action 'self' https://sandbox.payhere.lk https://www.payhere.lk");
  });

  it("does not print empty hosts", () => {
    expect(buildCsp({})).not.toMatch(/\s{2,}|undefined/);
  });
});

const redirects = parseRedirects(`# GENERATED
/order/:id  /order/_/  200
/order/:id/  /order/_/  200
/console/shop/:shopId/order/:orderId  /console/shop/_/order/_/  200
/console/shop/:shopId/order/:orderId/  /console/shop/_/order/_/  200
`);

describe("parseRedirects", () => {
  it("reads one rule per route and skips comments and trailing-slash twins", () => {
    expect(redirects).toEqual([
      { from: "/order/:id", to: "/order/_/" },
      { from: "/console/shop/:shopId/order/:orderId", to: "/console/shop/_/order/_/" },
    ]);
  });
});

describe("pagePatterns", () => {
  it.each([
    ["index.html", ["/"]],
    ["orders/index.html", ["/orders/"]],
    ["s/krishna/index.html", ["/s/krishna/"]],
    ["order/_/index.html", ["/order/:id", "/order/:id/"]],
    ["console/shop/_/order/_/index.html", ["/console/shop/:shopId/order/:orderId", "/console/shop/:shopId/order/:orderId/"]],
  ])("%s", (file, want) => {
    expect(pagePatterns(file, redirects)).toEqual(want);
  });

  it("does not turn the empty-list shop placeholder into a catch-all that hides the 404 page", () => {
    expect(pagePatterns("s/_/index.html", redirects)).toEqual(["/s/_/"]);
  });

  it("makes a real-looking url for checks", () => {
    expect(sampleUrl("order/_/index.html", redirects)).toBe("/order/sample1/");
  });
});
