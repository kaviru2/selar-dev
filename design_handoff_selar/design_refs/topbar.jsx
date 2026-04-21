// Topbar — nav + cohort indicator
const Topbar = ({ view, onView, cohort, onCohortChange }) => {
  const cohortLabel = {
    control: { label: 'Control', color: '#9a938a' },
    treatment_auto: { label: 'Auto-link', color: '#7a8c5c' },
    treatment_hitl: { label: 'HITL', color: '#c96442' },
  }[cohort];
  return (
    <div className="topbar">
      <div className="brand">
        <span className="brand-mark"/>
        <span>SELAR</span>
      </div>
      <div className="nav">
        {['library','reader','graph','quiz','settings'].map(k => (
          <button key={k} className={view === k ? 'active' : ''} onClick={() => onView(k)}>
            {k === 'reader' ? 'Reader' : k[0].toUpperCase() + k.slice(1)}
          </button>
        ))}
      </div>
      <div className="spacer" />
      <div className="meta">
        <span className="kbd-hint"><span className="kbd">J</span><span className="kbd">K</span> navigate · <span className="kbd">Y</span> confirm</span>
        <span className="cohort-chip" style={{ borderColor: cohortLabel.color + '66' }}>
          <span className="dot" style={{ background: cohortLabel.color }} />{cohortLabel.label}
        </span>
        <div className="avatar">AS</div>
      </div>
    </div>
  );
};

window.Topbar = Topbar;
