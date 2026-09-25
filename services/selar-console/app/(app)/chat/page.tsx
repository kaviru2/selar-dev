"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { clientFetch, type ChatMessage, type ChatThread } from "@/lib/api";

export default function ChatPage() {
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [activeThread, setActiveThread] = useState<string>("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [feedbackBusy, setFeedbackBusy] = useState("");
  const [commentFor, setCommentFor] = useState("");
  const [commentText, setCommentText] = useState("");
  const [correctionFor, setCorrectionFor] = useState("");
  const [correctionText, setCorrectionText] = useState("");
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

  async function deleteThread(threadId: string) {
    await clientFetch(`/api/chat/threads/${threadId}`, { method: "DELETE" });
    const remaining = threads.filter((thread) => thread.id !== threadId);
    setThreads(remaining);
    if (activeThread === threadId) {
      setActiveThread(remaining[0]?.id || "");
      setMessages([]);
    }
  }

  async function sendFeedback(messageId: string, action: "helpful" | "unhelpful" | "correction", note = "") {
    setFeedbackBusy(`${messageId}:${action}`);
    setError("");
    try {
      await clientFetch(`/api/chat/messages/${messageId}/feedback`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action,
          correction_text: action === "correction" ? correctionText : note.trim(),
        }),
      });
      await loadMessages(activeThread);
      setCorrectionFor("");
      setCorrectionText("");
      setCommentFor("");
      setCommentText("");
    } catch (feedbackError) {
      setError(feedbackError instanceof Error ? feedbackError.message : "Unable to record feedback");
    } finally {
      setFeedbackBusy("");
    }
  }

  function recordCitationOpen(citationId?: string) {
    if (!citationId) return;
    clientFetch(`/api/chat/citations/${citationId}/open`, { method: "POST" }).catch(() => undefined);
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
            <div key={thread.id} className={`chat-thread-row ${thread.id === activeThread ? "active" : ""}`}>
              <button className="chat-thread-open" onClick={() => setActiveThread(thread.id)}>
                <strong>{thread.title}</strong>
                <span>{new Date(thread.updated_at).toLocaleDateString()}</span>
              </button>
              <button className="chat-thread-delete" aria-label={`Delete ${thread.title}`} onClick={() => deleteThread(thread.id).catch(() => setError("Unable to delete conversation"))}>×</button>
            </div>
          ))}
        </div>
        <div className="chat-policy-note">
          <strong>Evidence-first</strong>
          <span>Answer ratings are feedback, not confirmation of a concept relationship. Review source passages before accepting a connection.</span>
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
            <article key={message.id} className={`chat-message ${message.role} ${message.status === "superseded" ? "superseded" : ""}`}>
              <div className="chat-message-label">
                {message.role === "user" ? "You" : message.role === "system" ? "Recorded correction" : "SELAR"}
                {message.status === "superseded" && <span> · superseded</span>}
              </div>
              <div className={`chat-message-content ${message.role === "assistant" ? "chat-markdown" : ""}`}>
                {message.role === "assistant" ? (
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
                ) : message.content}
              </div>
              {message.citations.length > 0 && (
                <div className="chat-citations">
                  {message.citations.map((citation) => (
                    <Link key={citation.chunk_id} href={citation.source_type === "pdf"
                      ? `/reader?docId=${citation.document_id}&page=${citation.page}`
                      : `/reader?docId=${citation.document_id}&block=${Number(citation.locator?.block_index ?? 0)}`
                    } onClick={() => recordCitationOpen(citation.id)}>
                      <span>[S{citation.rank}]</span>
                      <strong>{citation.document_title}</strong>
                      <small>{citation.source_type === "pdf" ? `Page ${citation.page}` : String(citation.locator?.heading || "Source passage")} · {citation.quote.slice(0, 150)}…</small>
                    </Link>
                  ))}
                </div>
              )}
              {message.graph_update && (message.graph_update.concepts_created > 0 || message.graph_update.concepts_reinforced > 0) && (
                <Link className="chat-graph-update" href="/graph">
                  <span className="chat-graph-update-mark">↗</span>
                  <span>
                    <strong>{message.graph_update.concepts_created > 0 ? "Candidate concept found in cited passage" : "Citation-concept associations recorded"}</strong>
                    <small>
                      {message.graph_update.concepts_created > 0 ? `${message.graph_update.concepts_created} candidate concept(s) from cited text · ` : ""}
                      {message.graph_update.concepts_reinforced} existing concept(s) associated with citations · Relationships require separate review.
                    </small>
                  </span>
                </Link>
              )}
              {message.role === "assistant" && message.status !== "superseded" && (
                <>
                  <div className="chat-feedback">
                    <span>Useful?</span>
                    {(() => {
                      const rating = message.feedback?.find((item) => item.action === "helpful" || item.action === "unhelpful");
                      return (
                        <>
                          <button
                            className={rating?.action === "helpful" ? "selected" : ""}
                            aria-label="Thumbs up — helpful"
                            title="Helpful"
                            disabled={feedbackBusy !== "" || Boolean(rating)}
                            onClick={() => sendFeedback(message.id, "helpful", commentFor === message.id ? commentText : "")}
                          >👍</button>
                          <button
                            className={rating?.action === "unhelpful" ? "selected negative" : ""}
                            aria-label="Thumbs down — not useful"
                            title="Not useful"
                            disabled={feedbackBusy !== "" || Boolean(rating)}
                            onClick={() => sendFeedback(message.id, "unhelpful", commentFor === message.id ? commentText : "")}
                          >👎</button>
                          <button
                            className="chat-comment-toggle"
                            disabled={feedbackBusy !== ""}
                            onClick={() => {
                              setCommentFor(commentFor === message.id ? "" : message.id);
                              setCommentText(rating?.correction_text || "");
                            }}
                          >{rating?.correction_text ? "Edit comment" : "Add comment"}</button>
                          {rating && <span className="chat-feedback-saved">Saved</span>}
                        </>
                      );
                    })()}
                    <button disabled={feedbackBusy !== ""} onClick={() => setCorrectionFor(correctionFor === message.id ? "" : message.id)}>Correct answer</button>
                  </div>
                  {commentFor === message.id && (
                    <div className="chat-feedback-comment">
                      <textarea
                        value={commentText}
                        onChange={(event) => setCommentText(event.target.value)}
                        maxLength={2000}
                        rows={2}
                        placeholder="Optional: what was useful or missing?"
                      />
                      {message.feedback?.some((item) => item.action === "helpful" || item.action === "unhelpful") ? (
                        <button
                          disabled={!commentText.trim() || feedbackBusy !== ""}
                          onClick={() => {
                            const rating = message.feedback?.find((item) => item.action === "helpful" || item.action === "unhelpful");
                            if (rating?.action === "helpful" || rating?.action === "unhelpful") {
                              sendFeedback(message.id, rating.action, commentText);
                            }
                          }}
                        >Save comment</button>
                      ) : <span>Your comment is sent with the thumb rating.</span>}
                    </div>
                  )}
                </>
              )}
              {correctionFor === message.id && (
                <div className="chat-correction">
                  <textarea value={correctionText} onChange={(event) => setCorrectionText(event.target.value)} maxLength={2000} rows={3} placeholder="Describe what should replace or supersede this answer…" />
                  <button disabled={!correctionText.trim() || feedbackBusy !== ""} onClick={() => sendFeedback(message.id, "correction")}>Record correction</button>
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
          <span className="chat-disclaimer">Answers cite retrieved passages; co-retrieval and ratings do not verify relationships.</span>
        </div>
      </main>
    </div>
  );
}
