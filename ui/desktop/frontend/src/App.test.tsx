import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { messages } from "./i18n";
import { loadPreferences, resolveTheme } from "./preferences";
import { invoke } from "./bridge";

afterEach(cleanup);
beforeEach(() => {
  localStorage.clear();
  delete window.go;
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  });
});
describe("preferences and localization", () => {
  it("has a Polish translation for every English message", () => {
    expect(Object.keys(messages.pl).sort()).toEqual(
      Object.keys(messages.en).sort(),
    );
    expect(messages.pl.connect).toBe("Połącz");
  });
  it("ignores malformed saved preferences", () => {
    localStorage.setItem(
      "conflux.preferences",
      '{"language":"zz","theme":"neon","uri":"secret"}',
    );
    expect(loadPreferences()).toEqual({ language: "en", theme: "system" });
  });
  it("resolves system theme without overwriting the preference", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("light", true)).toBe("light");
  });
});
describe("service boundary", () => {
  it("fails honestly when the Windows service bridge is unavailable", async () => {
    await expect(invoke("status", {})).rejects.toThrow("serviceUnavailable");
  });
  it("does not claim a connected tunnel in browser preview", async () => {
    render(<App />);
    await screen.findByText("Service unavailable");
    expect(
      (screen.getByRole("button", { name: "Connect" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
  it("saves language and theme and translates navigation", async () => {
    render(<App />);
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    fireEvent.change(screen.getByLabelText("Language"), {
      target: { value: "pl" },
    });
    expect(screen.getByRole("button", { name: "Ustawienia" })).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Wygląd"), {
      target: { value: "dark" },
    });
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("dark"),
    );
    expect(
      JSON.parse(localStorage.getItem("conflux.preferences") ?? "{}"),
    ).toEqual({ language: "pl", theme: "dark" });
  });
  it("waits for service state and sends path credentials only to the service", async () => {
    const call = vi.fn(async (method: string) =>
      method === "status"
        ? { state: "disconnected", paths: [], mode: "bond", mtu: 1280 }
        : {},
    );
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Paths" }));
    fireEvent.change(screen.getByLabelText("Path name"), {
      target: { value: "Berlin" },
    });
    fireEvent.change(screen.getByLabelText("VLESS URI"), {
      target: {
        value: "vless://secret-uuid@example.com:443?security=tls&type=tcp",
      },
    });
    expect(screen.getByText("example.com:443 · tcp · tls")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add path" }));
    await waitFor(() =>
      expect(call).toHaveBeenCalledWith("importPath", {
        name: "Berlin",
        uri: "vless://secret-uuid@example.com:443?security=tls&type=tcp",
      }),
    );
    expect(localStorage.getItem("conflux.preferences")).not.toContain(
      "secret-uuid",
    );
  });
});
describe("path and relay actions", () => {
  it("does not claim connection when the connect command fails", async () => {
    window.go = {
      main: {
        App: {
          Invoke: async (method) => {
            if (method === "connect") throw new Error("failure");
            return {
              state: "disconnected",
              paths: [],
              mode: "bond",
              mtu: 1280,
            };
          },
        },
      },
    };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await screen.findByRole("alert");
    expect(screen.queryByText("Connected")).toBeNull();
    expect(screen.getByText("Disconnected")).toBeTruthy();
  });
  it("tests the configured relay using the service and reports success", async () => {
    const call = vi.fn(async (method: string) =>
      method === "status"
        ? {
            state: "disconnected",
            paths: [],
            mode: "bond",
            mtu: 1280,
            relay: {
              name: "Relay",
              address: "relay.example:443",
              fingerprint: "ab".repeat(32),
            },
          }
        : { ok: true },
    );
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Relay" }));
    fireEvent.click(screen.getByRole("button", { name: "Test relay" }));
    await screen.findByText("Relay handshake succeeded.");
    expect(call).toHaveBeenCalledWith("testRelay", {});
  });
  it("allows replacing path credentials without displaying the saved URI", async () => {
    const call = vi.fn(async (method: string) =>
      method === "status"
        ? {
            state: "disconnected",
            paths: [
              {
                id: "p1",
                name: "Berlin",
                enabled: true,
                state: "idle",
                endpoint: "example.com:443",
              },
            ],
            mode: "bond",
            mtu: 1280,
          }
        : {},
    );
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Paths" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(
      screen.getByLabelText("Replacement VLESS URI (optional)"),
      { target: { value: "vless://replacement@example.org:443?security=tls" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(call).toHaveBeenCalledWith("updatePath", {
        id: "p1",
        name: "Berlin",
        enabled: true,
        uri: "vless://replacement@example.org:443?security=tls",
      }),
    );
  });
});
it("closes a path editor with Escape and restores keyboard focus", async () => {
  window.go = {
    main: {
      App: {
        Invoke: async () => ({
          state: "disconnected",
          paths: [{ id: "p1", name: "Berlin", enabled: true, state: "idle" }],
          mode: "bond",
          mtu: 1100,
        }),
      },
    },
  };
  render(<App />);
  await screen.findByText("Disconnected");
  fireEvent.click(screen.getByRole("button", { name: "Paths" }));
  const button = screen.getByRole("button", { name: "Edit" });
  button.focus();
  fireEvent.click(button);
  expect(screen.getByRole("dialog")).toBeTruthy();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(button);
});
it("masks VLESS credentials while showing only endpoint details", async () => {
  render(<App />);
  fireEvent.click(screen.getByRole("button", { name: "Paths" }));
  expect((screen.getByLabelText("VLESS URI") as HTMLInputElement).type).toBe(
    "password",
  );
});
it("requires relay fingerprint verification before sending enrollment", async () => {
  const call = vi.fn(async (method: string) =>
    method === "status"
      ? { state: "disconnected", paths: [], mode: "bond", mtu: 1100 }
      : {},
  );
  window.go = { main: { App: { Invoke: call } } };
  render(<App />);
  await screen.findByText("Disconnected");
  fireEvent.click(screen.getByRole("button", { name: "Relay" }));
  const uri = `conflux://relay.example:443?fp=${"ab".repeat(32)}&token=secret&cert=certificate`;
  fireEvent.change(screen.getByLabelText("Relay connection string"), {
    target: { value: uri },
  });
  expect(screen.getByText("ab".repeat(32))).toBeTruthy();
  expect(
    (screen.getByRole("button", { name: "Import relay" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(
    screen.getByLabelText(
      "I verified this fingerprint with my relay administrator.",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "Import relay" }));
  await waitFor(() =>
    expect(call).toHaveBeenCalledWith("importRelay", { uri }),
  );
});
it("keeps at most one pending status request when the service is slow", () => {
  vi.useFakeTimers();
  try {
    const call = vi.fn(() => new Promise(() => {}));
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    vi.advanceTimersByTime(9000);
    expect(call).toHaveBeenCalledTimes(1);
  } finally {
    vi.useRealTimers();
  }
});
it("imports an advanced outbound object without persisting credentials", async () => {
  const call = vi.fn(async (method: string) =>
    method === "status"
      ? { state: "disconnected", paths: [], mode: "bond", mtu: 1100 }
      : {},
  );
  window.go = { main: { App: { Invoke: call } } };
  render(<App />);
  await screen.findByText("Disconnected");
  fireEvent.click(screen.getByRole("button", { name: "Paths" }));
  fireEvent.change(screen.getByLabelText("Path name"), {
    target: { value: "Advanced" },
  });
  fireEvent.change(screen.getByLabelText("Import format"), {
    target: { value: "advanced" },
  });
  const input = screen.getByLabelText("Advanced sing-box outbound JSON");
  fireEvent.change(input, { target: { value: "[]" } });
  expect(
    (screen.getByRole("button", { name: "Add path" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  const outbound = {
    type: "vless",
    server: "node.example",
    server_port: 443,
    uuid: "private-uuid",
    tls: { enabled: true, server_name: "node.example" },
  };
  fireEvent.change(input, { target: { value: JSON.stringify(outbound) } });
  fireEvent.click(screen.getByRole("button", { name: "Add path" }));
  await waitFor(() =>
    expect(call).toHaveBeenCalledWith("importPath", {
      name: "Advanced",
      outbound,
    }),
  );
  expect(localStorage.getItem("conflux.preferences")).not.toContain(
    "private-uuid",
  );
});
describe("service failures", () => {
  it("shows the service reason for a failed operation", async () => {
    window.go = {
      main: {
        App: {
          Invoke: async (method) => {
            if (method === "connect")
              throw "enable at least two VLESS paths for Bond";
            return { state: "disconnected", paths: [], mode: "bond", mtu: 1100 };
          },
        },
      },
    };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    expect(
      await screen.findByText("enable at least two VLESS paths for Bond"),
    ).toBeTruthy();
  });
  it("offers network recovery when the service reports an error", async () => {
    const call = vi.fn(async () => ({
      state: "error",
      paths: [],
      mode: "bond",
      mtu: 1100,
      lastError: "network recovery incomplete",
    }));
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Restore networking" }),
    );
    await waitFor(() => expect(call).toHaveBeenCalledWith("recover", {}));
  });
});
describe("relay enrollment input", () => {
  const status = { state: "disconnected", paths: [], mode: "bond", mtu: 1100 };
  it("accepts a relay line wrapped across terminal lines", async () => {
    const call = vi.fn(async () => status);
    window.go = { main: { App: { Invoke: call } } };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Relay" }));
    const uri = `conflux://203.0.113.7:4433?cert=MIIBabc&fp=${"cd".repeat(32)}&token=secret`;
    fireEvent.change(screen.getByLabelText("Relay connection string"), {
      target: { value: `${uri.slice(0, 40)}\r\n  ${uri.slice(40)}\n` },
    });
    expect(screen.getByText("cd".repeat(32))).toBeTruthy();
    fireEvent.click(
      screen.getByLabelText(
        "I verified this fingerprint with my relay administrator.",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Import relay" }));
    await waitFor(() =>
      expect(call).toHaveBeenCalledWith("importRelay", { uri }),
    );
  });
  it("explains why an invalid relay line cannot be imported", async () => {
    window.go = { main: { App: { Invoke: async () => status } } };
    render(<App />);
    await screen.findByText("Disconnected");
    fireEvent.click(screen.getByRole("button", { name: "Relay" }));
    fireEvent.change(screen.getByLabelText("Relay connection string"), {
      target: { value: "vless://uuid@example.com:443" },
    });
    expect(screen.getByText(/Not a valid relay line/)).toBeTruthy();
  });
});
