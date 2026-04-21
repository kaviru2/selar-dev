// api.ts — Typed fetch wrapper for the SELAR Go API.
// Server-side calls use the internal Docker network URL;
// client-side calls go through Next.js API routes which proxy with httpOnly cookie.

const API_BASE =
  typeof window === "undefined"
    ? process.env.API_INTERNAL_URL || "http://localhost:8080"
    : "";  // Client-side: use relative paths through Next.js API routes

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
  suggested_at: string;
  responded_at: string | null;
}

// --- Concept types ---

export interface Concept {
  id: string;
  user_id: string;
  name: string;
  description: string;
  created_at: string;
}

export interface ConceptEdge {
  id: string;
  user_id: string;
  source_concept_id: string;
  target_concept_id: string;
  relation: string;
  created_via: string;
  confirmed_at: string | null;
  created_at: string;
}

export interface GraphData {
  nodes: Concept[];
  edges: ConceptEdge[];
}

// --- Annotation types ---

export interface Annotation {
  id: string;
  user_id: string;
  document_id: string;
  chunk_id: string | null;
  page: number;
  bbox: Record<string, number>;
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
