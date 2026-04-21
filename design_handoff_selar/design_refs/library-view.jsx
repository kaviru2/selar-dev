// Library view — dashboard with stats + doc table
const LibraryView = ({ onOpen }) => {
  return (
    <div className="library">
      <div className="head">
        <div>
          <h1>Library</h1>
          <div className="sub">8 documents · 291 chunks · 87 confirmed links</div>
        </div>
        <div className="spacer" />
        <button className="btn"><Icon name="filter" size={12}/> Filter</button>
        <button className="btn"><Icon name="drive" size={12}/> From Drive</button>
        <button className="btn primary"><Icon name="upload" size={12}/> Upload PDF</button>
      </div>
      <div className="stats">
        <div className="stat"><div className="lbl">Documents</div><div className="val">8</div><div className="delta">+2 this week</div></div>
        <div className="stat"><div className="lbl">Confirmed links</div><div className="val">87</div><div className="delta">+19 this week</div></div>
        <div className="stat"><div className="lbl">Reading time</div><div className="val">6h 24m</div><div className="delta" style={{ color: 'var(--ink-4)' }}>across 12 sessions</div></div>
        <div className="stat"><div className="lbl">Retention · predicted</div><div className="val">72%</div><div className="delta">post-test in 4d</div></div>
      </div>
      <div className="doc-table">
        <div className="doc-row head">
          <span></span>
          <span>Title</span>
          <span>Authors</span>
          <span>Year</span>
          <span>Pages</span>
          <span>Chunks · Links</span>
          <span>Status</span>
          <span></span>
        </div>
        {LibraryList.map(d => (
          <div key={d.id} className="doc-row" onClick={() => onOpen && onOpen(d.id)}>
            <span className={"dot " + d.status}/>
            <span className="title">{d.title}</span>
            <span className="authors">{d.authors}</span>
            <span className="mono">{d.year}</span>
            <span className="mono">{d.pages}p</span>
            <span className="mono">{d.chunks} · {d.links}</span>
            <span>
              {d.status === 'ready' ? (
                <span className="mono" style={{ color: 'var(--accent-2)' }}>ready</span>
              ) : (
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <div className="prog"><div className="fill" style={{ width: (d.progress * 100) + '%' }}/></div>
                  <span className="mono" style={{ fontSize: 10 }}>{Math.round(d.progress*100)}%</span>
                </span>
              )}
            </span>
            <button className="icon-btn" onClick={(e) => e.stopPropagation()}><Icon name="more" size={12}/></button>
          </div>
        ))}
      </div>
    </div>
  );
};

window.LibraryView = LibraryView;
