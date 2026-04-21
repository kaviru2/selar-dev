// Onboarding & Settings
const OnboardingView = () => {
  const steps = [
    { no: '01', lbl: 'Sign in with Google', sub: 'OAuth · basic profile only', done: true, cta: null },
    { no: '02', lbl: 'Connect Google Drive', sub: 'PDFs stay in your own Drive — SELAR reads, never uploads', done: true, cta: null },
    { no: '03', lbl: 'Receive seed corpus (8 papers)', sub: 'Pre-loaded ML fundamentals reading list · ~240 pages', done: true, cta: null },
    { no: '04', lbl: 'Take the 20-minute pre-test', sub: 'Establishes baseline retention before the study period', done: false, cta: 'Start pre-test' },
  ];
  return (
    <div className="onboard">
      <div className="left">
        <div className="brand-big">
          <span className="brand-mark" style={{ width: 18, height: 18, position: 'relative', display: 'inline-block' }}>
            <span style={{ position: 'absolute', inset: 0, background: 'var(--ink)', clipPath: 'polygon(0 0,100% 0,100% 100%,50% 100%,50% 50%,0 50%)' }}/>
            <span style={{ position: 'absolute', inset: 0, border: '1px solid var(--ink)', clipPath: 'polygon(50% 50%,100% 50%,100% 100%,50% 100%)', background: 'var(--accent)' }}/>
          </span>
          SELAR
          <span style={{ fontWeight: 400, color: 'var(--ink-4)', fontSize: 12, marginLeft: 4 }}>· onboarding</span>
        </div>
        <h1>Welcome, Ashen.</h1>
        <div className="lede">
          You're participating in a 14-day study on whether confirming semantic links between documents strengthens retention.
          Before you start reading, we need to get a baseline.
        </div>
        <div className="steps">
          {steps.map(s => (
            <div key={s.no} className={"step" + (s.done ? ' done' : '')}>
              <span className="no">{s.done ? '✓' : s.no}</span>
              <div>
                <div className="lbl">{s.lbl}</div>
                <div className="sub">{s.sub}</div>
              </div>
              <span>
                {s.cta && <button className="btn primary">{s.cta} <Icon name="arrow_right" size={12}/></button>}
              </span>
            </div>
          ))}
        </div>
        <div style={{ marginTop: 22, fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>
          UCSC ethics protocol SELAR-2026-04 · consent signed 18 Apr 2026
        </div>
      </div>
      <div className="right">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.1em', color: 'var(--ink-4)', textTransform: 'uppercase' }}>Your cohort assignment</div>
        <div className="cohort-card">
          <div className="lbl" style={{ color: 'var(--accent)' }}>Treatment · HITL</div>
          <div className="name">Confirm-then-link</div>
          <div className="desc">
            As you read, SELAR will surface candidate semantic matches in a side panel.
            You decide which to confirm, reject, or relabel. The confirmation itself is the study intervention.
          </div>
          <div style={{ borderTop: '1px solid var(--rule)', marginTop: 14, paddingTop: 10, display: 'flex', gap: 18, fontSize: 12, color: 'var(--ink-3)' }}>
            <span><b style={{ color: 'var(--ink)' }}>~10 min</b> reading / day</span>
            <span><b style={{ color: 'var(--ink)' }}>14 days</b> active</span>
            <span><b style={{ color: 'var(--ink)' }}>3</b> quizzes</span>
          </div>
        </div>
        <div className="mini-note">
          Cohort was assigned at random. You'll learn what the other cohorts saw at debrief on day 22.
        </div>
      </div>
    </div>
  );
};

const SettingsView = ({ cohort, onCohortChange, theme, setTheme, density, setDensity }) => (
  <div className="settings-wrap">
    <div className="settings-inner">
      <h1>Settings</h1>
      <div className="settings-group">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.08em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>Account</div>
        <div className="settings-row">
          <div className="k">Signed in as<span className="sub">via Clerk · Google OAuth</span></div>
          <div className="v">ashen.silva@stu.ucsc.cmb.ac.lk</div>
        </div>
        <div className="settings-row">
          <div className="k">Google Drive<span className="sub">PDF source</span></div>
          <div className="v" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Icon name="drive" size={12} style={{ color: 'var(--accent-2)' }}/>
            <span>Connected · /SELAR corpus</span>
            <button className="btn" style={{ marginLeft: 8 }}>Disconnect</button>
          </div>
        </div>
      </div>

      <div className="settings-group">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.08em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>Study</div>
        <div className="settings-row">
          <div className="k">Cohort<span className="sub">assigned at random — not user-configurable in production</span></div>
          <div className="v">
            <div className="toggle">
              {['control','treatment_auto','treatment_hitl'].map(c => (
                <button key={c} className={cohort === c ? 'on' : ''} onClick={() => onCohortChange(c)}>
                  {c === 'treatment_hitl' ? 'HITL' : c === 'treatment_auto' ? 'Auto' : 'Control'}
                </button>
              ))}
            </div>
            <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 4, fontFamily: 'var(--mono)' }}>(toggle exposed in mock for demo only)</div>
          </div>
        </div>
        <div className="settings-row">
          <div className="k">Study progress<span className="sub">days remaining</span></div>
          <div className="v" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div style={{ width: 180, height: 6, background: 'var(--bg-3)', borderRadius: 3 }}>
              <div style={{ width: '42%', height: '100%', background: 'var(--accent)', borderRadius: 3 }}/>
            </div>
            <span style={{ fontFamily: 'var(--mono)', fontSize: 11 }}>day 6 / 14 · 8 to go</span>
          </div>
        </div>
      </div>

      <div className="settings-group">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.08em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>Appearance</div>
        <div className="settings-row">
          <div className="k">Theme</div>
          <div className="v">
            <div className="toggle">
              {[['default','Paper'],['warm','Warm'],['sage','Sage'],['dark','Dark']].map(([k,l]) => (
                <button key={k} className={theme === k ? 'on' : ''} onClick={() => setTheme(k)}>{l}</button>
              ))}
            </div>
          </div>
        </div>
        <div className="settings-row">
          <div className="k">Density</div>
          <div className="v">
            <div className="toggle">
              {['compact','balanced','spacious'].map(k => (
                <button key={k} className={density === k ? 'on' : ''} onClick={() => setDensity(k)}>{k}</button>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div className="settings-group">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.08em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>Privacy & data</div>
        <div className="settings-row">
          <div className="k">Export my data<span className="sub">JSON of all annotations, confirmations, quiz responses</span></div>
          <div className="v"><button className="btn">Download .json</button></div>
        </div>
        <div className="settings-row">
          <div className="k">Withdraw from study<span className="sub">Irreversible · deletes all link and quiz data</span></div>
          <div className="v"><button className="btn" style={{ color: '#c0443a', borderColor: '#c0443a55' }}>Withdraw</button></div>
        </div>
      </div>
    </div>
  </div>
);

window.OnboardingView = OnboardingView;
window.SettingsView = SettingsView;
