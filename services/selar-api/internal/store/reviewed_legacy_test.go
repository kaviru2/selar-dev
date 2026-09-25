package store

import "testing"

func TestGroundedReviewQuarantinesLegacyMentalConceptEdges(t *testing.T) {
	s, ctx, owner, _, id, _, cleanup := reviewFixture(t)
	defer cleanup()
	q := func(sql string, args ...any) string {
		t.Helper()
		var result string
		if err := s.pool.QueryRow(ctx, sql, args...).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	a := q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'legacy source','confirmed') RETURNING id`, owner)
	b := q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'legacy target','confirmed') RETURNING id`, owner)
	if _, err := s.pool.Exec(ctx, `INSERT INTO chunk_concepts(chunk_id,concept_id) SELECT source_evidence_chunk_id,$2 FROM mental_model_links WHERE id=$1`, id, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO chunk_concepts(chunk_id,concept_id) SELECT target_evidence_chunk_id,$2 FROM mental_model_links WHERE id=$1`, id, b); err != nil {
		t.Fatal(err)
	}
	edge := q(`INSERT INTO concept_edges(user_id,source_concept_id,target_concept_id,relation,created_via,state) VALUES ($1,$2,$3,'related_to','user_confirmed','confirmed') RETURNING id`, owner, a, b)
	if _, err := s.pool.Exec(ctx, `INSERT INTO learning_events(user_id,event_type,mental_link_id,concept_edge_id,idempotency_key) VALUES ($1,'mental_link_graph_confirmed',$2,$3,'legacy:'||($3::uuid)::text)`, owner, id, edge); err != nil {
		t.Fatal(err)
	}
	edges, err := s.ListConceptEdges(ctx, owner)
	if err != nil || len(edges) != 0 {
		t.Fatalf("legacy ungoverned edge still projected: %+v %v", edges, err)
	}
}
