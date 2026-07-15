import os
import re
import json
import asyncio
from typing import List, Dict, Any, Optional
from pydantic import BaseModel
from fastapi import FastAPI, HTTPException, BackgroundTasks
import asyncpg
import pdfplumber
from google import genai
from dotenv import load_dotenv

# Load root .env.development
load_dotenv(os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), '.env.development'))

app = FastAPI(title="SELAR AI Ingestion Worker")

# Configure Gemini (new google.genai SDK)
api_key = os.getenv("GEMINI_API_KEY")
if not api_key:
    print("Warning: GEMINI_API_KEY is not set.")

client = genai.Client(api_key=api_key) if api_key else None

DATABASE_URL = os.getenv("DATABASE_URL", "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable")
EMBEDDING_MODEL = os.getenv("GEMINI_EMBEDDING_MODEL", "models/gemini-embedding-001")
TEXT_MODEL = os.getenv("GEMINI_TEXT_MODEL", "models/gemini-3-flash-preview")

class ProcessRequest(BaseModel):
    doc_id: str
    file_path: str


def safe_parse_json(text: str) -> Any:
    """Robustly extract JSON from LLM output that may be wrapped in markdown."""
    text = text.strip()
    # Strip markdown code fences
    if text.startswith("```json"):
        text = text[7:]
    elif text.startswith("```"):
        text = text[3:]
    if text.endswith("```"):
        text = text[:-3]
    text = text.strip()

    # Try direct parse first
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    # Regex fallback: find the first complete JSON object
    match = re.search(r'\{.*\}', text, re.DOTALL)
    if match:
        try:
            return json.loads(match.group())
        except json.JSONDecodeError:
            pass

    return {}


def text_items(value: Any, key: str = "text") -> List[str]:
    """Normalize structured or plain-string LLM fields into non-empty text."""
    if not isinstance(value, list):
        return []
    items: List[str] = []
    for item in value:
        text = item.get(key, "") if isinstance(item, dict) else item
        if isinstance(text, str) and text.strip():
            items.append(text.strip())
    return items


def normalize_mental_model(payload: Any) -> Dict[str, Any]:
    """Validate the stable article mental-model fields used by SELAR."""
    if not isinstance(payload, dict):
        payload = {}
    concept_objects: List[Dict[str, Any]] = []
    for item in payload.get("key_concepts", []):
        if isinstance(item, str) and item.strip():
            concept_objects.append({"name": item.strip(), "description": "", "evidence_chunk_index": 0})
        elif isinstance(item, dict) and str(item.get("name", "")).strip():
            concept_objects.append({
                "name": str(item.get("name", "")).strip()[:250],
                "description": str(item.get("description", "")).strip(),
                "evidence_chunk_index": item.get("evidence_chunk_index", 0),
            })
    return {
        "main_claim": str(payload.get("main_claim", "")).strip(),
        "key_concepts": concept_objects[:12],
        "assumptions": text_items(payload.get("assumptions", [])),
        "open_questions": text_items(payload.get("open_questions", [])),
        "domain": str(payload.get("domain", "")).strip()[:250],
        "concept_edges": payload.get("concept_edges", []) if isinstance(payload.get("concept_edges", []), list) else [],
    }


async def process_document_task(doc_id: str, file_path: str):
    """
    Background job: Parses PDF, chunks texts with bboxes, embeddings, semantic links,
    relation classification, and knowledge graph extraction.
    """
    conn = None
    try:
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File {file_path} not found.")

        # ── Phase 1: Parse PDF ──
        chunks_data = []

        with pdfplumber.open(file_path) as pdf:
            for page_num, page in enumerate(pdf.pages, start=1):
                words = page.extract_words()
                if not words:
                    continue

                width, height = page.width, page.height

                current_chunk_words = []
                x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0

                for word in words:
                    current_chunk_words.append(word['text'])
                    x0 = min(x0, word['x0'])
                    top = min(top, word['top'])
                    x1 = max(x1, word['x1'])
                    bottom = max(bottom, word['bottom'])

                    if len(current_chunk_words) > 200:
                        chunk_text = " ".join(current_chunk_words)
                        chunks_data.append({
                            "text": chunk_text,
                            "page": page_num,
                            "bbox": {
                                "x": x0 / width,
                                "y": top / height,
                                "w": (x1 - x0) / width,
                                "h": (bottom - top) / height
                            }
                        })
                        current_chunk_words = []
                        x0, top, x1, bottom = float('inf'), float('inf'), 0.0, 0.0

                if current_chunk_words:
                    chunks_data.append({
                        "text": " ".join(current_chunk_words),
                        "page": page_num,
                        "bbox": {
                                "x": x0 / width,
                                "y": top / height,
                                "w": (x1 - x0) / width,
                                "h": (bottom - top) / height
                        }
                    })

        # ── Phase 2: Embed chunks ──
        print(f"Extracted {len(chunks_data)} chunks. Generating embeddings...")

        contents = [chunk["text"] for chunk in chunks_data]

        batch_size = 100
        embeddings = []
        for i in range(0, len(contents), batch_size):
            batch = contents[i:i+batch_size]
            result = client.models.embed_content(
                model=EMBEDDING_MODEL,
                contents=batch,
                config={"task_type": "RETRIEVAL_DOCUMENT"}
            )
            for emb in result.embeddings:
                embeddings.append(emb.values)

        print(f"Generated {len(embeddings)} embeddings. Inserting into Postgres...")

        # ── Phase 3: Database Insertion ──
        conn = await asyncpg.connect(DATABASE_URL)

        row = await conn.fetchrow("SELECT user_id FROM documents WHERE id = $1", doc_id)
        if not row:
            raise Exception(f"Document {doc_id} not found in DB")
        user_id = row['user_id']

        # Serialize retries for the same document and replace its derived chunks.
        # The connection-scoped lock is released automatically on close/failure.
        await conn.execute("SELECT pg_advisory_lock(hashtextextended($1, 0))", doc_id)
        await conn.execute("DELETE FROM chunks WHERE document_id = $1", doc_id)

        chunk_ids = []
        for i, chunk in enumerate(chunks_data):
            bboxes_json = json.dumps([chunk['bbox']])
            vec = str(embeddings[i])

            chunk_id = await conn.fetchval("""
                INSERT INTO chunks (document_id, user_id, chunk_index, page_start, page_end, content, bboxes, embedding)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                RETURNING id
            """, doc_id, user_id, i, chunk['page'], chunk['page'], chunk['text'], bboxes_json, vec)
            chunk_ids.append(chunk_id)

        # ── Phase 4: Semantic Link Generation (wider threshold + LIMIT) ──
        print(f"Generating Semantic Links for {doc_id}...")

        # New → Existing. Each new chunk uses the HNSW half-vector projection;
        # this avoids an unbounded all-pairs comparison as the library grows.
        res1 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, new_chunk.id, match.id, 1 - match.distance, 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN LATERAL (
                SELECT existing.id,
                       existing.embedding::halfvec(3072) <=> new_chunk.embedding::halfvec(3072) AS distance
                FROM chunks existing
                WHERE existing.document_id != $1 AND existing.user_id = $2
                  AND existing.embedding IS NOT NULL
                ORDER BY existing.embedding::halfvec(3072) <=> new_chunk.embedding::halfvec(3072)
                LIMIT 2
            ) match
            WHERE new_chunk.document_id = $1
              AND new_chunk.embedding IS NOT NULL AND match.distance < 0.35
            ORDER BY match.distance ASC
            LIMIT 20
        """, doc_id, user_id)

        # Existing → New (limit 20 best matches)
        res2 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, match.id, new_chunk.id, 1 - match.distance, 'related_to', 'pending'
            FROM chunks new_chunk
            CROSS JOIN LATERAL (
                SELECT existing.id,
                       existing.embedding::halfvec(3072) <=> new_chunk.embedding::halfvec(3072) AS distance
                FROM chunks existing
                WHERE existing.document_id != $1 AND existing.user_id = $2
                  AND existing.embedding IS NOT NULL
                ORDER BY existing.embedding::halfvec(3072) <=> new_chunk.embedding::halfvec(3072)
                LIMIT 2
            ) match
            WHERE new_chunk.document_id = $1
              AND new_chunk.embedding IS NOT NULL AND match.distance < 0.35
            ORDER BY match.distance ASC
            LIMIT 20
        """, doc_id, user_id)

        print(f"Link Generation results - Outbound: {res1}, Inbound: {res2}")

        # ── Phase 4b: Classify relations + generate summaries ──
        print(f"Classifying link relations for {doc_id}...")
        try:
            pending_links = await conn.fetch("""
                SELECT ls.id, sc.content AS src_text, tc.content AS tgt_text
                FROM link_suggestions ls
                JOIN chunks sc ON ls.source_chunk_id = sc.id
                JOIN chunks tc ON ls.target_chunk_id = tc.id
                WHERE (sc.document_id = $1 OR tc.document_id = $1)
                  AND ls.status = 'pending'
                  AND ls.summary = ''
                LIMIT 40
            """, doc_id)

            if pending_links:
                # Batch classify in groups of 10
                for batch_start in range(0, len(pending_links), 10):
                    batch = pending_links[batch_start:batch_start+10]
                    pairs_text = ""
                    for idx, link in enumerate(batch):
                        src_snip = link['src_text'][:300]
                        tgt_snip = link['tgt_text'][:300]
                        pairs_text += f"\nPAIR {idx+1}:\nA: {src_snip}\nB: {tgt_snip}\n"

                    classify_prompt = f"""Classify each text pair's semantic relationship and write a one-sentence summary of how they connect.

Return a JSON array. Each element must have:
- "index": the pair number (1-based)
- "relation": one of 'prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends'
- "summary": one clear sentence explaining the connection (max 20 words)

{pairs_text}"""

                    response = client.models.generate_content(
                        model=TEXT_MODEL,
                        contents=classify_prompt,
                        config={"response_mime_type": "application/json"}
                    )

                    classifications = safe_parse_json(response.text)
                    if isinstance(classifications, dict) and 'results' in classifications:
                        classifications = classifications['results']
                    if isinstance(classifications, dict) and not isinstance(classifications, list):
                        classifications = [classifications]
                    if not isinstance(classifications, list):
                        classifications = []

                    for cls in classifications:
                        try:
                            idx = int(cls.get('index', 0)) - 1
                            if 0 <= idx < len(batch):
                                rel = cls.get('relation', 'related_to')
                                if rel not in ['prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends']:
                                    rel = 'related_to'
                                summary = cls.get('summary', '')[:200]

                                await conn.execute("""
                                    UPDATE link_suggestions SET relation = $1, summary = $2 WHERE id = $3
                                """, rel, summary, batch[idx]['id'])
                        except (ValueError, IndexError):
                            continue

                print(f"Classified {len(pending_links)} link relations")
        except Exception as e:
            print(f"Relation classification failed (non-fatal): {e}")

        # ── Phase 5: Structured Mental Model + Evidence Graph ──
        print(f"Generating structured mental model for {doc_id}...")
        try:
            numbered_chunks = []
            current_length = 0
            for index, content in enumerate(contents):
                block = f"[CHUNK {index}]\n{content}\n"
                if current_length + len(block) > 80000:
                    break
                numbered_chunks.append(block)
                current_length += len(block)

            prompt = """Build a structured article mental model for active learning.

Return ONLY valid JSON with this structure:
{
  "main_claim": "The article's central argument in 1-2 sentences",
  "key_concepts": [
    {"name": "Concept", "description": "Grounded description", "evidence_chunk_index": 0}
  ],
  "assumptions": [
    {"text": "An explicit or implicit premise", "evidence_chunk_index": 0}
  ],
  "open_questions": [
    {"text": "A question the article leaves unresolved", "evidence_chunk_index": 0}
  ],
  "domain": "Specific subject area",
  "concept_edges": [
    { "source": "Source Concept Name", "target": "Target Concept Name", "relation": "related_to" }
  ]
}

Rules:
- Extract 5-8 specific, domain-relevant concepts.
- Cite only chunk indexes that appear in the input.
- Do not invent claims that are not supported by the text.
- Valid relations: prerequisite_of, related_to, sub_concept_of, contradicts, extends
- Edge source/target must exactly match a key concept name."""

            response = client.models.generate_content(
                model=TEXT_MODEL,
                contents=f"{prompt}\n\nTEXT:\n{''.join(numbered_chunks)}",
                config={"response_mime_type": "application/json"}
            )

            mental_model = normalize_mental_model(safe_parse_json(response.text))
            if not mental_model["main_claim"]:
                raise ValueError("Mental model did not include a main claim")

            model_embedding_text = "\n".join([
                mental_model["main_claim"],
                *[concept["name"] for concept in mental_model["key_concepts"]],
                *mental_model["assumptions"],
                *mental_model["open_questions"],
                mental_model["domain"],
            ])
            model_embedding_result = client.models.embed_content(
                model=EMBEDDING_MODEL,
                contents=[model_embedding_text],
                config={"task_type": "RETRIEVAL_DOCUMENT"}
            )
            model_vector = str(model_embedding_result.embeddings[0].values)
            mental_model_id = await conn.fetchval("""
                INSERT INTO document_mental_models (
                    document_id, user_id, version, main_claim, key_concepts, assumptions,
                    open_questions, domain, embedding, model_version, prompt_version, status
                ) VALUES ($1, $2, 1, $3, $4, $5, $6, $7, $8, $9, 'mental-model-v1', 'ready')
                ON CONFLICT (document_id, version) DO UPDATE SET
                    main_claim = EXCLUDED.main_claim,
                    key_concepts = EXCLUDED.key_concepts,
                    assumptions = EXCLUDED.assumptions,
                    open_questions = EXCLUDED.open_questions,
                    domain = EXCLUDED.domain,
                    embedding = EXCLUDED.embedding,
                    model_version = EXCLUDED.model_version,
                    prompt_version = EXCLUDED.prompt_version,
                    status = 'ready',
                    generated_at = now()
                RETURNING id
            """, doc_id, user_id, mental_model["main_claim"],
                [concept["name"] for concept in mental_model["key_concepts"]],
                mental_model["assumptions"], mental_model["open_questions"],
                mental_model["domain"], model_vector, TEXT_MODEL)

            concepts = mental_model["key_concepts"]

            if concepts:
                concept_texts = [f"{c['name']}: {c.get('description', '')}" for c in concepts]
                c_result = client.models.embed_content(
                    model=EMBEDDING_MODEL,
                    contents=concept_texts,
                    config={"task_type": "RETRIEVAL_DOCUMENT"}
                )
                c_embeddings = [emb.values for emb in c_result.embeddings]

                name_to_uuid = {}
                for i, concept in enumerate(concepts):
                    c_vec = str(c_embeddings[i])
                    row = await conn.fetchrow("""
                        INSERT INTO concepts (user_id, name, description, embedding, state, model_version, prompt_version)
                        VALUES ($1, $2, $3, $4, 'supported', $5, 'mental-model-v1')
                        ON CONFLICT (user_id, name) DO UPDATE SET
                            description = EXCLUDED.description,
                            embedding = EXCLUDED.embedding,
                            state = CASE WHEN concepts.state = 'confirmed' THEN 'confirmed' ELSE 'supported' END,
                            model_version = EXCLUDED.model_version,
                            prompt_version = EXCLUDED.prompt_version
                        RETURNING id
                    """, user_id, concept['name'][:250], concept.get('description', ''), c_vec, TEXT_MODEL)

                    if row:
                        name_to_uuid[concept['name']] = row['id']
                        try:
                            evidence_index = int(concept.get("evidence_chunk_index", 0))
                        except (TypeError, ValueError):
                            evidence_index = 0
                        if 0 <= evidence_index < len(chunk_ids):
                            await conn.execute("""
                                INSERT INTO chunk_concepts (chunk_id, concept_id, confidence)
                                VALUES ($1, $2, 1.0)
                                ON CONFLICT (chunk_id, concept_id) DO UPDATE SET confidence = EXCLUDED.confidence
                            """, chunk_ids[evidence_index], row['id'])
                        else:
                            await conn.execute("""
                                INSERT INTO chunk_concepts (chunk_id, concept_id, confidence)
                                SELECT c.id, $2, 1 - (c.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072))
                                FROM chunks c WHERE c.document_id = $1 AND c.embedding IS NOT NULL
                                ORDER BY c.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072) LIMIT 2
                                ON CONFLICT (chunk_id, concept_id) DO UPDATE SET confidence = EXCLUDED.confidence
                            """, doc_id, row['id'], c_vec)

                        # Incremental nearest-neighbour graph growth; never compare every pair.
                        await conn.execute("""
                            INSERT INTO concept_edges (
                                user_id, source_concept_id, target_concept_id, relation,
                                created_via, state, confidence
                            )
                            SELECT $1, $2, existing.id, 'related_to', 'ai_suggested', 'candidate',
                                   1 - (existing.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072))
                            FROM concepts existing
                            WHERE existing.user_id = $1 AND existing.id != $2
                              AND existing.embedding IS NOT NULL
                              AND existing.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072) < 0.25
                            ORDER BY existing.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072)
                            LIMIT 3
                            ON CONFLICT (source_concept_id, target_concept_id, relation) DO NOTHING
                        """, user_id, row['id'], c_vec)

                edge_count = 0
                for edge in mental_model.get('concept_edges', []):
                    src_id = name_to_uuid.get(edge.get('source'))
                    tgt_id = name_to_uuid.get(edge.get('target'))

                    if src_id and tgt_id and src_id != tgt_id:
                        rel = edge.get('relation', 'related_to')
                        if rel not in ['prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends']:
                            rel = 'related_to'

                        await conn.execute("""
                            INSERT INTO concept_edges (
                                user_id, source_concept_id, target_concept_id, relation,
                                created_via, state, confidence
                            )
                            VALUES ($1, $2, $3, $4, 'ai_suggested', 'supported', 0.75)
                            ON CONFLICT (source_concept_id, target_concept_id, relation) DO NOTHING
                        """, user_id, src_id, tgt_id, rel)
                        edge_count += 1

                print(f"Mental model -> Concepts: {len(name_to_uuid)}, Edges: {edge_count}")

            # ── Phase 6: Bounded document-to-library mental-model linking ──
            prior_models = await conn.fetch("""
                SELECT mm.id, mm.document_id, d.title, mm.main_claim, mm.key_concepts,
                       mm.assumptions, mm.open_questions, mm.domain,
                       1 - (mm.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072)) AS similarity
                FROM document_mental_models mm
                JOIN documents d ON d.id = mm.document_id
                WHERE mm.user_id = $1 AND mm.document_id != $2
                  AND mm.status = 'ready' AND mm.embedding IS NOT NULL
                ORDER BY mm.embedding::halfvec(3072) <=> $3::vector(3072)::halfvec(3072)
                LIMIT 8
            """, user_id, doc_id, model_vector)

            if prior_models:
                source_evidence = await conn.fetchrow("""
                    SELECT id, content FROM chunks
                    WHERE document_id = $1 AND embedding IS NOT NULL
                    ORDER BY embedding::halfvec(3072) <=> $2::vector(3072)::halfvec(3072)
                    LIMIT 1
                """, doc_id, model_vector)
                source_evidence_chunk_id = source_evidence["id"] if source_evidence else None
                candidates = []
                prior_evidence_ids = []
                for index, prior in enumerate(prior_models, start=1):
                    evidence = await conn.fetchrow("""
                        SELECT c.id, c.content
                        FROM chunks c
                        JOIN document_mental_models mm ON mm.document_id = c.document_id
                        WHERE mm.id = $1 AND c.embedding IS NOT NULL
                        ORDER BY c.embedding::halfvec(3072) <=> mm.embedding::halfvec(3072)
                        LIMIT 1
                    """, prior["id"])
                    prior_evidence_ids.append(evidence["id"] if evidence else None)
                    candidates.append({
                        "index": index,
                        "title": prior["title"],
                        "main_claim": prior["main_claim"],
                        "key_concepts": list(prior["key_concepts"]),
                        "assumptions": list(prior["assumptions"]),
                        "open_questions": list(prior["open_questions"]),
                        "domain": prior["domain"],
                        "evidence_excerpt": evidence["content"][:700] if evidence else "",
                    })
                link_prompt = f"""Compare the new mental model with each prior mental model.
Return a JSON array with at most one strongest meaningful link per prior model.
Each result must contain: index, link_type, confidence, bridge_explanation.
Valid link_type values: concept_overlap, claim_extension, assumption_conflict, question_resolution.
Confidence must be between 0 and 1. Omit weak links below 0.55.
The bridge explanation must make the learner think and must stay under 35 words.

NEW MODEL:
{json.dumps(mental_model, ensure_ascii=False)}

NEW MODEL EVIDENCE:
{source_evidence["content"][:700] if source_evidence else ""}

PRIOR MODELS:
{json.dumps(candidates, ensure_ascii=False)}"""
                link_response = client.models.generate_content(
                    model=TEXT_MODEL,
                    contents=link_prompt,
                    config={"response_mime_type": "application/json"}
                )
                classified_links = safe_parse_json(link_response.text)
                if isinstance(classified_links, dict):
                    classified_links = classified_links.get("links", [])
                if not isinstance(classified_links, list):
                    classified_links = []

                valid_link_types = {
                    "concept_overlap", "claim_extension",
                    "assumption_conflict", "question_resolution"
                }
                created_links = 0
                for classified in classified_links:
                    try:
                        prior_index = int(classified.get("index", 0)) - 1
                        confidence = max(0.0, min(1.0, float(classified.get("confidence", 0))))
                    except (TypeError, ValueError):
                        continue
                    link_type = classified.get("link_type")
                    if not (0 <= prior_index < len(prior_models)) or link_type not in valid_link_types or confidence < 0.55:
                        continue
                    prior = prior_models[prior_index]
                    target_chunk_id = prior_evidence_ids[prior_index]
                    await conn.execute("""
                        INSERT INTO mental_model_links (
                            user_id, source_model_id, target_model_id, link_type,
                            similarity, confidence, bridge_explanation,
                            source_evidence_chunk_id, target_evidence_chunk_id,
                            status, created_via, model_version, prompt_version
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
                                  'candidate', 'ai_suggested', $10, 'mental-link-v1')
                        ON CONFLICT (source_model_id, target_model_id, link_type) DO UPDATE SET
                            similarity = EXCLUDED.similarity,
                            confidence = EXCLUDED.confidence,
                            bridge_explanation = EXCLUDED.bridge_explanation,
                            source_evidence_chunk_id = EXCLUDED.source_evidence_chunk_id,
                            target_evidence_chunk_id = EXCLUDED.target_evidence_chunk_id,
                            model_version = EXCLUDED.model_version,
                            prompt_version = EXCLUDED.prompt_version
                    """, user_id, mental_model_id, prior["id"], link_type,
                        float(prior["similarity"]), confidence,
                        str(classified.get("bridge_explanation", ""))[:500],
                        source_evidence_chunk_id, target_chunk_id, TEXT_MODEL)
                    created_links += 1
                print(f"Mental-model candidate links: {created_links}")
        except Exception as e:
            print(f"Failed to generate mental model for {doc_id}: {e}")

        # ── Finalize ──
        await conn.execute("""
            UPDATE documents SET status = 'ready', processed_at = NOW() WHERE id = $1
        """, doc_id)

        await conn.close()
        print(f"Successfully processed {doc_id}")

    except Exception as e:
        print(f"Failed to process {doc_id}: {e}")
        try:
            if conn:
                await conn.close()
            conn = await asyncpg.connect(DATABASE_URL)
            await conn.execute("UPDATE documents SET status = 'failed' WHERE id = $1", doc_id)
            await conn.close()
        except:
            pass

@app.post("/process")
async def process_document(req: ProcessRequest, background_tasks: BackgroundTasks):
    background_tasks.add_task(process_document_task, req.doc_id, req.file_path)
    return {"message": "Processing started in background", "doc_id": req.doc_id}

@app.get("/health")
def health():
    return {"status": "ok"}
