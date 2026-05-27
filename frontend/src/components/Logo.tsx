type Props = {
  size?: number;
  withWordmark?: boolean;
  /** "dark" = light glyph for dark backgrounds, "light" = dark glyph for light bg */
  variant?: "dark" | "light";
};

export function Logo({ size = 28, withWordmark = false, variant = "dark" }: Props) {
  const cylinderFill = variant === "dark" ? "#8a96aa" : "#5b6b8a";
  const cylinderTop = variant === "dark" ? "#a9b6c8" : "#7c8aa3";
  const cylinderShade = variant === "dark" ? "#5b6b8a" : "#3d4a63";
  const ironBody = variant === "dark" ? "#c0c7d1" : "#9aa3b0";
  const ironGrip = "#c25e15";
  const ironTip = variant === "dark" ? "#5b6b8a" : "#3d4a63";
  const sparkCore = "#fff3d6";
  const sparkGlow = "#ec9a4b";
  const sparkOuter = "#e07a25";
  const wordPrimary = variant === "dark" ? "#ffffff" : "#0f1115";
  const wordSecondary = variant === "dark" ? "#9aa3b0" : "#6b7280";

  return (
    <div className="flex items-center gap-2.5">
      <svg
        width={size}
        height={size}
        viewBox="0 0 64 64"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-label="SolderDB"
      >
        {/* Database cylinder body */}
        <ellipse cx="22" cy="20" rx="14" ry="4" fill={cylinderTop} />
        <path
          d="M8 20 L8 44 Q8 48 22 48 Q36 48 36 44 L36 20 Z"
          fill={cylinderFill}
        />
        {/* Lower disk shade lines */}
        <ellipse cx="22" cy="32" rx="14" ry="3.5" fill={cylinderShade} opacity="0.5" />
        <ellipse cx="22" cy="44" rx="14" ry="3.5" fill={cylinderShade} opacity="0.4" />
        {/* Top rim highlight */}
        <ellipse cx="22" cy="20" rx="14" ry="4" fill="none" stroke={cylinderTop} strokeWidth="0.5" opacity="0.6" />

        {/* Soldering iron */}
        <g transform="rotate(-32 44 28)">
          <rect x="38" y="6" width="14" height="22" rx="2" fill={ironBody} />
          <rect x="38" y="22" width="14" height="8" rx="1.5" fill={ironGrip} />
          <path d="M42 30 L48 30 L46 40 L44 40 Z" fill={ironTip} />
        </g>

        {/* Molten solder spark */}
        <g className="origin-center animate-spark" style={{ transformOrigin: "32px 34px" }}>
          <circle cx="32" cy="34" r="6" fill={sparkOuter} opacity="0.35" />
          <circle cx="32" cy="34" r="4" fill={sparkGlow} opacity="0.9" />
          <circle cx="32" cy="34" r="2" fill={sparkCore} />
        </g>

        {/* Subtle connection dots (network) */}
        <circle cx="6" cy="36" r="1.5" fill={cylinderShade} />
        <circle cx="38" cy="36" r="1.5" fill={cylinderShade} />
      </svg>

      {withWordmark && (
        <div className="leading-tight">
          <div className="text-[15px] font-semibold tracking-tight" style={{ color: wordPrimary }}>
            Solder<span style={{ color: wordSecondary }}>DB</span>
          </div>
          <div className="text-[10px] font-medium tracking-wide" style={{ color: wordSecondary }}>
            Precision · Connection · Data
          </div>
        </div>
      )}
    </div>
  );
}
