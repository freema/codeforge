import type { ReactNode } from "react";

/**
 * Lightweight markdown renderer for agent text output.
 * Handles: code blocks, inline code, bold, lists, headings, paragraphs.
 */
export default function MarkdownText({
  text,
  className = "",
}: {
  text: string;
  className?: string;
}) {
  const blocks = parseBlocks(text);

  return (
    <div className={`space-y-2 ${className}`}>
      {blocks.map((block, i) => (
        <Block key={i} block={block} />
      ))}
    </div>
  );
}

// ─── Block parsing ───────────────────────────────────────────────────

type BlockNode =
  | { type: "code"; lang: string; content: string }
  | { type: "heading"; level: number; content: string }
  | { type: "list"; items: string[] }
  | { type: "paragraph"; content: string };

function parseBlocks(text: string): BlockNode[] {
  const lines = text.split("\n");
  const blocks: BlockNode[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i]!;

    // Skip empty lines
    if (line.trim() === "") {
      i++;
      continue;
    }

    // Code block
    if (line.trimStart().startsWith("```")) {
      const lang = line.trimStart().slice(3).trim();
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i]!.trimStart().startsWith("```")) {
        codeLines.push(lines[i]!);
        i++;
      }
      i++; // skip closing ```
      blocks.push({ type: "code", lang, content: codeLines.join("\n") });
      continue;
    }

    // Heading
    const headingMatch = line.match(/^(#{1,3})\s+(.+)/);
    if (headingMatch) {
      blocks.push({
        type: "heading",
        level: headingMatch[1]!.length,
        content: headingMatch[2]!,
      });
      i++;
      continue;
    }

    // List items (- or * or numbered)
    if (line.match(/^\s*[-*]\s/) || line.match(/^\s*\d+\.\s/)) {
      const items: string[] = [];
      while (
        i < lines.length &&
        (lines[i]!.match(/^\s*[-*]\s/) || lines[i]!.match(/^\s*\d+\.\s/))
      ) {
        items.push(
          lines[i]!.replace(/^\s*[-*]\s/, "").replace(/^\s*\d+\.\s/, ""),
        );
        i++;
      }
      blocks.push({ type: "list", items });
      continue;
    }

    // Regular paragraph — collect lines until empty line, code block, heading, or list
    const paraLines: string[] = [];
    while (
      i < lines.length &&
      lines[i]!.trim() !== "" &&
      !lines[i]!.trimStart().startsWith("```") &&
      !lines[i]!.match(/^#{1,3}\s/) &&
      !lines[i]!.match(/^\s*[-*]\s/) &&
      !lines[i]!.match(/^\s*\d+\.\s/)
    ) {
      paraLines.push(lines[i]!);
      i++;
    }
    if (paraLines.length > 0) {
      blocks.push({ type: "paragraph", content: paraLines.join("\n") });
    }
  }

  return blocks;
}

// ─── Block rendering ─────────────────────────────────────────────────

function Block({ block }: { block: BlockNode }) {
  switch (block.type) {
    case "code":
      return (
        <pre className="overflow-x-auto rounded-lg border border-edge bg-surface p-3">
          <code className="text-[11px] leading-relaxed text-fg-2">
            {block.content}
          </code>
        </pre>
      );

    case "heading":
      if (block.level === 1)
        return (
          <h3 className="text-sm font-bold text-fg">
            {renderInline(block.content)}
          </h3>
        );
      if (block.level === 2)
        return (
          <h4 className="text-xs font-bold text-fg">
            {renderInline(block.content)}
          </h4>
        );
      return (
        <h5 className="text-xs font-bold text-fg-2">
          {renderInline(block.content)}
        </h5>
      );

    case "list":
      return (
        <ul className="space-y-1 pl-1">
          {block.items.map((item, j) => (
            <li
              key={j}
              className="flex gap-2 text-xs leading-relaxed text-fg-2"
            >
              <span className="mt-1 shrink-0 text-accent/40">-</span>
              <span>{renderInline(item)}</span>
            </li>
          ))}
        </ul>
      );

    case "paragraph":
      return (
        <p className="text-xs leading-relaxed text-fg-2">
          {renderInline(block.content)}
        </p>
      );
  }
}

// ─── Inline rendering ────────────────────────────────────────────────

function renderInline(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  // Match **bold** or `code`
  const regex = /(\*\*(.+?)\*\*|`([^`\n]+)`)/g;
  let lastIndex = 0;
  let match;
  let key = 0;

  while ((match = regex.exec(text)) !== null) {
    // Text before match
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }

    if (match[2]) {
      // Bold
      parts.push(
        <strong key={key++} className="font-bold text-fg">
          {match[2]}
        </strong>,
      );
    } else if (match[3]) {
      // Inline code
      parts.push(
        <code
          key={key++}
          className="rounded bg-surface px-1.5 py-0.5 font-mono text-[0.95em] text-accent/80"
        >
          {match[3]}
        </code>,
      );
    }

    lastIndex = match.index + match[0].length;
  }

  // Remaining text
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}
