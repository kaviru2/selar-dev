import { describe, expect, it } from "vitest";
import { createPositionStore, DEFAULT_READER_PREFERENCES, readerPreferences } from "./position";

function memoryStorage() {
  const data = new Map<string, string>();
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => void data.set(key, value),
    removeItem: (key: string) => void data.delete(key),
    data,
  };
}

describe("createPositionStore", () => {
  it("remembers the last page, offset and zoom per document", () => {
    const storage = memoryStorage();
    const store = createPositionStore(storage, () => 1000);
    store.save("doc-a", { page: 7, fraction: 0.4, zoom: "fit-width" });
    store.save("doc-b", { page: 2, fraction: 0, zoom: 1.25 });
    expect(store.load("doc-a")).toEqual({ page: 7, fraction: 0.4, zoom: "fit-width" });
    expect(store.load("doc-b")).toEqual({ page: 2, fraction: 0, zoom: 1.25 });
    expect(store.load("doc-c")).toBeNull();
  });

  it("drops corrupted or out-of-range entries instead of throwing", () => {
    const storage = memoryStorage();
    storage.setItem("selar.reader.position.v1", "{not json");
    const store = createPositionStore(storage, () => 1000);
    expect(store.load("doc-a")).toBeNull();
    storage.setItem("selar.reader.position.v1", JSON.stringify({ "doc-a": { page: -2, fraction: 9, zoom: "huge", at: 1 } }));
    expect(store.load("doc-a")).toBeNull();
  });

  it("keeps at most 50 documents, evicting the least recently read", () => {
    const storage = memoryStorage();
    let now = 0;
    const store = createPositionStore(storage, () => ++now);
    for (let i = 0; i < 55; i++) store.save(`doc-${i}`, { page: 1, fraction: 0, zoom: 1 });
    expect(store.load("doc-0")).toBeNull();
    expect(store.load("doc-4")).toBeNull();
    expect(store.load("doc-5")).not.toBeNull();
    expect(store.load("doc-54")).not.toBeNull();
  });

  it("works without storage (private mode) as a no-op", () => {
    const store = createPositionStore(null, () => 0);
    store.save("doc-a", { page: 3, fraction: 0, zoom: 1 });
    expect(store.load("doc-a")).toBeNull();
  });
});

describe("readerPreferences", () => {
  it("falls back to local defaults when the account has no reader settings", () => {
    expect(readerPreferences(undefined)).toEqual(DEFAULT_READER_PREFERENCES);
    expect(readerPreferences({ theme: "paper" })).toEqual(DEFAULT_READER_PREFERENCES);
  });

  it("reads a typed reader block from account preferences and ignores invalid fields", () => {
    expect(readerPreferences({ reader: { defaultZoom: "fit-page", rememberPosition: false, showThumbnails: true, highlightColor: "blue" } }))
      .toEqual({ ...DEFAULT_READER_PREFERENCES, defaultZoom: "fit-page", rememberPosition: false, showThumbnails: true, highlightColor: "blue" });
    expect(readerPreferences({ reader: { defaultZoom: 99, highlightColor: "chartreuse", rememberPosition: "yes" } }))
      .toEqual(DEFAULT_READER_PREFERENCES);
  });
});
