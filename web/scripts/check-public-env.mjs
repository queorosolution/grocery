// CI check XSS-09: no NEXT_PUBLIC_ name other than the ones in docs/13 section 3.
import { scan } from "./lib/publicEnv.mjs";

const problems = await scan(".");
if (problems.length > 0) {
  console.error(`Unknown NEXT_PUBLIC_ variables (they would ship to every browser):\n${problems.map((p) => `  - ${p}`).join("\n")}`);
  process.exit(1);
}
console.log("check-public-env: ok");
