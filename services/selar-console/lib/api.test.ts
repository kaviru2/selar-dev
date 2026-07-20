import { describe, it, expect, vi, beforeEach } from "vitest";
import { clientFetch } from "./api";
import type { Document, LinkSuggestion, DocumentStats } from "./api";

// Mock global fetch
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

beforeEach(() => {
  mockFetch.mockReset();
});

describe("clientFetch", () => {
  it("returns parsed JSON on success", async () => {
    const mockDocs: Document[] = [
      {
        id: "doc-1",
        user_id: "user-1",
        title: "Test Paper",
        authors: "Author",
        year: 2025,
        page_count: 10,
        status: "ready",
        progress: 1,
        source_type: "pdf",
        added_at: "2025-01-01T00:00:00Z",
        processed_at: null,
      },
    ];

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockDocs),
    });

    const result = await clientFetch<Document[]>("/api/documents");
    expect(result).toEqual(mockDocs);
    expect(mockFetch).toHaveBeenCalledWith("/api/documents", undefined);
  });

  it("throws on non-ok response with error message", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: "invalid credentials" }),
    });

    await expect(clientFetch("/api/auth/login")).rejects.toThrow(
      "invalid credentials"
    );
  });

  it("throws generic error when body parse fails", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: () => Promise.reject(new Error("not json")),
    });

    await expect(clientFetch("/api/documents")).rejects.toThrow(
      "Unknown error"
    );
  });

  it("passes init options to fetch", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ status: "deleted" }),
    });

    await clientFetch("/api/documents/abc", { method: "DELETE" });
    expect(mockFetch).toHaveBeenCalledWith("/api/documents/abc", {
      method: "DELETE",
    });
  });
});

describe("Type definitions", () => {
  it("Document status union covers all values", () => {
    const statuses: Document["status"][] = [
      "uploaded",
      "processing",
      "ready",
      "failed",
    ];
    expect(statuses).toHaveLength(4);
  });

  it("LinkSuggestion status union covers all values", () => {
    const statuses: LinkSuggestion["status"][] = [
      "pending",
      "confirmed",
      "rejected",
      "relabeled",
      "expired",
    ];
    expect(statuses).toHaveLength(5);
  });

  it("DocumentStats has required numeric fields", () => {
    const stats: DocumentStats = {
      total_documents: 5,
      total_chunks: 120,
      confirmed_links: 8,
      reading_time_min: 45.5,
    };
    expect(stats.total_documents).toBe(5);
    expect(stats.reading_time_min).toBe(45.5);
  });
});
