import os
import re
import json
import asyncio
import unicodedata
from collections import Counter
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
CHUNK_WORD_LIMIT = int(os.getenv("CHUNK_WORD_LIMIT", "120"))

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


NOISE_TERMS = {
    "arxiv", "cais", "doi", "github", "https", "http", "isbn", "issn", "jose",
    "copyright", "proceedings", "reference", "references", "sanjose",
}


def clean_extracted_text(text: str) -> str:
    """Normalize PDF glyphs and repair line-break hyphenation."""
    text = unicodedata.normalize("NFKC", text)
    text = re.sub(r"(?<=\w)-\s+(?=\w)", "", text)
    return re.sub(r"\s+", " ", text).strip()


def is_noisy_prose(text: str) -> bool:
    """Detect references, fused layout text, and pseudocode masquerading as prose."""
    lowered = text.lower()
    words = re.findall(r"[a-z0-9_]+", lowered)
    if not words:
        return True
    code_hits = len(re.findall(
        r"\b(?:algorithm|return|input|output|catch|rollback_report|record\.result|ifnot)\b|//|←|[{};]",
        lowered,
    ))
    fused_hits = sum(len(word) > 28 for word in words)
    noise_hits = sum(term in lowered for term in NOISE_TERMS)
    return (
        code_hits >= 3
        or fused_hits > max(1, len(words) // 12)
        or (noise_hits >= 2 and len(words) < 45)
    )


def is_valid_concept_name(name: str) -> bool:
    normalized = clean_extracted_text(name)
    words = re.findall(r"[A-Za-z0-9][A-Za-z0-9+.-]*", normalized)
    lowered_words = {word.lower().strip(".") for word in words}
    if not 1 <= len(words) <= 6 or len(normalized) > 90:
        return False
    if lowered_words & NOISE_TERMS or any(len(word) > 28 for word in words):
        return False
    if re.search(r"\b(?:may|june|july|august|september|october|november|december)\s*\d{2,4}\b", normalized, re.I):
        return False
    return sum(character.isalpha() for character in normalized) >= max(3, len(normalized) // 2)


def normalize_mental_model(payload: Any) -> Dict[str, Any]:
    """Validate the stable article mental-model fields used by SELAR."""
    if not isinstance(payload, dict):
        payload = {}
    concept_objects: List[Dict[str, Any]] = []
    for item in payload.get("key_concepts", []):
        if isinstance(item, str) and is_valid_concept_name(item):
            concept_objects.append({"name": clean_extracted_text(item), "description": "", "evidence_chunk_index": 0})
        elif isinstance(item, dict) and is_valid_concept_name(str(item.get("name", ""))):
            description = clean_extracted_text(str(item.get("description", "")))
            concept_objects.append({
                "name": clean_extracted_text(str(item.get("name", "")))[:250],
                "description": "" if is_noisy_prose(description) else description,
                "evidence_chunk_index": item.get("evidence_chunk_index", 0),
            })
    main_claim = clean_extracted_text(str(payload.get("main_claim", "")))
    assumptions = [text for text in text_items(payload.get("assumptions", [])) if not is_noisy_prose(text)]
    open_questions = [text for text in text_items(payload.get("open_questions", [])) if not is_noisy_prose(text)]
    domain = clean_extracted_text(str(payload.get("domain", "")))
    return {
        "main_claim": "" if is_noisy_prose(main_claim) else main_claim,
        "key_concepts": concept_objects[:12],
        "assumptions": assumptions,
        "open_questions": open_questions,
        "domain": domain[:250] if is_valid_concept_name(domain) else "",
        "concept_edges": payload.get("concept_edges", []) if isinstance(payload.get("concept_edges", []), list) else [],
    }


FALLBACK_STOP_WORDS = {
    "about", "abstract", "after", "again", "against", "also", "among", "because", "been",
    "before", "being", "between", "both", "could", "does", "during", "each",
    "every", "from", "further", "have", "having", "improve", "improves", "into",
    "introduce", "introduction", "more", "most", "other", "over", "paper", "present",
    "same", "should", "such", "than", "that",
    "their", "then", "there", "these", "they", "this", "those", "through",
    "under", "using", "very", "were", "what", "when", "where", "which",
    "while", "with", "within", "without", "would",
}


def deterministic_mental_model(contents: List[str], partial: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """Complete a missing mental model from PDF text without another LLM call."""
    result = normalize_mental_model(partial or {})
    sentence_candidates = []
    for chunk_index, content in enumerate(contents):
        for sentence_index, raw_sentence in enumerate(re.split(r"(?<=[.!?])\s+", content)):
            sentence = re.sub(r"\s+", " ", raw_sentence).strip(" -")
            abstract_match = re.search(r"\babstract\b\s*", sentence, re.I)
            if abstract_match:
                sentence = sentence[abstract_match.end():].strip()
            words = sentence.split()
            if (not 8 <= len(words) <= 70 or "@" in sentence or "http" in sentence.lower()
                    or is_noisy_prose(sentence)):
                continue
            lowered = sentence.lower()
            cue_score = sum(cue in lowered for cue in (
                "we present", "we propose", "we introduce", "we develop",
                "we demonstrate", "we show", "we argue", "this work",
            ))
            score = cue_score * 12 + min(len(words), 35) / 10 - chunk_index * 0.12 - sentence_index * 0.02
            sentence_candidates.append((score, chunk_index, sentence))

    if not result["main_claim"]:
        if sentence_candidates:
            result["main_claim"] = max(sentence_candidates, key=lambda item: item[0])[2][:1000]
        else:
            fallback_text = next((re.sub(r"\s+", " ", text).strip() for text in contents if text.strip()), "")
            result["main_claim"] = fallback_text[:1000]

    if len(result["key_concepts"]) < 5:
        phrase_counts: Counter = Counter()
        phrase_evidence: Dict[str, int] = {}
        phrase_sentence: Dict[str, str] = {}
        for chunk_index, content in enumerate(contents):
            for raw_sentence in re.split(r"(?<=[.!?])\s+", content):
                sentence = re.sub(r"\s+", " ", raw_sentence).strip()
                tokens = [token.lower() for token in re.findall(r"[A-Za-z][A-Za-z0-9-]{3,}", sentence)]
                for left, right in zip(tokens, tokens[1:]):
                    if left in FALLBACK_STOP_WORDS or right in FALLBACK_STOP_WORDS or left == right:
                        continue
                    phrase = f"{left} {right}"
                    phrase_counts[phrase] += 1
                    phrase_evidence.setdefault(phrase, chunk_index)
                    phrase_sentence.setdefault(phrase, sentence)

        existing_names = {concept["name"].lower() for concept in result["key_concepts"]}
        ranked_phrases = sorted(
            phrase_counts,
            key=lambda phrase: (-phrase_counts[phrase], phrase_evidence[phrase], phrase),
        )
        for phrase in ranked_phrases:
            if len(result["key_concepts"]) >= 8:
                break
            if (not is_valid_concept_name(phrase)
                    or is_noisy_prose(phrase_sentence[phrase])
                    or phrase in existing_names
                    or any(phrase in name or name in phrase for name in existing_names)):
                continue
            result["key_concepts"].append({
                "name": phrase.title(),
                "description": phrase_sentence[phrase][:500],
                "evidence_chunk_index": phrase_evidence[phrase],
            })
            existing_names.add(phrase)

    if not result["domain"] and result["key_concepts"]:
        result["domain"] = result["key_concepts"][0]["name"]
    return result


def merge_word_bboxes(words: List[Dict[str, Any]], width: float, height: float) -> List[Dict[str, float]]:
    """Merge adjacent PDF words into precise line rectangles."""
    if not words or not width or not height:
        return []
    lines: List[Dict[str, float]] = []
    for word in words:
        box = {
            "x0": float(word["x0"]), "top": float(word["top"]),
            "x1": float(word["x1"]), "bottom": float(word["bottom"]),
        }
        previous = lines[-1] if lines else None
        same_line = previous and abs(box["top"] - previous["top"]) <= 3
        nearby = previous and 0 <= box["x0"] - previous["x1"] <= width * 0.04
        if same_line and nearby:
            previous["x1"] = max(previous["x1"], box["x1"])
            previous["bottom"] = max(previous["bottom"], box["bottom"])
        else:
            lines.append(box)
    return [{
        "x": line["x0"] / width,
        "y": line["top"] / height,
        "w": (line["x1"] - line["x0"]) / width,
        "h": (line["bottom"] - line["top"]) / height,
    } for line in lines]


def semantic_tokens(text: str) -> set[str]:
    """Return stable content tokens for deterministic relationship scoring."""
    stop_words = {"about", "after", "also", "been", "between", "from", "have", "into",
                  "more", "that", "their", "then", "this", "through", "using", "with"}
    return {token for token in re.findall(r"[a-z0-9]+", text.lower())
            if len(token) > 3 and token not in stop_words}


def token_overlap(left: str, right: str) -> float:
    left_tokens, right_tokens = semantic_tokens(left), semantic_tokens(right)
    if not left_tokens or not right_tokens:
        return 0.0
    return len(left_tokens & right_tokens) / len(left_tokens | right_tokens)


def deterministic_model_link(source: Dict[str, Any], target: Dict[str, Any], similarity: float) -> Optional[Dict[str, Any]]:
    """Classify a bounded model pair without a generative model call."""
    negations = {"cannot", "never", "without", "unreliable", "fails", "failure", "not"}
    source_assumptions = source.get("assumptions", [])
    target_assumptions = target.get("assumptions", [])
    for source_text in source_assumptions:
        for target_text in target_assumptions:
            overlap = token_overlap(source_text, target_text)
            source_negative = bool(semantic_tokens(source_text) & negations)
            target_negative = bool(semantic_tokens(target_text) & negations)
            if overlap >= 0.18 and source_negative != target_negative:
                return {
                    "link_type": "assumption_conflict",
                    "confidence": min(0.95, 0.62 + overlap),
                    "bridge_explanation": "These readings make opposing assumptions about "
                                          + ", ".join(sorted(semantic_tokens(source_text) & semantic_tokens(target_text))[:3]) + ".",
                }

    resolution_scores = [
        (token_overlap(question, target.get("main_claim", "")), question)
        for question in source.get("open_questions", [])
    ] + [
        (token_overlap(question, source.get("main_claim", "")), question)
        for question in target.get("open_questions", [])
    ]
    best_resolution = max(resolution_scores, default=(0.0, ""))
    if best_resolution[0] >= 0.16:
        return {
            "link_type": "question_resolution",
            "confidence": min(0.92, 0.60 + best_resolution[0]),
            "bridge_explanation": "One reading directly informs an open question raised by the other.",
        }

    def concept_name(value: Any) -> str:
        return str(value.get("name", "")) if isinstance(value, dict) else str(value)

    source_concepts = [concept_name(value) for value in source.get("key_concepts", [])]
    target_concepts = [concept_name(value) for value in target.get("key_concepts", [])]
    concept_scores = [(token_overlap(left, right), left, right)
                      for left in source_concepts for right in target_concepts]
    best_concept = max(concept_scores, default=(0.0, "", ""))
    if similarity >= 0.58 and best_concept[0] >= 0.20:
        return {
            "link_type": "concept_overlap",
            "confidence": min(0.94, 0.58 + best_concept[0]),
            "bridge_explanation": f"Both readings develop the concept of {best_concept[1]}.",
        }

    claim_overlap = token_overlap(source.get("main_claim", ""), target.get("main_claim", ""))
    if similarity >= 0.62 and claim_overlap >= 0.10:
        return {
            "link_type": "claim_extension",
            "confidence": min(0.90, 0.42 + similarity * 0.45 + claim_overlap),
            "bridge_explanation": "The newer reading extends a closely related claim from the prior reading.",
        }
    return None


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
            page_count = len(pdf.pages)
            for page_num, page in enumerate(pdf.pages, start=1):
                # Follow the PDF's content stream so multi-column papers are not
                # interleaved line-by-line. A tighter x tolerance restores spaces
                # in densely typeset conference PDFs.
                words = page.extract_words(use_text_flow=True, x_tolerance=1, y_tolerance=3)
                if not words:
                    continue

                width, height = page.width, page.height

                current_chunk_words = []
                current_chunk_geometry = []

                for word in words:
                    current_chunk_words.append(word['text'])
                    current_chunk_geometry.append(word)

                    if len(current_chunk_words) >= CHUNK_WORD_LIMIT:
                        chunk_text = clean_extracted_text(" ".join(current_chunk_words))
                        chunks_data.append({
                            "text": chunk_text,
                            "page": page_num,
                            "bboxes": merge_word_bboxes(current_chunk_geometry, width, height),
                        })
                        current_chunk_words = []
                        current_chunk_geometry = []

                if current_chunk_words:
                    chunks_data.append({
                        "text": clean_extracted_text(" ".join(current_chunk_words)),
                        "page": page_num,
                        "bboxes": merge_word_bboxes(current_chunk_geometry, width, height),
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
        await conn.execute(
            "UPDATE documents SET progress = 0.30, page_count = $2 WHERE id = $1",
            doc_id, page_count
        )

        chunk_ids = []
        for i, chunk in enumerate(chunks_data):
            bboxes_json = json.dumps(chunk['bboxes'])
            vec = str(embeddings[i])

            chunk_id = await conn.fetchval("""
                INSERT INTO chunks (document_id, user_id, chunk_index, page_start, page_end, content, bboxes, embedding)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                RETURNING id
            """, doc_id, user_id, i, chunk['page'], chunk['page'], chunk['text'], bboxes_json, vec)
            chunk_ids.append(chunk_id)
        await conn.execute("UPDATE documents SET progress = 0.45 WHERE id = $1", doc_id)

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
            LIMIT 8
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
            LIMIT 8
        """, doc_id, user_id)

        print(f"Link Generation results - Outbound: {res1}, Inbound: {res2}")
        await conn.execute("UPDATE documents SET progress = 0.55 WHERE id = $1", doc_id)

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

            try:
                response = client.models.generate_content(
                    model=TEXT_MODEL,
                    contents=f"{prompt}\n\nTEXT:\n{''.join(numbered_chunks)}",
                    config={"response_mime_type": "application/json"}
                )
                partial_mental_model = normalize_mental_model(safe_parse_json(response.text or ""))
            except Exception as generation_error:
                print(f"Structured model call failed; using deterministic fallback: {generation_error}")
                partial_mental_model = normalize_mental_model({})

            used_fallback = not partial_mental_model["main_claim"]
            mental_model = deterministic_mental_model(contents, partial_mental_model)
            if used_fallback:
                print("Structured model response had no main claim; completed it deterministically.")
            if not mental_model["main_claim"]:
                raise ValueError("Mental model did not include a main claim")

            model_embedding_text = "\n".join([
                mental_model["main_claim"],
                *[concept["name"] for concept in mental_model["key_concepts"]],
                *mental_model["assumptions"],
                *mental_model["open_questions"],
                mental_model["domain"],
            ])
            concepts = mental_model["key_concepts"]
            concept_texts = [f"{c['name']}: {c.get('description', '')}" for c in concepts]
            model_embedding_result = client.models.embed_content(
                model=EMBEDDING_MODEL,
                contents=[model_embedding_text, *concept_texts],
                config={"task_type": "RETRIEVAL_DOCUMENT"}
            )
            model_vector = str(model_embedding_result.embeddings[0].values)
            concept_embeddings = [embedding.values for embedding in model_embedding_result.embeddings[1:]]
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
            await conn.execute("UPDATE documents SET progress = 0.75 WHERE id = $1", doc_id)

            if concepts:
                name_to_uuid = {}
                for i, concept in enumerate(concepts):
                    c_vec = str(concept_embeddings[i])
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
                await conn.execute("UPDATE documents SET progress = 0.88 WHERE id = $1", doc_id)

            # ── Phase 6: Bounded document-to-library mental-model linking ──
            await conn.execute("""
                DELETE FROM mental_model_links
                WHERE source_model_id = $1 AND status = 'candidate'
            """, mental_model_id)
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
                prior_evidence_ids = []
                for prior in prior_models:
                    evidence = await conn.fetchrow("""
                        SELECT c.id, c.content
                        FROM chunks c
                        JOIN document_mental_models mm ON mm.document_id = c.document_id
                        WHERE mm.id = $1 AND c.embedding IS NOT NULL
                        ORDER BY c.embedding::halfvec(3072) <=> mm.embedding::halfvec(3072)
                        LIMIT 1
                    """, prior["id"])
                    prior_evidence_ids.append(evidence["id"] if evidence else None)
                created_links = 0
                for prior_index, prior in enumerate(prior_models):
                    prior_model = {
                        "main_claim": prior["main_claim"],
                        "key_concepts": list(prior["key_concepts"]),
                        "assumptions": list(prior["assumptions"]),
                        "open_questions": list(prior["open_questions"]),
                        "domain": prior["domain"],
                    }
                    classified = deterministic_model_link(
                        mental_model, prior_model, float(prior["similarity"])
                    )
                    if not classified:
                        continue
                    link_type = classified["link_type"]
                    confidence = classified["confidence"]
                    target_chunk_id = prior_evidence_ids[prior_index]
                    await conn.execute("""
                        INSERT INTO mental_model_links (
                            user_id, source_model_id, target_model_id, link_type,
                            similarity, confidence, bridge_explanation,
                            source_evidence_chunk_id, target_evidence_chunk_id,
                            status, created_via, model_version, prompt_version
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
                                  'candidate', 'ai_suggested', $10, 'deterministic-link-v1')
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
                        str(classified["bridge_explanation"])[:500],
                        source_evidence_chunk_id, target_chunk_id, "deterministic-v1")
                    created_links += 1
                print(f"Mental-model candidate links: {created_links}")
            await conn.execute("UPDATE documents SET progress = 0.96 WHERE id = $1", doc_id)
        except Exception as e:
            print(f"Failed to generate mental model for {doc_id}: {e}")
            raise

        # ── Finalize ──
        await conn.execute("""
            UPDATE documents SET status = 'ready', progress = 1, processed_at = NOW() WHERE id = $1
        """, doc_id)

        await conn.close()
        print(f"Successfully processed {doc_id}")

    except Exception as e:
        print(f"Failed to process {doc_id}: {e}")
        try:
            if conn:
                await conn.close()
            conn = await asyncpg.connect(DATABASE_URL)
            await conn.execute("UPDATE documents SET status = 'failed', progress = 0 WHERE id = $1", doc_id)
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
