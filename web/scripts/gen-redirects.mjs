// Writes public/_redirects from the id-route folders (docs/08 section 11.2).
// Run it again (npm run gen:redirects) after adding a page with an id in its path.
import { writeFile } from "node:fs/promises";
import { buildRedirects, findIdRoutes } from "./lib/redirects.mjs";

const routes = await findIdRoutes("src/app");
await writeFile("public/_redirects", buildRedirects(routes));
console.log(`gen-redirects: ${routes.length} id routes`);
