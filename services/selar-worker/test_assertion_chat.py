"""Saved-assertion display is not factual entailment or primary-source QA."""
import json
import uuid
import pytest

from fastapi.testclient import TestClient
import main

USER, DOC, ASSERTION, CHUNK = [str(uuid.uuid4()) for _ in range(4)]
FIELDS = ["modified SagaLLM", "evaluated_on", "τ²-bench", "reported_about_other",
          "RAC comparison run", "modified baseline", ""]
QUESTION = "Show saved assertion: " + json.dumps(FIELDS, ensure_ascii=False, separators=(",", ":"))


def selection():
    return {"asserting_document_id": DOC, "assertion_id": ASSERTION, "revision": 2}


def row():
    return dict(zip(("subject", "predicate", "object", "scope", "experiment_context",
                     "subject_qualifier", "object_qualifier"), FIELDS),
                id=ASSERTION, revision=2, asserting_document_id=DOC, document_title="RAC comparison",
                chunk_id=CHUNK, quote="We evaluate a modified SagaLLM on τ²-bench.",
                page=1, source_type="pdf", locator={"page": 1})


def test_selected_saved_assertion_display_without_model(monkeypatch):
    class Connection:
        async def fetch(self, sql, *args):
            assert args == (USER, DOC, ASSERTION, 2)
            return [row()]
        async def close(self):
            pass
    async def connect(*_):
        return Connection()
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "client", None)
    response = TestClient(main.app).post("/chat", json={
        "user_id": USER, "thread_id": str(uuid.uuid4()), "question": QUESTION,
        "assertion_selection": selection()})
    assert response.status_code == 200, response.text
    data = response.json()
    assert data["model_version"] == "deterministic-saved-assertion-v1"
    assert "According to the selected document and saved assertion" in data["answer"]
    assert "not machine-verified entailment" in data["answer"]
    assert all(value in data["answer"] for value in FIELDS if value)
    assert data["citations"][0]["quote"] == row()["quote"]
    assert data["citations"][0]["document_id"] == DOC
    assert data["metrics"]["model_calls"] == 0


def test_display_request_without_selection_never_falls_back_to_model(monkeypatch):
    monkeypatch.setattr(main, "client", None)
    response = TestClient(main.app).post("/chat", json={
        "user_id": USER, "thread_id": str(uuid.uuid4()), "question": QUESTION})
    assert response.status_code == 200
    assert response.json()["citations"] == []
    assert "cannot answer" in response.json()["answer"].lower()


@pytest.mark.parametrize("question", [
    "Did the original SagaLLM use τ²-bench?",
    "Did modified SagaLLM not use τ²-bench?",
    "Does this quote prove the saved claim?",
    QUESTION.replace("evaluated_on", "uses_benchmark"),
    QUESTION.replace("modified SagaLLM", "original SagaLLM"),
    QUESTION.replace("reported_about_other", "own_work"),
    QUESTION.replace("τ²-bench", "TAU Benchmark"),
    QUESTION.replace("RAC comparison run", "original experiment"),
    QUESTION.replace("modified baseline", "original implementation"),
    QUESTION[:-3] + '"other qualifier"]',
    QUESTION + " Is that true?",
])
def test_exact_tuple_not_keywords_or_quote_membership(monkeypatch, question):
    class Connection:
        async def fetch(self, *_):
            # An unrelated exact witness must NEVER enable a factual answer.
            return [{**row(), "quote": "The appendix discusses unrelated boats."}]
        async def close(self):
            pass
    async def connect(*_):
        return Connection()
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "client", None)
    result = TestClient(main.app).post("/chat", json={
        "user_id": USER, "thread_id": str(uuid.uuid4()), "question": question,
        "assertion_selection": selection()})
    assert result.status_code == 200
    assert result.json()["citations"] == []
    assert "cannot answer" in result.json()["answer"].lower()
