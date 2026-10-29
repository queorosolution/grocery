import { Suspense } from "react";
import { SearchScreen } from "@/features/search/SearchScreen";

// useSearchParams needs a Suspense boundary in a static export: the query string only exists in the browser.
export default function Page() {
  return (
    <Suspense>
      <SearchScreen />
    </Suspense>
  );
}
