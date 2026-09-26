import { Conversation, Session, sessionLive } from './api';
import { conversationKey } from './Conversations';

export type SessionItem = {
  key: string;
  hostId: string;
  title: string;
  preview: string;
  updatedAt: string;
} & (
  | { conversation: Conversation; session?: Session }
  | { conversation?: never; session: Session }
);

export function conversationSession(sessions: Session[], c: Conversation) {
  const matches = sessions.filter((s) => s.hostId === c.hostId && s.conversationId === c.id);
  return matches.find(sessionLive) || matches[matches.length - 1];
}

export function sessionItemKey(session: Session) {
  return session.conversationId
    ? conversationKey({ hostId: session.hostId, id: session.conversationId })
    : 'session:' + session.id;
}

export function sessionItems(conversations: Conversation[], sessions: Session[]): SessionItem[] {
  const items: SessionItem[] = conversations.map((conversation) => ({
    key: conversationKey(conversation),
    hostId: conversation.hostId,
    title: conversation.title,
    preview: conversation.preview || '新的话题',
    updatedAt: conversation.updatedAt,
    conversation,
    session: conversationSession(sessions, conversation),
  }));
  for (const session of sessions) {
    const key = sessionItemKey(session);
    const existing = items.find((item) => item.key === key);
    if (existing?.conversation || (existing?.session && sessionLive(existing.session))) continue;
    const item: SessionItem = {
      key,
      hostId: session.hostId,
      title: session.name,
      preview: 'SSH + AI 工作区',
      updatedAt: session.endedAt || session.createdAt,
      session,
    };
    if (existing) items[items.indexOf(existing)] = item;
    else items.unshift(item);
  }
  return items;
}

export function sessionItemHidden(item: SessionItem, hidden: Set<string>) {
  return hidden.has(item.key) || !!(item.session && hidden.has('session:' + item.session.id));
}
