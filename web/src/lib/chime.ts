// A short two-note chime for "new bill", made with the Web Audio API so there is no sound file to
// ship (docs/08 section 4). Browsers only allow sound after the person has tapped something, so the
// queue has a sound switch; turning it on is that tap.

let context: AudioContext | null = null;

/** Call from a tap. Returns false when this browser cannot play sound. */
export function unlockChime(): boolean {
  try {
    context ??= new AudioContext();
    void context.resume();
    return true;
  } catch {
    return false;
  }
}

export function playChime(): void {
  if (!context || context.state !== "running") return;
  const start = context.currentTime;
  [880, 1175].forEach((hz, i) => {
    const osc = context!.createOscillator();
    const gain = context!.createGain();
    osc.frequency.value = hz;
    gain.gain.setValueAtTime(0.0001, start + i * 0.18);
    gain.gain.exponentialRampToValueAtTime(0.2, start + i * 0.18 + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, start + i * 0.18 + 0.3);
    osc.connect(gain).connect(context!.destination);
    osc.start(start + i * 0.18);
    osc.stop(start + i * 0.18 + 0.32);
  });
}
