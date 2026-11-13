// Tipos para las propiedades creadas por los scripts clásicos del panel.
// Solo se usan durante el análisis de JavaScript por el editor/TypeScript.
interface Window {
  gatewayURL: (path: string) => string;
  preparePolicyEditor?: () => Promise<void>;
  hgwOriginalACL?: string;
  toastMessage?: (message: string, error?: boolean) => void;
  gatewayLanguage?: () => string;
  tr?: (message: string) => string;
  proxySchema?: Array<{ key: string; label?: string; description?: string }>;
  configSchema?: Array<{ key: string; label?: string; description?: string }>;
  markRestartRequired?: () => void;
  notifyRestartRequested?: () => void;
  _gatewayOfflineToast?: boolean;
  renderStatus?: (status: any) => void;
}
