export interface AlertRule {
  id: string;
  name: string;
  metric: 'memory' | 'disk' | 'load' | 'collection' | 'stale' | 'offline' | 'keyword';
  enabled: boolean;
  threshold: number;
  severity: 'warning' | 'critical';
  panelId: string;
  keyword: string;
}
export const petStyles = [
  { id: 'classic', label: '监控橘猫' },
  { id: 'hacker', label: '黑客猫猫' },
  { id: 'space', label: '太空猫猫' },
] as const;
export type PetStyle = (typeof petStyles)[number]['id'];

export interface AlertPolicy {
  enabled: boolean;
  petEnabled: boolean;
  petStyle: PetStyle;
  confirmSeconds: number;
  recoverySeconds: number;
  repeatMinutes: number;
  staleSeconds: number;
  notifyRecovery: boolean;
  rules: AlertRule[];
}
export interface Incident {
  id: string;
  sessionId: string;
  sessionName: string;
  ruleId: string;
  title: string;
  detail: string;
  severity: 'warning' | 'critical';
  status: 'firing' | 'recovered' | 'stopped';
  openedAt: string;
  updatedAt: string;
  closedAt?: string;
  acknowledged: boolean;
}
export interface AlertNotice {
  sequence: number;
  incident: Incident;
  kind: 'firing' | 'repeat' | 'recovered';
  time: string;
}
export interface MonitoringState {
  monitors: MonitorSummary[];
  policy: AlertPolicy;
  incidents: Incident[];
  notices: AlertNotice[];
  snoozedUntil: string;
  liveSessions: number;
  petRunning: boolean;
  error: string;
}
export interface PetTaskNotice {
  sequence: number;
  sessionId: string;
  sessionName: string;
  kind: 'completed' | 'failed' | 'approval' | 'reply';
  time: string;
}
export interface PetState extends MonitoringState {
  taskNotices: PetTaskNotice[];
  read?: Record<string, { alert: number; task: number }>;
}
export interface PetBackend {
  Snapshot(): Promise<PetState>;
  Action(kind: string, id: string): Promise<void>;
}
export interface MonitorCondition {
  enabled: boolean;
  kind: 'match' | 'not_match' | 'above' | 'below';
  pattern: string;
  threshold: number;
  recoveryThreshold: number;
  severity: 'warning' | 'critical';
  forSeconds: number;
  recoverySeconds: number;
}
export interface MonitorSummary {
  command: string;
  origin: string;
  updatedAt: string;
  error: string;
  sessionId: string;
  sessionName: string;
  panelId: string;
  title: string;
  intervalMs: number;
  alert?: MonitorCondition;
  status: 'watching' | 'unknown' | 'collecting' | 'waiting' | 'error' | 'stale' | 'stopped';
}
export function conditionSummary(c: MonitorCondition) {
  if (!c.enabled) return '异常提醒已关闭';
  const when =
    c.kind === 'above'
      ? `数值 ≥ ${c.threshold}`
      : c.kind === 'below'
        ? `数值 ≤ ${c.threshold}`
        : c.kind === 'match'
          ? '采集结果命中异常表达式'
          : '采集结果未命中正常表达式';
  return `${when}，持续 ${c.forSeconds} 秒提醒；连续恢复 ${c.recoverySeconds} 秒后通知`;
}
export const realtimePrompt = (text: string) =>
  `请为当前 SSH 服务器创建只读实时展示面板，通过 panelActions 设置 purpose=realtime、alert=null，只展示数据。不要创建或修改告警任务。不限于内置采集器，信息不足请先询问，说明刷新周期和仅当前连接生效的范围。我的需求：${text.trim()}`;
export const alertMonitorPrompt = (text: string) =>
  `请为当前 SSH 服务器创建独立的只读告警任务，通过 panelActions 设置 purpose=alert 和有效且启用的 panel.alert，使用独立 ID、采集命令和检查周期。不要创建或修改实时展示面板。不限于内置采集器，信息不足请先询问，说明异常条件、恢复条件和仅当前连接生效的范围。我的需求：${text.trim()}`;
