export type Method =
  | "status"
  | "configure"
  | "connect"
  | "disconnect"
  | "recover"
  | "testPath"
  | "diagnostics"
  | "importRelay"
  | "importPath"
  | "deletePath"
  | "updatePath"
  | "testRelay";
export interface Path {
  id: string;
  name: string;
  enabled: boolean;
  state: string;
  endpoint?: string;
  rttMs?: number;
  lossPercent?: number;
  txBytes?: number;
  rxBytes?: number;
  lastError?: string;
  eligible?: boolean;
}
export interface Status {
  state: string;
  relay?: { name: string; address: string; fingerprint: string };
  paths: Path[];
  mode: "bond";
  mtu: number;
  uptimeSeconds?: number;
  txBytes?: number;
  rxBytes?: number;
  lastError?: string;
  serviceVersion?: string;
  protocolVersion?: string;
  relayVersion?: string;
  singboxVersion?: string;
  adapter?: string;
  wintun?: string;
  routes?: string[];
  recentErrors?: string[];
}
declare global {
  interface Window {
    go?: {
      main: {
        App: {
          Invoke: (
            method: string,
            params: Record<string, unknown>,
          ) => Promise<unknown>;
        };
      };
    };
  }
}
export async function invoke<T = unknown>(
  method: Method,
  params: Record<string, unknown>,
): Promise<T> {
  const bridge = window.go?.main?.App;
  if (!bridge) throw new Error("serviceUnavailable");
  return (await bridge.Invoke(method, params)) as T;
}
export function pathSummary(uri: string): string | undefined {
  try {
    const u = new URL(uri);
    if (u.protocol !== "vless:" || !u.hostname || !u.port || !u.username)
      return undefined;
    return `${u.hostname}:${u.port} · ${u.searchParams.get("type") ?? "tcp"} · ${u.searchParams.get("security") ?? "none"}`;
  } catch {
    return undefined;
  }
}

// Preview only public enrollment fields. The service validates the certificate and pin.
export function relaySummary(
  uri: string,
): { address: string; fingerprint: string } | undefined {
  try {
    const u = new URL(uri);
    const fp = u.searchParams.get("fp");
    if (
      !["conflux:", "bondrelay:"].includes(u.protocol) ||
      !u.hostname ||
      !u.port ||
      u.username ||
      !fp ||
      !/^[a-fA-F0-9]{64}$/.test(fp)
    )
      return undefined;
    return { address: u.host, fingerprint: fp.toLowerCase() };
  } catch {
    return undefined;
  }
}

export function parseOutbound(
  raw: string,
): Record<string, unknown> | undefined {
  try {
    const value: unknown = JSON.parse(raw);
    if (value && typeof value === "object" && !Array.isArray(value))
      return value as Record<string, unknown>;
  } catch {
    /* The service is the authority for detailed schema validation. */
  }
  return undefined;
}
