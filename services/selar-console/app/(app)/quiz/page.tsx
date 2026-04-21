// page.tsx — Retention quiz view.
// Centered card layout with progress segments, multiple-choice options, and concept tags.
// Matches design_handoff_selar §6 — Retention quiz.

"use client";

import { useState } from "react";
import { Icon } from "@/components/ui/Icon";

const QUESTION = {
  type: "mcq",
  prompt:
    "In the back-propagation algorithm, the error signal at a hidden unit is computed as:",
  options: [
    "The sum of error signals from all units in the next layer, weighted by the connections to those units, times the derivative of the activation.",
    "The sum of output values of all units in the previous layer.",
    "The difference between the target and the hidden unit's output directly.",
    "The product of the input vector and the bias term of the unit.",
  ],
  tags: ["backpropagation", "chain_rule"],
};

const LETTERS = ["A", "B", "C", "D"];

export default function QuizPage() {
  const [answer, setAnswer] = useState<number | null>(null);

  return (
    <div className="quiz-wrap">
      <div className="quiz-inner">
        <span className="phase-chip">Post-test · Day 15</span>
        <h1 style={{ fontSize: 22, fontWeight: 600, letterSpacing: "-0.02em", margin: "10px 0 4px" }}>
          Machine learning fundamentals — retention test
        </h1>
        <div style={{ color: "var(--ink-3)", fontSize: "var(--t-md)", marginBottom: 22 }}>
          24 questions · ~18 minutes. Answers are saved as you go. You can&apos;t
          revisit questions after submitting.
        </div>

        <div className="quiz-progress">
          {Array.from({ length: 24 }).map((_, i) => (
            <div
              key={i}
              className={`seg${i < 7 ? " done" : i === 7 ? " cur" : ""}`}
            />
          ))}
        </div>

        <div className="q-card">
          <div className="q-num">
            Question 08 · Multiple choice · 1 correct
          </div>
          <div className="q-prompt">{QUESTION.prompt}</div>
          <div className="q-options">
            {QUESTION.options.map((o, i) => (
              <div
                key={i}
                className={`q-option${answer === i ? " sel" : ""}`}
                onClick={() => setAnswer(i)}
              >
                <span className="letter">{LETTERS[i]}</span>
                <span>{o}</span>
              </div>
            ))}
          </div>
          <div className="q-tags">
            <span style={{ color: "var(--ink-4)", marginRight: 6 }}>
              concept tags:
            </span>
            {QUESTION.tags.map((t) => (
              <span key={t} className="tag">
                {t}
              </span>
            ))}
          </div>
        </div>

        <div className="quiz-foot">
          <button className="btn">
            <Icon name="chevron_left" size={12} /> Previous
          </button>
          <div style={{ flex: 1 }} />
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: 11,
              color: "var(--ink-3)",
              marginRight: 12,
            }}
          >
            <Icon name="clock" size={11} /> 12:34 left
          </span>
          <button className="btn primary">
            Next <Icon name="arrow_right" size={12} />
          </button>
        </div>
      </div>
    </div>
  );
}
