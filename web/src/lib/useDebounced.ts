import { useEffect, useState } from "react";

/** `value`, but only after it has stopped changing for `ms` (search box: 300 ms, docs/08 section 5). */
export function useDebounced<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return settled;
}
