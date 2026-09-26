export interface WidgetMeta {
  saveError?: string;
  hash: string;
  revision: number;
  bindingId: string;
  sequence: number;
}
export interface WidgetCandidate {
  protocolVersion?: number;
  sample?: WidgetData;
  data?: { format: string; fields: { name: string; type: string; nullable: boolean }[] };
  id: string;
  panelId: string;
  title: string;
  hash: string;
  expiresAt: string;
}
export interface WidgetArtifact {
  hash: string;
  bundle: string;
  view: { kind: 'custom'; sdkVersion: number; stateVersion: number; source: string };
}
export interface WidgetData {
  bindingId: string;
  sourceRevision: number;
  sequence: number;
  data: Record<string, unknown> | null;
  lastGood: Record<string, unknown> | null;
  updatedAt: string;
  lastGoodAt: string;
  error: string;
  issues: string[];
  history: { time: string; values: Record<string, number | null> }[];
  truncated: boolean;
  intervalMs: number;
}
export type ViewNode =
  | { kind: 'text'; text: string }
  | {
      kind: 'element';
      type: string;
      props: Record<string, any>;
      events: Record<string, string>;
      children: ViewNode[];
    };
const tags = new Set(
  'div section article span p strong small em h2 h3 h4 ul ol li button label input select option table thead tbody tr td th svg g path rect circle line polyline text MetricCard StatusBadge LineChart BarChart DataTable'.split(
    ' ',
  ),
);
const styles = new Set(
  'display gridTemplateColumns gridTemplateRows gap rowGap columnGap flexDirection flexWrap alignItems justifyContent flexGrow flexShrink padding paddingTop paddingBottom paddingLeft paddingRight margin marginTop marginBottom marginLeft marginRight width height minWidth maxWidth minHeight maxHeight color background backgroundColor border borderRadius borderColor borderWidth borderStyle borderCollapse fontSize fontWeight fontFamily lineHeight textAlign whiteSpace wordBreak overflowWrap overflow overflowX overflowY opacity'.split(
    ' ',
  ),
);
const attrs = new Set(
  'title aria-label aria-live aria-expanded role value placeholder disabled checked maxLength min max step type viewBox width height d x y x1 x2 y1 y2 cx cy r rx ry points fill stroke strokeWidth opacity fontSize textAnchor colSpan rowSpan'.split(
    ' ',
  ),
);
const eventTags: Record<string, Set<string>> = {
  onClick: new Set(['button']),
  onChange: new Set(['input', 'select']),
  onInput: new Set(['input']),
};
const fail = (detail = '') => {
  throw new Error('组件包含不支持或超限的界面内容' + (detail ? '：' + detail.slice(0, 100) : ''));
};
function scalar(value: unknown) {
  return (
    value === null ||
    typeof value === 'boolean' ||
    (typeof value === 'string' && value.length <= 4000) ||
    (typeof value === 'number' &&
      Number.isFinite(value) &&
      Math.abs(value) <= Number.MAX_SAFE_INTEGER)
  );
}
function object(v: any): v is Record<string, any> {
  return (
    v !== null &&
    typeof v === 'object' &&
    !Array.isArray(v) &&
    Object.getPrototypeOf(v) === Object.prototype
  );
}
export function validateTree(input: unknown): ViewNode[] {
  if (JSON.stringify(input).length > 262144) fail();
  let count = 0;
  function node(n: any, depth: number): ViewNode {
    if (++count > 2000 || depth > 32 || !object(n)) return fail();
    if (n.kind === 'text') {
      if (typeof n.text !== 'string' || n.text.length > 16000) return fail();
      return { kind: 'text', text: n.text };
    }
    if (
      n.kind !== 'element' ||
      !tags.has(n.type) ||
      !object(n.props) ||
      !object(n.events) ||
      !Array.isArray(n.children)
    )
      return fail();
    const props: Record<string, any> = {};
    for (const [key, v] of Object.entries(n.props)) {
      if (['__proto__', 'constructor', 'prototype'].includes(key)) return fail();
      if (key === 'style') {
        if (!object(v) || Object.keys(v).length > 40) return fail();
        const style: Record<string, string | number> = {};
        for (const [k, x] of Object.entries(v)) {
          if (
            !styles.has(k) ||
            !(
              (typeof x === 'number' && Number.isFinite(x) && Math.abs(x) <= 4000) ||
              (typeof x === 'string' && x.length <= 128 && /^[-#%,().a-zA-Z0-9_\s]+$/.test(x))
            )
          )
            return fail(`样式 ${k.slice(0, 40)} 不受支持或值无效`);
          if (
            typeof x === 'string' &&
            /url|expression|var\(|fixed|sticky|import|filter|content|image/i.test(x)
          )
            return fail(`样式 ${k.slice(0, 40)} 包含禁止的表达式`);
          if (
            typeof x === 'string' &&
            // Hexadecimal colors are not pixel dimensions (e.g. #15803d).
            [
              ...x
                .replace(/#(?:[a-f0-9]{8}|[a-f0-9]{6}|[a-f0-9]{4}|[a-f0-9]{3})\b/gi, '')
                .matchAll(/\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi),
            ].some((n) => Number(n[0]) > 4000)
          )
            return fail(`样式 ${k.slice(0, 40)} 的数值超过 4000`);
          if (
            typeof x === 'string' &&
            /repeat\(\s*(\d+)/.test(x) &&
            Number(x.match(/repeat\(\s*(\d+)/)?.[1]) > 12
          )
            return fail();
          style[k] = x as string | number;
        }
        props.style = style;
        continue;
      }
      if (['MetricCard', 'StatusBadge'].includes(n.type)) {
        if (!['label', 'value', 'unit', 'tone', 'percent'].includes(key) || !scalar(v))
          return fail();
        if (key === 'tone' && !['normal', 'success', 'warning', 'danger'].includes(v as string))
          return fail();
        props[key] = v;
        continue;
      }
      if (n.type === 'LineChart' || n.type === 'BarChart') {
        if (key === 'unit' || key === 'label') {
          if (typeof v !== 'string' || v.length > 128) return fail();
        } else if (key === 'series') {
          if (!Array.isArray(v) || v.length > 600) return fail();
          for (const p of v)
            if (
              !object(p) ||
              !(
                p.value === null ||
                (typeof p.value === 'number' &&
                  Number.isFinite(p.value) &&
                  Math.abs(p.value) <= Number.MAX_SAFE_INTEGER)
              ) ||
              typeof p.time !== 'string' ||
              !Number.isFinite(Date.parse(p.time))
            )
              return fail();
        } else return fail();
        props[key] = v;
        continue;
      }
      if (n.type === 'DataTable') {
        if (key === 'columns') {
          if (
            !Array.isArray(v) ||
            v.length > 50 ||
            v.some(
              (x) =>
                typeof x !== 'string' ||
                x.length > 64 ||
                ['__proto__', 'constructor', 'prototype'].includes(x),
            )
          )
            return fail();
        } else if (key === 'rows') {
          if (
            !Array.isArray(v) ||
            v.length > 1000 ||
            v.some(
              (row) =>
                !object(row) ||
                Object.keys(row).length > 50 ||
                Object.values(row).some((x) => !scalar(x)),
            )
          )
            return fail();
        } else return fail();
        props[key] = v;
        continue;
      }
      if (!attrs.has(key) || !scalar(v)) return fail();
      if (
        key === 'type' &&
        !['text', 'number', 'checkbox', 'range', 'button'].includes(v as string)
      )
        return fail();
      if (
        key === 'role' &&
        !['group', 'status', 'note', 'table', 'row', 'cell', 'list', 'listitem'].includes(
          v as string,
        )
      )
        return fail();
      if (
        ['fill', 'stroke'].includes(key) &&
        !(typeof v === 'string' && /^(none|currentColor|[a-zA-Z]+|#[0-9a-fA-F]{3,8})$/.test(v))
      )
        return fail();
      if (
        ['d', 'points', 'viewBox'].includes(key) &&
        !(
          typeof v === 'string' &&
          v.length <= 4000 &&
          /^[-+.,\deE\sMmLlHhVvCcSsQqTtAaZz]*$/.test(v)
        )
      )
        return fail();
      if (key === 'maxLength' && !(typeof v === 'number' && v >= 0 && v <= 512)) return fail();
      if (
        ['width', 'height', 'r', 'rx', 'ry', 'fontSize', 'strokeWidth'].includes(key) &&
        typeof v === 'number' &&
        (v < 0 || v > 4000)
      )
        return fail();
      props[key] = v;
    }
    const events: Record<string, string> = {};
    for (const [key, value] of Object.entries(n.events)) {
      if (!eventTags[key]?.has(n.type) || typeof value !== 'string' || !/^\d{1,12}$/.test(value))
        return fail();
      events[key] = value;
    }
    return {
      kind: 'element',
      type: n.type,
      props,
      events,
      children: n.children.map((c: any) => node(c, depth + 1)),
    };
  }
  if (!Array.isArray(input)) return fail();
  return input.map((n) => node(n, 0));
}
