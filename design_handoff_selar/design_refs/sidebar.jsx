// Sidebar / library list — shared across reader views
const LibraryList = [
  { id: 'backprop', title: 'Learning representations by back-propagating errors', authors: 'Rumelhart et al.', year: 1986, status: 'ready', chunks: 42, links: 18, pages: 14 },
  { id: 'gradient', title: 'Stochastic gradient descent: an overview', authors: 'Bottou', year: 2010, status: 'ready', chunks: 31, links: 12, pages: 10 },
  { id: 'linalg', title: 'Matrix computations · Ch. 1', authors: 'Golub & Van Loan', year: 2013, status: 'ready', chunks: 58, links: 9, pages: 22 },
  { id: 'deep', title: 'Deep Learning · Ch. 5: ML basics', authors: 'Goodfellow, Bengio, Courville', year: 2016, status: 'ready', chunks: 74, links: 24, pages: 28 },
  { id: 'lecture2', title: 'Lecture 2 · Loss & optimization', authors: 'ML 101 course', year: 2025, status: 'ready', chunks: 22, links: 7, pages: 18 },
  { id: 'attention', title: 'Attention is all you need', authors: 'Vaswani et al.', year: 2017, status: 'processing', chunks: 0, links: 0, pages: 15, progress: 0.62 },
  { id: 'adam', title: 'Adam: a method for stochastic optimization', authors: 'Kingma & Ba', year: 2014, status: 'ready', chunks: 28, links: 6, pages: 11 },
  { id: 'dropout', title: 'Dropout: A simple way to prevent overfitting', authors: 'Srivastava et al.', year: 2014, status: 'ready', chunks: 36, links: 11, pages: 13 },
];

const Sidebar = ({ currentId, onPick }) => {
  return (
    <aside className="sidebar">
      <div className="sb-head">
        <span className="lbl">Library</span>
        <span className="count">{LibraryList.length}</span>
        <button title="Upload" style={{ marginLeft: 6, background: 'transparent', border: '1px solid var(--rule)', borderRadius: 3, width: 20, height: 20, display: 'grid', placeItems: 'center', cursor: 'pointer', color: 'var(--ink-3)' }}>
          <Icon name="plus" size={10} />
        </button>
      </div>
      <div className="sb-search">
        <Icon name="search" size={11} style={{ color: 'var(--ink-4)' }}/>
        <input placeholder="Filter…" defaultValue=""/>
        <span className="kbd">⌘K</span>
      </div>
      <div className="sb-group">ML fundamentals · corpus</div>
      <div style={{ paddingBottom: 8 }}>
        {LibraryList.map(d => (
          <div key={d.id} className={"sb-item" + (currentId === d.id ? ' active' : '')} onClick={() => onPick && onPick(d.id)}>
            <span className={"dot " + d.status} />
            <span className="title">
              <span className="t">{d.title}</span>
              <span className="meta">{d.authors.split(' ')[0]}{d.authors.includes('et al.') ? ' et al.' : ''} · {d.year} · {d.status === 'processing' ? `embedding ${Math.round(d.progress * 100)}%` : `${d.links} links`}</span>
            </span>
          </div>
        ))}
      </div>
      <div className="sb-foot">
        <Icon name="drive" size={12} style={{ color: 'var(--accent-2)' }}/>
        <span style={{ fontFamily: 'var(--mono)', fontSize: 10 }}>Drive · synced 2m ago</span>
      </div>
    </aside>
  );
};

window.LibraryList = LibraryList;
window.Sidebar = Sidebar;
