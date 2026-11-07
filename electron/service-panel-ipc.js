const { MAX_PUBLIC_ORIGIN_LENGTH, MAX_PUBLIC_BASE_URL_LENGTH } = require("./public-access-config");

const SERVICE_PANEL_CHANNELS = Object.freeze({
  snapshot: "myapi:service:snapshot",
  start: "myapi:service:start",
  stop: "myapi:service:stop",
  quit: "myapi:service:quit",
  openConsole: "myapi:service:open-console",
  setLanguage: "myapi:service:set-language",
  copyKnownAddress: "myapi:service:copy-known-address",
  openKnownLogs: "myapi:service:open-known-logs",
  showLAN: "myapi:service:show-lan",
  showPublicHelp: "myapi:service:show-public-help",
  restartLAN: "myapi:service:restart-lan",
  restartLoopback: "myapi:service:restart-loopback",
  savePublicEntry: "myapi:service:save-public-entry",
  clearPublicEntry: "myapi:service:clear-public-entry",
  copyPublicBaseURL: "myapi:service:copy-public-base-url",
  copyProxyTarget: "myapi:service:copy-proxy-target",
});
const SERVICE_PANEL_STATE_CHANNEL = "myapi:service:state";

function createServicePanelIPC({ ipcMain, getWindow, panelURL, actions }) {
  const isTrusted = (event) => {
    try {
      const window = getWindow();
      if (
        !window ||
        window.isDestroyed() ||
        !event ||
        event.sender !== window.webContents ||
        window.webContents.isDestroyed()
      )
        return false;
      return Boolean(
        event.senderFrame &&
        event.senderFrame === window.webContents.mainFrame &&
        event.senderFrame.url === panelURL &&
        window.webContents.getURL() === panelURL,
      );
    } catch {
      // Electron can dispose a frame while a native confirmation is open.
      return false;
    }
  };
  for (const [name, channel] of Object.entries(SERVICE_PANEL_CHANNELS)) {
    ipcMain.handle(channel, async (event, ...args) => {
      if (!isTrusted(event)) throw new Error("Untrusted service panel request.");
      if (name === "setLanguage") {
        if (args.length !== 1 || !["en", "zh"].includes(args[0]))
          throw new Error("Invalid service panel language.");
      } else if (name === "copyKnownAddress") {
        if (
          args.length !== 1 ||
          typeof args[0] !== "string" ||
          args[0].length === 0 ||
          args[0].length > 2048
        )
          throw new Error("Invalid configured address selection.");
      } else if (["savePublicEntry", "clearPublicEntry", "copyPublicBaseURL"].includes(name)) {
        const limit =
          name === "copyPublicBaseURL" ? MAX_PUBLIC_BASE_URL_LENGTH : MAX_PUBLIC_ORIGIN_LENGTH;
        const validValue =
          (name === "clearPublicEntry" && args[0] === null) ||
          (typeof args[0] === "string" && args[0].length > 0 && args[0].length <= limit);
        if (args.length !== 2 || !validValue || !Number.isSafeInteger(args[1]) || args[1] < 0) {
          throw new Error("Invalid public declaration arguments.");
        }
      } else if (name === "copyProxyTarget") {
        if (
          args.length !== 1 ||
          typeof args[0] !== "string" ||
          args[0].length === 0 ||
          args[0].length > MAX_PUBLIC_ORIGIN_LENGTH
        ) {
          throw new Error("Invalid configured proxy target.");
        }
      } else if (args.length !== 0) {
        throw new Error("Unexpected service panel arguments.");
      }
      if (typeof actions[name] !== "function")
        throw new Error("Service panel action is unavailable.");
      const sourceWindow = getWindow();
      const sourceFrame = event.senderFrame;
      const sourceSender = event.sender;
      return actions[name](
        {
          isTrusted: () =>
            getWindow() === sourceWindow &&
            event.senderFrame === sourceFrame &&
            event.sender === sourceSender &&
            isTrusted(event),
        },
        ...args,
      );
    });
  }
  return {
    isTrusted,
    dispose() {
      for (const channel of Object.values(SERVICE_PANEL_CHANNELS)) ipcMain.removeHandler(channel);
    },
  };
}

module.exports = { SERVICE_PANEL_CHANNELS, SERVICE_PANEL_STATE_CHANNEL, createServicePanelIPC };
