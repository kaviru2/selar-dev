"""Synthetic-only offline evidence tests; no owner gold labels are copied here."""
import hashlib
import json
import subprocess
import sys

import pytest

from offline_evidence import evaluate_frozen, validate_gold


def fixture():
    docs = [
        {"id": "D1", "ref": "P1", "title": "One", "source_file": "one.pdf",
         "chunks": [{"id": "C1", "locator": "page 1", "text": "Alpha supports beta."}]},
        {"id": "D2", "ref": "P2", "title": "Two", "source_file": "two.pdf",
         "chunks": [{"id": "C2", "locator": "page 2", "text": "Beta follows alpha."}]},
        {"id": "D3", "ref": "P3", "title": "Three", "source_file": "three.pdf",
         "chunks": [{"id": "C3", "locator": "page 3", "text": "Gamma is distinct."}]},
        {"id": "D4", "ref": "P4", "title": "Four", "source_file": "four.pdf",
         "chunks": [{"id": "C4", "locator": "page 4", "text": "Delta is separate."}]},
    ]
    def pair(id, q, c, split, label, relevant):
        return {"id": id, "query_id": q, "candidate_id": c, "split": split,
                "relevance": relevant, "relationship_type": label,
                "supporting_query_chunks": ["C1"] if relevant and q == "D1" else [],
                "supporting_candidate_chunks": ["C2"] if relevant and c == "D2" else [],
                "annotation_status": "initial agent annotation",
                "proposed_adjudication_status": "agent recommendation",
                "owner_authorized_status": "accepted; not independent review",
                "owner_authorized_relevance": relevant,
                "owner_authorized_relationship_type": label,
                "owner_authorized_supporting_query_chunks": ["C1"] if relevant and q == "D1" else [],
                "owner_authorized_supporting_candidate_chunks": ["C2"] if relevant and c == "D2" else []}
    gold = {"dataset_id": "synthetic", "version": "v1", "documents": docs,
            "annotation_policy": {"reviewers": ["agent"], "adjudication": "owner-authorized"},
            "owner_authorization": {"row_by_row_independent_human_review": False},
            "pairs": [pair("L1", "D1", "D2", "development", "concept_overlap", 1),
                      pair("L2", "D2", "D1", "development", "none", 0),
                      pair("L3", "D3", "D4", "held_out", "none", 0)]}
    return gold


def frozen(tmp_path, gold):
    raw = json.dumps(gold).encode()
    path = tmp_path / "frozen.json"
    path.write_bytes(raw)
    return path, hashlib.sha256(raw).hexdigest()


def test_exact_two_sided_evidence_and_provenance(tmp_path):
    path, digest = frozen(tmp_path, fixture())
    predictions = [{"pair_id": "L1", "label": "concept_overlap",
                    "query_evidence": {"chunk_id": "C1", "quote": "Alpha supports beta."},
                    "candidate_evidence": {"chunk_id": "C2", "quote": "Beta follows alpha."}},
                   {"pair_id": "L2", "label": "none"}]
    report = evaluate_frozen(path, digest, predictions, split="development")
    assert report["dataset_sha256"] == digest
    assert report["counts"]["evaluated"] == 2
    assert report["counts"]["unsupported_links"] == 0
    evidence = report["evidence"]["L1"]
    assert evidence["supported"] is True
    assert evidence["query"]["source_ref"] == "P1"
    assert evidence["candidate"]["locator"] == "page 2"
    assert evidence["query"]["text_sha256"] == hashlib.sha256(b"Alpha supports beta.").hexdigest()


def test_rejects_leakage_duplicate_ids_and_missing_provenance(tmp_path):
    gold = fixture()
    gold["pairs"][2]["query_id"] = "D1"  # held-out touches development
    with pytest.raises(ValueError, match="crosses split"):
        validate_gold(gold)
    gold = fixture()
    gold["documents"][1]["chunks"][0]["id"] = "C1"
    with pytest.raises(ValueError, match="duplicate chunk"):
        validate_gold(gold)
    gold = fixture()
    del gold["pairs"][0]["owner_authorized_status"]
    with pytest.raises(ValueError, match="owner_authorized_status"):
        validate_gold(gold)


def test_checksum_and_prediction_coverage_fail_closed(tmp_path):
    path, digest = frozen(tmp_path, fixture())
    with pytest.raises(ValueError, match="SHA-256"):
        evaluate_frozen(path, "0" * 64, [], split="development")
    with pytest.raises(ValueError, match="exactly once"):
        evaluate_frozen(path, digest, [{"pair_id": "L1", "label": "none"}], split="development")


def test_per_class_abstention_and_unsupported_denominators(tmp_path):
    path, digest = frozen(tmp_path, fixture())
    report = evaluate_frozen(path, digest, [
        {"pair_id": "L1", "label": "concept_overlap",
         "query_evidence": {"chunk_id": "C1", "quote": "Alpha supports beta."},
         "candidate_evidence": {"chunk_id": "C2", "quote": "fabricated"}},
        {"pair_id": "L2", "label": "abstain"},
    ], split="development")
    assert report["counts"] == {"evaluated": 2, "decided": 1, "abstained": 1,
                                "predicted_links": 1, "unsupported_links": 1}
    assert report["coverage"] == 0.5
    assert report["unsupported_link_rate"] == 1.0
    assert report["classes"]["concept_overlap"] == {
        "tp": 1, "fp": 0, "fn": 0, "support": 1,
        "precision": 1.0, "recall": 1.0, "f1": 1.0}
    assert report["classes"]["none"]["support"] == 1
    assert report["classes"]["none"]["fn"] == 1
    assert report["classes"]["none"]["recall"] == 0.0


def test_missing_quote_wrong_source_and_empty_predictions(tmp_path):
    path, digest = frozen(tmp_path, fixture())
    with pytest.raises(ValueError, match="evidence"):
        evaluate_frozen(path, digest, [{"pair_id": "L1", "label": "concept_overlap"},
                                       {"pair_id": "L2", "label": "none"}], split="development")
    report = evaluate_frozen(path, digest, [
        {"pair_id": "L1", "label": "concept_overlap",
         "query_evidence": {"chunk_id": "C2", "quote": "Beta follows alpha."},
         "candidate_evidence": {"chunk_id": "C1", "quote": "Alpha supports beta."}},
        {"pair_id": "L2", "label": "none"}], split="development")
    assert report["counts"]["unsupported_links"] == 1
    assert report["evidence"]["L1"]["supported"] is False
    empty = evaluate_frozen(path, digest, [{"pair_id": "L3", "label": "abstain"}],
                            split="held_out")
    assert empty["unsupported_link_rate"] is None
    assert empty["coverage"] == 0.0


def test_authorized_labels_not_agent_proposals(tmp_path):
    gold = fixture()
    gold["pairs"][0]["proposed_relationship_type"] = "none"
    gold["pairs"][0]["proposed_relevance"] = 0
    path, digest = frozen(tmp_path, gold)
    report = evaluate_frozen(path, digest, [
        {"pair_id": "L1", "label": "none"}, {"pair_id": "L2", "label": "none"}],
        split="development")
    assert report["classes"]["concept_overlap"]["fn"] == 1
    assert report["annotation_provenance"]["owner_authorized"]["rows"] == 2
    assert report["annotation_provenance"]["independent_human_inter_rater_review"] is False


def test_development_report_does_not_reveal_heldout_class(tmp_path):
    gold = fixture()
    gold["pairs"][2]["owner_authorized_relationship_type"] = "heldout_only"
    gold["pairs"][2]["owner_authorized_relevance"] = 1
    gold["pairs"][2]["owner_authorized_supporting_query_chunks"] = ["C3"]
    gold["pairs"][2]["owner_authorized_supporting_candidate_chunks"] = ["C4"]
    path, digest = frozen(tmp_path, gold)
    result = evaluate_frozen(path, digest, [{"pair_id": "L1", "label": "none"},
                                           {"pair_id": "L2", "label": "none"}],
                             split="development")
    assert "heldout_only" not in result["classes"]


def test_cli_reads_predictions_without_writing_gold(tmp_path):
    path, digest = frozen(tmp_path, fixture())
    predictions = tmp_path / "predictions.json"
    predictions.write_text(json.dumps([{"pair_id": "L1", "label": "abstain"},
                                       {"pair_id": "L2", "label": "none"}]))
    before = path.read_bytes()
    result = subprocess.run([sys.executable, "-m", "offline_evidence", "--gold", str(path),
                             "--sha256", digest, "--predictions", str(predictions),
                             "--split", "development"], capture_output=True, text=True)
    assert result.returncode == 0, result.stderr
    assert json.loads(result.stdout)["counts"]["abstained"] == 1
    assert path.read_bytes() == before


def test_duplicate_directed_pair_and_missing_rubric_rejected():
    gold = fixture()
    gold["pairs"][1]["query_id"] = "D1"
    gold["pairs"][1]["candidate_id"] = "D2"
    with pytest.raises(ValueError, match="duplicate directed pair"):
        validate_gold(gold)
    gold = fixture()
    gold["annotation_policy"].pop("reviewers")
    with pytest.raises(ValueError, match="reviewers"):
        validate_gold(gold)
