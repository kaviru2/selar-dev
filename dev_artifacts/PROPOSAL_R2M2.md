# Proposal: Decoupled & Governed Learner Mental Model (R2M2)

## Problem

Currently, SELAR's concept extraction and linking model is static and batch-oriented. When a document is processed by `selar-worker`, concepts and relationships are extracted in a single LLM pass and saved. Link suggestions are computed using a static cosine similarity threshold.

This static approach introduces several limitations:
1.  **Memory Drift & Context Rot**: As the database grows, querying or feeding the entire concept graph to LLM prompts causes distraction and degrades context quality.
2.  **No Adaptation/Learning**: The system does not adapt based on user reactions to link suggestions (e.g., if a user rejects several suggestions, the threshold remains unchanged).
3.  **Lack of Real-Time Interaction**: User annotations, highlights, and quiz failures are not dynamically reflected back into the concept graph's edge weights or active nodes at runtime.
4.  **Conflating Domain Semantics with Learner State**: Treating the user's cognitive state (memory decay, focus, mistakes) and the library's domain factual graph as a single self-modifying system leads to model errors hardening into long-term facts, rendering the system difficult to audit.

---

## Proposed Solution (The Four-Layer Architecture)

We propose separating the mental model implementation into four distinct, decoupled layers:

```
+-----------------------------------------------------------------+
|                       Learner-State Overlay                     |
|  (Time-varying estimates: mastery, half-life, recall_prob)      |
+-------------------------------+---------------------------------+
                                | Schedules/Ranks
                                v
+-----------------------------------------------------------------+
|                         Semantic Graph                          |
|  (DocumentMentalModels: Claims, Assumptions, Concepts, Edges)   |
+-------------------------------+---------------------------------+
                                | Grounded by
                                v
+-----------------------------------------------------------------+
|                         Evidence Graph                          |
|  (Immutable: Documents -> Chunks -> Grounded Annotations)        |
+-----------------------------------------------------------------+

*All interactions and recommendation outcomes are logged to the Decision Trace.
```

### 1. Evidence Graph (Immutable Source of Truth)
*   Maintains strict provenance: `Document -> contains -> Chunk`, `Chunk -> mentions -> Concept`, and `Annotation -> grounds_to -> Chunk`.
*   Every node or edge in the semantic layer must point to its supporting chunks, creation source (human/model), and schema version.

### 2. Semantic Graph (Governed Domain Knowledge)
*   Contains the structured mental model of the library (`DocumentMentalModel`, `Claims`, `Assumptions`, `OpenQuestions`, `Concepts`).
*   Implements a strict candidate lifecycle: `candidate -> supported -> confirmed -> rejected -> archived`.
*   Newly induced concepts are marked as `candidate` and must pass deterministic safety, duplicate, and graduation checks before promotion to `confirmed` status.

### 3. Learner-State Overlay (Time-Varying Estimates)
*   Stores temporal cognitive state separately for each `(user, concept)` pair using a deterministic half-life regression decay model:
    $$P(\text{recall}) = 2^{-\frac{\Delta t}{h}}$$
*   **Events**: Reading a chunk or annotation boosts activation and adjusts half-life ($h$); quiz failures reduce it; passive page views provide little or no boost.

### 4. Decision Trace (Causal Lineage)
*   Logs the exact context of every recommendation: triggering event, candidate set, retrieved neighbourhood, applied policy version, user response, and retention outcome.
*   Enables auditable, explainable recommendations (e.g., *"Shown because you confirmed this concept 12 days ago, recall is estimated to be low..."*).

---

## Service Changes

### 1. Database Schema
*   **New table `learning_events`**: Immutable event ledger logging `chunk_exposed`, `annotation_created`, `note_created`, `suggestion_confirmed/rejected`, `quiz_answer_correct/incorrect`.
*   **New table `learner_concept_state`**: Stores `mastery_estimate`, `recall_probability`, `half_life_seconds`, and `last_exposed_at` for each concept/user.
*   **Updated `concepts` and `concept_edges`**: Add `state` column (`candidate`, `supported`, `confirmed`, `archived`) and foreign key references for chunk provenance.

### 2. Python Worker (`selar-worker`)
*   Implement bounded local induction: Instead of processing the whole graph, retrieve a top-k local neighbourhood from Postgres using pgvector, and send only the evidence packet to the LLM to propose candidates.

### 3. Go API (`selar-api`)
*   Implement a deterministic event reducer to consume `learning_events` and update `learner_concept_state`.
*   Incorporate predicted recall need (`1 - recall_probability`) and evidence quality directly into the SQL ranking query for link suggestions.

---

## Suggested Delivery Sequence

*   **Phase 0 — Correctness and Provenance**: Ground annotations to chunks, populate `chunk_concepts`, enforce authenticated ownership, and replace quadratic comparison with indexed nearest-neighbour searches.
*   **Phase 1 — Event Ledger and Deterministic Learner Model**: Implement the `learning_events` logger and the half-life decay reducer. Expose score components in a diagnostic view.
*   **Phase 2 — Adaptive Ranking in Shadow Mode**: Calculate suggestion rankings in the background without affecting the UI, validating predictions against later quiz outcomes.
*   **Phase 3 — Governed Runtime Expansion**: Trigger asynchronous candidate generation from high-intent notes/highlights using an outbox pattern.
*   **Phase 4 — Learned Memory & Retrieval Policies**: Fit Half-Life Regression coefficients from logged interaction data, comparing learned models against the deterministic baseline.
