"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { clientFetch, type ChatMessage, type ChatThread } from "@/lib/api";

export default function ChatPage() {
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [activeThread, setActiveThread] = useState<string>("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  const loadThreads = useCallback(async () => {
    const data = await clientFetch<ChatThread[]>("/api/chat/threads");
    setThreads(data);
    setActiveThread((current) => current || data[0]?.id || "");
    setLoading(false);
  }, []);

  const loadMessages = useCallback(async (threadId: string) => {
    if (!threadId) {
      setMessages([]);
      return;
    }
    const data = await clientFetch<ChatMessage[]>(`/api/chat/threads/${threadId}/messages`);
    setMessages(data);
  }, []);

  useEffect(() => {
    let cancelled = false;
    clientFetch<ChatThread[]>("/api/chat/threads")
      .then((data) => {
        if (cancelled) return;
        setThreads(data);
        setActiveThread((current) => current || data[0]?.id || "");
        setLoading(false);
      })
      .catch((loadError) => {
        if (cancelled) return;
        setError(loadError instanceof Error ? loadError.message : "Unable to load conversations");
        setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!activeThread) return;
    let cancelled = false;
    clientFetch<ChatMessage[]>(`/api/chat/threads/${activeThread}/messages`)
      .then((data) => {
        if (!cancelled) setMessages(data);
      })
      .catch((loadError) => {
        if (!cancelled) setError(loadError instanceof Error ? loadError.message : "Unable to load messages");
      });
    return () => { cancelled = true; };
  }, [activeThread]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, sending]);

  async function createThread() {
    const thread = await clientFetch<ChatThread>("/api/chat/threads", { method: "POST" });
    setThreads((current) => [thread, ...current]);
    setActiveThread(thread.id);
    setMessages([]);
    return thread.id;
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const question = input.trim();
    if (!question || sending) return;
    setSending(true);
    setError("");
    setInput("");
    try {
      const threadId = activeThread || await createThread();
      await clientFetch<ChatMessage>(`/api/chat/threads/${threadId}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: question }),
      });
      await Promise.all([loadMessages(threadId), loadThreads()]);
    } catch (sendError) {
      setInput(question);
      setError(sendError instanceof Error ? sendError.message : "Unable to send message");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="chat-layout">
      <aside className="chat-sidebar">
        <div className="chat-sidebar-head">
          <div>
            <span className="chat-eyebrow">Library conversations</span>
            <h1>Research chat</h1>
          </div>
          <button className="chat-new" onClick={() => createThread().catch(() => setError("Unable to create conversation"))} aria-label="New conversation">+</button>
        </div>
        <div className="chat-thread-list">
          {loading && <span className="chat-muted">Loading conversations…</span>}
          {!loading && threads.length === 0 && <span className="chat-muted">Start a conversation with your processed library.</span>}
          {threads.map((thread) => (
            <button key={thread.id} className={thread.id === activeThread ? "active" : ""} onClick={() => setActiveThread(thread.id)}>
              <strong>{thread.title}</strong>
              <span>{new Date(thread.updated_at).toLocaleDateString()}</span>
            </button>
          ))}
        </div>
        <div className="chat-policy-note">
          <strong>Evidence-first</strong>
          <span>Answers use bounded hybrid retrieval. Graph updates are recorded as deterministic events.</span>
        </div>
      </aside>

      <main className="chat-main">
        <header className="chat-header">
          <div>
            <span className="chat-eyebrow">Adaptive context graph</span>
            <strong>{threads.find((thread) => thread.id === activeThread)?.title || "New conversation"}</strong>
          </div>
          <span className="chat-grounded-chip">Grounded in your library</span>
        </header>

        <div ref={scrollRef} className="chat-messages">
          {messages.length === 0 && !sending && (
            <div className="chat-welcome">
              <span className="chat-welcome-mark">S</span>
              <h2>Ask across your research library</h2>
              <p>Compare claims, trace assumptions, or ask how concepts connect. SELAR will show the passages behind its answer.</p>
              <div className="chat-prompts">
                {["Compare the main claims in my papers", "Where do the authors disagree?", "Explain a concept using cited evidence"].map((prompt) => (
                  <button key={prompt} onClick={() => setInput(prompt)}>{prompt}</button>
                ))}
              </div>
            </div>
          )}

          {messages.map((message) => (
            <article key={message.id} className={`chat-message ${message.role}`}>
              <div className="chat-message-label">{message.role === "user" ? "You" : "SELAR"}</div>
              <div className="chat-message-content">{message.content}</div>
              {message.citations.length > 0 && (
                <div className="chat-citations">
                  {message.citations.map((citation) => (
                    <Link key={citation.chunk_id} href={`/reader?docId=${citation.document_id}&page=${citation.page}`}>
                      <span>[S{citation.rank}]</span>
                      <strong>{citation.document_title}</strong>
                      <small>Page {citation.page} · {citation.quote.slice(0, 150)}…</small>
                    </Link>
                  ))}
                </div>
              )}
            </article>
          ))}
          {sending && <div className="chat-thinking"><span /><span /><span /> Retrieving evidence and assembling an answer…</div>}
        </div>

        <div className="chat-composer-wrap">
          {error && <div className="chat-error">{error}</div>}
          <form className="chat-composer" onSubmit={submit}>
            <textarea value={input} onChange={(event) => setInput(event.target.value)} placeholder="Ask about claims, concepts, assumptions, or connections…" rows={2} maxLength={4000} onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }} />
            <button type="submit" disabled={!input.trim() || sending}>Ask</button>
          </form>
          <span className="chat-disclaimer">Answers are limited to retrieved library evidence and may identify gaps or conflicts.</span>
        </div>
      </main>
    </div>
  );
}
