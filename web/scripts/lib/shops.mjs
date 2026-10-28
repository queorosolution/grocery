/** Keeps only well-formed shops from the API answer. Anything odd is dropped, never trusted. */
export function cleanShopList(body) {
  const rows = Array.isArray(body?.data) ? body.data : [];
  const shops = [];
  for (const row of rows) {
    if (typeof row?.slug !== "string" || !/^[a-z0-9][a-z0-9-]{0,59}$/.test(row.slug)) continue;
    if (typeof row.name !== "string" || row.name.length === 0 || row.name.length > 120) continue;
    shops.push({ slug: row.slug, name: row.name, logoUrl: httpsOrNull(row.logoUrl) });
  }
  return shops.sort((a, b) => a.slug.localeCompare(b.slug));
}

function httpsOrNull(value) {
  try {
    return typeof value === "string" && new URL(value).protocol === "https:" ? value : null;
  } catch {
    return null;
  }
}
