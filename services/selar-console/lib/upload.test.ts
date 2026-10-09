import { afterEach, describe, expect, it, vi } from "vitest";
import { uploadPDF } from "./upload";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

afterEach(() => vi.unstubAllGlobals());

describe("uploadPDF", () => {
  const file = () => new File(["%PDF-1.7 synthetic"], "Synthetic Reading.pdf", { type: "application/pdf" });

  it("uses the multipart endpoint when the API reports local storage", async () => {
    const fetchSpy = vi.fn()
      .mockResolvedValueOnce(json({ mode: "multipart", max_bytes: 52428800 }))
      .mockResolvedValueOnce(json({ document: { id: "doc-1" } }, 202));
    vi.stubGlobal("fetch", fetchSpy);
    await uploadPDF(file());
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    const [planUrl, planInit] = fetchSpy.mock.calls[0];
    expect(planUrl).toBe("/api/documents/upload-url");
    expect(JSON.parse(planInit.body)).toEqual({ filename: "Synthetic Reading.pdf", size: 18, content_type: "application/pdf" });
    const [uploadUrl, uploadInit] = fetchSpy.mock.calls[1];
    expect(uploadUrl).toBe("/api/documents/upload");
    expect(uploadInit.body).toBeInstanceOf(FormData);
  });

  it("uploads directly to storage with the signed headers, then finalizes", async () => {
    const fetchSpy = vi.fn()
      .mockResolvedValueOnce(json({
        mode: "direct", upload_id: "upload-1",
        upload: { method: "PUT", url: "https://bucket.example/users/u/uploads/upload-1.pdf?X-Amz-Signature=sig", headers: { "Content-Type": "application/pdf" } },
      }))
      .mockResolvedValueOnce(new Response(null, { status: 200 }))
      .mockResolvedValueOnce(json({ document: { id: "doc-2" } }, 202));
    vi.stubGlobal("fetch", fetchSpy);
    await uploadPDF(file());
    const [putUrl, putInit] = fetchSpy.mock.calls[1];
    expect(putUrl).toContain("https://bucket.example/users/u/uploads/upload-1.pdf");
    expect(putInit.method).toBe("PUT");
    expect(putInit.headers).toEqual({ "Content-Type": "application/pdf" });
    expect(putInit.credentials).toBe("omit");
    expect(putInit.body).toBeInstanceOf(File);
    const [completeUrl, completeInit] = fetchSpy.mock.calls[2];
    expect(completeUrl).toBe("/api/documents/upload-complete");
    expect(JSON.parse(completeInit.body)).toEqual({ upload_id: "upload-1", filename: "Synthetic Reading.pdf" });
  });

  it("surfaces API rejections and storage failures without finalizing", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ error: "file too large" }, 413)));
    await expect(uploadPDF(file())).rejects.toThrow("file too large");

    const fetchSpy = vi.fn()
      .mockResolvedValueOnce(json({ mode: "direct", upload_id: "u", upload: { method: "PUT", url: "https://bucket.example/x", headers: {} } }))
      .mockResolvedValueOnce(new Response("denied", { status: 403 }));
    vi.stubGlobal("fetch", fetchSpy);
    await expect(uploadPDF(file())).rejects.toThrow(/storage rejected the upload/i);
    expect(fetchSpy).toHaveBeenCalledTimes(2);

    const failingComplete = vi.fn()
      .mockResolvedValueOnce(json({ mode: "direct", upload_id: "u", upload: { method: "PUT", url: "https://bucket.example/x", headers: {} } }))
      .mockResolvedValueOnce(new Response(null, { status: 200 }))
      .mockResolvedValueOnce(json({ error: "file is not a valid PDF" }, 400));
    vi.stubGlobal("fetch", failingComplete);
    await expect(uploadPDF(file())).rejects.toThrow("file is not a valid PDF");
  });

  it("rejects non-PDF files before contacting the API", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    await expect(uploadPDF(new File(["x"], "notes.doc", { type: "application/msword" }))).rejects.toThrow(/only PDF/i);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
