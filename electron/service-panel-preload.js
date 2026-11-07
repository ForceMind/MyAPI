const { contextBridge, ipcRenderer } = require("electron");

// Only fixed operations are available; neither channel names nor paths cross
// this bridge. The main process separately verifies the exact file main frame.
contextBridge.exposeInMainWorld("myapiService", {
  snapshot: () => ipcRenderer.invoke("myapi:service:snapshot"),
  start: () => ipcRenderer.invoke("myapi:service:start"),
  stop: () => ipcRenderer.invoke("myapi:service:stop"),
  quit: () => ipcRenderer.invoke("myapi:service:quit"),
  openConsole: () => ipcRenderer.invoke("myapi:service:open-console"),
  setLanguage: (language) => ipcRenderer.invoke("myapi:service:set-language", language),
  copyKnownAddress: (address) => ipcRenderer.invoke("myapi:service:copy-known-address", address),
  openKnownLogs: () => ipcRenderer.invoke("myapi:service:open-known-logs"),
  showLAN: () => ipcRenderer.invoke("myapi:service:show-lan"),
  showPublicHelp: () => ipcRenderer.invoke("myapi:service:show-public-help"),
  restartLAN: () => ipcRenderer.invoke("myapi:service:restart-lan"),
  restartLoopback: () => ipcRenderer.invoke("myapi:service:restart-loopback"),
  onState: (listener) => {
    if (typeof listener !== "function") return () => {};
    const receive = (_event, snapshot) => listener(snapshot);
    ipcRenderer.on("myapi:service:state", receive);
    return () => ipcRenderer.removeListener("myapi:service:state", receive);
  },
});
