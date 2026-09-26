import { Catalog, Resource } from './api';
import { Modal } from './components';
import { ExternalConsole } from './ExternalConsole';

export function discoveryStatus(resource: Resource) {
  switch (resource.discovery?.status) {
    case 'ready':
      return '已探索';
    case 'partial':
      return '探索不完整';
    case 'failed':
      return '探索失败';
    case 'stale':
      return '需要重新探索';
    default:
      return '尚未探索';
  }
}

export function ResourceGuide({
  resource,
  data,
  onClose,
  onExplore,
}: {
  resource: Resource;
  data: Catalog;
  onClose: () => void;
  onExplore: () => Promise<void>;
}) {
  return (
    <Modal
      wide
      title={resource.name + ' · 操作目录'}
      subtitle="管理接口定义、调用授权和风险规则。"
      onClose={onClose}
    >
      <ExternalConsole resource={resource} data={data} onExplore={onExplore} />
    </Modal>
  );
}
