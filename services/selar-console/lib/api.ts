// api.ts — Typed fetch wrapper for the SELAR Go API.
// Server-side calls use the internal Docker network URL;
// client-side calls go through Next.js API routes which proxy with httpOnly cookie.

interface ApiError {
  error: string;
}

/**
 * Server-side API fetch (used in Server Components / Route Handlers).
 * Requires an explicit token parameter.
 */
export async function serverFetch<T>(
  path: string,
  token: string,
  init?: RequestInit
): Promise<T> {
  const base = process.env.API_INTERNAL_URL || "http://localhost:8080";
  const url = `${base}${path}`;
  const res = await fetch(url, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...init?.headers,
    },
    cache: "no-store",
  });

  if (!res.ok) {
    const body = (await res.json().catch(() => ({
      error: "Unknown error",
    }))) as ApiError;
    throw new Error(body.error || `API error: ${res.status}`);
  }

  return res.json() as Promise<T>;
}

// --- Document types ---

export interface Document {
  id: string;
  user_id: string;
  title: string;
  authors: string;
  year: number;
  page_count: number;
  status: "uploaded" | "processing" | "ready" | "failed";
  progress: number;
  added_at: string;
  processed_at: string | null;
}

export interface DocumentStats {
  total_documents: number;
  total_chunks: number;
  confirmed_links: number;
  reading_time_min: number;
}

// --- Suggestion types ---

export interface LinkSuggestion {
  id: string;
  user_id: string;
  source_chunk_id: string;
  target_chunk_id: string;
  similarity: number;
  relation: string;
  status: "pending" | "confirmed" | "rejected" | "relabeled" | "expired";
  user_label: string | null;
  src_text: string;
  tgt_text: string;
  src_doc: string;
  tgt_doc: string;
  tgt_page: number;
  summary: string;
  suggested_at: string;
  responded_at: string | null;
  src_bboxes: Array<{ x: number; y: number; w: number; h: number }> | string;
}

// --- Concept types ---

export interface Concept {
  id: string;
  user_id: string;
  name: string;
  description: string;
  state: "candidate" | "supported" | "confirmed" | "rejected" | "archived";
  model_version?: string;
  prompt_version?: string;
  created_at: string;
}

export interface ConceptEdge {
  id: string;
  user_id: string;
  source_concept_id: string;
  target_concept_id: string;
  relation: string;
  created_via: string;
  state: "candidate" | "supported" | "confirmed" | "rejected" | "archived";
  confidence: number;
  confirmed_at: string | null;
  created_at: string;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export type GraphNodeType = "concept" | "document" | "claim" | "assumption" | "question";

export interface GraphNode {
  id: string;
  name: string;
  description: string;
  node_type: GraphNodeType;
  state: string;
  document_id?: string;
  document_title?: string;
  confidence?: number;
  created_at: string;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  relation: string;
  state: string;
  confidence?: number;
  created_via: string;
  explanation?: string;
}

// --- Runtime mental-model types ---

export type MentalLinkType =
  | "concept_overlap"
  | "claim_extension"
  | "assumption_conflict"
  | "question_resolution";

export interface DocumentMentalModel {
  id: string;
  document_id: string;
  user_id: string;
  document_title: string;
  version: number;
  main_claim: string;
  key_concepts: string[];
  assumptions: string[];
  open_questions: string[];
  domain: string;
  model_version: string;
  prompt_version: string;
  status: "draft" | "ready" | "failed" | "superseded";
  generated_at: string;
}

export interface MentalModelLink {
  id: string;
  user_id: string;
  source_model_id: string;
  target_model_id: string;
  source_document_id: string;
  target_document_id: string;
  source_document_title: string;
  target_document_title: string;
  link_type: MentalLinkType;
  similarity: number;
  confidence: number;
  bridge_explanation: string;
  source_evidence_chunk_id?: string;
  target_evidence_chunk_id?: string;
  source_evidence?: string;
  target_evidence?: string;
  status: "candidate" | "confirmed" | "rejected" | "relabeled" | "archived";
  created_via: string;
  user_label?: string;
  suggested_at: string;
  responded_at?: string;
}

export interface LearnerConceptState {
  user_id: string;
  concept_id: string;
  concept_name: string;
  mastery_estimate: number;
  recall_probability: number;
  half_life_seconds: number;
  evidence_count: number;
  uncertainty: number;
  updated_at: string;
}

// --- Annotation types ---

export interface Annotation {
  id: string;
  user_id: string;
  document_id: string;
  chunk_id: string | null;
  page: number;
  bbox: Array<{ x: number; y: number; w: number; h: number }> | string;
  color: "wheat" | "yellow" | "coral" | "sage";
  type: "highlight" | "underline" | "note" | "suggestion";
  comment: string;
  created_at: string;
  updated_at: string;
}

// --- Server-side API functions ---

export function getDocuments(token: string): Promise<Document[]> {
  return serverFetch("/api/documents", token);
}

export function getDocument(id: string, token: string): Promise<Document> {
  return serverFetch(`/api/documents/${id}`, token);
}

export function getDocumentStats(token: string): Promise<DocumentStats> {
  return serverFetch("/api/documents/stats", token);
}

export function getSuggestions(
  docId: string,
  page: number,
  token: string
): Promise<LinkSuggestion[]> {
  return serverFetch(`/api/documents/${docId}/suggestions?page=${page}`, token);
}

export function getAnnotations(
  docId: string,
  page: number,
  token: string
): Promise<Annotation[]> {
  return serverFetch(`/api/documents/${docId}/annotations?page=${page}`, token);
}

export function getConcepts(token: string): Promise<Concept[]> {
  return serverFetch("/api/concepts", token);
}

export function getGraph(token: string): Promise<GraphData> {
  return serverFetch("/api/graph", token);
}

// --- Client-side API fetch wrapper ---
// Automatically targets the Next.js API proxy routes, which attach the httpOnly
// cookie before forwarding to the Golang backend.

export async function clientFetch<T>(
  path: string, // should start with '/api/'
  init?: RequestInit
): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) {
    const errorBody = await res.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(errorBody.error || `HTTP error ${res.status}`);
  }
  return res.json() as Promise<T>;
}

// Example usage: const docs = await clientFetch<Document[]>('/api/documents');
