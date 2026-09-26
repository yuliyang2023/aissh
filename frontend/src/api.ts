export interface Credential {
  id: string;
  name: string;
  kind: 'password' | 'key' | 'token' | 'agent';
  username: string;
  identityFile: string;
  hasSecret: boolean;
  updatedAt: string;
}
export interface Host {
  id: string;
  name: string;
  address: string;
  port: number;
  user: string;
  group: string;
  credentialId: string;
  jumpHostId: string;
  aiProfileId: string;
  workspace: string;
}
export interface Resource {
  id: string;
  name: string;
  kind: 'http' | 'postgres' | 'mysql' | 'redis';
  endpoint: string;
  database: string;
  credentialId: string;
  hostId: string;
  route: 'local' | 'ssh';
  allowLocal: boolean;
  grant: 'ask' | 'session' | 'read';
  tls: boolean;
}
export interface AIProfile {
  id: string;
  name: string;
  provider: string;
  url: string;
  model: string;
  credentialId: string;
  cliPath: string;
}
export interface Catalog {
  version: number;
  revision: number;
  credentials: Credential[];
  hosts: Host[];
  conversationHosts?: Host[];
  resources: Resource[];
  aiProfiles: AIProfile[];
  defaultAIProfileId: string;
  pendingDeletes?: string[];
  recentConnections?: { hostId: string; openedAt: string }[];
}
export interface Session {
  id: string;
  hostId: string;
  conversationId: string;
  name: string;
  address: string;
  port: number;
  user: string;
  createdAt: string;
  endedAt: string;
  status: 'starting' | 'running' | 'ended' | 'failed';
}
export interface Conversation {
  id: string;
  hostId: string;
  title: string;
  preview: string;
  startedAt: string;
  updatedAt: string;
  messageCount: number;
}
export interface ChatMessage {
  role: string;
  content: string;
  time: string;
  proposal?: { command: string; explanation: string; risk: string };
}
export const sessionLive = (s: Session) => s.status === 'starting' || s.status === 'running';
export const sessionStatus = {
  starting: '正在启动',
  running: '运行中',
  ended: '已结束',
  failed: '异常结束',
};
export interface HostCheck {
  hostId: string;
  name: string;
  address: string;
  known: boolean;
  keys: { algorithm: string; fingerprint: string }[];
  token: string;
}
export interface Operation {
  resourceId: string;
  action: string;
  path?: string;
  query?: string;
  args?: string[];
}
type Backend = {
  Snapshot(): Promise<Catalog>;
  SaveCredential(input: {
    credential: Credential;
    secret: string;
    clearSecret: boolean;
  }): Promise<Credential>;
  SaveHost(host: Host): Promise<Host>;
  SaveResource(resource: Resource): Promise<Resource>;
  SaveAIProfile(profile: AIProfile, makeDefault: boolean): Promise<AIProfile>;
  Delete(kind: string, id: string): Promise<void>;
  CleanupSecrets(): Promise<void>;
  PickKeyFile(): Promise<string>;
  PickWorkspace(): Promise<string>;
  ImportSSH(): Promise<Host[]>;
  CheckHost(id: string): Promise<HostCheck>;
  TrustHost(token: string): Promise<void>;
  TestHost(id: string): Promise<void>;
  CreateSession(id: string): Promise<Session>;
  ListConversations(): Promise<Conversation[]>;
  CreateConversation(hostId: string): Promise<Conversation>;
  CreateDirectConversation(
    host: Host,
    saveHost: boolean,
  ): Promise<{ host: Host; conversation: Conversation }>;
  GetConversation(hostId: string, id: string): Promise<ChatMessage[]>;
  OpenConversationSession(hostId: string, id: string): Promise<Session>;
  ReconnectSession(id: string): Promise<Session>;
  ListSessions(): Promise<Session[]>;
  ClearRecentConnections(): Promise<void>;
  StartSession(id: string, cols: number, rows: number): Promise<void>;
  WriteTerminal(id: string, data: string): Promise<void>;
  ResizeTerminal(id: string, cols: number, rows: number): Promise<void>;
  CloseSession(id: string): Promise<void>;
  RunResource(op: Operation): Promise<string>;
  TestResource(id: string): Promise<string>;
  ResolveCredentialRequest(id: string, accepted: boolean): Promise<void>;
};
declare global {
  interface Window {
    go?: { desktop: { Service: Backend } };
    runtime?: { EventsOn(name: string, callback: (...args: any[]) => void): () => void };
  }
}
export const connected = () => !!window.go?.desktop?.Service;
export const api: Backend = new Proxy({} as Backend, {
  get:
    (_, key) =>
    (...args: unknown[]) => {
      const backend = window.go?.desktop?.Service;
      if (!backend)
        return Promise.reject(
          new Error('请从 WRD AI SSH 桌面客户端打开此界面。浏览器预览不连接系统凭据库。'),
        );
      return (backend[key as keyof Backend] as (...args: unknown[]) => unknown)(...args);
    },
});
export const emptyCatalog: Catalog = {
  version: 1,
  revision: 0,
  hosts: [],
  credentials: [],
  resources: [],
  aiProfiles: [],
  defaultAIProfileId: '',
};
export const allHosts = (data: Catalog) => [...data.hosts, ...(data.conversationHosts || [])];
export function defaultHostName(host: Host, data: Catalog): string {
  const address = host.address.trim();
  if (!address) return '';
  const user =
    host.user.trim() ||
    data.credentials.find((credential) => credential.id === host.credentialId)?.username.trim() ||
    '';
  const endpoint = `${address.includes(':') ? `[${address}]` : address}:${host.port || 22}`;
  return user ? `${user}@${endpoint}` : endpoint;
}
export const newHost = (): Host => ({
  id: '',
  name: '',
  address: '',
  port: 22,
  user: '',
  group: '',
  credentialId: '',
  jumpHostId: '',
  aiProfileId: '',
  workspace: '',
});
export const newCredential = (): Credential => ({
  id: '',
  name: '',
  kind: 'password',
  username: '',
  identityFile: '',
  hasSecret: false,
  updatedAt: '',
});
export const newResource = (): Resource => ({
  id: '',
  name: '',
  kind: 'http',
  endpoint: '',
  database: '',
  credentialId: '',
  hostId: '',
  route: 'local',
  allowLocal: true,
  grant: 'ask',
  tls: true,
});
export const newAI = (): AIProfile => ({
  id: '',
  name: '',
  provider: 'codex',
  url: '',
  model: '',
  credentialId: '',
  cliPath: '',
});
export const errorText = (e: unknown) => (e instanceof Error ? e.message : String(e));
