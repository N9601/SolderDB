import { useEffect, useRef, useState } from "react";

type Props = {
  value: number;
  /** Custom formatter (e.g. formatBytes). Defaults to integer toString(). */
  format?: (n: number) => string;
  /** Milliseconds to interpolate over. Default 320. */
  ms?: number;
  className?: string;
};

/**
 * Smoothly animates from the previous displayed value to the next prop value.
 * Cheap, runs ~16 frames per transition via rAF. Cancels & restarts if value
 * changes mid-flight.
 */
export function CountUp({ value, format, ms = 320, className }: Props) {
  const [displayed, setDisplayed] = useState(value);
  const fromRef = useRef(value);
  const startRef = useRef(0);
  const rafRef = useRef<number | null>(null);

  useEffect(() => {
    if (Math.abs(value - displayed) < 0.5) {
      setDisplayed(value);
      return;
    }
    fromRef.current = displayed;
    startRef.current = performance.now();

    const step = (now: number) => {
      const elapsed = now - startRef.current;
      const t = Math.min(1, elapsed / ms);
      const eased = 1 - Math.pow(1 - t, 3); // ease-out cubic
      const next = fromRef.current + (value - fromRef.current) * eased;
      setDisplayed(next);
      if (t < 1) {
        rafRef.current = window.requestAnimationFrame(step);
      } else {
        setDisplayed(value);
        rafRef.current = null;
      }
    };
    if (rafRef.current !== null) window.cancelAnimationFrame(rafRef.current);
    rafRef.current = window.requestAnimationFrame(step);

    return () => {
      if (rafRef.current !== null) window.cancelAnimationFrame(rafRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, ms]);

  const out = format ? format(displayed) : Math.round(displayed).toLocaleString();
  return <span className={className}>{out}</span>;
}
