// /quizzes — learner list of available, upcoming and completed quizzes.
// Availability is decided by the Go API; this page only renders it.

import Link from "next/link";
import { getAuthToken } from "@/lib/auth";
import { serverFetch } from "@/lib/api";
import { Badge, Card, PageHeader } from "@/components/ui/Card";
import { ButtonLink } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { KIND_LABEL, describeAvailability, groupCards, type LearnerQuizCard } from "@/lib/quiz/quiz";

export const dynamic = "force-dynamic";

function QuizCard({ card }: { card: LearnerQuizCard }) {
  const a = card.availability;
  const last = card.last_attempt;
  const canStart = a.state === "available" || a.state === "in_progress";
  const attemptsLeft = a.attempts_allowed > 0 ? Math.max(0, a.attempts_allowed - a.attempts_used) : null;
  return (
    <Card as="li" className="quiz-card">
      <div className="quiz-card-head">
        <Badge tone={card.kind === "practice" ? "sky" : "green"}>{KIND_LABEL[card.kind]}</Badge>
        {a.state === "in_progress" && <Badge tone="amber" dot>In progress</Badge>}
        {a.state === "missed" && <Badge>Missed</Badge>}
      </div>
      <h3>{card.title}</h3>
      {card.description && <p className="quiz-card-desc">{card.description}</p>}
      <ul className="quiz-meta" aria-label="Quiz details">
        <li>{card.question_count} question{card.question_count === 1 ? "" : "s"}</li>
        {card.time_limit_seconds ? <li>{Math.round(card.time_limit_seconds / 60)} min time limit</li> : <li>No time limit</li>}
        {card.no_going_back && <li>One question at a time, no going back</li>}
        {attemptsLeft !== null && canStart && <li>{attemptsLeft} attempt{attemptsLeft === 1 ? "" : "s"} left</li>}
      </ul>
      <p className="quiz-when">{describeAvailability(card)}</p>
      {last && (
        <p className="quiz-last">
          {last.result_visible && last.score != null && last.max_points
            ? `Last score: ${last.score} / ${last.max_points}`
            : last.result_visible && last.pending_review
              ? "Submitted · awaiting review"
              : "Submitted"}
        </p>
      )}
      <div className="quiz-card-actions">
        {canStart && (
          <ButtonLink href={`/quizzes/${card.id}`} variant="primary">
            {a.state === "in_progress" ? "Resume" : a.attempts_used > 0 ? "Try again" : "Start"}
          </ButtonLink>
        )}
        {last?.id && (
          <Link className="quiz-link" href={`/quizzes/result/${last.id}`}>
            {last.result_visible ? "View results" : "View submission"}
          </Link>
        )}
      </div>
    </Card>
  );
}

function Section({ id, title, cards, empty }: { id: string; title: string; cards: LearnerQuizCard[]; empty: string }) {
  return (
    <section className="quiz-section" aria-labelledby={`h-${id}`}>
      <h2 id={`h-${id}`}>{title}</h2>
      {cards.length === 0 ? <p className="quiz-empty-line">{empty}</p> : (
        <ul className="quiz-grid">{cards.map((c) => <QuizCard key={c.id} card={c} />)}</ul>
      )}
    </section>
  );
}

export default async function QuizzesPage() {
  const token = await getAuthToken();
  let cards: LearnerQuizCard[] = [];
  let failed = false;
  if (token) {
    try {
      cards = await serverFetch<LearnerQuizCard[]>("/api/quizzes", token);
    } catch {
      failed = true;
    }
  }
  const g = groupCards(cards);
  return (
    <main className="quiz-page">
      <PageHeader eyebrow="Quizzes" title="Your quizzes" description="Quizzes open on the schedule your study team set. Answers save as you go." />
      {failed && <p role="alert" className="quiz-error">Quizzes could not be loaded. Refresh to try again.</p>}
      {!failed && cards.length === 0 ? (
        <EmptyState illustration="reading" title="No quizzes yet">When a quiz is assigned to you it will appear here.</EmptyState>
      ) : (
        <>
          <Section id="available" title="Available" cards={g.available} empty="Nothing to take right now." />
          <Section id="upcoming" title="Upcoming" cards={g.upcoming} empty="No upcoming quizzes." />
          <Section id="completed" title="Completed" cards={g.completed} empty="You have not completed a quiz yet." />
        </>
      )}
    </main>
  );
}
