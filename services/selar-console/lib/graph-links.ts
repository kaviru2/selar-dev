// graph-links.ts: how the Graph presents cross-reading links (issue #117).
//
// Three kinds of link can join two readings:
//   - "candidate": proposed by SELAR from an exact concept named in both
//     readings, NOT reviewed. Drawn dashed in its own "To review" layer.
//   - "yours": a link you confirmed or relabelled (or another change you made).
//     Drawn solid.
//   - "pdf": structure extracted from one reading (claim, assumptions, concepts).
// Similarity-only passage matches are never graph links.
import type { GraphEdge, GraphNode } from "./api";

export type LinkLayer = "candidate" | "yours" | "pdf";

type EdgeLike = Pick<GraphEdge, "state" | "created_via"> & { candidate_link_id?: string };

const INTERACTION = ["deterministic_chat", "user_confirmed", "user_created", "user_reviewed"];
const INACTIVE = ["rejected", "archived", "superseded"];

export function isCandidateLink(edge: EdgeLike): boolean {
  return edge.state === "candidate" && (edge.created_via === "ai_suggested" || Boolean(edge.candidate_link_id));
}

export function linkLayer(edge: EdgeLike): LinkLayer {
  if (isCandidateLink(edge)) return "candidate";
  if (INTERACTION.includes(edge.created_via)) return "yours";
  return "pdf";
}

/** Canvas dash pattern (in screen pixels) for a link: unreviewed or inactive = dashed. */
export function linkDash(edge: EdgeLike): number[] {
  if (isCandidateLink(edge)) return [6, 4];
  if (INACTIVE.includes(edge.state)) return [2, 3];
  return [];
}

type EndpointEdge = EdgeLike & { source: string; target: string };

export interface CrossReadingSummary {
  readings: number;
  reviewed: number;
  candidates: number;
  /** Readings with no reviewed or suggested link to another reading. */
  unconnected: number;
}

function isReadingNode(id: string, readingIds: Set<string>) {
  return readingIds.has(id);
}

export function crossReadingSummary(nodes: Pick<GraphNode, "id" | "node_type">[], edges: EndpointEdge[]): CrossReadingSummary {
  const readingIds = new Set(nodes.filter((n) => n.node_type === "document").map((n) => n.id));
  const connected = new Set<string>();
  let reviewed = 0;
  let candidates = 0;
  for (const edge of edges) {
    if (!isReadingNode(edge.source, readingIds) || !isReadingNode(edge.target, readingIds) || edge.source === edge.target) continue;
    if (INACTIVE.includes(edge.state)) continue;
    if (isCandidateLink(edge)) candidates += 1;
    else reviewed += 1;
    connected.add(edge.source);
    connected.add(edge.target);
  }
  return { readings: readingIds.size, reviewed, candidates, unconnected: readingIds.size - connected.size };
}

export interface GraphNotice {
  title: string;
  body: string;
}

/** Honest explanation of why readings are (un)connected, or null when nothing needs saying. */
export function crossReadingNotice(summary: CrossReadingSummary, candidatesShown: boolean): GraphNotice | null {
  if (summary.readings < 2) return null;
  const how = "SELAR suggests a link only when two readings both name the same concept in their own text. "
    + "Passages that are merely similar appear in the Reader as similar passages to compare. They are not links.";
  if (summary.candidates > 0) {
    const n = summary.candidates;
    return {
      title: `${n} suggested link${n === 1 ? "" : "s"} to review`,
      body: (candidatesShown
        ? "Dashed lines are prompts for reflection, not established relationships. "
        : "Turn on “To review” to see them. ")
        + "Select one to compare both source passages in the Reader, try an optional reflection, or quietly hide a wrong prompt. Opening or flagging a prompt is not evidence of recall or mastery.",
    };
  }
  if (summary.reviewed === 0) {
    return {
      title: "Your readings are not linked yet",
      body: `${how} No reading pair has a shared named concept to review yet. Open a reading and use its Connections panel; new uploads are checked against your whole library.`,
    };
  }
  if (summary.unconnected > 0) {
    return {
      title: `${summary.unconnected} reading${summary.unconnected === 1 ? " is" : "s are"} not linked to the others`,
      body: how,
    };
  }
  return null;
}

/** Reader URL that opens the guided review for a candidate link. */
export function candidateReviewURL(edge: Pick<GraphEdge, "source_document_id"> & { candidate_link_id?: string }): string | null {
  if (!edge.source_document_id || !edge.candidate_link_id) return null;
  const params = new URLSearchParams({ docId: edge.source_document_id, linkId: edge.candidate_link_id });
  return `/reader?${params}`;
}
