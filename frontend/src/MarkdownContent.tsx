import { Children, isValidElement, memo, ReactNode, useId, useState } from 'react';
import Markdown, { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Check, Copy } from 'lucide-react';

const plugins = [remarkGfm];

function textContent(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) =>
      isValidElement<{ children?: ReactNode }>(child)
        ? textContent(child.props.children)
        : typeof child === 'string' || typeof child === 'number'
          ? String(child)
          : '',
    )
    .join('');
}

function safeURL(value: string): string {
  // Model text may contain arbitrary protocols. Only web/email links and
  // in-document references are meaningful inside a chat message.
  if (value.startsWith('#')) return value;
  try {
    const url = new URL(value);
    if (['https:', 'http:', 'mailto:'].includes(url.protocol)) return url.href;
  } catch {
    // Relative paths do not identify files on the remote SSH server.
  }
  return '';
}

const MarkdownLink: Components['a'] = ({ href, children, title, ...props }) => {
  if (!href) return <span>{children}</span>;
  const external = !href.startsWith('#');
  return (
    <a
      href={href}
      title={title}
      id={props.id}
      aria-label={props['aria-label']}
      target={external ? '_blank' : undefined}
      rel={external ? 'noopener noreferrer' : undefined}
      onClick={(event) => {
        if (external && window.runtime?.BrowserOpenURL) {
          event.preventDefault();
          window.runtime.BrowserOpenURL(href);
        }
      }}
    >
      {children}
    </a>
  );
};

const MarkdownCode: Components['pre'] = ({ children }) => {
  const [copiedText, setCopiedText] = useState<string | null>(null);
  const [copyError, setCopyError] = useState(false);
  const child = Children.toArray(children)[0];
  const language = isValidElement<{ className?: string }>(child)
    ? child.props.className?.match(/\blanguage-([^\s]+)/)?.[1]
    : '';
  // The parser adds one final line ending to fenced code. Keep all other
  // whitespace, including deliberate blank lines and indentation, when copying.
  const code = textContent(children).replace(/\n$/, '');
  const copied = copiedText === code;
  return (
    <div className="markdown-code">
      <div className="markdown-code-toolbar">
        <span>{language || '代码'}</span>
        <button
          type="button"
          className="button small"
          aria-label="复制代码"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(code);
              setCopiedText(code);
              setCopyError(false);
            } catch {
              setCopyError(true);
            }
          }}
        >
          {copied ? <Check size={12} /> : <Copy size={12} />}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre tabIndex={0}>{children}</pre>
      {copyError && <small role="status">复制失败，请选中代码后手动复制。</small>}
    </div>
  );
};

const components: Components = {
  a: MarkdownLink,
  pre: MarkdownCode,
  table: ({ children }) => (
    <div className="markdown-table" tabIndex={0} role="region" aria-label="Markdown 表格">
      <table>{children}</table>
    </div>
  ),
  // The desktop CSP keeps remote resources from loading automatically. Image
  // references can still be opened explicitly, using the same web-link policy.
  img: ({ src, alt }) => (
    <MarkdownLink href={typeof src === 'string' ? safeURL(src) : ''}>
      {alt ? `查看图片：${alt}` : '查看图片'}
    </MarkdownLink>
  ),
};

export const MarkdownContent = memo(function MarkdownContent({ content }: { content: string }) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, '');
  return (
    <div className="markdown-content">
      <Markdown
        remarkPlugins={plugins}
        remarkRehypeOptions={{ clobberPrefix: `markdown-${id}-` }}
        components={components}
        urlTransform={safeURL}
      >
        {content}
      </Markdown>
    </div>
  );
});
