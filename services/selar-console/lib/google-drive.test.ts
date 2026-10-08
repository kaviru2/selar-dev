import { afterEach, describe, expect, it, vi } from "vitest";
import { DRIVE_FILE_SCOPE, driveImportConfig, driveStatusLabel, importDriveFile } from "./google-drive";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("google-drive", () => {
  it("requests only the drive.file scope", () => {
    expect(DRIVE_FILE_SCOPE).toBe("https://www.googleapis.com/auth/drive.file");
  });

  it("is disabled unless both the client id and the Picker key are set", () => {
    expect(driveImportConfig()).toBeNull();
    vi.stubEnv("NEXT_PUBLIC_GOOGLE_CLIENT_ID", "834233085605-abc.apps.googleusercontent.com");
    expect(driveImportConfig()).toBeNull();
    vi.stubEnv("NEXT_PUBLIC_GOOGLE_PICKER_API_KEY", "key");
    expect(driveImportConfig()).toEqual({ clientId: "834233085605-abc.apps.googleusercontent.com", apiKey: "key", appId: "834233085605" });
  });

  it("labels are honest", () => {
    expect(driveStatusLabel(true)).toBe("Import from Google Drive");
    expect(driveStatusLabel(false)).toBe("Google Drive import not set up");
  });

  it("posts the picked file id and token to the import endpoint and surfaces API errors", async () => {
    const fetchMock = vi.fn(async () => Response.json({}, { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
    await importDriveFile("file123", "tok");
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/documents/import/drive");
    expect(JSON.parse(String(init.body))).toEqual({ file_id: "file123", access_token: "tok" });

    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "file too large" }, { status: 413 })));
    await expect(importDriveFile("f", "t")).rejects.toThrow("file too large");
  });
});
