-- Quiz system (issue #86): admin-authored initial / follow-up / practice
-- quizzes with server-enforced windows, attempts, time limits and feedback.
--
-- 001 created placeholder quiz tables that no code path ever wrote. They are
-- replaced here. The migration refuses to run if any of them hold rows, so no
-- assessment data can be dropped silently.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM quiz_responses) OR EXISTS (SELECT 1 FROM quiz_attempts)
     OR EXISTS (SELECT 1 FROM quiz_questions) OR EXISTS (SELECT 1 FROM quizzes) THEN
    RAISE EXCEPTION 'legacy quiz tables contain rows; export them before migration 012 replaces them';
  END IF;
END $$;

DROP TABLE quiz_responses;
DROP TABLE quiz_attempts;
DROP TABLE quiz_questions;
DROP TABLE quizzes;

CREATE TABLE quizzes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL CHECK (btrim(title) <> ''),
    description  TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL CHECK (kind IN ('initial', 'follow_up', 'practice')),
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'closed')),
    -- Delivery rules (windows, relative availability, limits, feedback policy,
    -- audience, shuffling, no-going-back). Validated by internal/quiz.
    settings     JSONB NOT NULL DEFAULT '{}',
    -- Bumped whenever questions change; attempts record the version they saw.
    version      INT NOT NULL DEFAULT 1,
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    closed_at    TIMESTAMPTZ
);
CREATE INDEX idx_quizzes_status ON quizzes(status);

CREATE TABLE quiz_questions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id          UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    position         INT NOT NULL CHECK (position >= 0),
    type             TEXT NOT NULL CHECK (type IN ('single_choice', 'multiple_choice', 'true_false',
                                                    'short_answer', 'free_recall', 'cued_recall')),
    prompt           TEXT NOT NULL CHECK (btrim(prompt) <> ''),
    cue              TEXT NOT NULL DEFAULT '',
    -- [{id, text, correct}] — the correct flags never leave the API for learners
    -- before the feedback policy allows.
    options          JSONB NOT NULL DEFAULT '[]',
    accepted_answers TEXT[] NOT NULL DEFAULT '{}',
    points           NUMERIC(8,2) NOT NULL DEFAULT 1 CHECK (points >= 0),
    rubric           TEXT NOT NULL DEFAULT '',
    explanation      TEXT NOT NULL DEFAULT '',
    source_document  TEXT NOT NULL DEFAULT '',
    source_url       TEXT NOT NULL DEFAULT '',
    UNIQUE (quiz_id, position) DEFERRABLE INITIALLY DEFERRED
);

-- Free-form audience labels ("group:<label>" audiences). Cohort audiences use users.cohort.
CREATE TABLE quiz_group_members (
    label    TEXT NOT NULL CHECK (btrim(label) <> ''),
    user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (label, user_id)
);
CREATE INDEX idx_quiz_group_members_user ON quiz_group_members(user_id);

CREATE TABLE quiz_attempts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id          UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempt_number   INT NOT NULL CHECK (attempt_number > 0),
    quiz_version     INT NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Server-computed: min(start + time limit, effective close). NULL = none.
    deadline_at      TIMESTAMPTZ,
    submitted_at     TIMESTAMPTZ,
    submit_reason    TEXT CHECK (submit_reason IN ('learner', 'time_limit', 'closed')),
    -- Order this learner sees (after optional shuffling), fixed at start.
    question_order   UUID[] NOT NULL,
    option_order     JSONB NOT NULL DEFAULT '{}',
    -- No-going-back mode: index into question_order the learner is on.
    current_position INT NOT NULL DEFAULT 0,
    max_points       NUMERIC(10,2) NOT NULL DEFAULT 0,
    auto_points      NUMERIC(10,2) NOT NULL DEFAULT 0,
    pending_review   INT NOT NULL DEFAULT 0,
    score            NUMERIC(10,2),
    UNIQUE (quiz_id, user_id, attempt_number),
    CHECK ((submitted_at IS NULL) = (submit_reason IS NULL))
);
-- At most one unsubmitted attempt per learner and quiz.
CREATE UNIQUE INDEX idx_quiz_attempts_one_open ON quiz_attempts(quiz_id, user_id) WHERE submitted_at IS NULL;
CREATE INDEX idx_quiz_attempts_user ON quiz_attempts(user_id, quiz_id);

CREATE TABLE quiz_answers (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id         UUID NOT NULL REFERENCES quiz_attempts(id) ON DELETE CASCADE,
    question_id        UUID NOT NULL REFERENCES quiz_questions(id) ON DELETE CASCADE,
    selected           TEXT[] NOT NULL DEFAULT '{}',
    answer_text        TEXT NOT NULL DEFAULT '',
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at        TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    revisions          INT NOT NULL DEFAULT 0,
    -- Client-reported active time on the question, capped server-side by wall time.
    time_on_question_ms BIGINT NOT NULL DEFAULT 0 CHECK (time_on_question_ms >= 0),
    auto_score         NUMERIC(8,2),
    is_correct         BOOL,
    needs_review       BOOL NOT NULL DEFAULT false,
    manual_score       NUMERIC(8,2) CHECK (manual_score IS NULL OR manual_score >= 0),
    grader_note        TEXT NOT NULL DEFAULT '',
    graded_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    graded_at          TIMESTAMPTZ,
    UNIQUE (attempt_id, question_id)
);

-- Submitted answers are immutable for learners; only grading columns may change.
CREATE FUNCTION quiz_answer_lock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (SELECT submitted_at FROM quiz_attempts WHERE id = NEW.attempt_id) IS NOT NULL AND (
       NEW.selected IS DISTINCT FROM OLD.selected OR
       NEW.answer_text IS DISTINCT FROM OLD.answer_text OR
       NEW.time_on_question_ms IS DISTINCT FROM OLD.time_on_question_ms OR
       NEW.question_id IS DISTINCT FROM OLD.question_id OR
       NEW.attempt_id IS DISTINCT FROM OLD.attempt_id) THEN
    RAISE EXCEPTION 'answers of a submitted quiz attempt are immutable';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER quiz_answer_lock BEFORE UPDATE ON quiz_answers
  FOR EACH ROW EXECUTE FUNCTION quiz_answer_lock();
