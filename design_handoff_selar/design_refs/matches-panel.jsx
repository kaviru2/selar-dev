// Matches panel — 5 variants
const relationLabel = {
  related_to: 'related to',
  prerequisite_of: 'prerequisite of',
  sub_concept_of: 'sub-concept of',
  contradicts: 'contradicts',
  extends: 'extends',
};

const SimBar = ({ value }) => (
  <div className="sim-bar"><div className="fill" style={{ width: (value * 100) + '%' }} /></div>
);

// Variant A — Cards with snippet + similarity bar
const MatchesCards = ({ matches, responses, onRespond, activeId, onPick, readOnly }) => {
  if (!matches) return <EmptyMatches />;
  return (
    <>
      <div className="match-group-lbl">Pending · {matches.filter(m => !responses[m.id]).length}</div>
      {matches.map(m => {
        const st = responses[m.id];
        return (
          <div key={m.id} className={"match-card" + (activeId === m.id ? ' active' : '') + (st === 'confirmed' ? ' confirmed' : st === 'rejected' ? ' rejected' : '')}
               onClick={() => onPick && onPick(m.id)}>
            <div className="row1">
              <span className="sim">{(m.sim * 100).toFixed(0)}%</span>
              <SimBar value={m.sim} />
              <span className="src">{m.docTitle}</span>
            </div>
            <div className="snippet">{m.tgtText}</div>
            <div className="foot">
              {readOnly ? (
                <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--accent-2)' }}>
                  <Icon name="link" size={11} /> linked · {relationLabel[m.relation]}
                </span>
              ) : st === 'confirmed' ? (
                <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--accent-2)' }}>
                  <Icon name="check" size={11} /> confirmed · {relationLabel[m.relation]}
                </span>
              ) : st === 'rejected' ? (
                <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>
                  <Icon name="x" size={11} /> rejected
                </span>
              ) : (
                <>
                  <button className="confirm" onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'confirmed'); }}>
                    <Icon name="check" size={11} /> Confirm
                  </button>
                  <button onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'rejected'); }}>
                    <Icon name="x" size={11} /> Reject
                  </button>
                  <button><Icon name="tag" size={11} /> Label</button>
                </>
              )}
              <div className="spacer" />
              <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>p.{m.page}</span>
            </div>
          </div>
        );
      })}
    </>
  );
};

// Variant B — Diff-style side-by-side
const MatchesDiff = ({ matches, responses, onRespond, activeId, onPick }) => {
  if (!matches) return <EmptyMatches />;
  return (
    <>
      <div className="match-group-lbl">Side-by-side comparison</div>
      {matches.map(m => {
        const st = responses[m.id];
        return (
          <div key={m.id} className={"match-card" + (activeId === m.id ? ' active' : '') + (st === 'confirmed' ? ' confirmed' : st === 'rejected' ? ' rejected' : '')}
               onClick={() => onPick && onPick(m.id)}>
            <div className="row1">
              <span className="sim">{(m.sim * 100).toFixed(0)}%</span>
              <SimBar value={m.sim} />
              <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>{relationLabel[m.relation]}</span>
            </div>
            <div className="match-diff">
              <div className="col src">
                <span className="lbl">this doc</span>
                {m.srcText}
              </div>
              <div className="col">
                <span className="lbl">{m.srcLabel}</span>
                {m.tgtText}
              </div>
            </div>
            <div className="foot">
              {st === 'confirmed' ? (
                <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--accent-2)' }}><Icon name="check" size={11} /> linked</span>
              ) : st === 'rejected' ? (
                <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>rejected</span>
              ) : (
                <>
                  <button className="confirm" onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'confirmed'); }}><Icon name="check" size={11} /> Link</button>
                  <button onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'rejected'); }}><Icon name="x" size={11} /> Dismiss</button>
                </>
              )}
              <div className="spacer" />
              <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)' }}>{m.docTitle}</span>
            </div>
          </div>
        );
      })}
    </>
  );
};

// Variant C — Stacked feed
const MatchesFeed = ({ matches, responses, onRespond, activeId, onPick }) => {
  if (!matches) return <EmptyMatches />;
  return (
    <>
      <div className="match-group-lbl">Feed · scroll to review</div>
      {matches.map(m => {
        const st = responses[m.id];
        return (
          <div key={m.id} className={"match-card feed-card" + (activeId === m.id ? ' active' : '') + (st === 'confirmed' ? ' confirmed' : st === 'rejected' ? ' rejected' : '')}
               onClick={() => onPick && onPick(m.id)}>
            <div className="row1">
              <span className="sim">{(m.sim * 100).toFixed(0)}</span>
              <SimBar value={m.sim} />
              <span className="src">{m.docTitle} · p.{m.page}</span>
            </div>
            <div className="snippet">{m.tgtText}</div>
            <div className="foot">
              <button className="confirm" onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'confirmed'); }}>✓</button>
              <button onClick={(e) => { e.stopPropagation(); onRespond(m.id, 'rejected'); }}>✗</button>
              <button>label</button>
              <div className="spacer" />
              <span className="kbd-hint">
                <span className="kbd">J</span><span className="kbd">K</span> next
              </span>
            </div>
          </div>
        );
      })}
    </>
  );
};

// Variant D — Keyboard-driven single-card mode
const MatchesKeyboard = ({ matches, responses, onRespond, activeId, onPick }) => {
  if (!matches) return <EmptyMatches />;
  const pending = matches.filter(m => !responses[m.id]);
  const active = pending[0] || matches[0];
  const i = matches.indexOf(active) + 1;
  return (
    <>
      <div style={{ padding: '10px 14px', display: 'flex', alignItems: 'center', gap: 8, fontFamily: 'var(--mono)', fontSize: 10.5, color: 'var(--ink-3)' }}>
        <span>{i} of {matches.length}</span>
        <div style={{ flex: 1, height: 2, background: 'var(--bg-3)', borderRadius: 1 }}>
          <div style={{ width: ((matches.length - pending.length) / matches.length * 100) + '%', height: '100%', background: 'var(--accent)' }}/>
        </div>
        <span>{matches.length - pending.length}/{matches.length} reviewed</span>
      </div>
      <div className="active-card" style={{ margin: '0 12px', padding: 14, background: 'var(--bg-2)', border: '1px solid var(--rule-2)', borderRadius: 6 }}>
        <div className="row1" style={{ fontFamily: 'var(--mono)', fontSize: 10.5, color: 'var(--ink-3)', display: 'flex', gap: 6, marginBottom: 6, alignItems: 'center' }}>
          <span style={{ color: 'var(--accent)', fontWeight: 600 }}>{(active.sim * 100).toFixed(0)}%</span>
          <SimBar value={active.sim} />
          <span>{relationLabel[active.relation]}</span>
        </div>
        <div style={{ fontFamily: 'var(--serif)', fontSize: 13.5, lineHeight: 1.5, color: 'var(--ink-2)', marginBottom: 10 }}>
          {active.tgtText}
        </div>
        <div style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)', marginBottom: 10 }}>
          → {active.docTitle} · {active.srcLabel}
        </div>
        <div style={{ display: 'flex', gap: 6, fontFamily: 'var(--mono)', fontSize: 10.5 }}>
          <button onClick={() => onRespond(active.id, 'confirmed')}
                  style={{ background: 'var(--ink)', color: 'var(--bg)', border: 'none', padding: '5px 10px', borderRadius: 3, cursor: 'pointer', fontFamily: 'inherit', fontSize: 10.5, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <span className="kbd" style={{ background: 'rgba(255,255,255,0.15)', color: '#fff', borderColor: 'transparent' }}>Y</span>
            Yes, link
          </button>
          <button onClick={() => onRespond(active.id, 'rejected')}
                  style={{ background: 'var(--bg)', color: 'var(--ink-2)', border: '1px solid var(--rule)', padding: '5px 10px', borderRadius: 3, cursor: 'pointer', fontFamily: 'inherit', fontSize: 10.5, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <span className="kbd">N</span> No
          </button>
          <button style={{ background: 'var(--bg)', color: 'var(--ink-2)', border: '1px solid var(--rule)', padding: '5px 10px', borderRadius: 3, cursor: 'pointer', fontFamily: 'inherit', fontSize: 10.5, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <span className="kbd">L</span> Label
          </button>
          <button style={{ background: 'transparent', color: 'var(--ink-3)', border: 'none', padding: '5px 10px', cursor: 'pointer', fontFamily: 'inherit', fontSize: 10.5, display: 'inline-flex', alignItems: 'center', gap: 5, marginLeft: 'auto' }}>
            <span className="kbd">K</span> skip
          </button>
        </div>
      </div>
      <div style={{ padding: '14px 14px 8px', fontFamily: 'var(--mono)', fontSize: 9.5, letterSpacing: '0.06em', color: 'var(--ink-4)', textTransform: 'uppercase' }}>Queue</div>
      <div style={{ padding: '0 14px' }}>
        {pending.slice(1, 5).map(m => (
          <div key={m.id} style={{ padding: '6px 0', borderBottom: '1px dashed var(--rule)', fontFamily: 'var(--serif)', fontSize: 12, color: 'var(--ink-3)', display: 'flex', gap: 8 }}>
            <span style={{ fontFamily: 'var(--mono)', fontSize: 10, color: 'var(--ink-4)', flexShrink: 0 }}>{(m.sim * 100).toFixed(0)}</span>
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{m.tgtText}</span>
          </div>
        ))}
      </div>
    </>
  );
};

const EmptyMatches = () => (
  <div style={{ padding: 40, textAlign: 'center', color: 'var(--ink-4)', fontSize: 12 }}>
    <div style={{ fontFamily: 'var(--mono)', fontSize: 10, letterSpacing: '0.1em', textTransform: 'uppercase', marginBottom: 6 }}>No passage selected</div>
    <div>Click a highlighted passage to see its semantic matches across your library.</div>
  </div>
);

const MatchesPanel = ({ variant, matches, responses, onRespond, activeId, onPick, cohort, onStyleChange }) => {
  const Body =
    variant === 'diff' ? MatchesDiff :
    variant === 'feed' ? MatchesFeed :
    variant === 'keyboard' ? MatchesKeyboard :
    MatchesCards;
  const readOnly = cohort === 'treatment_auto';
  const pendingCount = matches ? matches.filter(m => !responses[m.id] && !readOnly).length : 0;
  const confirmedCount = matches ? matches.filter(m => responses[m.id] === 'confirmed' || readOnly).length : 0;
  return (
    <aside className={"matches" + (variant === 'keyboard' ? ' keyboard' : '')}>
      <div className="matches-head">
        <span className="ttl">
          {cohort === 'treatment_auto' ? 'Applied links' : 'Matches'}
        </span>
        {matches && (
          <>
            {cohort !== 'treatment_auto' && <span className="chip">{pendingCount} pending</span>}
            <span className="chip" style={{ color: 'var(--accent-2)', borderColor: 'rgba(122,140,92,0.4)' }}>{confirmedCount} linked</span>
          </>
        )}
        <div className="spacer" />
        {onStyleChange && (
          <div className="seg">
            {['cards','diff','feed','keyboard'].map(v => (
              <button key={v} className={variant === v ? 'on' : ''} onClick={() => onStyleChange(v)}>{v}</button>
            ))}
          </div>
        )}
      </div>
      <div className="matches-body">
        <Body matches={matches} responses={responses} onRespond={onRespond} activeId={activeId} onPick={onPick} readOnly={readOnly} />
      </div>
    </aside>
  );
};

window.MatchesPanel = MatchesPanel;
