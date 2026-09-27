import type { AlertNotice, PetState, PetTaskNotice } from './monitoring-api';

export type PetNotice = {
  id: string;
  sequence: number;
  key: string;
  time: string;
  sessionId: string;
  sessionName: string;
  heading: string;
  title: string;
  detail: string;
  positive: boolean;
  taskKind?: PetTaskNotice['kind'];
  alert?: AlertNotice;
};

// A notice keeps its own severity even while the cat reflects another active alert.
export function petNoticeAppearance(notice: PetNotice): {
  tone: 'warning' | 'critical' | 'positive' | 'attention';
  label: string;
} {
  if (notice.alert) {
    if (notice.alert.kind === 'recovered') return { tone: 'positive', label: '已恢复' };
    return notice.alert.incident.severity === 'critical'
      ? { tone: 'critical', label: '严重告警' }
      : { tone: 'warning', label: '警告' };
  }
  if (notice.taskKind === 'completed') return { tone: 'positive', label: '已完成' };
  if (notice.taskKind === 'failed') return { tone: 'warning', label: '任务失败' };
  return { tone: 'attention', label: notice.taskKind === 'approval' ? '待确认' : '待回复' };
}

function alertNotice(notice: AlertNotice): PetNotice {
  return {
    id: `alert:${notice.sequence}`,
    sequence: notice.sequence,
    key: `alert:${notice.incident.id}`,
    time: notice.time,
    sessionId: notice.incident.sessionId,
    sessionName: notice.incident.sessionName,
    heading:
      notice.kind === 'recovered'
        ? '已经恢复啦'
        : notice.incident.severity === 'critical'
          ? '有个问题需要你看看'
          : '发现一个异常',
    title: notice.incident.title,
    detail: notice.incident.detail,
    positive: notice.kind === 'recovered',
    alert: notice,
  };
}

const taskMessages = {
  completed: {
    heading: '任务完成啦',
    title: 'AI 任务已完成',
    detail: '结果已经准备好了，点击查看。',
  },
  failed: {
    heading: '任务遇到问题了',
    title: 'AI 任务未完成',
    detail: '点击返回会话，查看原因并继续处理。',
  },
  approval: {
    heading: '需要你确认一下',
    title: 'AI 等待命令确认',
    detail: '命令建议已准备好，点击查看并确认。',
  },
  reply: {
    heading: '等你回复哦',
    title: 'AI 需要补充信息',
    detail: '点击返回会话，补充信息后继续。',
  },
};

function taskNotice(notice: PetTaskNotice): PetNotice {
  return {
    id: `task:${notice.sequence}`,
    sequence: notice.sequence,
    key: `task:${notice.sessionId}`,
    time: notice.time,
    sessionId: notice.sessionId,
    sessionName: notice.sessionName,
    ...taskMessages[notice.kind],
    positive: notice.kind === 'completed',
    taskKind: notice.kind,
  };
}

function priority(item: PetNotice) {
  if (item.alert && !item.positive && item.alert.incident.severity === 'critical') return 0;
  return item.positive ? 2 : 1;
}

// Even simultaneous tasks on identically named hosts must be distinguishable.
export function petSessionLabel(item: PetNotice, items: PetNotice[]) {
  let length = 6;
  while (
    length < item.sessionId.length &&
    items.some(
      (other) =>
        other.sessionId !== item.sessionId &&
        other.sessionId.slice(0, length) === item.sessionId.slice(0, length),
    )
  )
    length++;
  return `#${item.sessionId.slice(0, length)} · ${item.sessionName || '未命名会话'}`;
}

export type PetInbox = {
  items: PetNotice[];
  selected: string;
  open: boolean;
  view: 'detail' | 'list';
  latest: { alert: number; task: number };
};
export const emptyPetInbox: PetInbox = {
  items: [],
  selected: '',
  open: false,
  view: 'detail',
  latest: { alert: 0, task: 0 },
};
type InboxAction =
  | { type: 'snapshot'; state: PetState; now: number }
  | { type: 'select'; id: string }
  | { type: 'read'; id: string }
  | { type: 'hide' }
  | { type: 'list' }
  | { type: 'clear' };

export function petInboxReducer(inbox: PetInbox, action: InboxAction): PetInbox {
  if (action.type === 'hide') return { ...inbox, open: false };
  if (action.type === 'list') return { ...inbox, open: inbox.items.length > 0, view: 'list' };
  if (action.type === 'clear') return { ...inbox, items: [], selected: '', open: false };
  if (action.type === 'select')
    return inbox.items.some((item) => item.id === action.id)
      ? { ...inbox, selected: action.id, open: true, view: 'detail' }
      : inbox;
  if (action.type === 'read') {
    const index = inbox.items.findIndex((item) => item.id === action.id);
    const items = inbox.items.filter((item) => item.id !== action.id);
    return {
      ...inbox,
      items,
      selected:
        inbox.selected === action.id
          ? items[Math.min(index, items.length - 1)]?.id || ''
          : inbox.selected,
      open: inbox.open && items.length > 0,
    };
  }

  const { state, now } = action;
  const latest = { ...inbox.latest };
  const incoming: PetNotice[] = [];
  for (const notice of state.notices) {
    if (notice.sequence <= latest.alert) continue;
    latest.alert = notice.sequence;
    incoming.push(alertNotice(notice));
  }
  for (const notice of state.taskNotices || []) {
    if (notice.sequence <= latest.task) continue;
    latest.task = notice.sequence;
    incoming.push(taskNotice(notice));
  }
  if (!state.policy.enabled || Date.parse(state.snoozedUntil) > now)
    return { ...emptyPetInbox, latest };
  const allowed = (item: PetNotice) => {
    const read = state.read?.[item.sessionId];
    if (item.sequence <= (item.alert ? read?.alert || 0 : read?.task || 0)) return false;
    return (
      !item.alert ||
      item.alert.kind === 'recovered' ||
      state.incidents.some(
        (i) => i.id === item.alert!.incident.id && i.status === 'firing' && !i.acknowledged,
      )
    );
  };
  let items = inbox.items.filter(allowed);
  let changed = false;
  let selected = inbox.selected;
  for (const item of incoming.sort((a, b) => Date.parse(a.time) - Date.parse(b.time))) {
    const age = now - Date.parse(item.time);
    if (age < 0 || age >= 20000 || !allowed(item)) continue;
    const previous = items.find((entry) => entry.key === item.key);
    if (previous?.id === selected) selected = item.id;
    items = [...items.filter((entry) => entry.key !== item.key), item];
    changed = true;
  }
  // Keep the newest 50 unread entries, with problems and requests first in the list.
  items.sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
  items = items
    .slice(0, 50)
    .sort((a, b) => priority(a) - priority(b) || Date.parse(b.time) - Date.parse(a.time));
  if (!items.some((item) => item.id === selected) || (!inbox.open && changed))
    selected = items[0]?.id || '';
  return {
    items,
    selected,
    latest,
    open: items.length > 0 && (inbox.open || changed),
    view: inbox.open ? inbox.view : 'detail',
  };
}
