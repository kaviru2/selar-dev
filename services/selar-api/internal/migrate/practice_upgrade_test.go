package migrate

import (
	"context"
	"testing"
)

func TestIntegrationPracticeUpgradePreservesFormalInstruments(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	cut := 0
	for i, file := range files {
		if file.Version <= "020" {
			cut = i + 1
		}
	}
	if _, err = Run(ctx, conn, files[:cut], noLog); err != nil {
		t.Fatal(err)
	}
	var owner, quiz, question, attempt string
	for _, x := range []struct {
		sql string
		out *string
	}{
		{`INSERT INTO users(email,password_hash) VALUES('formal-upgrade@example.invalid','synthetic') RETURNING id`, &owner},
		{`INSERT INTO quizzes(title,kind,status,settings) VALUES('Protected fixture','initial','published','{"feedback_policy":"never"}') RETURNING id`, &quiz},
	} {
		if err = conn.QueryRow(ctx, x.sql).Scan(x.out); err != nil {
			t.Fatal(err)
		}
	}
	if err = conn.QueryRow(ctx, `INSERT INTO quiz_questions(quiz_id,position,type,prompt,rubric) VALUES($1,0,'free_recall','Synthetic protected prompt','Protected key') RETURNING id`, quiz).Scan(&question); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `INSERT INTO quiz_attempts(quiz_id,user_id,attempt_number,quiz_version,question_order,pending_review) VALUES($1,$2,1,1,ARRAY[$3::uuid],1) RETURNING id`, quiz, owner, question).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO quiz_answers(attempt_id,question_id,answer_text,needs_review) VALUES($1,$2,'Synthetic formal recall',true)`, attempt, question); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `UPDATE quiz_attempts SET submitted_at=now(),submit_reason='learner' WHERE id=$1`, attempt); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var text string
		err := conn.QueryRow(ctx, `SELECT jsonb_build_object('quiz',(SELECT to_jsonb(q) FROM quizzes q WHERE id=$1),'question',(SELECT to_jsonb(q) FROM quiz_questions q WHERE id=$2),'attempt',(SELECT to_jsonb(a) FROM quiz_attempts a WHERE id=$3),'answers',(SELECT jsonb_agg(to_jsonb(a)) FROM quiz_answers a WHERE attempt_id=$3))::text`, quiz, question, attempt).Scan(&text)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	before := snapshot()
	upgrade, err := Run(ctx, conn, files, noLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(upgrade.Applied) != len(files)-cut {
		t.Fatal("upgrade did not apply practice migrations")
	}
	rerun, err := Run(ctx, conn, files, noLog)
	if err != nil || len(rerun.Applied) != 0 {
		t.Fatalf("rerun: %+v %v", rerun, err)
	}
	if snapshot() != before {
		t.Fatal("formal instruments or submitted recall changed during practice upgrade")
	}
}
