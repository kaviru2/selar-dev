import os
import re
import json
import asyncio
import contextlib
import socket
import shutil
import unicodedata
import time
from collections import Counter
from typing import List, Dict, Any, Optional
from pydantic import BaseModel, Field
from fastapi import FastAPI, HTTPException, BackgroundTasks
import asyncpg
from google import genai
from google.genai import types
from dotenv import load_dotenv
from ingestion import extract_source
from candidate_contract import persist_grounded_overlap

# Load root .env.development
load_dotenv(os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), '.env.development'))

@contextlib.asynccontextmanager
async def lifespan(_: FastAPI):
    global queue_poller_tasks
    queue_poller_tasks = []
    if INGESTION_QUEUE_ENABLED:
        queue_poller_tasks = [
            asyncio.create_task(ingestion_queue_poller())
            for _ in range(INGESTION_CONCURRENCY)
        ]
    else:
        print("Durable ingestion queue polling is disabled (set INGESTION_QUEUE_ENABLED=true to enable it).")
    try:
        yield
    finally:
        for task in queue_poller_tasks:
            task.cancel()
        await asyncio.gather(*queue_poller_tasks, return_exceptions=True)
        queue_poller_tasks = []


app = FastAPI(title="SELAR AI Ingestion Worker", lifespan=lifespan)

# Configure Gemini (new google.genai SDK)
api_key = os.getenv("GEMINI_API_KEY")
if not api_key:
    print("Warning: GEMINI_API_KEY is not set.")

client = genai.Client(api_key=api_key) if api_key else None

DATABASE_URL = os.getenv("DATABASE_URL", "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable")
EMBEDDING_MODEL = os.getenv("GEMINI_MULTIMODAL_EMBEDDING_MODEL", "gemini-embedding-2")
EMBEDDING_DIMENSION = int(os.getenv("GEMINI_EMBEDDING_DIMENSION", "3072"))
TEXT_MODEL = os.getenv("GEMINI_TEXT_MODEL", "models/gemini-3-flash-preview")
CHUNK_WORD_LIMIT = int(os.getenv("CHUNK_WORD_LIMIT", "120"))
INGESTION_CONCURRENCY = max(1, int(os.getenv("INGESTION_CONCURRENCY", "1")))
INGESTION_QUEUE_ENABLED = os.getenv("INGESTION_QUEUE_ENABLED", "false").strip().lower() in {
    "1", "true", "yes", "on",
}
ingestion_semaphore = asyncio.Semaphore(INGESTION_CONCURRENCY)
INGESTION_POLL_SECONDS = max(0.25, float(os.getenv("INGESTION_POLL_SECONDS", "1")))
INGESTION_LEASE_SECONDS = max(60, int(os.getenv("INGESTION_LEASE_SECONDS", "300")))
WORKER_ID = os.getenv("INGESTION_WORKER_ID", f"{socket.gethostname()}-{os.getpid()}")
queue_poller_tasks: List[asyncio.Task] = []

class ProcessRequest(BaseModel):
    doc_id: str
    source_id: str = ""
    run_id: str = ""
    source_type: str = "pdf"
    file_path: str = ""
    source_url: str = ""
    raw_text: str = ""
    title: str = ""


class ChatHistoryItem(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    user_id: str
    thread_id: str
    question: str
    history: List[ChatHistoryItem] = Field(default_factory=list)


def is_explicit_graph_command(question: str) -> bool:
    """Route only direct mutation requests, not questions about graph concepts."""
    return bool(re.match(
        r"^\s*(?:(?:please|could you|can you|would you)\s+)*"
        r"(?:update|correct|fix|change|modify|edit)\s+"
        r"(?:(?:my|the|this)\s+)?(?:knowledge\s+)?graph\b",
        question, re.I,
    ))


def graph_command_response(started: float) -> Dict[str, Any]:
    # Without a reviewed, request-matched witness, a random existing candidate
    # would be a misleading suggestion for this particular correction.
    answer = ("No graph change was made, and no preview was created for this request. "
              "Chat cannot verify or apply a graph correction. To inspect an existing "
              "two-sided candidate, open its document in the reader and check both source "
              "quotes before deciding; a chat citation alone does not establish a relation.")
    return {
        "answer": answer,
        "model_version": "deterministic-graph-command-boundary-v1",
        "ranking_policy": "no-research-retrieval-graph-command-v1",
        "citations": [], "candidates": [],
        "metrics": {"embedding_ms": 0, "retrieval_ms": 0, "generation_ms": 0,
                    "total_ms": (time.perf_counter() - started) * 1000,
                    "candidate_count": 0, "model_calls": 0},
    }


def reciprocal_rank_fusion(result_sets: Dict[str, List[Dict[str, Any]]], limit: int = 6) -> List[Dict[str, Any]]:
    """Fuse independently ranked retrieval signals with fixed, replayable weights."""
    if limit <= 0:
        return []
    weights = {
        "vector": 0.40,
        "lexical": 0.25,
        "graph": 0.15,
        "learner": 0.10,
        "evidence": 0.07,
        "recency": 0.03,
    }
    fused: Dict[str, Dict[str, Any]] = {}
    for signal, results in result_sets.items():
        for rank, candidate in enumerate(results, start=1):
            chunk_id = str(candidate["chunk_id"])
            entry = fused.setdefault(chunk_id, {**candidate, "rrf_score": 0.0, "signals": {}})
            entry["rrf_score"] += weights.get(signal, 0) / (60 + rank)
            entry["signals"][signal] = {"rank": rank, "score": float(candidate.get("score", 0))}
    # Prefer diverse documents, then fill any unused slots with the next-best
    # chunks. A one-document library must not lose most of its evidence.
    selected: List[Dict[str, Any]] = []
    overflow: List[Dict[str, Any]] = []
    per_document: Counter = Counter()
    for item in sorted(fused.values(), key=lambda entry: (-entry["rrf_score"], entry["chunk_id"])):
        document = item.get("document_id") or item["chunk_id"]
        if per_document[document] >= 2:
            overflow.append(item)
            continue
        selected.append(item)
        per_document[document] += 1
        if len(selected) == limit:
            break
    return selected + overflow[:max(0, limit - len(selected))]


def referenced_citation_ranks(answer: str, maximum: int) -> set[int]:
    """Return only source labels explicitly referenced in the generated answer."""
    return {
        rank for rank in (int(value) for value in re.findall(r"\[S(\d+)\]", answer, re.I))
        if 1 <= rank <= maximum
    }


def requires_primary_source_verification(question: str) -> bool:
    """Identify direct source-use questions that cannot be verified by citations alone.

    A title match or generated label cannot establish a document as the original.
    """
    words = question.lower()
    predicate = re.search(r"\b(?:us(?:e|ed|ing)|evaluat\w*|test(?:ed|ing)?|employ\w*|appl(?:y|ied))\b", words)
    original = re.search(r"\boriginal(?:ly)?\b", words)
    benchmark_or_method = re.search(r"\b\w*bench\w*\b|\b(?:method|algorithm|technique)\w*\b", words)
    direct_question = re.search(r"^\s*(?:did|does|do|was|were|has|have|whether)\b", words)
    # Reporting results on a benchmark implies original-study use, even when
    # the question does not literally say "use" or "evaluate". Keep this
    # extension tied to an explicit original-source question so an ordinary
    # comparison paper's reported results remain answerable.
    reported_original_results = original and re.search(r"\breport(?:ed|s|ing)?\s+results?\s+on\b", words)
    return bool(reported_original_results or (predicate and (original or (direct_question and benchmark_or_method))))


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


def embed_text_documents(contents: List[str], title: str = "") -> List[List[float]]:
    """Embed separate retrieval objects in Gemini Embedding 2's shared space."""
    if not client:
        raise RuntimeError("Gemini client is not configured")
    embeddings: List[List[float]] = []
    batch_size = 100
    for offset in range(0, len(contents), batch_size):
        batch = [
            types.Content(parts=[types.Part.from_text(
                text=f"title: {title or 'none'} | text: {content}"
            )])
            for content in contents[offset:offset + batch_size]
        ]
        result = client.models.embed_content(
            model=EMBEDDING_MODEL,
            contents=batch,
            config=types.EmbedContentConfig(output_dimensionality=EMBEDDING_DIMENSION),
        )
        embeddings.extend(embedding.values for embedding in result.embeddings)
    return embeddings


def embed_query_text(query: str) -> List[float]:
    result = client.models.embed_content(
        model=EMBEDDING_MODEL,
        contents=f"task: question answering | query: {query}",
        config=types.EmbedContentConfig(output_dimensionality=EMBEDDING_DIMENSION),
    )
    return result.embeddings[0].values


def embed_visual_asset(asset: Dict[str, Any]) -> List[float]:
    parts = []
    context = clean_extracted_text(" ".join(filter(None, [
        asset.get("caption", ""), asset.get("alt_text", ""), asset.get("description", "")
    ]))) or "Source visual evidence"
    parts.append(types.Part.from_text(text=f"title: visual evidence | text: {context}"))
    with open(asset["storage_path"], "rb") as image_file:
        parts.append(types.Part.from_bytes(data=image_file.read(), mime_type=asset["mime_type"]))
    result = client.models.embed_content(
        model=EMBEDDING_MODEL,
        contents=types.Content(parts=parts),
        config=types.EmbedContentConfig(output_dimensionality=EMBEDDING_DIMENSION),
    )
    return result.embeddings[0].values


async def process_document_task(
    doc_id: str,
    file_path: str = "",
    source_id: str = "",
    run_id: str = "",
    source_type: str = "pdf",
    source_url: str = "",
    raw_text: str = "",
    title: str = "",
):
    """
    Background job: Parses PDF, chunks texts with bboxes, embeddings, semantic links,
    relation classification, and knowledge graph extraction.
    """
    conn = None
    try:
        # ── Phase 1: Normalize source ──
        if run_id:
            conn = await asyncpg.connect(DATABASE_URL)
            await conn.execute(
                "UPDATE ingestion_runs SET status = 'fetching', started_at = now() WHERE id = $1",
                run_id,
            )
            await conn.close()
            conn = None
        normalized = await extract_source(
            source_type=source_type,
            doc_id=doc_id,
            word_limit=CHUNK_WORD_LIMIT,
            merge_bboxes=merge_word_bboxes,
            file_path=file_path,
            source_url=source_url,
            raw_text=raw_text,
            title=title,
        )
        if source_id and source_type == "web" and normalized.content_hash:
            conn = await asyncpg.connect(DATABASE_URL)
            previous_hash = await conn.fetchval(
                "SELECT last_content_hash FROM content_sources WHERE id = $1", source_id
            )
            if previous_hash and previous_hash == normalized.content_hash:
                await conn.execute(
                    """UPDATE documents SET status = 'ready', progress = 1, visible = false,
                              title = $2, canonical_url = $3, content_hash = $4,
                              mime_type = $5, fetched_at = now(), processed_at = now()
                       WHERE id = $1""",
                    doc_id, normalized.title, normalized.canonical_url,
                    normalized.content_hash, normalized.mime_type,
                )
                await conn.execute(
                    """UPDATE content_sources SET canonical_uri = $2, last_fetched_at = now(),
                              last_error = '', status = 'active', updated_at = now()
                       WHERE id = $1""",
                    source_id, normalized.canonical_url,
                )
                if run_id:
                    await conn.execute(
                        """UPDATE ingestion_runs SET status = 'unchanged', completed_at = now(),
                                  extractor = $2, extractor_version = $3,
                                  metrics = $4, error = '' WHERE id = $1""",
                        run_id, normalized.extractor, normalized.extractor_version,
                        json.dumps({"source_type": source_type, "content_hash": normalized.content_hash}),
                    )
                await conn.close()
                shutil.rmtree(os.path.join("/tmp/selar_uploads", doc_id), ignore_errors=True)
                print(f"Source {source_id} is unchanged; hidden snapshot {doc_id}")
                return
            await conn.close()
            conn = None
        # Adapters may encounter the same bytes through responsive-image URLs.
        # Keep one visual representation per source snapshot so embedding and
        # persistence remain deterministic.
        deduplicated_assets = []
        seen_asset_hashes = set()
        for asset in normalized.assets:
            asset_hash = asset.get("content_hash", "")
            if asset_hash and asset_hash in seen_asset_hashes:
                continue
            if asset_hash:
                seen_asset_hashes.add(asset_hash)
            deduplicated_assets.append(asset)
        normalized.assets = deduplicated_assets
        chunks_data = normalized.chunks
        page_count = normalized.page_count
        if not chunks_data:
            raise ValueError("source did not produce any retrievable chunks")

        # ── Phase 2: Embed chunks ──
        print(f"Extracted {len(chunks_data)} chunks. Generating embeddings...")

        contents = [chunk["text"] for chunk in chunks_data]

        embeddings = await asyncio.to_thread(embed_text_documents, contents, normalized.title)
        for asset in normalized.assets:
            visual_embedding = await asyncio.to_thread(embed_visual_asset, asset)
            asset["embedding"] = visual_embedding
            asset["chunk_index"] = len(chunks_data)
            visual_text = clean_extracted_text(" ".join(filter(None, [
                asset.get("caption", ""), asset.get("alt_text", ""), asset.get("description", "")
            ]))) or "Source visual evidence"
            chunks_data.append({
                "text": visual_text,
                "page": int(asset.get("locator", {}).get("page", 1)),
                "bboxes": [],
                "locator": asset.get("locator", {}),
                "modality": "mixed",
            })
            contents.append(visual_text)
            embeddings.append(visual_embedding)

        print(f"Generated {len(embeddings)} embeddings. Inserting into Postgres...")

        # ── Phase 3: Database Insertion ──
        conn = await asyncpg.connect(DATABASE_URL)

        row = await conn.fetchrow("SELECT user_id, title, source_id FROM documents WHERE id = $1", doc_id)
        if not row:
            raise Exception(f"Document {doc_id} not found in DB")
        user_id = row['user_id']

        # Serialize retries for the same document and replace its derived chunks.
        # The connection-scoped lock is released automatically on close/failure.
        await conn.execute("SELECT pg_advisory_lock(hashtextextended($1, 0))", doc_id)
        await conn.execute("DELETE FROM chunks WHERE document_id = $1", doc_id)
        await conn.execute("DELETE FROM content_blocks WHERE document_id = $1", doc_id)
        await conn.execute("DELETE FROM assets WHERE document_id = $1", doc_id)
        for block in normalized.blocks:
            await conn.execute("""
                INSERT INTO content_blocks (document_id, user_id, block_index, kind, text, locator, metadata)
                VALUES ($1, $2, $3, $4, $5, $6, $7)
            """, doc_id, user_id, block["block_index"], block["kind"], block.get("text", ""),
                json.dumps(block.get("locator", {})), json.dumps(block.get("metadata", {})))
        await conn.execute(
            """UPDATE documents SET progress = 0.30, page_count = $2, title = $3,
                   authors = $4, year = $5, canonical_url = $6, content_hash = $7,
                   mime_type = $8, metadata = $9, fetched_at = now()
               WHERE id = $1""",
            doc_id, page_count, normalized.title or row["title"], normalized.authors,
            normalized.year, normalized.canonical_url, normalized.content_hash,
            normalized.mime_type, json.dumps(normalized.metadata),
        )
        if run_id:
            await conn.execute("""
                UPDATE ingestion_runs SET status = 'embedding', extractor = $2,
                    extractor_version = $3, embedding_model = $4, embedding_dimension = $5
                WHERE id = $1
            """, run_id, normalized.extractor, normalized.extractor_version,
                EMBEDDING_MODEL, EMBEDDING_DIMENSION)

        chunk_ids = []
        for i, chunk in enumerate(chunks_data):
            bboxes_json = json.dumps(chunk.get('bboxes', []))
            locator_json = json.dumps(chunk.get('locator', {}))
            vec = str(embeddings[i])

            chunk_id = await conn.fetchval("""
                INSERT INTO chunks (
                    document_id, user_id, chunk_index, page_start, page_end, content,
                    bboxes, locator, modality, embedding, embedding_model, embedding_version
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'v1')
                RETURNING id
            """, doc_id, user_id, i, chunk.get('page', 1), chunk.get('page', 1),
                chunk['text'], bboxes_json, locator_json, chunk.get('modality', 'text'), vec, EMBEDDING_MODEL)
            chunk_ids.append(chunk_id)
        for asset in normalized.assets:
            asset_id = await conn.fetchval("""
                INSERT INTO assets (
                    document_id, user_id, block_index, kind, storage_path, source_url,
                    mime_type, width, height, content_hash, caption, alt_text, description,
                    locator, embedding, embedding_model, embedding_version
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, 'v1')
                ON CONFLICT (document_id, content_hash) DO UPDATE
                    SET content_hash = EXCLUDED.content_hash
                RETURNING id
            """, doc_id, user_id, asset.get("block_index"), asset["kind"], asset["storage_path"],
                asset.get("source_url", ""), asset["mime_type"], asset["width"], asset["height"],
                asset["content_hash"], asset.get("caption", ""), asset.get("alt_text", ""),
                asset.get("description", ""), json.dumps(asset.get("locator", {})),
                str(asset["embedding"]), EMBEDDING_MODEL)
            await conn.execute("""
                INSERT INTO chunk_assets (chunk_id, asset_id, relation)
                VALUES ($1, $2, 'contains') ON CONFLICT DO NOTHING
            """, chunk_ids[asset["chunk_index"]], asset_id)
        await conn.execute("UPDATE documents SET progress = 0.45 WHERE id = $1", doc_id)

        # ── Phase 4: Semantic Link Generation (wider threshold + LIMIT) ──
        print(f"Generating Semantic Links for {doc_id}...")

        # New → Existing. Each new chunk uses the HNSW half-vector projection;
        # this avoids an unbounded all-pairs comparison as the library grows.
        res1 = await conn.execute("""
            INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status)
            SELECT $2, new_chunk.id, match.id, 1 - match.distance, 'unclassified', 'pending'
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

        # The Reader orients this one stored relationship to whichever document
        # is open, avoiding mirrored duplicate rows and duplicated highlights.
        print(f"Link Generation results - New relationships: {res1}")
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
            model_embeddings = await asyncio.to_thread(
                embed_text_documents, [model_embedding_text, *concept_texts], normalized.title
            )
            model_vector = str(model_embeddings[0])
            concept_embeddings = model_embeddings[1:]
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

                        # Concept membership is not a verified cross-concept assertion.
                        # Keep chunk_concepts for retrieval; no edge is promoted here.

                # Generated concept_edges lack pair-specific asserting-source
                # evidence; preserve extracted concepts but do not persist edges.
                print(f"Mental model -> Concepts: {len(name_to_uuid)}, unverified edges deferred")
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
                # Similarity orders candidate documents; it never asserts a relation.
                # Search actual owner-matched passages for exact, unambiguous
                # instances of a named current-document concept on both sides.
                source_rows = await conn.fetch("""
                    SELECT c.id, c.user_id, c.document_id, c.content, c.locator,
                           d.title, d.content_hash
                    FROM chunks c JOIN documents d ON d.id = c.document_id
                    WHERE c.document_id = $1 AND c.user_id = $2 AND d.user_id = $2
                    ORDER BY c.chunk_index LIMIT 40
                """, doc_id, user_id)
                created_links = 0
                for prior in prior_models:
                    target_rows = await conn.fetch("""
                        SELECT c.id, c.user_id, c.document_id, c.content, c.locator,
                               d.title, d.content_hash
                        FROM chunks c JOIN documents d ON d.id = c.document_id
                        WHERE c.document_id = $1 AND c.user_id = $2 AND d.user_id = $2
                        ORDER BY c.chunk_index LIMIT 40
                    """, prior["document_id"], user_id)
                    for source_row in source_rows:
                        for target_row in target_rows:
                            if await persist_grounded_overlap(
                                conn, mental_model, dict(source_row), dict(target_row),
                                user_id, doc_id, prior["document_id"], mental_model_id,
                                prior["id"], float(prior["similarity"])
                            ):
                                created_links += 1
                                break
                        else:
                            continue
                        break
                print(f"Grounded mental-model candidate links: {created_links}")
            await conn.execute("UPDATE documents SET progress = 0.96 WHERE id = $1", doc_id)
        except Exception as e:
            print(f"Failed to generate mental model for {doc_id}: {e}")
            raise

        # ── Finalize ──
        await conn.execute("""
            UPDATE documents SET status = 'ready', progress = 1, processed_at = NOW() WHERE id = $1
        """, doc_id)
        if source_id:
            await conn.execute("""
                UPDATE content_sources SET title = $2, canonical_uri = CASE WHEN $3 = '' THEN canonical_uri ELSE $3 END,
                    last_content_hash = $4, last_fetched_at = now(), last_error = '', status = 'active', updated_at = now()
                WHERE id = $1
            """, source_id, normalized.title, normalized.canonical_url, normalized.content_hash)
        if run_id:
            await conn.execute("""
                UPDATE ingestion_runs SET status = 'ready', completed_at = now(),
                    metrics = $2
                WHERE id = $1
            """, run_id, json.dumps({
                "blocks": len(normalized.blocks),
                "chunks": len(chunks_data),
                "assets": len(normalized.assets),
                "source_type": source_type,
            }))

        await conn.close()
        print(f"Successfully processed {doc_id}")

    except Exception as e:
        print(f"Failed to process {doc_id}: {e}")
        try:
            if conn:
                await conn.close()
            conn = await asyncpg.connect(DATABASE_URL)
            # A failed snapshot must not leak partially indexed chunks into
            # retrieval. Cascades also remove derived links and asset joins.
            await conn.execute("DELETE FROM chunks WHERE document_id = $1", doc_id)
            await conn.execute("DELETE FROM content_blocks WHERE document_id = $1", doc_id)
            await conn.execute("DELETE FROM assets WHERE document_id = $1", doc_id)
            await conn.execute("UPDATE documents SET status = 'failed', progress = 0 WHERE id = $1", doc_id)
            if source_id:
                await conn.execute("""
                    UPDATE content_sources SET status = 'failed', last_error = $2, updated_at = now()
                    WHERE id = $1
                """, source_id, str(e)[:1000])
            if run_id:
                await conn.execute("""
                    UPDATE ingestion_runs SET status = 'failed', error = $2, completed_at = now()
                    WHERE id = $1
                """, run_id, str(e)[:2000])
            await conn.close()
        except:
            pass
        raise

async def process_document_with_limit(**kwargs):
    """Keep expensive PDF extraction and embedding within a memory-safe bound."""
    async with ingestion_semaphore:
        await process_document_task(**kwargs)


async def claim_ingestion_job() -> Optional[Dict[str, Any]]:
    conn = await asyncpg.connect(DATABASE_URL)
    try:
        async with conn.transaction():
            await conn.execute("""
                UPDATE ingestion_jobs
                SET status = 'queued', worker_id = '', lease_expires_at = NULL,
                    available_at = now(), error = CASE WHEN error = '' THEN 'worker lease expired' ELSE error END,
                    updated_at = now()
                WHERE status = 'leased' AND lease_expires_at < now() AND attempts < max_attempts
            """)
            expired = await conn.fetch("""
                UPDATE ingestion_jobs
                SET status = 'failed', worker_id = '', lease_expires_at = NULL,
                    error = CASE WHEN error = '' THEN 'worker lease expired after maximum attempts' ELSE error END,
                    updated_at = now()
                WHERE status = 'leased' AND lease_expires_at < now() AND attempts >= max_attempts
                RETURNING run_id, document_id, source_id, error
            """)
            for row in expired:
                await conn.execute(
                    "UPDATE ingestion_runs SET status = 'failed', error = $2, completed_at = now() WHERE id = $1",
                    row["run_id"], row["error"],
                )
                await conn.execute(
                    "UPDATE documents SET status = 'failed', progress = 0 WHERE id = $1", row["document_id"]
                )
                await conn.execute(
                    "UPDATE content_sources SET status = 'failed', last_error = $2, updated_at = now() WHERE id = $1",
                    row["source_id"], row["error"],
                )
            row = await conn.fetchrow("""
                WITH candidate AS (
                    SELECT id FROM ingestion_jobs
                    WHERE status = 'queued' AND available_at <= now() AND attempts < max_attempts
                    ORDER BY created_at
                    FOR UPDATE SKIP LOCKED
                    LIMIT 1
                )
                UPDATE ingestion_jobs job
                SET status = 'leased', attempts = attempts + 1, worker_id = $1,
                    lease_expires_at = now() + ($2 * interval '1 second'),
                    error = '', updated_at = now()
                FROM candidate
                WHERE job.id = candidate.id
                RETURNING job.*
            """, WORKER_ID, INGESTION_LEASE_SECONDS)
            return dict(row) if row else None
    finally:
        await conn.close()


async def heartbeat_ingestion_job(job_id: str) -> None:
    while True:
        await asyncio.sleep(max(20, INGESTION_LEASE_SECONDS // 3))
        conn = await asyncpg.connect(DATABASE_URL)
        try:
            await conn.execute("""
                UPDATE ingestion_jobs
                SET lease_expires_at = now() + ($3 * interval '1 second'), updated_at = now()
                WHERE id = $1 AND status = 'leased' AND worker_id = $2
            """, job_id, WORKER_ID, INGESTION_LEASE_SECONDS)
        finally:
            await conn.close()


async def finish_ingestion_job(job: Dict[str, Any], error: Optional[Exception] = None) -> None:
    conn = await asyncpg.connect(DATABASE_URL)
    try:
        if error is None:
            await conn.execute("""
                UPDATE ingestion_jobs SET status = 'completed', lease_expires_at = NULL,
                    worker_id = '', error = '', updated_at = now()
                WHERE id = $1 AND worker_id = $2
            """, job["id"], WORKER_ID)
            return
        message = str(error)[:2000]
        if job["attempts"] < job["max_attempts"]:
            delay_seconds = min(60, 2 ** job["attempts"])
            await conn.execute("""
                UPDATE ingestion_jobs SET status = 'queued', lease_expires_at = NULL,
                    worker_id = '', error = $3,
                    available_at = now() + ($4 * interval '1 second'), updated_at = now()
                WHERE id = $1 AND worker_id = $2
            """, job["id"], WORKER_ID, message, delay_seconds)
            await conn.execute(
                "UPDATE ingestion_runs SET status = 'queued', error = $2, completed_at = NULL WHERE id = $1",
                job["run_id"], message,
            )
            await conn.execute(
                "UPDATE documents SET status = 'processing', progress = 0.01 WHERE id = $1", job["document_id"]
            )
            await conn.execute(
                "UPDATE content_sources SET status = 'active', last_error = $2, updated_at = now() WHERE id = $1",
                job["source_id"], f"retrying after: {message}"[:1000],
            )
        else:
            await conn.execute("""
                UPDATE ingestion_jobs SET status = 'failed', lease_expires_at = NULL,
                    worker_id = '', error = $3, updated_at = now()
                WHERE id = $1 AND worker_id = $2
            """, job["id"], WORKER_ID, message)
    finally:
        await conn.close()


async def run_claimed_ingestion_job(job: Dict[str, Any]) -> None:
    heartbeat = asyncio.create_task(heartbeat_ingestion_job(str(job["id"])))
    error: Optional[Exception] = None
    try:
        await process_document_with_limit(
            doc_id=str(job["document_id"]), file_path=job["file_path"],
            source_id=str(job["source_id"]), run_id=str(job["run_id"]),
            source_type=job["source_type"], source_url=job["source_url"],
            raw_text=job["raw_text"], title=job["title"],
        )
    except Exception as exc:
        error = exc
    finally:
        heartbeat.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await heartbeat
        await finish_ingestion_job(job, error)


async def ingestion_queue_poller() -> None:
    while True:
        try:
            job = await claim_ingestion_job()
            if job:
                await run_claimed_ingestion_job(job)
                continue
        except asyncio.CancelledError:
            raise
        except Exception as exc:
            print(f"Ingestion queue poll failed: {exc}")
        await asyncio.sleep(INGESTION_POLL_SECONDS)


@app.post("/process")
async def process_document(req: ProcessRequest, background_tasks: BackgroundTasks):
    background_tasks.add_task(
        process_document_direct_request,
        doc_id=req.doc_id,
        file_path=req.file_path,
        source_id=req.source_id,
        run_id=req.run_id,
        source_type=req.source_type,
        source_url=req.source_url,
        raw_text=req.raw_text,
        title=req.title,
    )
    return {"message": "Processing started in background", "doc_id": req.doc_id}


async def process_document_direct_request(**kwargs):
    """Compatibility endpoint for manual development calls; durable work uses the DB queue."""
    try:
        await process_document_with_limit(**kwargs)
    except Exception as exc:
        print(f"Direct ingestion request failed: {exc}")


@app.post("/chat")
async def grounded_chat(req: ChatRequest):
    total_started = time.perf_counter()
    question = req.question.strip()
    if not question or len(question) > 4000:
        raise HTTPException(status_code=400, detail="question must contain 1 to 4000 characters")
    if is_explicit_graph_command(question):
        return graph_command_response(total_started)
    if not client:
        raise HTTPException(status_code=503, detail="Gemini client is not configured")

    embedding_started = time.perf_counter()
    query_values = await asyncio.to_thread(embed_query_text, question)
    embedding_ms = (time.perf_counter() - embedding_started) * 1000
    query_vector = str(query_values)
    retrieval_started = time.perf_counter()
    conn = await asyncpg.connect(DATABASE_URL)
    try:
        vector_rows = await conn.fetch("""
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type,
                   1 - (c.embedding::halfvec(3072) <=> $2::vector(3072)::halfvec(3072)) AS score
            FROM chunks c JOIN documents d ON d.id = c.document_id
            WHERE c.user_id = $1 AND d.user_id = $1 AND d.status = 'ready' AND c.embedding IS NOT NULL
            ORDER BY c.embedding::halfvec(3072) <=> $2::vector(3072)::halfvec(3072)
            LIMIT 20
        """, req.user_id, query_vector)
        lexical_rows = await conn.fetch("""
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type,
                   ts_rank_cd(to_tsvector('english', c.content), websearch_to_tsquery('english', $2)) AS score
            FROM chunks c JOIN documents d ON d.id = c.document_id
            WHERE c.user_id = $1 AND d.user_id = $1 AND d.status = 'ready'
              AND to_tsvector('english', c.content) @@ websearch_to_tsquery('english', $2)
            ORDER BY score DESC, c.id
            LIMIT 20
        """, req.user_id, question)
        graph_rows = await conn.fetch("""
            WITH nearest_concepts AS (
                SELECT id, 1 - (embedding::halfvec(3072) <=> $2::vector(3072)::halfvec(3072)) AS concept_score
                FROM concepts
                WHERE user_id = $1 AND embedding IS NOT NULL
                  AND state NOT IN ('rejected', 'archived')
                ORDER BY embedding::halfvec(3072) <=> $2::vector(3072)::halfvec(3072)
                LIMIT 8
            ), expanded_concepts AS (
                SELECT id, concept_score FROM nearest_concepts
                UNION
                SELECT CASE WHEN e.source_concept_id = nc.id THEN e.target_concept_id ELSE e.source_concept_id END,
                       nc.concept_score * 0.85 * GREATEST(e.confidence, 0.25)
                FROM nearest_concepts nc
                JOIN concept_edges e ON (e.source_concept_id = nc.id OR e.target_concept_id = nc.id)
                WHERE e.user_id = $1 AND e.state IN ('supported', 'confirmed') AND e.valid_to IS NULL
            )
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type,
                   MAX(ec.concept_score * cc.confidence) AS score
            FROM expanded_concepts ec
            JOIN chunk_concepts cc ON cc.concept_id = ec.id
            JOIN chunks c ON c.id = cc.chunk_id
            JOIN documents d ON d.id = c.document_id
            WHERE c.user_id = $1 AND d.user_id = $1 AND d.status = 'ready'
            GROUP BY c.id, c.document_id, d.title, c.page_start, c.content, c.locator, d.source_type
            ORDER BY score DESC, c.id
            LIMIT 20
        """, req.user_id, query_vector)
        learner_rows = await conn.fetch("""
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type,
                   MAX(lp.interest_score * cc.confidence) AS score
            FROM chat_learner_projection lp
            JOIN chunk_concepts cc ON cc.concept_id = lp.concept_id
            JOIN chunks c ON c.id = cc.chunk_id
            JOIN documents d ON d.id = c.document_id
            WHERE lp.user_id = $1 AND c.user_id = $1 AND d.user_id = $1
              AND lp.interest_score > 0 AND d.status = 'ready'
            GROUP BY c.id, c.document_id, d.title, c.page_start, c.content, c.locator, d.source_type
            ORDER BY score DESC, c.id LIMIT 20
        """, req.user_id)
        evidence_rows = await conn.fetch("""
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type, MAX(cc.confidence) AS score
            FROM chunk_concepts cc
            JOIN chunks c ON c.id = cc.chunk_id
            JOIN documents d ON d.id = c.document_id
            JOIN concepts concept ON concept.id = cc.concept_id
            WHERE c.user_id = $1 AND d.user_id = $1 AND d.status = 'ready'
              AND concept.state IN ('supported', 'confirmed')
            GROUP BY c.id, c.document_id, d.title, c.page_start, c.content, c.locator, d.source_type
            ORDER BY score DESC, c.id LIMIT 20
        """, req.user_id)
        recency_rows = await conn.fetch("""
            SELECT c.id AS chunk_id, c.document_id, d.title AS document_title,
                   c.page_start AS page, c.content, c.locator, d.source_type,
                   MAX(1.0 / (1.0 + EXTRACT(EPOCH FROM (now() - mc.created_at)) / 2592000.0)
                       + LEAST(mc.open_count, 5) * 0.05) AS score
            FROM message_citations mc
            JOIN chat_messages m ON m.id = mc.message_id
            JOIN chunks c ON c.id = mc.chunk_id
            JOIN documents d ON d.id = c.document_id
            WHERE m.user_id = $1 AND c.user_id = $1 AND d.user_id = $1
              AND d.status = 'ready' AND m.status <> 'superseded'
            GROUP BY c.id, c.document_id, d.title, c.page_start, c.content, c.locator, d.source_type
            ORDER BY score DESC, c.id LIMIT 20
        """, req.user_id)
    finally:
        await conn.close()

    def serialize(rows: Any) -> List[Dict[str, Any]]:
        serialized = []
        for row in rows:
            locator = row["locator"] or {}
            if isinstance(locator, str):
                locator = json.loads(locator)
            serialized.append({
                "chunk_id": str(row["chunk_id"]),
                "document_id": str(row["document_id"]),
                "document_title": row["document_title"],
                "page": row["page"],
                "content": row["content"],
                "locator": locator,
                "source_type": row["source_type"],
                "score": float(row["score"]),
            })
        return serialized

    ranked = reciprocal_rank_fusion({
        "vector": serialize(vector_rows),
        "lexical": serialize(lexical_rows),
        "graph": serialize(graph_rows),
        "learner": serialize(learner_rows),
        "evidence": serialize(evidence_rows),
        "recency": serialize(recency_rows),
    })
    retrieval_ms = (time.perf_counter() - retrieval_started) * 1000
    if requires_primary_source_verification(question):
        # Even a retrieved document titled like the original does not establish
        # identity or entailment. Do not let generated [Sx] labels manufacture
        # original-source evidence; require a future explicit primary-document
        # selection and verified claim witness before answering affirmatively.
        candidates = [{
            "chunk_id": item["chunk_id"], "rrf_score": item["rrf_score"],
            "signals": item["signals"],
        } for item in ranked]
        return {
            "answer": "I cannot verify what the original source used from primary-source evidence here. "
                      "A later paper's comparison baseline does not establish what the original paper did. "
                      "Check the original document before making that attribution.",
            "model_version": "deterministic-primary-source-abstention-v1",
            "ranking_policy": "hybrid-rrf-v1",
            "citations": [], "candidates": candidates,
            "metrics": {
                "embedding_ms": embedding_ms, "retrieval_ms": retrieval_ms,
                "generation_ms": 0, "total_ms": (time.perf_counter() - total_started) * 1000,
                "candidate_count": len(candidates), "model_calls": 1,
            },
        }
    if not ranked:
        return {
            "answer": "I could not find evidence for that question in your processed library.",
            "model_version": "deterministic-no-evidence-v1",
            "ranking_policy": "hybrid-rrf-v1",
            "citations": [],
            "candidates": [],
            "metrics": {
                "embedding_ms": embedding_ms,
                "retrieval_ms": retrieval_ms,
                "generation_ms": 0,
                "total_ms": (time.perf_counter() - total_started) * 1000,
                "candidate_count": 0,
                "model_calls": 1,
            },
        }

    sources = []
    citations = []
    for rank, candidate in enumerate(ranked, start=1):
        quote = candidate["content"][:1000]
        location = f"page {candidate['page']}" if candidate["source_type"] == "pdf" else candidate["locator"].get("heading", "source block")
        sources.append(f"[S{rank}] {candidate['document_title']}, {location}\n{quote}")
        citations.append({
            "chunk_id": candidate["chunk_id"],
            "document_id": candidate["document_id"],
            "document_title": candidate["document_title"],
            "page": candidate["page"],
            "rank": rank,
            "score": candidate["rrf_score"],
            "quote": quote,
            "source_type": candidate["source_type"],
            "locator": candidate["locator"],
        })

    history_text = "\n".join(
        f"{item.role.upper()}: {item.content[:1200]}" for item in req.history[-6:]
    )
    prompt = f"""You are SELAR, a research-library assistant.

Answer the question only from the supplied evidence. Cite factual statements using [S1], [S2], and so on. If the evidence is incomplete or conflicting, say so explicitly. Never invent a citation. Keep the answer concise and useful.

RECENT CONVERSATION:
{history_text}

QUESTION:
{question}

EVIDENCE:
{chr(10).join(sources)}
"""
    generation_started = time.perf_counter()
    response = await asyncio.to_thread(
        client.models.generate_content,
        model=TEXT_MODEL,
        contents=prompt,
    )
    generation_ms = (time.perf_counter() - generation_started) * 1000
    answer_text = (response.text or "").strip()
    if not answer_text:
        raise HTTPException(status_code=502, detail="answer model returned no text")

    cited_ranks = referenced_citation_ranks(answer_text, len(citations))
    citations = [citation for citation in citations if citation["rank"] in cited_ranks]
    if not citations:
        answer_text += "\n\nNo library citation was produced for this answer; treat it as unsupported."

    candidates = [{
        "chunk_id": item["chunk_id"],
        "rrf_score": item["rrf_score"],
        "signals": item["signals"],
    } for item in ranked]
    return {
        "answer": answer_text,
        "model_version": TEXT_MODEL,
        "ranking_policy": "hybrid-rrf-v1",
        "citations": citations,
        "candidates": candidates,
        "metrics": {
            "embedding_ms": embedding_ms,
            "retrieval_ms": retrieval_ms,
            "generation_ms": generation_ms,
            "total_ms": (time.perf_counter() - total_started) * 1000,
            "candidate_count": len(candidates),
            "model_calls": 2,
        },
    }

@app.get("/health")
def health():
    return {"status": "ok"}
