// Graph view — SVG force-graph-ish visualization
const GraphView = () => {
  const nodes = [
    { id: 'lin',  label: 'Linear algebra',     x: 140, y: 180, r: 22, color: '#8a6a3d' },
    { id: 'cal',  label: 'Calculus',           x: 220, y: 310, r: 20, color: '#8a6a3d' },
    { id: 'grad', label: 'Gradient descent',   x: 330, y: 220, r: 28, color: '#c96442' },
    { id: 'sgd',  label: 'SGD',                x: 440, y: 320, r: 22, color: '#c96442' },
    { id: 'loss', label: 'Loss function',      x: 340, y: 400, r: 20, color: '#c96442' },
    { id: 'bp',   label: 'Backpropagation',    x: 510, y: 220, r: 30, color: '#c96442' },
    { id: 'nn',   label: 'Neural network',     x: 520, y: 120, r: 24, color: '#c96442' },
    { id: 'adm',  label: 'Adam optimizer',     x: 580, y: 400, r: 20, color: '#7a8c5c' },
    { id: 'drop', label: 'Dropout',            x: 660, y: 280, r: 20, color: '#7a8c5c' },
    { id: 'att',  label: 'Attention',          x: 720, y: 140, r: 22, color: '#7a8c5c' },
    { id: 'trf',  label: 'Transformers',       x: 830, y: 220, r: 26, color: '#7a8c5c' },
    { id: 'emb',  label: 'Embeddings',         x: 780, y: 380, r: 20, color: '#7a8c5c' },
  ];
  const edges = [
    ['lin','grad','prerequisite_of'], ['cal','grad','prerequisite_of'],
    ['grad','sgd','extends'], ['grad','bp','related_to'],
    ['loss','grad','related_to'], ['bp','nn','sub_concept_of'],
    ['sgd','adm','extends'], ['nn','drop','related_to'],
    ['nn','att','prerequisite_of'], ['att','trf','sub_concept_of'],
    ['trf','emb','related_to'], ['bp','loss','related_to'],
    ['lin','cal','related_to'], ['emb','att','prerequisite_of'],
  ];
  const byId = Object.fromEntries(nodes.map(n => [n.id, n]));
  const [selected, setSelected] = useState('bp');
  const sel = byId[selected];

  return (
    <div className="graph-wrap">
      <div className="graph-main">
        <svg width="100%" height="100%" viewBox="0 0 960 540" style={{ display: 'block' }}>
          <defs>
            <marker id="arr" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
              <path d="M0 0 L8 4 L0 8 Z" fill="#9a938a"/>
            </marker>
          </defs>
          {edges.map(([a,b,r], i) => {
            const n1 = byId[a], n2 = byId[b];
            const highlighted = a === selected || b === selected;
            const dx = n2.x - n1.x, dy = n2.y - n1.y, L = Math.sqrt(dx*dx+dy*dy);
            const ux = dx/L, uy = dy/L;
            const x1 = n1.x + ux*n1.r, y1 = n1.y + uy*n1.r;
            const x2 = n2.x - ux*n2.r, y2 = n2.y - uy*n2.r;
            return (
              <g key={i}>
                <line x1={x1} y1={y1} x2={x2} y2={y2}
                      stroke={highlighted ? '#c96442' : '#c4beb2'}
                      strokeWidth={highlighted ? 1.5 : 1}
                      markerEnd="url(#arr)" />
                {highlighted && (
                  <text x={(x1+x2)/2} y={(y1+y2)/2 - 4} fontSize="9" fontFamily="var(--mono)" fill="#c96442" textAnchor="middle">
                    {r.replace('_',' ')}
                  </text>
                )}
              </g>
            );
          })}
          {nodes.map(n => (
            <g key={n.id} style={{ cursor: 'pointer' }} onClick={() => setSelected(n.id)}>
              <circle cx={n.x} cy={n.y} r={n.r}
                      fill={selected === n.id ? n.color : n.color + '22'}
                      stroke={n.color} strokeWidth={selected === n.id ? 2 : 1.2}/>
              <text x={n.x} y={n.y + n.r + 14} fontSize="11.5" fontFamily="var(--sans)"
                    fill={selected === n.id ? '#1a1816' : '#3a3633'}
                    fontWeight={selected === n.id ? 600 : 400}
                    textAnchor="middle">{n.label}</text>
            </g>
          ))}
        </svg>
        <div className="graph-legend">
          <div style={{ fontWeight: 600, color: 'var(--ink-2)', marginBottom: 4 }}>Concept origin</div>
          <div className="row"><span className="sw" style={{ background: '#8a6a3d' }}/> prerequisite</div>
          <div className="row"><span className="sw" style={{ background: '#c96442' }}/> core</div>
          <div className="row"><span className="sw" style={{ background: '#7a8c5c' }}/> extension</div>
        </div>
      </div>
      <div className="graph-side">
        <div style={{ fontFamily: 'var(--mono)', fontSize: 9.5, letterSpacing: '0.08em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 4 }}>Concept</div>
        <h2>{sel.label}</h2>
        <p>The algorithm that enables deep networks to learn by propagating output-layer error gradients backward through each weight via the chain rule.</p>
        <div className="kv">
          <span className="k">Sources</span><span>3 documents · 9 chunks</span>
          <span className="k">Confirmed</span><span style={{ color: 'var(--accent-2)' }}>6 edges</span>
          <span className="k">Pending</span><span style={{ color: 'var(--accent)' }}>2 edges</span>
          <span className="k">Last seen</span><span>2h ago · Rumelhart 1986 p.4</span>
        </div>
        <div className="edge-list">
          <div style={{ fontFamily: 'var(--mono)', fontSize: 9.5, letterSpacing: '0.06em', color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>Relations</div>
          {edges.filter(e => e[0] === selected || e[1] === selected).map((e, i) => {
            const other = e[0] === selected ? e[1] : e[0];
            const dir = e[0] === selected ? '→' : '←';
            return (
              <div key={i} className="edge-row">
                <span className="rel">{e[2].replace('_',' ')}</span>
                <span style={{ color: 'var(--ink-4)' }}>{dir}</span>
                <span>{byId[other].label}</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};

window.GraphView = GraphView;
