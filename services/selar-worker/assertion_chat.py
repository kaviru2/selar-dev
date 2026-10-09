"""Read-only display of one owner-selected saved claim; never entailment QA.

The sole accepted question is the exact canonical tuple shown by the picker.
No keyword/alias matching, model, extraction, identity inference or graph writes.
"""
import json
import time
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class AssertionSelection(BaseModel):
    model_config = ConfigDict(extra="forbid")
    asserting_document_id: UUID
    assertion_id: UUID
    revision: int = Field(strict=True, ge=1)


FIELDS = ("subject", "predicate", "object", "scope", "experiment_context",
          "subject_qualifier", "object_qualifier")


def saved_assertion_question(row):
    return "Show saved assertion: " + json.dumps(
        [row[key] for key in FIELDS], ensure_ascii=False, separators=(",", ":"))


ABSTENTION = ("I cannot answer this request from the selected saved assertion. "
              "Select a current confirmed assertion and use its exact display request; "
              "free-text factual questions, changed scope or variants, and original-source "
              "identity claims are not supported by this mode.")


def response(answer, citations, started):
    return {"answer": answer, "model_version": "deterministic-saved-assertion-v1",
            "ranking_policy": "owner-selected-saved-assertion-v1", "citations": citations,
            "candidates": [], "metrics": {"embedding_ms": 0, "retrieval_ms": 0,
                "generation_ms": 0, "total_ms": (time.perf_counter() - started) * 1000,
                "candidate_count": 0, "model_calls": 0}}


async def selected_assertion_answer(connect, database_url, user_id, question, selection, started):
    if selection is None:
        return response(ABSTENTION, [], started)
    # One statement snapshot: every stored witness must still belong to the
    # selected owner/document, match its complete content hash AND contain the
    # exact saved quote. Mere substring membership does not establish entailment.
    conn = await connect(database_url)
    try:
        rows = await conn.fetch("""
            SELECT a.id, a.revision, a.subject, a.predicate, a.object, a.scope,
                   a.experiment_context, a.subject_qualifier, a.object_qualifier,
                   a.asserting_document_id, d.title AS document_title,
                   e.chunk_id, e.quote, c.page_start AS page, c.locator, d.source_type
            FROM research_assertions a
            JOIN documents d ON d.id = a.asserting_document_id AND d.user_id = a.user_id
            JOIN research_assertion_evidence e ON e.assertion_id = a.id
            JOIN chunks c ON c.id = e.chunk_id AND c.user_id = a.user_id AND c.document_id = d.id
            WHERE a.user_id = $1 AND a.asserting_document_id = $2 AND a.id = $3
              AND a.revision = $4 AND a.state = 'confirmed' AND a.superseded_by IS NULL
              AND d.status = 'ready'
              AND NOT EXISTS (
                  SELECT 1 FROM research_assertion_evidence ev
                  LEFT JOIN chunks witness ON witness.id = ev.chunk_id
                  WHERE ev.assertion_id = a.id AND (
                      witness.id IS NULL OR witness.user_id <> a.user_id
                      OR witness.document_id <> d.id OR ev.quote = ''
                      OR ev.text_sha256 <> encode(digest(witness.content, 'sha256'), 'hex')
                      OR strpos(witness.content, ev.quote) = 0))
            ORDER BY e.chunk_id
        """, user_id, str(selection.asserting_document_id), str(selection.assertion_id), selection.revision)
    finally:
        await conn.close()
    citations = []
    answer = ABSTENTION
    if rows and question == saved_assertion_question(rows[0]):
        a = rows[0]
        # Render user-controlled values as a fenced JSON record, not Markdown
        # instructions, asserted prose, or model input. JSON escapes newlines.
        record = {key: a[key] for key in FIELDS}
        record.update(assertion_id=str(a["id"]), revision=a["revision"],
                      asserting_document_id=str(a["asserting_document_id"]),
                      asserting_document_title=a["document_title"])
        encoded = json.dumps(record, ensure_ascii=False, indent=2).replace("`", "\\u0060")
        answer = ("According to the selected document and saved assertion, the owner-confirmed "
                  "record is shown below. This displays a saved claim, not an independently "
                  "verified fact. Exact quote membership is not machine-verified entailment; "
                  "the quote may not support the claim. Document selection does not establish "
                  "original-source identity.\n\n```json\n" + encoded + "\n```\n\n"
                  "Saved source witnesses: " + " ".join(f"[S{i}]" for i in range(1, len(rows) + 1)))
        for i, row in enumerate(rows, 1):
            locator = row["locator"] or {}
            if isinstance(locator, str):
                locator = json.loads(locator)
            citations.append({"chunk_id": str(row["chunk_id"]),
                              "document_id": str(row["asserting_document_id"]),
                              "document_title": row["document_title"], "page": row["page"],
                              "rank": i, "score": 0, "quote": row["quote"],
                              "source_type": row["source_type"], "locator": locator})
    return response(answer, citations, started)
