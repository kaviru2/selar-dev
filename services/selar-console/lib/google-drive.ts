// google-drive.ts — "Import from Google Drive" (Google Picker + drive.file).
//
// Flow, all on user click (incremental consent, separate from sign-in):
//   1. Load Google Identity Services and gapi's Picker on demand.
//   2. Ask for a short-lived access token with ONLY the drive.file scope.
//      drive.file grants access to files the user picks for SELAR, not to the
//      rest of their Drive.
//   3. Show the Picker filtered to PDFs; the user picks one file.
//   4. POST { file_id, access_token } to the Go API, which downloads the PDF
//      server-side and queues it like an upload. The token lives only in this
//      page's memory and that one request; it is not stored.
//
// Feature flag: requires NEXT_PUBLIC_GOOGLE_CLIENT_ID and
// NEXT_PUBLIC_GOOGLE_PICKER_API_KEY (both public, inlined at build time).

export const DRIVE_FILE_SCOPE = "https://www.googleapis.com/auth/drive.file";
export const GIS_SRC = "https://accounts.google.com/gsi/client";
export const GAPI_SRC = "https://apis.google.com/js/api.js";

export interface DriveConfig {
  clientId: string;
  apiKey: string;
  /** Cloud project number: lets drive.file cover picked files (Picker setAppId). */
  appId: string;
}

export function driveImportConfig(): DriveConfig | null {
  const clientId = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID?.trim();
  const apiKey = process.env.NEXT_PUBLIC_GOOGLE_PICKER_API_KEY?.trim();
  if (!clientId || !apiKey) return null;
  // The project number is the numeric prefix of a Google OAuth client id.
  const appId = process.env.NEXT_PUBLIC_GOOGLE_APP_ID?.trim() || clientId.split("-")[0];
  return { clientId, apiKey, appId };
}

export function driveImportEnabled(): boolean {
  return driveImportConfig() !== null;
}

/** What the sidebar says about Drive. Honest: nothing is "connected". */
export function driveStatusLabel(enabled: boolean): string {
  return enabled ? "Import from Google Drive" : "Google Drive import not set up";
}

// ---- minimal typings for the Google globals we use ----
interface TokenResponse {
  access_token?: string;
  error?: string;
  scope?: string;
}
interface TokenClient {
  requestAccessToken(overrides?: { prompt?: string }): void;
}
interface PickerDoc {
  id: string;
  name?: string;
  mimeType?: string;
}
interface PickerResult {
  action: string;
  docs?: PickerDoc[];
}
interface GoogleGlobal {
  accounts: {
    oauth2: {
      initTokenClient(cfg: {
        client_id: string;
        scope: string;
        include_granted_scopes?: boolean;
        callback: (r: TokenResponse) => void;
        error_callback?: (e: { type: string }) => void;
      }): TokenClient;
    };
  };
  picker: {
    Action: { PICKED: string; CANCEL: string };
    ViewId: { DOCS: string };
    DocsView: new (viewId?: string) => {
      setMimeTypes(m: string): unknown;
      setIncludeFolders(b: boolean): unknown;
      setSelectFolderEnabled(b: boolean): unknown;
    };
    PickerBuilder: new () => PickerBuilder;
  };
}
interface PickerBuilder {
  addView(v: unknown): PickerBuilder;
  setOAuthToken(t: string): PickerBuilder;
  setDeveloperKey(k: string): PickerBuilder;
  setAppId(id: string): PickerBuilder;
  setCallback(cb: (r: PickerResult) => void): PickerBuilder;
  setTitle(t: string): PickerBuilder;
  build(): { setVisible(v: boolean): void };
}
declare global {
  interface Window {
    google?: GoogleGlobal;
    gapi?: { load(lib: string, cb: { callback: () => void; onerror: () => void }): void };
  }
}

const loading = new Map<string, Promise<void>>();

export function loadScript(src: string): Promise<void> {
  const existing = loading.get(src);
  if (existing) return existing;
  const p = new Promise<void>((resolve, reject) => {
    const el = document.createElement("script");
    el.src = src;
    el.async = true;
    el.onload = () => resolve();
    el.onerror = () => {
      loading.delete(src);
      reject(new Error("Couldn't load Google's Drive picker. Check your connection or content blockers."));
    };
    document.head.appendChild(el);
  });
  loading.set(src, p);
  return p;
}

async function loadGoogle(): Promise<GoogleGlobal> {
  await Promise.all([loadScript(GIS_SRC), loadScript(GAPI_SRC)]);
  await new Promise<void>((resolve, reject) =>
    window.gapi!.load("picker", { callback: resolve, onerror: () => reject(new Error("Couldn't load the Drive picker.")) }),
  );
  return window.google!;
}

function requestDriveToken(google: GoogleGlobal, clientId: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const client = google.accounts.oauth2.initTokenClient({
      client_id: clientId,
      scope: DRIVE_FILE_SCOPE,
      include_granted_scopes: false,
      callback: (r) => {
        if (r.error || !r.access_token) {
          reject(new DriveCancelled(r.error === "access_denied" ? "Drive access wasn't granted." : "Google didn't grant Drive access."));
          return;
        }
        resolve(r.access_token);
      },
      error_callback: () => reject(new DriveCancelled("Drive access was cancelled.")),
    });
    client.requestAccessToken();
  });
}

export class DriveCancelled extends Error {}

function pickPdf(google: GoogleGlobal, cfg: DriveConfig, token: string): Promise<PickerDoc | null> {
  return new Promise((resolve) => {
    const view = new google.picker.DocsView(google.picker.ViewId.DOCS);
    view.setMimeTypes("application/pdf");
    view.setIncludeFolders(true);
    view.setSelectFolderEnabled(false);
    new google.picker.PickerBuilder()
      .addView(view)
      .setOAuthToken(token)
      .setDeveloperKey(cfg.apiKey)
      .setAppId(cfg.appId)
      .setTitle("Choose a PDF to import into SELAR")
      .setCallback((r) => {
        if (r.action === google.picker.Action.PICKED && r.docs?.[0]) resolve(r.docs[0]);
        else if (r.action === google.picker.Action.CANCEL) resolve(null);
      })
      .build()
      .setVisible(true);
  });
}

/** Send the picked file to the API, which downloads and queues it. */
export async function importDriveFile(fileId: string, accessToken: string): Promise<void> {
  const res = await fetch("/api/documents/import/drive", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ file_id: fileId, access_token: accessToken }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error((data && typeof data.error === "string" && data.error) || "Import from Google Drive failed");
  }
}

/**
 * Runs the whole flow. Resolves with the imported file name, or null if the
 * user closed the picker.
 */
export async function importFromDrive(): Promise<string | null> {
  const cfg = driveImportConfig();
  if (!cfg) throw new Error("Google Drive import isn't set up.");
  const google = await loadGoogle();
  const token = await requestDriveToken(google, cfg.clientId);
  const doc = await pickPdf(google, cfg, token);
  if (!doc) return null;
  await importDriveFile(doc.id, token);
  return doc.name ?? "PDF";
}
