---
name: "Proposal: Decoupled Learner Mental Model (R2M2)"
about: Architectural blueprint for four-layer evidence, semantic, learner-state overlay, and decision trace memory.
title: "[Proposal] Decoupled Learner Mental Model (R2M2)"
labels: architectural-change, enhancement
assignees: ''
---

# Exploration Report: Reactive, Deterministic, Runtime-Expanding Mental Models for SELAR

**Scope:** Research and architecture exploration only. This report does not propose creating a GitHub issue or implementing the system immediately.

## Executive conclusion

The idea is a strong fit for SELAR, but the best version is **not one self-modifying knowledge graph**. SELAR should separate four concerns:

1. **Evidence graph** — immutable links from documents to chunks and annotations.
2. **Semantic graph** — concepts and evidence-backed relationships found in the library.
3. **Learner-state overlay** — a deterministic, time-varying estimate of what the user is likely to remember or misunderstand.
4. **Decision trace** — why a suggestion was shown, how the user reacted, and what changed afterward.

The semantic graph should represent claims about the corpus. The learner-state overlay should represent claims about the user. Mixing them would make the graph difficult to audit and could cause temporary reading behaviour or model errors to become long-term “facts.”

SELAR already stores most of the raw signals needed for an initial version: chunks, embeddings, concepts, edges, annotations, reading sessions, link responses, and quiz-oriented tables. The missing layer is a reliable event-to-state pipeline that connects those signals. The recommended first milestone is therefore a **deterministic, event-sourced learner model with governed graph expansion**, not reinforcement learning or online fine-tuning.

## 1. Project-first interpretation

The SELAR research proposal defines the core intervention more precisely than a general-purpose agent-memory system:

> Build a structured mental model of each reading, compare it with the learner's prior library, and surface argument-level connections that require the learner to perform elaborative encoding.

The proposed article mental model contains:

- `main_claim`
- `key_concepts`
- `assumptions`
- `open_questions`
- `domain`
- `model_vector`

Its experimental link types are:

- concept overlap;
- claim extension;
- assumption conflict; and
- question resolution.

This should remain the center of the architecture. Agent-memory research is useful insofar as it helps SELAR maintain, retrieve, and expand these mental models over a growing personal library. It should not turn SELAR into a general autonomous agent or replace the learner with an answer-generating RAG chatbot.

### 1.1 Gap between the research proposal and the current implementation

| Research construct | Current implementation | Required adaptation |
|---|---|---|
| Structured article mental model | No persisted article-level model | Add a versioned `document_mental_models` representation |
| Main claim | Not extracted as a first-class object | Store claim text, embedding, and supporting chunks |
| Assumptions | Not extracted | Store assumption nodes with evidence and confidence |
| Open questions | Not extracted | Store question nodes that can later be resolved by another document |
| Domain | Not extracted | Add controlled or normalised domain metadata |
| Key concepts | Extracted as global concept nodes | Add chunk provenance, entity resolution, and model versioning |
| Concept overlap | Collapsed into `related_to` | Preserve the experimental link type explicitly |
| Claim extension | Partially represented as `extends` | Link claims or document models, not only concepts |
| Assumption conflict | Partially represented as `contradicts` | Record the two assumptions and cited passages |
| Question resolution | Not represented | Add a relationship from an open question to a resolving claim/passage |
| Two-stage comparison | Chunk similarity plus a separate graph pass | Retrieve prior mental models first, then classify bounded model pairs |
| Human-in-the-loop surfacing | Link confirmation exists | Promotion must update the governed semantic graph and decision trace |

The present concept graph is therefore a useful substrate, not yet the mental-model implementation described in the study.

## 2. What the idea means for SELAR

The original idea combines four mechanisms that should be designed independently:

| Mechanism | Meaning in SELAR | Recommended role |
|---|---|---|
| Knowledge graph | Concepts, relationships, chunks, documents, and provenance | Stable, inspectable representation of the library |
| Deterministic algorithm | Explicit rules for decay, evidence strength, graduation, ranking, and pruning | Safe first implementation and experimental baseline |
| Reactive system | User events update learner state and enqueue bounded local analysis | Near-real-time adaptation without reprocessing the full library |
| Runtime expansion | New candidate concepts and edges can appear while the user reads | Controlled candidate pipeline, never direct promotion to trusted knowledge |

This produces a useful product loop:

```text
User action
   -> immutable learning event
   -> deterministic state update
   -> bounded retrieval from the local subgraph
   -> optional LLM proposal
   -> evidence and policy checks
   -> candidate suggestion
   -> user response
   -> new event and updated learner state
```

The loop is reactive, but its important transitions remain explainable and replayable.

## 3. What SELAR currently has

### 3.1 Existing foundations

| Existing capability | Evidence in the repository | Relevance |
|---|---|---|
| Document chunks with 3,072-dimensional embeddings | `chunks` in `services/selar-api/internal/store/migrations/001_init.sql` | Semantic retrieval substrate |
| Cross-document link suggestions and user responses | `link_suggestions`; `RespondToSuggestion` in the Go store and handler | Explicit positive/negative feedback |
| Concepts and typed directed edges | `concepts`, `chunk_concepts`, and `concept_edges` | Lightweight knowledge graph in PostgreSQL |
| Highlights and notes | `annotations` and the Reader UI | High-intent interaction signals |
| Reading sessions | `reading_sessions` and session API methods | Exposure and recency signals |
| Quiz-oriented data model | `quizzes`, `quiz_questions`, `quiz_attempts`, `quiz_responses` | Potential recall labels |
| Cohort assignment | `users.cohort` | Controlled evaluation support |
| Graph UI | `services/selar-console/app/(app)/graph/page.tsx` | A place to expose graph state and explanations |

### 3.2 Current ingestion behaviour

The worker currently:

1. chunks a PDF in groups of roughly 200 words;
2. embeds each chunk;
3. creates up to 20 cross-document suggestions in each direction using a fixed cosine-distance threshold of `0.35` (equivalent to similarity above roughly `0.65`);
4. asks the text model to classify pending link relations;
5. sends up to 80,000 characters of a document to the model to extract 5–8 concepts and edges; and
6. compares every concept pair and creates `related_to` edges above similarity `0.75`.

This is a useful batch graph builder, but it is not yet a mental model. Graph production happens during ingestion and does not incorporate later reading, annotation, suggestion, or quiz events.

### 3.3 Important gaps found in the implementation

1. **Concept provenance is declared but not populated.** The `chunk_concepts` table exists, but the worker never inserts into it. A concept therefore cannot currently be traced back to the exact passages that support it.
2. **Feedback does not update the graph.** Confirming, rejecting, or relabelling a link only changes the `link_suggestions` row. It does not create, strengthen, weaken, or challenge a concept edge.
3. **The graph does not distinguish proposals from trusted knowledge.** AI-created edges are returned by the graph API even when `confirmed_at` is empty. There is no candidate/graduated lifecycle.
4. **Signals are stored in separate tables rather than a unified event stream.** No deterministic reducer converts them into current per-user, per-concept state.
5. **Quiz data is not yet a real signal source.** The quiz page is static UI and the API exposes no quiz workflow, so the strongest possible memory label is currently unavailable.
6. **Annotations are not reliably attached to chunks.** The Reader creates annotations with `chunk_id: null`, which prevents a highlight or note from updating a specific concept without a later grounding step.
7. **Concept identity is brittle.** Uniqueness is based on exact `(user_id, name)` text. Capitalisation, abbreviations, synonyms, or model wording changes can create duplicate semantic nodes.
8. **Cross-concept linking is quadratic.** The worker loads all concepts and compares pairs in Python. This will become expensive as a user's graph grows.
9. **Background execution is not durable.** FastAPI `BackgroundTasks` and a fire-and-forget Go request can lose work during restarts and provide no idempotency or retry guarantees.
10. **Some ownership boundaries need tightening before adding persistent memory.** Several update/read paths identify a row by ID without also constraining it by the authenticated user. Persistent learner state raises the cost of such mistakes.
11. **The current full-document concept pass can drift.** It truncates by character count and asks the model to induce a graph from a large unstructured input. This is precisely the relational setting in which long-context performance can degrade.

## 4. SELAR-adapted dynamic RAG architecture

SELAR needs two connected retrieval pipelines, not one generic RAG pipeline.

### 4.1 Pipeline A: document-to-library mental-model linking

Run when a new document is ready:

```text
PDF
 -> structure-aware chunks
 -> per-section evidence extraction
 -> article mental model
 -> mental-model embedding
 -> top-k prior mental models via pgvector
 -> deterministic eligibility and diversity filters
 -> LLM pair classification using both schemas and cited passages
 -> link candidates
 -> read-time user confirmation
 -> confirmed mental-model graph
```

The LLM should receive a small pair of structured mental models and their best supporting passages, not two entire documents or the full user graph. This keeps the comparison close to the research construct and makes the bridge explanation evidence-grounded.

### 4.2 Pipeline B: reading-time local graph expansion

Run only for high-intent events such as a substantive note, relabel, explicit link, or question:

```text
event text + grounded chunk
 -> embedding
 -> top-k passages and mental-model elements
 -> one/two-hop confirmed neighbourhood
 -> bounded candidate induction
 -> duplicate, provenance, and policy checks
 -> temporary candidate
```

This pipeline expands only the relevant local neighbourhood. It must not rebuild a document model or compare every graph node after each interaction.

### 4.3 Event-specific behaviour

| Trigger | Fast deterministic action | Optional background action | Should the trusted graph expand? |
|---|---|---|---|
| `new_document` | Persist chunks and enqueue model extraction | Build mental model; retrieve and classify top-k prior models | Candidates only |
| `highlight_created` | Ground to chunks; record exposure | Usually none; induce only if the selection is substantive | Normally no |
| `note_created` | Ground note and update activity state | Search local graph and propose claim/concept/question links | Candidates only |
| `link_confirmed` | Promote candidate and log feedback | Refresh affected projections/caches | Yes |
| `link_rejected` | Archive candidate and record negative evidence | Update ranking statistics | No |
| `link_relabeled` | Create a versioned corrected relationship | Re-score similar candidates if policy permits | Yes |
| `quiz_failed` | Update learner-state overlay | Retrieve supporting evidence for later review | No factual expansion |
| `weekly_summary` | Rank eligible confirmed items | Generate a bounded review set | No; retrieval only |

### 4.4 Efficient storage choice for the prototype

The project proposal mentions Qdrant plus Neo4j. For SELAR's DSR prototype, the current PostgreSQL + pgvector architecture is a better default:

- one transactional boundary for users, evidence, candidates, feedback, and study data;
- no cross-database consistency problem during candidate promotion;
- vector pre-filtering through pgvector;
- graph traversal through indexed joins and bounded recursive CTEs; and
- substantially lower deployment and operational complexity.

Qdrant becomes justified if measured vector volume, filtering, or latency exceeds pgvector's demonstrated capacity. Neo4j becomes justified if product requirements depend on deep, irregular traversals that are measurably awkward or slow in PostgreSQL. Neither should be introduced only because the conceptual model is a graph.

### 4.5 Efficient incremental expansion techniques

1. **Index before comparing.** Enable an HNSW index for chunk and mental-model embeddings once representative data is available; never scan all prior pairs in application code.
2. **Retrieve mental models before passages.** First select a small set of prior documents/models, then retrieve evidence passages inside those candidates.
3. **Use top-k budgets per stage.** For example, retrieve tens of model candidates, deterministically reduce them, and send only a small handful to semantic classification. Exact values must be tuned in the relevance pilot.
4. **Mix vector and lexical retrieval.** Technical concepts, named doctrines, quotations, and abbreviations benefit from exact or full-text matching in addition to embeddings.
5. **Resolve entities before inserting.** Normalise names and compare a candidate concept against nearby existing nodes before creating a new node.
6. **Cache projections, not truth.** Cache active neighbourhoods and bridge packets using graph/model version keys; retain PostgreSQL evidence as the source of truth.
7. **Re-embed selectively.** Store embedding model and dimension metadata. When models change, migrate in the background rather than blocking the library.
8. **Use an outbox-backed worker.** Database transactions should record jobs; workers claim, retry, and complete them idempotently.
9. **Bound graph traversal.** Most reading-time retrieval should stop at one or two hops and enforce a token/evidence budget.
10. **Separate active views from archived history.** Queries can exclude rejected or superseded candidates without deleting them.

### 4.6 Proposed graph vocabulary

The graph should grow beyond concept-only nodes while staying deliberately small:

```text
DocumentMentalModel
  -[HAS_CLAIM]-> Claim
  -[HAS_ASSUMPTION]-> Assumption
  -[RAISES]-> OpenQuestion
  -[USES_CONCEPT]-> Concept

Claim -[EXTENDS]-> Claim
Assumption -[CONFLICTS_WITH]-> Assumption
Claim -[RESOLVES]-> OpenQuestion
Concept -[OVERLAPS_WITH]-> Concept

Every semantic node -[SUPPORTED_BY]-> Chunk
```

The database can implement these as typed relational tables rather than a property-graph engine. For the retention study, it is especially important that the four experimental relationship types remain distinguishable in logs and outcomes.

## 5. Recommended target model

### 5.1 Layer A: evidence graph

This layer is the source of truth for provenance:

```text
Document -> contains -> Chunk
Chunk -> mentions -> Concept
Annotation -> grounds_to -> Chunk
Quiz question -> tests -> Concept
Suggestion -> supported_by -> Source chunk + Target chunk
```

Every induced concept or edge should retain its source chunks, extraction method, model version, prompt/version identifier, confidence, and creation time. Evidence should be append-only where practical. Corrections should supersede earlier claims rather than silently rewrite history.

### 5.2 Layer B: semantic graph

The semantic graph remains the user's map of the literature:

```text
Concept -[prerequisite_of|sub_concept_of|contradicts|extends|related_to]-> Concept
```

Recommended lifecycle for both nodes and edges:

```text
candidate -> supported -> confirmed
    |            |           |
    +--------> rejected <-----+
                  |
               archived
```

- **Candidate:** proposed by a model or heuristic, visible only as a suggestion.
- **Supported:** has sufficient independent passage evidence but is not user-confirmed.
- **Confirmed:** explicitly accepted by the user or admitted by a conservative policy.
- **Rejected:** contradicted or declined; retained for audit and negative learning.
- **Archived:** no longer active, but not physically deleted.

This lifecycle is more appropriate than deleting low-activation concepts. A concept can be educationally inactive yet remain factually valid.

### 5.3 Layer C: learner-state overlay

Store learner state separately for each `(user, concept)` pair. A minimal state could include:

| Field | Purpose |
|---|---|
| `mastery_estimate` | Probability that the concept is understood |
| `recall_probability` | Estimated probability of recall at the current time |
| `half_life_seconds` | How quickly recall is expected to decay |
| `last_exposed_at` | Most recent grounded exposure |
| `last_retrieved_at` | Most recent successful active recall |
| `success_count`, `failure_count` | Simple interpretable history |
| `evidence_count` | Number of grounded observations supporting the estimate |
| `uncertainty` | Prevents sparse data from looking certain |
| `state_version` | Makes reducer changes replayable |

For the deterministic baseline, use a half-life model rather than treating a raw “activation” score as memory itself:

```text
recall_probability = 2 ^ (-elapsed_time / half_life)
```

The half-life changes after meaningful events. A correct free-recall answer should increase it substantially; an incorrect answer should reduce it; a passive page view should provide little or no increase; a confirmed cross-paper relationship may provide a modest increase. The exact coefficients must be hypotheses recorded as versioned configuration, not hidden constants.

This matches the direction of Half-Life Regression, which models recall probability using time since practice and a learned concept/user feature vector. The deterministic baseline can later be upgraded by fitting its coefficients from SELAR's own event history.

### 5.4 Layer D: decision traces

For each generated recommendation, record:

- triggering event;
- candidate set and retrieval scores;
- graph neighbourhood used;
- applied policy and policy version;
- model and prompt version, if an LLM was used;
- final reason code;
- user response; and
- downstream quiz or retention outcome.

This is the practical version of “reasoning memory.” It should store compact causal lineage, not unrestricted chain-of-thought. The goal is to explain and evaluate system decisions.

## 6. Event model and reactive processing

Introduce an immutable `learning_events` table or equivalent event log. Suggested fields:

```text
id, user_id, event_type, occurred_at,
document_id, chunk_id, concept_id, suggestion_id,
payload, source, schema_version, idempotency_key
```

Useful event types include:

- `chunk_exposed`
- `annotation_created`
- `note_created`
- `suggestion_confirmed`
- `suggestion_rejected`
- `suggestion_relabeled`
- `quiz_answer_correct`
- `quiz_answer_incorrect`
- `connection_explained`
- `concept_candidate_created`
- `concept_candidate_confirmed`

A deterministic reducer consumes events and updates materialized `learner_concept_state`. A separate durable job handles slower LLM induction. This creates two latency paths:

1. **Synchronous:** write the event and update simple learner state.
2. **Asynchronous:** ground text, retrieve a bounded neighbourhood, induce candidates, and run governance checks.

At-least-once delivery is sufficient if every operation has an idempotency key and reducers are safe to replay.

## 7. Runtime graph expansion

### 7.1 Bounded local induction

Do not ask the model to rebuild or inspect the whole graph after each event. For a note or highlight:

1. ground the selected text to one or more chunk IDs;
2. embed the selected text;
3. retrieve a small set of semantically close chunks and concepts;
4. expand one or two graph hops using deterministic SQL;
5. send only that evidence packet to the LLM;
6. require structured candidate output with cited chunk IDs;
7. validate relation types, ownership, duplicates, and contradictions; and
8. save candidates without promoting them automatically.

The LLM proposes semantics; code controls state transitions.

### 7.2 Candidate graduation policy

A first deterministic policy might require:

- at least one valid source passage for a concept;
- two distinct supporting passages or explicit user confirmation for an edge;
- no unresolved contradiction with a confirmed edge;
- similarity/entity-resolution checks against existing nodes;
- a confidence threshold calibrated on a labelled sample; and
- provenance and model/version metadata.

“Activation” should affect ranking and review priority, not factual validity. Low-use knowledge can be hidden from the active view but should not be treated as false or deleted.

### 7.3 Context tiers

The three-tier pattern from the notes is useful if adapted to SELAR:

| Tier | SELAR representation | Retention policy |
|---|---|---|
| Stable knowledge | Confirmed evidence and semantic graph | Long-lived, versioned, auditable |
| Working context | Current document, session, recent events, local graph neighbourhood | Short-lived and bounded |
| Decision memory | Recommendation lineage and outcomes | Compact, append-only, retained for evaluation |

This is a logical separation; it does not require Neo4j or three physical databases. PostgreSQL, pgvector, recursive queries, and well-designed tables are adequate for the first several versions.

## 8. Deterministic ranking before learned ranking

Use an explicit scoring function to rank interventions, while keeping semantic retrieval and learning need separate. An illustrative starting point is:

```text
priority =
    semantic_relevance
  * evidence_quality
  * (1 - recall_probability)
  * diversity_factor
  * policy_eligibility
```

Where:

- `semantic_relevance` comes from vector and lexical retrieval;
- `evidence_quality` reflects provenance and candidate state;
- `1 - recall_probability` represents learning need;
- `diversity_factor` prevents repeated presentation of the same neighbourhood; and
- `policy_eligibility` is zero for unsafe, stale, cross-user, or experimentally disallowed items.

Avoid adding these inputs into one opaque “heat” value. Keeping the components visible makes ranking debuggable and allows offline replay.

Hybrid retrieval can combine:

1. vector similarity;
2. exact/lexical matches for technical names;
3. graph distance and relation type;
4. evidence quality;
5. learner need; and
6. reciprocal-rank fusion or another deterministic rank merger.

## 9. Predicting whether knowledge is retained

The proposed “model to predict if data is kept in memory” is feasible, but it needs a precise target.

### 9.1 Define the prediction target

Recommended target:

> Given the user's history for concept `c` up to time `t`, predict whether the user will correctly retrieve or apply `c` in a future assessment after delay `d`.

This is different from predicting clicks, reading time, or link confirmation. Those are useful features but weak labels for retention.

### 9.2 Labels

Strong labels:

- delayed quiz correctness;
- free-recall or short-answer correctness;
- correctly explaining a relationship between two concepts;
- transferring a concept to a new paper or problem.

Weak labels:

- confirming a semantic link;
- writing a grounded note;
- returning to a passage without prompting.

Very weak signals that should not be treated as mastery:

- a page entering the viewport;
- scroll depth alone;
- dwell time alone;
- accepting a suggestion without an active-recall task.

### 9.3 Recommended model progression

1. **Versioned heuristic baseline:** fixed half-life updates by event type.
2. **Half-Life Regression:** fit interpretable coefficients for recall probability.
3. **Time-aware knowledge tracing:** only when enough per-concept assessment sequences exist.
4. **Learned retrieval/ranking policy:** train from logged outcomes after counterfactual-safe data collection.
5. **RL or trainable graph memory:** consider only if offline replay and controlled experiments show a clear need.

Do not start with a deep model. SELAR's early dataset will be sparse, user-specific, and affected by the product's own recommendation policy. A simple calibrated model is more likely to be trustworthy.

### 9.4 Evaluation

Prediction metrics:

- log loss;
- Brier score;
- calibration error and reliability plots;
- AUROC as a secondary ranking metric; and
- performance by user, concept frequency, and time delay.

Product and learning outcomes:

- delayed-test retention;
- quality of cross-document explanations;
- suggestion acceptance and rejection rates;
- repeated-suggestion burden;
- time-to-useful-connection; and
- user trust or perceived interruption.

Calibration matters more than raw accuracy if the probability will schedule interventions.

## 10. High-value product capabilities

### 10.1 Adaptive match ranking

Re-rank existing cross-document suggestions using evidence quality and predicted recall need. This uses the current product surface and requires no new autonomous agent.

### 10.2 Weak-subgraph review

Show a “concepts at risk” view: confirmed concepts with low predicted recall, high uncertainty, or recent failures, together with their supporting passages and neighbours.

### 10.3 Reactive note linking

When a user writes a substantive note, ground it to chunks and look for a bounded set of related or contradictory evidence elsewhere. Save results as candidates.

### 10.4 Relationship recall

SELAR's distinctive value is connections between papers, not isolated flashcards. Assess edges as well as nodes: ask why concept A extends, contradicts, or depends on concept B.

### 10.5 Explainable recommendations

Each surfaced item should be able to say, in product language:

> “Shown because you confirmed this concept 12 days ago, recall is estimated to be low, and this passage provides independent supporting evidence.”

The decision trace should make that explanation reproducible.

## 11. What not to do first

1. **Do not add Neo4j solely because the data is a graph.** The current graph is small and PostgreSQL provides sufficient relational, vector, and recursive-query capabilities.
2. **Do not let the LLM write directly to confirmed memory.** It should produce candidates with evidence.
3. **Do not decay or prune factual nodes based on user inactivity.** Decay learner activation and active-view priority instead.
4. **Do not use online fine-tuning or RL before establishing clean events, labels, and replayable baselines.** Otherwise it will optimise logging artefacts and product-policy bias.
5. **Do not equate exposure with learning.** Reading and scrolling are noisy; active recall provides stronger evidence.
6. **Do not feed the whole graph into the model.** Retrieve a bounded evidence subgraph.
7. **Do not silently alter behaviour across research cohorts.** Adaptive ranking changes the intervention and may invalidate the current experimental design.
8. **Do not physically delete rejected or stale candidates by default.** Archive them so mistakes, drift, and policy changes remain measurable.

## 12. Research-design implications

SELAR's schema indicates a controlled study with `control`, `treatment_auto`, and `treatment_hitl` cohorts. Introducing personalised decay, reactive graph growth, or adaptive retrieval changes the treatment itself.

Before rollout:

- freeze and version the policy used by each cohort;
- decide whether adaptation is a new treatment arm;
- avoid training on delayed-test outcomes from the same evaluation split;
- log every candidate that was eligible, not only the one displayed;
- use temporal train/validation/test splits;
- report results by user to prevent high-activity users dominating metrics; and
- preserve a non-adaptive baseline.

An offline “shadow mode” is ideal: compute learner state and candidate rankings without changing the UI, then compare predictions with later quiz outcomes.

## 13. Suggested delivery sequence

### Phase 0 — Correctness and provenance

- persist the proposal's full article mental-model schema;
- populate `chunk_concepts`;
- ground claims, assumptions, and open questions to supporting chunks;
- ground annotations to chunks;
- enforce authenticated ownership on every graph, annotation, session, suggestion, and PDF operation;
- add idempotency to ingestion;
- version model- and policy-produced records; and
- replace quadratic concept comparison with indexed nearest-neighbour retrieval.

**Exit criterion:** every node, edge, response, and annotation can be traced to a user and evidence source.

### Phase 1 — Event and deterministic learner model

- add immutable learning events;
- implement a replayable reducer;
- create per-user/per-concept learner state;
- use fixed, documented half-life updates; and
- expose the score components in an internal diagnostic view.

**Exit criterion:** replaying the same events produces identical learner state and rankings.

### Phase 2 — Adaptive ranking in shadow mode

- implement model-to-model top-k retrieval and the four research link types;
- rank current suggestions using semantic relevance, evidence, and learning need;
- log eligible candidate sets and reasons;
- do not change the user experience yet; and
- evaluate calibration against later assessments.

**Exit criterion:** the adaptive ranker beats fixed similarity on predeclared offline metrics without harming coverage or fairness.

### Phase 3 — Governed runtime expansion

- add a durable job queue or outbox;
- trigger bounded induction from high-intent notes, highlights, and relabels;
- introduce candidate/support/confirmation/archive states; and
- add user review and provenance UI.

**Exit criterion:** new candidates are evidence-grounded, idempotent, auditable, and cannot bypass graduation rules.

### Phase 4 — Learned memory and retrieval policies

- fit Half-Life Regression from real assessment data;
- consider a time-aware knowledge-tracing model;
- learn candidate ranking from outcomes; and
- only then evaluate trainable graph-memory or RL approaches against the deterministic baseline.

**Exit criterion:** a learned policy improves delayed retention in a controlled test, not merely clicks or acceptance.

## 14. Recommended initial experiments

The project should keep its planned link-relevance pilot before introducing adaptive memory. The two experiments answer different questions.

### Experiment A — mental-model link quality

> Does structured mental-model retrieval produce more relevant argument-level links than chunk-vector similarity alone?

Compare:

1. current chunk cosine similarity;
2. article mental-model vector retrieval;
3. mental-model retrieval plus bounded LLM link classification; and
4. the same pipeline with graph/evidence features.

Measure top-1 and top-k relevance, relation-label agreement, evidence correctness, latency, and cost. Preserve the proposal's human relevance ratings and analyse each of the four link types separately.

### Experiment B — learner-state prediction

The lowest-risk, highest-information experiment is:

> Can a deterministic per-concept half-life estimate predict delayed quiz correctness better than recency, frequency, and cosine-similarity baselines?

Procedure:

1. instrument grounded exposure, confirmation, rejection, and quiz events;
2. run the learner model in shadow mode;
3. pre-register event weights and evaluation metrics;
4. predict correctness at several delay buckets;
5. compare against simple baselines;
6. inspect calibration and failure cases; and
7. only if successful, use predicted learning need to re-rank suggestions in a new treatment arm.

This directly tests the central claim—whether SELAR can estimate retained knowledge—without first building an autonomous or self-modifying system.

Experiment B should follow instrumentation and should initially run in shadow mode. It must not change the main two-condition retention intervention unless added as a separately versioned treatment.

## 15. Final recommendation

Proceed with the concept under the working architecture **“structured article mental models + governed semantic graph + deterministic learner-state overlay + reactive candidate expansion.”**

The immediate opportunity is to bring the implementation into alignment with the research proposal: persist claims, assumptions, open questions, concepts, and evidence; retrieve only a bounded set of prior mental models; classify the proposal's four argument-level relationships; and promote links through explicit user confirmation. Runtime expansion should reuse that same schema on a small local neighbourhood.

The next opportunity is to connect SELAR's interaction history into an explainable learner model. The research-heavy ideas—trainable finite-state graph memory, reinforcement-learned edge weights, automated harness optimisation, and local information-geometric memory—are valuable references, but they solve later-stage optimisation problems. They should not precede provenance, event quality, calibrated retention labels, and a deterministic baseline.

If reduced to one design rule:

> **Let models propose meaning; let evidence justify it; let deterministic policy govern it; and let user outcomes teach the system only after every step is replayable.**

## 16. Research notes and sources

### Directly relevant

- Liu et al., [Context as a Tool: Context Management for Long-Horizon SWE-Agents](https://aclanthology.org/2026.findings-acl.1032/) (Findings of ACL 2026). Supports bounded, actively managed context split into stable semantics, condensed long-term memory, and high-fidelity short-term interactions. The SWE-agent results do not directly establish learning benefits for SELAR.
- Tan et al., [In Prospect and Retrospect: Reflective Memory Management for Long-term Personalized Dialogue Agents](https://aclanthology.org/2025.acl-long.413/) (ACL 2025). Supports multiple memory granularities and adaptive retrieval. Its dialogue benchmarks differ from academic reading and retention.
- Rasmussen et al., [Zep: A Temporal Knowledge Graph Architecture for Agent Memory](https://arxiv.org/abs/2501.13956) (2025 preprint). Supports temporally aware graph memory and preservation of historical relationships. Treat performance claims as preprint evidence and domain-specific.
- Yousuf et al., [Can an LLM Induce a Graph? Investigating Memory Drift and Context Length](https://arxiv.org/abs/2510.03611) (2025 preprint). Supports pre-structuring and bounded graph induction rather than relying on a single long unstructured prompt.
- Lam et al., [Governing Evolving Memory in LLM Agents: The SSGM Framework](https://arxiv.org/abs/2603.11768) (2026 preprint). Motivates consistency verification, temporal controls, access control, and separating memory consolidation from execution. It is a conceptual framework rather than direct empirical validation of SELAR's design.
- Xia et al., [From Experience to Strategy: Empowering LLM Agents with Trainable Graph Memory](https://arxiv.org/abs/2511.07800) (2025 preprint). Motivates trainable graph memory and reward-weighted strategy retrieval, but focuses on agent trajectories rather than human learning states.
- Lee et al., [Meta-Harness: End-to-End Optimization of Model Harnesses](https://arxiv.org/abs/2603.28052) (2026 preprint). Shows that storage, retrieval, and context-presentation code can itself be optimised. This is most relevant after SELAR has stable evaluation traces.

### Human-memory and learner modelling

- Settles and Meeder, [A Trainable Spaced Repetition Model for Language Learning](https://aclanthology.org/P16-1174.pdf) (ACL 2016). Provides the Half-Life Regression formulation and evidence that an interpretable recall model can outperform fixed scheduling baselines in its evaluated language-learning setting.
- Corbett and Anderson, [Knowledge Tracing: Modeling the Acquisition of Procedural Knowledge](https://doi.org/10.1007/BF01099821) (1994/1995). Establishes Bayesian Knowledge Tracing as a way to infer changing skill mastery from observed performance. Classic BKT does not model forgetting by default, so a time-aware variant would be required for SELAR's long delays.

### Source-quality cautions

- Agent-memory papers primarily evaluate agents recalling conversations or execution history. That is not the same as predicting a human learner's retention.
- Several central sources in the notes are recent arXiv preprints. They are useful design inputs, not settled evidence.
- Product articles, newsletters, videos, and vendor benchmarks can generate hypotheses but should not determine the architecture without reproducible primary evidence.
- Terms such as “information-geometric memory,” “Fisher–Rao forgetting,” and “Langevin memory dynamics” were not necessary to justify the proposed SELAR baseline. They should require a specific primary source, measurable advantage, and comparison with simpler models before adoption.
