/** Author API, v1. TSX uses @wrd/monitor-ui/jsx-runtime, not React DOM. */
declare module '@wrd/monitor-ui' {
  export type Node = unknown;
  export type Point = {time: string; value: number | null};
  export type Tone = 'normal' | 'success' | 'warning' | 'danger';
  export interface Context<T, S> {
    data: T | null;
    lastGoodSample: {data: T | null; updatedAt: string};
    history: {series(field: string, options?: {minutes?: number}): Point[]};
    state: S;
    setState(next: S | ((previous: S) => S)): void;
    theme: Record<string, string>;
  }
  export function defineWidget<T, S>(definition: {
    stateVersion?: number;
    initialState?: S;
    render(context: Context<T, S>): Node;
  }): unknown;
  export function MetricCard(props: {label: string; value: number | string | null; unit?: string; percent?: number; tone?: Tone}): Node;
  export function StatusBadge(props: {label: string; value?: number | string | null; tone?: Tone}): Node;
  export function LineChart(props: {series: Point[]; label?: string; unit?: string}): Node;
  export function BarChart(props: {series: Point[]; label?: string; unit?: string}): Node;
  export function DataTable(props: {columns: string[]; rows: Record<string, string | number | boolean | null>[]}): Node;
}
