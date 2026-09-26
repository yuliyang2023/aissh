import { memo, useMemo } from 'react';
import Prism from 'prismjs';
import type { TokenStream } from 'prismjs';
import 'prismjs/components/prism-bash';

type Segment = { text: string; className: string };
type Line = { segments: Segment[]; ending: string };

// Tokenize the entire command first so quotes and heredocs keep their context
// across lines. Render text nodes, preserving the source and escaping any HTML.
function highlightedLines(command: string): Line[] {
  const lines: Line[] = [{ segments: [], ending: '' }];
  function append(tokens: TokenStream, className = '') {
    if (typeof tokens === 'string') {
      const parts = tokens.split(/(\r\n|\r|\n)/);
      parts.forEach((text, index) => {
        const line = lines[lines.length - 1];
        if (index % 2) {
          line.ending = text;
          lines.push({ segments: [], ending: '' });
        } else if (text) {
          line.segments.push({ text, className });
        }
      });
    } else if (Array.isArray(tokens)) {
      tokens.forEach((token) => append(token, className));
    } else {
      const aliases = typeof tokens.alias === 'string' ? [tokens.alias] : tokens.alias || [];
      append(tokens.content, ['shell-token', tokens.type, ...aliases].join(' '));
    }
  }
  let tokens: TokenStream = command;
  try {
    // Keep unusually large payloads readable without blocking on highlighting.
    if (command.length <= 256 * 1024) tokens = Prism.tokenize(command, Prism.languages.bash);
  } catch {
    // A highlighting failure must never prevent reviewing the original command.
  }
  append(tokens);
  return lines;
}

export const ShellCode = memo(function ShellCode({
  command,
  wrap,
}: {
  command: string;
  wrap: boolean;
}) {
  const lines = useMemo(() => highlightedLines(command), [command]);
  return (
    <pre
      className={'command-review-code' + (wrap ? ' wrap' : '')}
      tabIndex={0}
      aria-label="完整命令"
    >
      <code>
        {lines.map((line, index) => (
          <span className="command-code-line" key={index}>
            <span className="command-line-number" data-line={index + 1} aria-hidden="true" />
            <span className="command-line-text">
              {line.segments.map((segment, part) => (
                <span className={segment.className || undefined} key={part}>
                  {segment.text}
                </span>
              ))}
              {line.ending}
            </span>
          </span>
        ))}
      </code>
    </pre>
  );
});
