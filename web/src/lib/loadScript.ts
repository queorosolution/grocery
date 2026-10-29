// Loads an outside script once, on demand (Google sign-in, Maps, Turnstile). The caller passes a
// fixed https address from its own code, never one that came from a user or the API.

const pending = new Map<string, Promise<void>>();

export function loadScript(src: string): Promise<void> {
  const known = pending.get(src);
  if (known) return known;
  const loading = new Promise<void>((resolve, reject) => {
    const el = document.createElement("script");
    el.src = src;
    el.async = true;
    el.onload = () => resolve();
    el.onerror = () => {
      // Forget it so the next try (a retry button) can load it again.
      pending.delete(src);
      el.remove();
      reject(new Error("Could not load a script"));
    };
    document.head.append(el);
  });
  pending.set(src, loading);
  return loading;
}
