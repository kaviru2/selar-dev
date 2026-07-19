"""Deterministic retrieval metrics for SELAR multimodal experiments.

Input rows are intentionally model-agnostic so a frozen gold corpus can compare
text, image, mixed, and fused retrieval runs without changing the evaluator.
"""

from __future__ import annotations

import math
from collections import defaultdict
from typing import Iterable


def recall_at_k(relevant: set[str], ranked: list[str], k: int) -> float:
    if not relevant:
        return 0.0
    return len(relevant.intersection(ranked[:k])) / len(relevant)


def reciprocal_rank(relevant: set[str], ranked: list[str]) -> float:
    for rank, item_id in enumerate(ranked, start=1):
        if item_id in relevant:
            return 1.0 / rank
    return 0.0


def ndcg_at_k(relevance: dict[str, float], ranked: list[str], k: int) -> float:
    def dcg(values: list[float]) -> float:
        return sum((2**value - 1) / math.log2(index + 2) for index, value in enumerate(values))

    observed = dcg([relevance.get(item_id, 0.0) for item_id in ranked[:k]])
    ideal = dcg(sorted(relevance.values(), reverse=True)[:k])
    return observed / ideal if ideal else 0.0


def evaluate(rows: Iterable[dict], k_values: tuple[int, ...] = (1, 5, 10)) -> dict:
    """Return macro metrics overall and stratified by answer modality."""
    groups: dict[str, list[dict]] = defaultdict(list)
    materialized = list(rows)
    groups["all"] = materialized
    for row in materialized:
        groups[str(row.get("answer_modality", "unknown"))].append(row)

    report: dict[str, dict[str, float | int]] = {}
    for group, items in groups.items():
        metrics: dict[str, float | int] = {"queries": len(items)}
        if not items:
            report[group] = metrics
            continue
        metrics["mrr"] = sum(
            reciprocal_rank(set(item["relevance"]), item["ranked_ids"]) for item in items
        ) / len(items)
        for k in k_values:
            metrics[f"recall@{k}"] = sum(
                recall_at_k(set(item["relevance"]), item["ranked_ids"], k) for item in items
            ) / len(items)
            metrics[f"ndcg@{k}"] = sum(
                ndcg_at_k(item["relevance"], item["ranked_ids"], k) for item in items
            ) / len(items)
        report[group] = metrics
    return report
