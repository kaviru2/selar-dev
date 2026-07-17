"use client";

import Image from "next/image";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { DocumentContent } from "@/lib/api";

function Markdown({ children, inline = false }: { children: string; inline?: boolean }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={inline ? { p: ({ children: value }) => <>{value}</> } : undefined}
    >
      {children}
    </ReactMarkdown>
  );
}

export function ArticleReader({ content }: { content: DocumentContent }) {
  const assetsByBlock = new Map(content.assets.map((asset) => [asset.block_index, asset]));
  return (
    <article className="article-reader">
      <header>
        <div className="article-source-type">{content.document.source_type}</div>
        <h1>{content.document.title}</h1>
        <div className="article-byline">
          {content.document.authors || "Unknown author"}
          {content.document.year ? ` · ${content.document.year}` : ""}
          {content.document.source_url && <> · <a href={content.document.source_url} target="_blank" rel="noreferrer">Original source ↗</a></>}
        </div>
      </header>
      <div className="article-body">
        {content.blocks.map((block) => {
          const asset = assetsByBlock.get(block.block_index);
          if (block.kind === "figure" && asset) {
            const usefulAlt = asset.alt_text && !/^image\/(png|jpe?g|webp|gif)$/i.test(asset.alt_text) ? asset.alt_text : "";
            const visualLabel = asset.caption || usefulAlt || asset.description || "Source visual";
            return (
              <figure id={`block-${block.block_index}`} key={block.id}>
                <Image src={`/api/documents/${content.document.id}/assets/${asset.id}`} alt={visualLabel} width={asset.width || 900} height={asset.height || 600} unoptimized />
                {visualLabel && <figcaption>{visualLabel}</figcaption>}
              </figure>
            );
          }
          if (block.kind === "heading") return <h2 id={`block-${block.block_index}`} key={block.id}><Markdown inline>{block.text}</Markdown></h2>;
          if (block.kind === "quote") return <blockquote id={`block-${block.block_index}`} key={block.id}><Markdown>{block.text}</Markdown></blockquote>;
          if (block.kind === "list") return <div className="article-list" id={`block-${block.block_index}`} key={block.id}>• <Markdown inline>{block.text}</Markdown></div>;
          if (block.kind === "code") return <pre id={`block-${block.block_index}`} key={block.id}>{block.text}</pre>;
          return <div className="article-paragraph" id={`block-${block.block_index}`} key={block.id}><Markdown>{block.text}</Markdown></div>;
        })}
      </div>
    </article>
  );
}
