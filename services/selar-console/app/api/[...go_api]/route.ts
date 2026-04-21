// route.ts — Catch-all proxy for the Go API.
// Allows client components to fetch from Next.js `/api/...` and transparently
// forwards the request to the internal Go API, attaching the httpOnly JWT.

import { NextResponse } from "next/server";
import { getAuthToken } from "@/lib/auth";

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
    };

    if (request.method !== "GET" && request.method !== "HEAD") {
      init.body = request.body;
      (init as any).duplex = "half"; // Required for Node 18+ streaming request bodies
    }

    const res = await fetch(targetUrl, init);
    const contentType = res.headers.get("Content-Type") || "";

    // If it's a JSON response, we can safely read text, but for streaming/binaries
    // it's best to return the raw body as a standard stream.
    // However, Next.js requires returning the body directly.
    return new NextResponse(res.body, {
      status: res.status,
      headers: {
        "Content-Type": contentType,
      },
    });
  } catch (error) {
    console.error("Proxy error:", error);
    return NextResponse.json({ error: "gateway error" }, { status: 502 });
  }
}
