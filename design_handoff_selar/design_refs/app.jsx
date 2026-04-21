// SELAR — main app shell
const { useState, useRef, useEffect } = React;

function ReaderView({ cohort, matchesStyle, onStyleChange }) {
  const [docId, setDocId] = useState('backprop');
  const [activeSuggest, setActiveSuggest] = useState('s1');
  const [responses, setResponses] = useState(
    cohort === 'treatment_auto'
      ? Object.fromEntries(Object.values(sampleSuggestions).flat().map(m => [m.id, 'confirmed']))
      : { m2: 'confirmed' }
  );
  const [activeMatchId, setActiveMatchId] = useState(null);
  const [zoom, setZoom] = useState(1);
  const [annotationsOn, setAnnotationsOn] = useState(true);
  const [popover, setPopover] = useState(null);

  useEffect(() => {
    if (cohort === 'treatment_auto') {
      setResponses(Object.fromEntries(Object.values(sampleSuggestions).flat().map(m => [m.id, 'confirmed'])));
    } else if (cohort === 'control') {
      setResponses({});
    } else {
      setResponses({ m2: 'confirmed' });
    }
  }, [cohort]);

  const paper = PAPERS.backprop;
  const matches = cohort === 'control' ? null : sampleSuggestions[activeSuggest];

  const respond = (id, status) => {
    setResponses(r => ({ ...r, [id]: status }));
    setPopover(null);
  };

  const handlePassageClick = (suggestId, el) => {
    setActiveSuggest(suggestId);
    if (matchesStyle === 'popover' && cohort !== 'control') {
      const r = el.getBoundingClientRect();
      setPopover({ id: suggestId, top: r.bottom + 6, left: r.left });
    } else {
      setPopover(null);
    }
  };

  return (
    <div className="reader" onClick={() => setPopover(null)}>
      <Sidebar currentId={docId} onPick={setDocId} />
      <div className="doc-pane" style={{ transform: `scale(1)` }}>
        <DocToolbar zoom={zoom} setZoom={setZoom} page={4} total={14}
          annotationsOn={annotationsOn} setAnnotationsOn={setAnnotationsOn}/>
        <div style={{ transform: `scale(${zoom})`, transformOrigin: 'top center', transition: 'transform 0.12s' }}>
          <PdfPage
            paper={paper}
            activeSuggestId={activeSuggest}
            onPassageClick={cohort === 'control' ? null : handlePassageClick}
            showOverlays={cohort !== 'control'}
            annotationsOn={annotationsOn}
            pageNo={4}
          />
        </div>

        {popover && matches && (
          <div className="popover" style={{ position: 'fixed', top: popover.top, left: popover.left }} onClick={e => e.stopPropagation()}>
            <div className="head">
              <span className="src">{matches.length} matches · top {(matches[0].sim * 100).toFixed(0)}%</span>
              <div style={{ flex: 1 }}/>
              <button onClick={() => setPopover(null)} style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--ink-4)', fontSize: 14 }}>×</button>
            </div>
            <div className="snippet">{matches[0].tgtText}</div>
            <div style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)', marginBottom: 6 }}>
              → {matches[0].docTitle}
            </div>
            <div className="actions">
              <button className="primary" onClick={() => respond(matches[0].id, 'confirmed')}><Icon name="check" size={11}/> Confirm <span className="kbd" style={{ background: 'rgba(255,255,255,0.15)', color:'#fff', borderColor: 'transparent' }}>Y</span></button>
              <button onClick={() => respond(matches[0].id, 'rejected')}><Icon name="x" size={11}/> Reject <span className="kbd">N</span></button>
              <button><Icon name="tag" size={11}/> Label</button>
              <div style={{ flex: 1 }}/>
              <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>1 of {matches.length}</span>
            </div>
          </div>
        )}
      </div>

      {cohort !== 'control' && (
        <MatchesPanel
          variant={matchesStyle === 'popover' ? 'cards' : matchesStyle}
          matches={matches}
          responses={responses}
          onRespond={respond}
          activeId={activeMatchId}
          onPick={setActiveMatchId}
          cohort={cohort}
          onStyleChange={onStyleChange}
        />
      )}
    </div>
  );
}

const TWEAK_DEFAULS = /*EDITMODE-BEGIN*/{
  "cohort": "treatment_hitl",
  "matchesStyle": "cards",
  "theme": "default",
  "density": "compact",
  "overlays": true
}/*EDITMODE-END*/;

function SelarApp({ initialView = 'reader', initialCohort, initialMatchesStyle, showTweaks = false }) {
  const [view, setView] = useState(initialView);
  const [cohort, setCohort] = useState(initialCohort || TWEAK_DEFAULS.cohort);
  const [matchesStyle, setMatchesStyle] = useState(initialMatchesStyle || TWEAK_DEFAULS.matchesStyle);
  const [theme, setTheme] = useState(TWEAK_DEFAULS.theme);
  const [density, setDensity] = useState(TWEAK_DEFAULS.density);
  const [tweaksOpen, setTweaksOpen] = useState(false);

  useEffect(() => {
    if (!showTweaks) return;
    const onMsg = (e) => {
      if (e.data?.type === '__activate_edit_mode') setTweaksOpen(true);
      if (e.data?.type === '__deactivate_edit_mode') setTweaksOpen(false);
    };
    window.addEventListener('message', onMsg);
    window.parent.postMessage({ type: '__edit_mode_available' }, '*');
    return () => window.removeEventListener('message', onMsg);
  }, [showTweaks]);

  const persist = (edits) => {
    if (!showTweaks) return;
    window.parent.postMessage({ type: '__edit_mode_set_keys', edits }, '*');
  };

  const themeCls = theme === 'warm' ? 'theme-warm' : theme === 'sage' ? 'theme-sage' : theme === 'dark' ? 'theme-dark' : '';

  return (
    <div className={"selar-app " + themeCls}>
      <Topbar view={view} onView={setView} cohort={cohort} onCohortChange={setCohort}/>
      {view === 'library' && <LibraryView onOpen={(id) => { setView('reader'); }}/>}
      {view === 'reader' && <ReaderView cohort={cohort} matchesStyle={matchesStyle} onStyleChange={(v) => { setMatchesStyle(v); persist({ matchesStyle: v }); }}/>}
      {view === 'graph' && <GraphView/>}
      {view === 'quiz' && <QuizView/>}
      {view === 'settings' && <SettingsView cohort={cohort} onCohortChange={(c) => { setCohort(c); persist({ cohort: c }); }} theme={theme} setTheme={(t) => { setTheme(t); persist({ theme: t }); }} density={density} setDensity={(d) => { setDensity(d); persist({ density: d }); }}/>}
      {view === 'onboard' && <OnboardingView/>}

      {showTweaks && tweaksOpen && (
        <div className="tweaks-panel" onClick={e => e.stopPropagation()}>
          <h3>Tweaks</h3>
          <div className="t-row">
            <span className="tlbl">Cohort</span>
            <div className="t-seg">
              {[['control','Ctrl'],['treatment_auto','Auto'],['treatment_hitl','HITL']].map(([k,l]) => (
                <button key={k} className={cohort === k ? 'on' : ''} onClick={() => { setCohort(k); persist({ cohort: k }); }}>{l}</button>
              ))}
            </div>
          </div>
          <div className="t-row">
            <span className="tlbl">Matches</span>
            <div className="t-seg">
              {['cards','diff','feed','keyboard','popover'].map(k => (
                <button key={k} className={matchesStyle === k ? 'on' : ''} onClick={() => { setMatchesStyle(k); persist({ matchesStyle: k }); }}>{k}</button>
              ))}
            </div>
          </div>
          <div className="t-row">
            <span className="tlbl">Theme</span>
            <div className="t-seg">
              {['default','warm','sage','dark'].map(k => (
                <button key={k} className={theme === k ? 'on' : ''} onClick={() => { setTheme(k); persist({ theme: k }); }}>{k}</button>
              ))}
            </div>
          </div>
          <div className="t-row">
            <span className="tlbl">View</span>
            <div className="t-seg">
              {['library','reader','graph','quiz','settings','onboard'].map(k => (
                <button key={k} className={view === k ? 'on' : ''} onClick={() => setView(k)}>{k.slice(0,4)}</button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

window.SelarApp = SelarApp;
