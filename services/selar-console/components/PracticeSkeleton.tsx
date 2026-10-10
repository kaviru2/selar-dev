// PracticeSkeleton — placeholder shown while practice data loads, so the page
// keeps its shape instead of flashing a lone line of text.

export function PracticeSkeleton({ label, cards = 1 }: { label: string; cards?: number }) {
  return (
    <div className="practice-skeleton" role="status" aria-live="polite" aria-busy="true">
      <span className="practice-sr-only">{label}</span>
      {Array.from({ length: cards }, (_, i) => (
        <div key={i} className="ui-card practice-skeleton-card" aria-hidden="true">
          <span className="practice-skeleton-line practice-skeleton-line--short" />
          <span className="practice-skeleton-line" />
          <span className="practice-skeleton-line practice-skeleton-line--block" />
        </div>
      ))}
    </div>
  );
}
