// route.ts — Catch-all proxy for the Go API.
// Allows client components to fetch from Next.js `/api/...` and transparently
// forwards the request to the internal Go API, attaching the httpOnly JWT.

import { NextResponse } from "next/server";
import { getAuthToken } from "@/lib/auth";
import { expireAuthCookie } from "@/lib/auth-cookie";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ go_api: string[] }> }
) {
  return handleProxyRequest(request, await params);
}

export async function POST(
  request: Request,
  { params }: { params: Promise<{ go_api: string[] }> }
) {
  return handleProxyRequest(request, await params);
}

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ go_api: string[] }> }
) {
  return handleProxyRequest(request, await params);
}

export async function PATCH(
  request: Request,
  { params }: { params: Promise<{ go_api: string[] }> }
) {
  return handleProxyRequest(request, await params);
}

export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ go_api: string[] }> }
) {
  return handleProxyRequest(request, await params);
}

async function handleProxyRequest(
  request: Request,
  params: { go_api: string[] }
) {
  const token = await getAuthToken();
  if (!token) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  // Next.js params.go_api is an array of path segments
  const path = params.go_api.join("/");
  
  // Forward query string too
  const url = new URL(request.url);
  const targetUrl = `${API_BASE}/api/${path}${url.search}`;

  try {
    const headers = new Headers(request.headers);
    headers.set("Authorization", `Bearer ${token}`);
    
    // We shouldn't forward Host or Cookie headers directly to avoid confusion
    headers.delete("host");

    const init: RequestInit = {
      method: request.method,
      headers,
      cache: "no-store",
      // Object-storage downloads come back as 302s to short-lived signed
      // URLs; hand them to the browser instead of proxying the bytes.
      redirect: "manual",
    };

    if (request.method !== "GET" && request.method !== "HEAD") {
      init.body = request.body;
      const streamingInit = init as RequestInit & { duplex?: "half" };
      streamingInit.duplex = "half"; // Required for Node 18+ streaming request bodies
    }

    const res = await fetch(targetUrl, init);
    if (res.status >= 300 && res.status < 400 && res.headers.get("Location")) {
      return new NextResponse(null, {
        status: res.status,
        headers: {
          Location: res.headers.get("Location") as string,
          "Cache-Control": res.headers.get("Cache-Control") || "private, no-store",
        },
      });
    }
    const contentType = res.headers.get("Content-Type") || "";

    // If it's a JSON response, we can safely read text, but for streaming/binaries
    // it's best to return the raw body as a standard stream.
    // However, Next.js requires returning the body directly.
    const outHeaders: Record<string, string> = { "Content-Type": contentType };
    // Downloads (e.g. the data export zip) need their filename and caching rules.
    for (const name of ["Content-Disposition", "Cache-Control"]) {
      const value = res.headers.get(name);
      if (value) outHeaders[name] = value;
    }
    const response = new NextResponse(res.body, { status: res.status, headers: outHeaders });
    // The Go API rejected our JWT: drop the cookie so the proxy stops treating
    // the browser as signed in and /login is reachable again (no redirect loop).
    return res.status === 401 ? expireAuthCookie(response) : response;
  } catch (error) {
    console.error("Proxy error:", error);
    return NextResponse.json({ error: "gateway error" }, { status: 502 });
  }
}
