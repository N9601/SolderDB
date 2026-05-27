import { useEffect, useState } from "react";

/**
 * Boot-up animation shown while the app verifies the session and initializes.
 * Always runs for at least MIN_DURATION_MS so a fast init doesn't flicker.
 *
 * Stages, layered in CSS animation delays:
 *  0–250ms   logo materializes
 *  150–600ms solder spark pulses, copper rings expand outward
 *  500–900ms wordmark fades up
 *  800–1400ms boot-log lines tick in
 *  1400ms+   parent fades away (handled by `done` prop on consumer)
 */
type Props = {
  /** Called when the splash has finished its minimum runtime. The consumer
   *  can choose to keep showing it (while loading auth) or fade it out. */
  onMinDurationElapsed?: () => void;
  /** When true, plays the exit animation. The consumer is expected to
   *  unmount the component after ~400ms. */
  leaving?: boolean;
};

const MIN_DURATION_MS = 1600;

const BOOT_LINES = [
  { delay: 850, text: "open() data dir" },
  { delay: 1000, text: "replay wal.bin" },
  { delay: 1150, text: "load sstable index + bloom" },
  { delay: 1300, text: "bind 127.0.0.1:8787" },
  { delay: 1450, text: "ready" }
];

export function BootSplash({ onMinDurationElapsed, leaving }: Props) {
  const [revealed, setRevealed] = useState<number[]>([]);

  useEffect(() => {
    const timers: number[] = [];
    BOOT_LINES.forEach((l, i) => {
      const t = window.setTimeout(() => {
        setRevealed((prev) => [...prev, i]);
      }, l.delay);
      timers.push(t);
    });
    const done = window.setTimeout(() => {
      onMinDurationElapsed?.();
    }, MIN_DURATION_MS);
    timers.push(done);
    return () => timers.forEach((t) => window.clearTimeout(t));
  }, [onMinDurationElapsed]);

  return (
    <div className={`boot-splash ${leaving ? "boot-leaving" : ""}`} style={leaving ? { pointerEvents: "none" } : undefined}>
      {/* Ambient radial glow */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "radial-gradient(circle at 50% 45%, rgba(224,122,37,0.18) 0%, transparent 55%)"
        }}
      />

      {/* Grid */}
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.18]"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.06) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.06) 1px, transparent 1px)",
          backgroundSize: "32px 32px",
          maskImage: "radial-gradient(ellipse at center, black 30%, transparent 75%)",
          WebkitMaskImage: "radial-gradient(ellipse at center, black 30%, transparent 75%)"
        }}
      />

      <div className="relative flex flex-col items-center">
        {/* Expanding rings */}
        <div className="boot-rings">
          <span className="boot-ring boot-ring-1" />
          <span className="boot-ring boot-ring-2" />
          <span className="boot-ring boot-ring-3" />
        </div>

        {/* Logo */}
        <div className="boot-logo">
          <BootLogo />
        </div>

        {/* Wordmark */}
        <div className="boot-wordmark mt-7">
          <div className="text-[28px] font-semibold tracking-tight text-white">
            Solder<span className="text-copper-400">DB</span>
          </div>
          <div className="mt-1 text-center text-[10.5px] font-mono uppercase tracking-[0.24em] text-canvas-300">
            Precision · Connection · Data
          </div>
        </div>

        {/* Boot log */}
        <div className="boot-log mt-8 font-mono text-[11px] leading-[1.8] text-canvas-300">
          {BOOT_LINES.map((l, i) => (
            <div
              key={l.text}
              className="boot-log-line"
              style={{
                opacity: revealed.includes(i) ? 1 : 0,
                transform: revealed.includes(i) ? "translateY(0)" : "translateY(4px)",
                transition: "all 220ms cubic-bezier(0.22, 1, 0.36, 1)"
              }}
            >
              <span className="text-copper-500">›</span> {l.text}
            </div>
          ))}
        </div>
      </div>

      {/* Progress bar across the bottom */}
      <div className="absolute bottom-0 left-0 right-0 h-[2px] bg-gunmetal-800">
        <div className="boot-progress h-full" />
      </div>
    </div>
  );
}

/** A pared-down version of the main Logo, tuned for the splash — animated
 *  spark and copper-glow tip, with a slight scale-in entrance. */
function BootLogo() {
  return (
    <svg width="84" height="84" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <radialGradient id="boot-spark" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="#fff3d6" stopOpacity="1" />
          <stop offset="40%" stopColor="#ec9a4b" stopOpacity="0.9" />
          <stop offset="100%" stopColor="#e07a25" stopOpacity="0" />
        </radialGradient>
        <linearGradient id="boot-cyl" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#a9b6c8" />
          <stop offset="100%" stopColor="#5b6b8a" />
        </linearGradient>
      </defs>

      <g className="boot-logo-cyl">
        <ellipse cx="22" cy="20" rx="14" ry="4" fill="#a9b6c8" />
        <path d="M8 20 L8 44 Q8 48 22 48 Q36 48 36 44 L36 20 Z" fill="url(#boot-cyl)" />
        <ellipse cx="22" cy="32" rx="14" ry="3.5" fill="#3d4a63" opacity="0.5" />
        <ellipse cx="22" cy="44" rx="14" ry="3.5" fill="#3d4a63" opacity="0.4" />
      </g>

      <g transform="rotate(-32 44 28)" className="boot-logo-iron">
        <rect x="38" y="6" width="14" height="22" rx="2" fill="#c0c7d1" />
        <rect x="38" y="22" width="14" height="8" rx="1.5" fill="#c25e15" />
        <path d="M42 30 L48 30 L46 40 L44 40 Z" fill="#5b6b8a" />
      </g>

      <g className="boot-logo-spark" style={{ transformOrigin: "32px 34px" }}>
        <circle cx="32" cy="34" r="11" fill="url(#boot-spark)" opacity="0.7" />
        <circle cx="32" cy="34" r="4.5" fill="#ec9a4b" opacity="0.95" />
        <circle cx="32" cy="34" r="2" fill="#fff3d6" />
      </g>
    </svg>
  );
}
