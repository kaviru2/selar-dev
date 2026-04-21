// Quiz view
const quizQuestions = [
  { type: 'mcq', prompt: "In the back-propagation algorithm, the error signal at a hidden unit is computed as:",
    options: [
      "The sum of error signals from all units in the next layer, weighted by the connections to those units, times the derivative of the activation.",
      "The sum of output values of all units in the previous layer.",
      "The difference between the target and the hidden unit's output directly.",
      "The product of the input vector and the bias term of the unit.",
    ],
    tags: ['backpropagation','chain_rule'] },
];

const QuizView = () => {
  const [answer, setAnswer] = useState(null);
  const q = quizQuestions[0];
  const letters = ['A','B','C','D','E'];
  return (
    <div className="quiz-wrap">
      <div className="quiz-inner">
        <span className="phase-chip">Post-test · Day 15</span>
        <h1>Machine learning fundamentals — retention test</h1>
        <div className="desc">24 questions · ~18 minutes. Answers are saved as you go. You can't revisit questions after submitting.</div>
        <div className="quiz-progress">
          {Array.from({ length: 24 }).map((_, i) => (
            <div key={i} className={"seg" + (i < 7 ? ' done' : i === 7 ? ' cur' : '')} />
          ))}
        </div>
        <div className="q-card">
          <div className="q-num">Question 08 · Multiple choice · 1 correct</div>
          <div className="q-prompt">{q.prompt}</div>
          <div className="q-options">
            {q.options.map((o, i) => (
              <div key={i} className={"q-option" + (answer === i ? ' sel' : '')} onClick={() => setAnswer(i)}>
                <span className="letter">{letters[i]}</span>
                <span>{o}</span>
              </div>
            ))}
          </div>
          <div className="q-tags">
            <span style={{ color: 'var(--ink-4)', marginRight: 6 }}>concept tags:</span>
            {q.tags.map(t => <span key={t} className="tag">{t}</span>)}
          </div>
        </div>
        <div className="quiz-foot">
          <button className="btn"><Icon name="chevron_left" size={12}/> Previous</button>
          <div className="spacer" />
          <span style={{ fontFamily: 'var(--mono)', fontSize: 11, color: 'var(--ink-3)', marginRight: 12 }}>
            <Icon name="clock" size={11}/> 12:34 left
          </span>
          <button className="btn primary">Next <Icon name="arrow_right" size={12}/></button>
        </div>
      </div>
    </div>
  );
};

window.QuizView = QuizView;
