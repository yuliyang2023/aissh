// Keep aliases aligned with agent.NormalizeRisk so labels and warning styles agree.
export function commandRisk(value: string) {
  const risk = value.trim().toLowerCase();
  switch (risk) {
    case 'read_only':
    case 'readonly':
    case 'read-only':
    case 'safe':
      return { level: 'read_only', label: '只读' };
    case 'low':
      return { level: 'low', label: '低风险' };
    case 'medium':
      return { level: 'medium', label: '中风险' };
    case 'high':
    case 'destructive':
    case 'dangerous':
      return { level: 'high', label: '高风险' };
    case 'critical':
      return { level: 'critical', label: '极高风险' };
    default:
      return { level: 'unknown', label: '风险未知' };
  }
}
