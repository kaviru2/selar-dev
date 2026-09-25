"""Read-only, deterministic evaluation of frozen SELAR passage-link predictions.

This module never generates labels, ranks candidates, or writes to the corpus.
"""
from __future__ import annotations

import hashlib
import json
import argparse
from pathlib import Path


def _sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def validate_gold(gold: dict) -> tuple[dict, dict]:
    """Index immutable source snapshots and pair annotations."""
    policy = gold.get("annotation_policy", {})
    if not policy.get("reviewers") or not policy.get("adjudication"):
        raise ValueError("missing rubric reviewers or adjudication")
    documents = {doc["id"]: doc for doc in gold["documents"]}
    if len(documents) != len(gold["documents"]):
        raise ValueError("duplicate document ID")
    chunks = {chunk["id"]: (doc, chunk)
              for doc in gold["documents"] for chunk in doc["chunks"]}
    if len(chunks) != sum(len(doc["chunks"]) for doc in gold["documents"]):
        raise ValueError("duplicate chunk ID")
    for doc in gold["documents"]:
        for field in ("id", "ref", "title", "source_file"):
            if not doc.get(field):
                raise ValueError(f"missing source {field}")
        for chunk in doc["chunks"]:
            for field in ("id", "locator", "text"):
                if not chunk.get(field):
                    raise ValueError(f"missing chunk {field}")
    pairs = {pair["id"]: pair for pair in gold["pairs"]}
    if len(pairs) != len(gold["pairs"]):
        raise ValueError("duplicate pair ID")
    split_by_doc: dict[str, str] = {}
    directed_pairs: set[tuple[str, str]] = set()
    for pair in gold["pairs"]:
        directed = pair["query_id"], pair["candidate_id"]
        if directed in directed_pairs:
            raise ValueError("duplicate directed pair")
        directed_pairs.add(directed)
        split = pair.get("split")
        if split not in ("development", "held_out"):
            raise ValueError("invalid split")
        for doc_id in (pair["query_id"], pair["candidate_id"]):
            if doc_id not in documents:
                raise ValueError("unknown document ID")
            if doc_id in split_by_doc and split_by_doc[doc_id] != split:
                raise ValueError("pair crosses split boundary")
            split_by_doc[doc_id] = split
        for field in ("annotation_status", "proposed_adjudication_status",
                      "owner_authorized_status", "owner_authorized_relationship_type",
                      "owner_authorized_relevance"):
            if field not in pair or pair[field] is None or pair[field] == "":
                raise ValueError(f"missing {field}")
        label = pair["owner_authorized_relationship_type"]
        relevant = pair["owner_authorized_relevance"]
        if (relevant not in (0, 1) or (label == "none") != (relevant == 0)):
            raise ValueError("inconsistent authorized label/relevance")
        for side, doc_id in (("query", pair["query_id"]),
                             ("candidate", pair["candidate_id"])):
            ids = pair.get(f"owner_authorized_supporting_{side}_chunks")
            if not isinstance(ids, list) or (relevant == 1 and not ids):
                raise ValueError(f"missing authorized {side} support")
            for chunk_id in ids:
                if chunk_id not in chunks or chunks[chunk_id][0]["id"] != doc_id:
                    raise ValueError(f"invalid authorized {side} support")
    return chunks, pairs


def _evidence(chunks: dict, doc_id: str, evidence: dict | None) -> dict:
    if not isinstance(evidence, dict) or not isinstance(evidence.get("quote"), str) or not evidence.get("chunk_id"):
        raise ValueError("both evidence sides require chunk_id and quote")
    if evidence["chunk_id"] not in chunks:
        return {"chunk_id": evidence["chunk_id"], "quote": evidence["quote"],
                "supported": False, "reason": "unknown chunk"}
    doc, chunk = chunks[evidence["chunk_id"]]
    text = chunk["text"]
    return {"source_id": doc["id"], "source_ref": doc["ref"],
            "source_title": doc["title"], "source_file": doc["source_file"],
            "chunk_id": chunk["id"], "locator": chunk["locator"],
            "text_sha256": _sha(text.encode("utf-8")), "quote": evidence["quote"],
            "supported": doc["id"] == doc_id and bool(evidence["quote"])
            and evidence["quote"] in text}


def evaluate_frozen(path: str | Path, expected_sha256: str, predictions: list[dict],
                    *, split: str) -> dict:
    """Score only supplied predictions against a checksum-pinned split."""
    raw = Path(path).read_bytes()
    digest = _sha(raw)
    if digest != expected_sha256:
        raise ValueError("frozen gold SHA-256 mismatch")
    gold = json.loads(raw)
    chunks, pairs = validate_gold(gold)
    if split not in ("development", "held_out"):
        raise ValueError("invalid split")
    selected = {key: pair for key, pair in pairs.items() if pair["split"] == split}
    predicted = {row["pair_id"]: row for row in predictions}
    if set(predicted) != set(selected) or len(predicted) != len(predictions):
        raise ValueError("predictions must cover each selected pair exactly once")
    evidence = {}
    unsupported = 0
    predicted_links = 0
    for pair_id, row in predicted.items():
        label = row.get("label")
        if not isinstance(label, str) or not label.strip():
            raise ValueError("unknown prediction label")
        if label in ("none", "abstain"):
            continue
        predicted_links += 1
        pair = selected[pair_id]
        query = _evidence(chunks, pair["query_id"], row.get("query_evidence"))
        candidate = _evidence(chunks, pair["candidate_id"], row.get("candidate_evidence"))
        supported = query["supported"] and candidate["supported"]
        evidence[pair_id] = {"query": query, "candidate": candidate, "supported": supported}
        unsupported += not supported
    classes = {}
    for label in sorted({"none"} | {pair["owner_authorized_relationship_type"]
                                    for pair in selected.values()} |
                        {row["label"] for row in predicted.values() if row["label"] != "abstain"}):
        tp = sum(row["label"] == label and selected[key]["owner_authorized_relationship_type"] == label
                 for key, row in predicted.items())
        fp = sum(row["label"] == label and selected[key]["owner_authorized_relationship_type"] != label
                 for key, row in predicted.items())
        support = sum(pair["owner_authorized_relationship_type"] == label for pair in selected.values())
        fn = support - tp  # abstentions count as misses, not as a class
        precision = tp / (tp + fp) if tp + fp else 0.0
        recall = tp / support if support else 0.0
        classes[label] = {"tp": tp, "fp": fp, "fn": fn, "support": support,
                          "precision": precision, "recall": recall,
                          "f1": 2 * precision * recall / (precision + recall)
                          if precision + recall else 0.0}
    abstained = sum(row["label"] == "abstain" for row in predicted.values())
    counts = {"evaluated": len(selected), "decided": len(selected) - abstained,
              "abstained": abstained, "predicted_links": predicted_links,
              "unsupported_links": unsupported}
    return {"dataset_sha256": digest, "split": split,
            "dataset_id": gold["dataset_id"], "version": gold["version"],
            "counts": counts,
            "coverage": counts["decided"] / len(selected) if selected else None,
            "unsupported_link_rate": unsupported / predicted_links if predicted_links else None,
            "classes": classes,
            "annotation_provenance": {
                "rubric": gold["annotation_policy"],
                "owner_authorized": {"rows": len(selected),
                                     "authorization": gold.get("owner_authorization", {})},
                "initial_agent_rows": sum(bool(pair.get("annotation_status")) for pair in selected.values()),
                "agent_proposal_rows": sum(bool(pair.get("proposed_adjudication_status"))
                                           for pair in selected.values()),
                "independent_human_inter_rater_review": gold.get("owner_authorization", {}).get(
                    "row_by_row_independent_human_review", False)},
            "evidence": evidence}


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate frozen offline passage links")
    parser.add_argument("--gold", type=Path, required=True)
    parser.add_argument("--sha256", required=True, help="Expected SHA-256 of exact gold bytes")
    parser.add_argument("--predictions", type=Path, required=True)
    parser.add_argument("--split", choices=("development", "held_out"), required=True)
    args = parser.parse_args()
    predictions = json.loads(args.predictions.read_text(encoding="utf-8"))
    print(json.dumps(evaluate_frozen(args.gold, args.sha256, predictions, split=args.split),
                     indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
