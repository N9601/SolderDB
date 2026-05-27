/**
 * Skeleton placeholders. Used while data is loading so the layout doesn't
 * jump and the user gets immediate visual feedback.
 */
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={`shimmer rounded-md bg-canvas-200 ${className ?? ""}`}
      aria-hidden
    />
  );
}

export function StatSkeleton() {
  return (
    <div className="stat">
      <Skeleton className="h-3 w-16" />
      <div className="mt-3">
        <Skeleton className="h-6 w-20" />
      </div>
    </div>
  );
}

export function RowSkeleton() {
  return (
    <div className="flex items-center gap-3 border-b border-canvas-150 px-4 py-3">
      <Skeleton className="h-2.5 w-2.5 rounded-full" />
      <Skeleton className="h-3 flex-1 max-w-[160px]" />
      <Skeleton className="h-3 flex-1 max-w-[260px]" />
      <Skeleton className="h-3 w-12" />
    </div>
  );
}
