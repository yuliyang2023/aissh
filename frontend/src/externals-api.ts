import type { Operation, ResourceDiscovery } from './api';
export type Schema = Record<string, unknown>;
export type Effects = {
  access: 'read' | 'write' | 'unknown';
  destructive: boolean;
  serviceImpact: boolean;
  permissionChange: boolean;
  sensitive: boolean;
};
export interface ExternalDefinition {
  id: string;
  revision: number;
  name: string;
  kind: string;
  description: string;
  inputSchema: Schema;
  outputSchema?: Schema;
  request: {
    method?: string;
    path?: string;
    parameters: { input: string; in: string; name: string }[];
    bodyInput?: string;
    query?: string;
    sqlParameters?: string[];
    redisCommand?: string;
    redisParameters?: string[];
  };
  assessment: {
    effects: Effects;
    risk: string;
    assurance: string;
    evidence: string;
    reviewedAt?: string;
  };
  lifecycle: string;
  source: string;
  blocked?: string;
  quantityArgument?: string;
  changeReason?: string;
  updatedAt: string;
}
export interface ExternalGrant {
  id: string;
  name: string;
  subject: { kind: string; id?: string };
  operations: Record<string, number>;
  permissions: string[];
  constraints: Record<string, { values?: unknown[]; prefix?: string; maximum?: number }>;
  allowedFields: string[];
  validFrom?: string;
  expiresAt?: string;
  revoked: boolean;
  deny: boolean;
  autoRead: boolean;
  maxUses: number;
  used: number;
  requestDigest?: string;
  grantedBy: string;
  updatedAt: string;
}
export interface PermissionPolicy {
  enabled: boolean;
  ceiling: string[];
  deniedOperations: string[];
}
export interface RiskPolicy {
  denySensitive: boolean;
  denyDestructive: boolean;
  allowHigh: boolean;
  denyCritical: boolean;
  confirmRead: boolean;
  productionFloor: string;
  maxQuantity: number;
  maxResponseBytes: number;
  maxRows: number;
  timeoutSeconds: number;
}
export interface ExternalState {
  version: number;
  connectionId: string;
  revision: number;
  definitionBinding: string;
  operations: ExternalDefinition[];
  candidates: ExternalDefinition[];
  history: ExternalDefinition[];
  grants: ExternalGrant[];
  permissions: PermissionPolicy;
  risk: RiskPolicy;
  changes: { time: string; actor: string; kind: string; reason: string; revision: number }[];
}
export interface DiscoveryJob {
  id: string;
  connectionId: string;
  status: string;
  startedAt: string;
  finishedAt?: string;
  error?: string;
}
export interface Decision {
  permission: string;
  risk: string;
  action: string;
  reasons: string[];
  required: string[];
  grantId?: string;
}
export interface Invocation {
  id: string;
  time: string;
  operationId: string;
  operationRevision: number;
  caller: string;
  stateRevision: number;
  requestDigest: string;
  decision: Decision;
  approved: boolean;
  outcome: string;
  summary: string;
}
export interface ExternalView {
  state: ExternalState;
  job: DiscoveryJob;
  discovery?: ResourceDiscovery;
  invocations: Invocation[];
}
export interface ExternalCaller {
  hostId: string;
  sessionId: string;
  local: boolean;
}
export interface ExternalPreview {
  token: string;
  connectionId: string;
  operation: ExternalDefinition;
  stateRevision: number;
  arguments: Record<string, unknown>;
  request: Record<string, unknown>;
  requestDigest: string;
  decision: Decision;
  caller: ExternalCaller;
  expiresAt: string;
}
export interface ExternalBackend {
  GetExternalState(id: string): Promise<ExternalView>;
  SaveExternalOperation(
    id: string,
    revision: number,
    op: ExternalDefinition,
    reason: string,
  ): Promise<ExternalState>;
  SaveExternalPermissions(
    id: string,
    revision: number,
    policy: PermissionPolicy,
    grants: ExternalGrant[],
    reason: string,
  ): Promise<ExternalState>;
  SaveExternalRisk(
    id: string,
    revision: number,
    risk: RiskPolicy,
    reason: string,
  ): Promise<ExternalState>;
  CopyExternalDefinitions(
    source: string,
    target: string,
    revision: number,
    reason: string,
  ): Promise<ExternalState>;
  PreviewExternalCall(op: Operation): Promise<ExternalPreview>;
  SimulateExternalCall(op: Operation, caller: ExternalCaller): Promise<ExternalPreview>;
  InvokeExternalCall(token: string): Promise<string>;
  StartResourceDiscovery(id: string): Promise<DiscoveryJob>;
  CancelResourceDiscovery(id: string): Promise<void>;
}
export const permissionLabels: Record<string, string> = {
  read: '读取',
  write: '写入',
  delete: '删除',
  operate: '服务操作',
  remote_admin: '远端权限管理',
};
export const riskLabels: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  critical: '严重',
  unknown: '待确认',
};
