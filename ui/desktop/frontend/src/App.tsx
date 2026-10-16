import { useCallback, useEffect, useRef, useState } from "react";
import {
  invoke,
  pathSummary,
  parseOutbound,
  relaySummary,
  type Method,
  type Path,
  type Status,
} from "./bridge";
import { messages, type MessageKey } from "./i18n";
import {
  loadPreferences,
  resolveTheme,
  savePreferences,
  type Language,
  type Theme,
} from "./preferences";
type Screen = "overview" | "paths" | "relay" | "diagnostics" | "settings";
const screens: Screen[] = [
  "overview",
  "paths",
  "relay",
  "diagnostics",
  "settings",
];
const icons = {
  overview: "◉",
  paths: "⇄",
  relay: "◇",
  diagnostics: "▤",
  settings: "⚙",
};
function bytes(value?: number) {
  if (value === undefined) return "—";
  const units = ["B", "KiB", "MiB", "GiB"];
  let index = 0;
  while (value >= 1024 && index < 3) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(index ? 1 : 0)} ${units[index]}`;
}
export function App() {
  const [preferences, setPreferences] = useState(loadPreferences);
  const t = (key: MessageKey) => messages[preferences.language][key];
  const [screen, setScreen] = useState<Screen>("overview");
  const [status, setStatus] = useState<Status>();
  const [unavailable, setUnavailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [notice, setNotice] = useState("");
  const [replacement, setReplacement] = useState("");
  const [uri, setURI] = useState("");
  const [format, setFormat] = useState("vless");
  const [outboundJSON, setOutboundJSON] = useState("");
  const outbound = parseOutbound(outboundJSON);
  const [name, setName] = useState("");
  const [relayURI, setRelayURI] = useState("");
  const [verified, setVerified] = useState(false);
  const relayPreview = relaySummary(relayURI);
  const [edit, setEdit] = useState<Path>();
  const [deleting, setDeleting] = useState<Path>();
  const [mtu, setMTU] = useState(1100);
  const returnFocus = useRef<HTMLElement | null>(null);
  const modalOpen = Boolean(edit || deleting);
  useEffect(() => {
    if (!modalOpen) return;
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setEdit(undefined);
        setDeleting(undefined);
        setReplacement("");
        return;
      }
      if (event.key !== "Tab") return;
      const controls = Array.from(
        document.querySelectorAll<HTMLElement>(
          ".modal input,.modal textarea,.modal button:not(:disabled)",
        ),
      );
      const first = controls[0],
        last = controls[controls.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    document.addEventListener("keydown", keydown);
    return () => {
      document.removeEventListener("keydown", keydown);
      returnFocus.current?.focus();
    };
  }, [modalOpen]);
  const statusPending = useRef(false);
  const refresh = useCallback(async () => {
    if (statusPending.current) return;
    statusPending.current = true;
    try {
      const next = await invoke<Status>("status", {});
      if (!next || typeof next.state !== "string" || !Array.isArray(next.paths))
        throw new Error("invalidStatus");
      setStatus(next);
      setUnavailable(false);
    } catch {
      setUnavailable(true);
      setStatus(undefined);
    } finally {
      statusPending.current = false;
    }
  }, []);
  useEffect(() => {
    if (busy) return;
    void refresh();
    const timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, 3000);
    return () => clearInterval(timer);
  }, [refresh, busy]);
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme = resolveTheme(
        preferences.theme,
        media.matches,
      );
    };
    apply();
    media.addEventListener("change", apply);
    savePreferences(preferences);
    document.documentElement.lang = preferences.language;
    return () => media.removeEventListener("change", apply);
  }, [preferences]);
  useEffect(() => {
    if (status?.mtu) setMTU(status.mtu);
  }, [status?.mtu]);
  async function run(
    method: Method,
    params: Record<string, unknown> = {},
    done?: () => void,
  ) {
    if (busy) return;
    setBusy(true);
    setError(false);
    setNotice("");
    try {
      const result = await invoke<{ path?: string }>(method, params);
      done?.();
      if (method === "diagnostics" && result.path) setNotice(result.path);
      else if (method === "testRelay") setNotice("relayTested");
      await refresh();
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  }
  const state = (value: string) =>
    Object.hasOwn(messages.en, value)
      ? t(value as MessageKey)
      : t("unavailable");
  const active = status?.state === "connected" || status?.state === "degraded";
  const stopped = status?.state === "disconnected" || status?.state === "error";
  const paths = status?.paths ?? [];
  const locked = busy || unavailable || !status;
  const editable = locked || !stopped;
  const healthy = paths.filter(
    (p) => p.enabled && ["healthy", "connected", "ready"].includes(p.state),
  ).length;
  function pathRows(compact = false) {
    return paths.length ? (
      <div className="path-list">
        {paths.map((path) => (
          <article className="path-row" key={path.id}>
            <div className="path-identity">
              <strong>{path.name}</strong>
              <span>{path.endpoint ?? "—"}</span>
            </div>
            <span className={`badge ${path.state}`}>
              {path.enabled ? state(path.state) : t("disabled")}
            </span>
            <div className="path-metrics">
              <span>
                {t("latency")}{" "}
                <b>
                  {path.rttMs === undefined
                    ? "—"
                    : `${path.rttMs.toFixed(0)} ms`}
                </b>
              </span>
              <span>
                {t("loss")}{" "}
                <b>
                  {path.lossPercent === undefined
                    ? "—"
                    : `${path.lossPercent.toFixed(1)}%`}
                </b>
              </span>
            </div>
            {!compact && (
              <>
                <span className="eligibility">
                  {t("eligibility")}:{" "}
                  {path.eligible ||
                  ["ready", "connected", "healthy"].includes(path.state)
                    ? t("eligible")
                    : t("notTested")}
                </span>
                <div className="actions">
                  <label className="toggle">
                    <input
                      type="checkbox"
                      checked={path.enabled}
                      disabled={editable}
                      onChange={() =>
                        void run("updatePath", {
                          id: path.id,
                          name: path.name,
                          enabled: !path.enabled,
                        })
                      }
                    />
                    {t("enabled")}
                  </label>
                  <button
                    disabled={locked}
                    onClick={() => void run("testPath", { id: path.id })}
                  >
                    {t("test")}
                  </button>
                  <button
                    disabled={editable}
                    onClick={(event) => {
                      returnFocus.current = event.currentTarget;
                      setReplacement("");
                      setEdit({ ...path });
                    }}
                  >
                    {t("edit")}
                  </button>
                  <button
                    className="danger"
                    disabled={editable}
                    onClick={(event) => {
                      returnFocus.current = event.currentTarget;
                      setDeleting(path);
                    }}
                  >
                    {t("delete")}
                  </button>
                </div>
              </>
            )}
            {path.lastError && <p className="inline-error">{path.lastError}</p>}
          </article>
        ))}
      </div>
    ) : (
      <div className="empty">
        <span className="empty-symbol">⇄</span>
        <strong>{t("noPaths")}</strong>
        <p>{t("noPathsHelp")}</p>
      </div>
    );
  }
  return (
    <div className="app">
      <aside inert={modalOpen}>
        <div className="brand">
          <span className="brand-mark">C</span>
          <div>
            Conflux<small>VPN</small>
          </div>
        </div>
        <nav aria-label="Conflux">
          {screens.map((item) => (
            <button
              key={item}
              className={screen === item ? "selected" : ""}
              aria-current={screen === item ? "page" : undefined}
              onClick={() => {
                setScreen(item);
                setNotice("");
                setError(false);
              }}
            >
              <span aria-hidden="true">{icons[item]}</span>
              {t(item)}
            </button>
          ))}
        </nav>
        <div className="sidebar-footer">
          <span className={`status-dot ${active ? "on" : ""}`} />
          {t("bond")}
          <small>Conflux · Windows</small>
        </div>
      </aside>
      <main inert={modalOpen}>
        <header>
          <div>
            <p className="eyebrow">CONFLUX / {t("bond")}</p>
            <h1>{t(screen)}</h1>
          </div>
          <button disabled={busy} onClick={() => void refresh()}>
            {t("refresh")}
          </button>
        </header>
        {unavailable && (
          <section className="alert" role="alert">
            <strong>{t("serviceUnavailable")}</strong>
            <p>{t("serviceHelp")}</p>
            <button onClick={() => void refresh()}>{t("retry")}</button>
          </section>
        )}
        {error && (
          <section className="alert" role="alert">
            <strong>{t("operationFailed")}</strong>
            <p>
              {active
                ? t("protected")
                : stopped
                  ? t("unprotected")
                  : t("uncertain")}
            </p>
          </section>
        )}
        {busy && (
          <p role="status" className="working">
            {t("working")}
          </p>
        )}
        {screen === "overview" && (
          <>
            <section className="panel connection">
              <div>
                <span className={`badge ${status?.state ?? ""}`}>
                  {status ? state(status.state) : "—"}
                </span>
                <h2>{t("connection")}</h2>
                <p>{t("connectionHint")}</p>
                <p className="protection">
                  {active
                    ? t("protected")
                    : stopped
                      ? t("unprotected")
                      : t("uncertain")}
                </p>
                {status?.lastError && (
                  <p className="inline-error">{status.lastError}</p>
                )}
                {status?.state === "degraded" && (
                  <p className="warning">{t("recovering")}</p>
                )}
              </div>
              <button
                className={`connect-button ${active ? "disconnect" : ""}`}
                disabled={locked || (!active && !stopped)}
                onClick={() => void run(active ? "disconnect" : "connect")}
              >
                <span aria-hidden="true">⏻</span>
                {active ? t("disconnect") : t("connect")}
              </button>
              <div className="connection-relay">
                <span>{t("relay")}</span>
                <strong>{status?.relay?.name ?? t("noRelay")}</strong>
                <small>{status?.relay?.address ?? "—"}</small>
              </div>
            </section>
            <section className="metrics" aria-label={t("connection")}>
              {[
                [t("upload"), bytes(status?.txBytes)],
                [t("download"), bytes(status?.rxBytes)],
                [
                  t("duration"),
                  status?.uptimeSeconds === undefined
                    ? "—"
                    : `${Math.floor(status.uptimeSeconds / 60)}:${String(Math.floor(status.uptimeSeconds % 60)).padStart(2, "0")}`,
                ],
                [
                  t("healthyPaths"),
                  status
                    ? `${healthy} / ${paths.filter((p) => p.enabled).length}`
                    : "—",
                ],
              ].map(([label, value]) => (
                <div className="metric" key={label}>
                  <span>{label}</span>
                  <strong>{value}</strong>
                </div>
              ))}
            </section>
            <section className="panel">
              <div className="section-heading">
                <h2>{t("paths")}</h2>
                <button onClick={() => setScreen("paths")}>
                  {t("addPath")}
                </button>
              </div>
              {pathRows(true)}
            </section>
          </>
        )}
        {screen === "paths" && (
          <>
            <section className="panel">
              <h2>{t("addPath")}</h2>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(
                    "importPath",
                    format === "advanced" ? { name, outbound } : { name, uri },
                    () => {
                      setOutboundJSON("");
                      setURI("");
                      setName("");
                    },
                  );
                }}
              >
                <label>
                  {t("pathName")}
                  <input
                    required
                    maxLength={80}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <label>
                  {t("importFormat")}
                  <select
                    aria-label={t("importFormat")}
                    value={format}
                    onChange={(e) => {
                      setFormat(e.target.value);
                      setURI("");
                      setOutboundJSON("");
                    }}
                  >
                    <option value="vless">{t("vlessFormat")}</option>
                    <option value="advanced">{t("advancedFormat")}</option>
                  </select>
                </label>
                {format === "vless" ? (
                  <>
                    <label>
                      {t("vlessURI")}
                      <input
                        type="password"
                        required
                        spellCheck={false}
                        autoComplete="off"
                        maxLength={16000}
                        value={uri}
                        onChange={(e) => setURI(e.target.value)}
                      />
                    </label>
                    {uri && (
                      <div className="summary">
                        <strong>{t("summary")}</strong>
                        <span>{pathSummary(uri) ?? t("invalidURI")}</span>
                      </div>
                    )}
                  </>
                ) : (
                  <>
                    <label>
                      {t("advancedJSON")}
                      <input
                        type="password"
                        required
                        autoComplete="off"
                        spellCheck={false}
                        maxLength={16000}
                        value={outboundJSON}
                        onChange={(e) => setOutboundJSON(e.target.value)}
                      />
                    </label>
                    {outboundJSON && (
                      <div className="summary">
                        <strong>{t("summary")}</strong>
                        <span>
                          {outbound && typeof outbound.server === "string"
                            ? `${outbound.server}:${typeof outbound.server_port === "number" ? outbound.server_port : "—"}`
                            : t("invalidJSON")}
                        </span>
                      </div>
                    )}
                  </>
                )}
                <p className="hint">{t("importHint")}</p>
                <button
                  className="primary"
                  disabled={
                    editable ||
                    (format === "advanced" ? !outbound : !pathSummary(uri)) ||
                    !name.trim()
                  }
                >
                  {t("addPath")}
                </button>
              </form>
            </section>
            <section className="panel">
              <h2>{t("paths")}</h2>
              {pathRows()}
            </section>
          </>
        )}
        {screen === "relay" && (
          <>
            <section className="panel">
              <h2>{status?.relay?.name ?? t("noRelay")}</h2>
              <dl>
                <dt>{t("address")}</dt>
                <dd>{status?.relay?.address ?? "—"}</dd>
                <dt>{t("fingerprint")}</dt>
                <dd className="mono">{status?.relay?.fingerprint ?? "—"}</dd>
              </dl>
              <button
                disabled={locked || !status?.relay}
                onClick={() => void run("testRelay")}
              >
                {t("testRelay")}
              </button>
              {notice === "relayTested" && (
                <p role="status">{t("relayTested")}</p>
              )}
            </section>
            <section className="panel">
              <h2>{t("importRelay")}</h2>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run("importRelay", { uri: relayURI }, () =>
                    setRelayURI(""),
                  );
                }}
              >
                <label>
                  {t("relayURI")}
                  <input
                    type="password"
                    required
                    value={relayURI}
                    maxLength={16000}
                    autoComplete="off"
                    spellCheck={false}
                    onChange={(e) => {
                      setRelayURI(e.target.value);
                      setVerified(false);
                    }}
                  />
                </label>
                <p className="hint">{t("relayHint")}</p>
                {relayPreview && (
                  <>
                    <dl>
                      <dt>{t("address")}</dt>
                      <dd>{relayPreview.address}</dd>
                      <dt>{t("fingerprint")}</dt>
                      <dd className="mono">{relayPreview.fingerprint}</dd>
                    </dl>
                    <label className="toggle">
                      <input
                        type="checkbox"
                        checked={verified}
                        onChange={(e) => setVerified(e.target.checked)}
                      />
                      {t("verifyFingerprint")}
                    </label>
                  </>
                )}
                <button
                  className="primary"
                  disabled={editable || !relayPreview || !verified}
                >
                  {t("importRelay")}
                </button>
              </form>
            </section>
          </>
        )}
        {screen === "diagnostics" && (
          <>
            <section className="panel">
              <h2>{t("diagnostics")}</h2>
              <p className="hint">{t("diagnosticsHint")}</p>
              <button
                className="primary"
                disabled={locked}
                onClick={() => void run("diagnostics")}
              >
                {t("exportDiagnostics")}
              </button>
              {notice && (
                <p role="status">
                  {t("exported")} <span className="mono">{notice}</span>
                </p>
              )}
              <dl>
                {(
                  [
                    "serviceVersion",
                    "protocolVersion",
                    "relayVersion",
                    "singboxVersion",
                    "adapter",
                    "wintun",
                  ] as const
                ).map((key) => (
                  <div key={key}>
                    <dt>{t(key)}</dt>
                    <dd>{status?.[key] ?? t("unavailable")}</dd>
                  </div>
                ))}
              </dl>
            </section>
            <section className="panel">
              <h2>{t("routes")}</h2>
              <pre>{status?.routes?.join("\n") ?? t("unavailable")}</pre>
              <h2>{t("recentErrors")}</h2>
              <pre>{status?.recentErrors?.join("\n") ?? t("unavailable")}</pre>
            </section>
          </>
        )}
        {screen === "settings" && (
          <>
            <section className="panel">
              <h2>{t("settings")}</h2>
              <p className="hint">{t("preferencesHint")}</p>
              <div className="settings-grid">
                <label>
                  {t("language")}
                  <select
                    aria-label={t("language")}
                    value={preferences.language}
                    onChange={(e) =>
                      setPreferences({
                        ...preferences,
                        language: e.target.value as Language,
                      })
                    }
                  >
                    <option value="en">English</option>
                    <option value="pl">Polski</option>
                  </select>
                </label>
                <label>
                  {t("appearance")}
                  <select
                    aria-label={t("appearance")}
                    value={preferences.theme}
                    onChange={(e) =>
                      setPreferences({
                        ...preferences,
                        theme: e.target.value as Theme,
                      })
                    }
                  >
                    <option value="system">{t("system")}</option>
                    <option value="light">{t("light")}</option>
                    <option value="dark">{t("dark")}</option>
                  </select>
                </label>
              </div>
            </section>
            <section className="panel">
              <h2>{t("advanced")}</h2>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run("configure", { mode: "bond", mtu });
                }}
              >
                <label>
                  {t("mtu")}
                  <input
                    type="number"
                    min={576}
                    max={1100}
                    required
                    value={mtu}
                    onChange={(e) => setMTU(Number(e.target.value))}
                  />
                </label>
                <button className="primary" disabled={editable}>
                  {t("apply")}
                </button>
              </form>
            </section>
          </>
        )}
      </main>
      {edit && (
        <div className="modal-backdrop">
          <section
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="edit-title"
          >
            <h2 id="edit-title">{t("edit")}</h2>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void run(
                  "updatePath",
                  {
                    id: edit.id,
                    name: edit.name,
                    enabled: edit.enabled,
                    ...(replacement ? { uri: replacement } : {}),
                  },
                  () => {
                    setReplacement("");
                    setEdit(undefined);
                  },
                );
              }}
            >
              <label>
                {t("pathName")}
                <input
                  autoFocus
                  required
                  maxLength={80}
                  value={edit.name}
                  onChange={(e) => setEdit({ ...edit, name: e.target.value })}
                />
              </label>
              <label>
                {t("replacementURI")}
                <input
                  type="password"
                  value={replacement}
                  onChange={(e) => setReplacement(e.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  maxLength={16000}
                />
              </label>
              {replacement && (
                <p className="summary">
                  {pathSummary(replacement) ?? t("invalidURI")}
                </p>
              )}
              <div className="actions">
                <button type="button" onClick={() => setEdit(undefined)}>
                  {t("cancel")}
                </button>
                <button className="primary" disabled={busy}>
                  {t("save")}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
      {deleting && (
        <div className="modal-backdrop">
          <section
            className="modal"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="delete-title"
          >
            <h2 id="delete-title">{t("deleteQuestion")}</h2>
            <p>{deleting.name}</p>
            <div className="actions">
              <button autoFocus onClick={() => setDeleting(undefined)}>
                {t("cancel")}
              </button>
              <button
                className="danger"
                disabled={busy}
                onClick={() =>
                  void run("deletePath", { id: deleting.id }, () =>
                    setDeleting(undefined),
                  )
                }
              >
                {t("confirmDelete")}
              </button>
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
