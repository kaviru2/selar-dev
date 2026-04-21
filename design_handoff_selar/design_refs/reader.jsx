// Reader — PDF page + overlays + matches panel
const { useState, useRef, useEffect } = React;

const PdfPage = ({ paper, activeSuggestId, onPassageClick, showOverlays = true, annotationsOn = true, pageNo = 4 }) => {
  return (
    <div className="pdf-page">
      <div style={{ marginBottom: 16, fontFamily: 'var(--mono)', fontSize: 10, color: '#aaa', display: 'flex', justifyContent: 'space-between' }}>
        <span>{paper.authors.split(' — ')[1] || ''}</span>
        <span>page {pageNo}</span>
      </div>
      {pageNo === 4 && (
        <>
          <h1>{paper.title}</h1>
          <div className="authors">{paper.authors}</div>
        </>
      )}
      {paper.sections.map((sec, i) => (
        <section key={i}>
          <h2>{sec.heading}</h2>
          {sec.paragraphs.map((pg, j) => {
            let body = pg.text;
            const content = pg.hl === 'suggest' && showOverlays ? (
              <span
                className={"hl-suggest" + (activeSuggestId === pg.suggestId ? ' active' : '')}
                onClick={(e) => { e.stopPropagation(); onPassageClick && onPassageClick(pg.suggestId, e.currentTarget); }}
              >
                {body}
                {pg.badge && <span className="pin">{pg.badge}</span>}
              </span>
            ) : pg.hl === 'wheat' && annotationsOn ? (
              <span className="hl">{body}</span>
            ) : body;
            return (
              <p key={j} style={{ position: 'relative' }}>
                {content}
                {pg.eqn && (
                  <span className="eqn" style={{ display: 'block', marginTop: 6 }}>
                    {pg.eqn}<span className="eq-num">({pg.eqnum})</span>
                  </span>
                )}
              </p>
            );
          })}
        </section>
      ))}
      <div className="pg-num">— {pageNo} —</div>
    </div>
  );
};

const DocToolbar = ({ zoom, setZoom, page, total, annotationsOn, setAnnotationsOn }) => (
  <div className="doc-toolbar">
    <div className="grp">
      <button>‹</button>
      <span className="page-indicator">{page} / {total}</span>
      <button>›</button>
    </div>
    <div className="grp">
      <button onClick={() => setZoom(Math.max(0.5, zoom - 0.1))}><Icon name="zoom_out" size={12} /></button>
      <span className="page-indicator" style={{ padding: '0 6px' }}>{Math.round(zoom * 100)}%</span>
      <button onClick={() => setZoom(Math.min(2, zoom + 0.1))}><Icon name="zoom_in" size={12} /></button>
    </div>
    <div className="grp">
      <button className={annotationsOn ? 'on' : ''} onClick={() => setAnnotationsOn(!annotationsOn)}>
        <Icon name="highlight" size={12} /> Marks
      </button>
    </div>
    <div className="tool-spacer" />
    <div className="grp">
      <button><Icon name="search" size={12} /></button>
      <button><Icon name="note" size={12} /> Note</button>
      <button><Icon name="more" size={12} /></button>
    </div>
  </div>
);

const sampleSuggestions = {
  s1: [
    { id: 'm1', sim: 0.88, docTitle: "Bottou 2010 · Stochastic gradient descent", page: 2,
      srcText: "the output vector produced by the network is the same as (or sufficiently close to) the desired output vector",
      tgtText: "The learning problem reduces to finding weights w that minimize the empirical risk — the mean loss between predicted and target outputs across the training sample.",
      srcLabel: "Backprop § 3",
      relation: "related_to" },
    { id: 'm2', sim: 0.81, docTitle: "Goodfellow et al. · Deep Learning, Ch. 5", page: 103,
      srcText: "output vector produced by the network is the same as the desired output vector",
      tgtText: "Supervised learning algorithms learn a function that maps inputs to outputs, given a dataset of example input-output pairs (x, y). Training seeks parameters θ such that f(x;θ) ≈ y.",
      srcLabel: "Ch 5.1", relation: "sub_concept_of" },
    { id: 'm3', sim: 0.74, docTitle: "Lecture 2 slides · ML101", page: 12,
      srcText: "the desired output vector",
      tgtText: "Training labels y constitute the supervision signal. The loss function measures disagreement between the model's prediction ŷ and the ground-truth y.",
      srcLabel: "slide 12", relation: "prerequisite_of" },
  ],
  s2: [
    { id: 'm4', sim: 0.92, docTitle: "Bottou 2010 · Stochastic gradient descent", page: 3,
      srcText: "minimize E by gradient descent it is necessary to compute the partial derivative of E with respect to each weight",
      tgtText: "Gradient descent iteratively updates parameters in the direction opposite to the gradient of the cost, θₜ₊₁ = θₜ − η ∇E(θₜ), where η is the learning rate.",
      srcLabel: "eq. 1.4", relation: "extends" },
    { id: 'm5', sim: 0.79, docTitle: "Golub & Van Loan · Ch. 1", page: 8,
      srcText: "compute the partial derivative",
      tgtText: "For a differentiable scalar field f: ℝⁿ→ℝ, the gradient ∇f is the vector of partial derivatives. It points in the direction of steepest ascent.",
      srcLabel: "§ 1.3", relation: "prerequisite_of" },
  ],
  s3: [
    { id: 'm6', sim: 0.71, docTitle: "Vaswani et al. 2017 · Attention is all you need", page: 3,
      srcText: "An input vector is presented to the network by clamping the states of the input units",
      tgtText: "Each input token is first embedded into a dense vector representation; these embeddings are then fed to the encoder stack as the initial input activation.",
      srcLabel: "§ 3.1", relation: "related_to" },
  ],
};

window.PdfPage = PdfPage;
window.DocToolbar = DocToolbar;
window.sampleSuggestions = sampleSuggestions;
