const { app, BrowserWindow, dialog, Tray, Menu, shell, ipcMain, clipboard } = require("electron");
const { spawn } = require("child_process");
const crypto = require("crypto");
const path = require("path");
const { pathToFileURL } = require("url");
const http = require("http");
const fs = require("fs");
const {
  DEFAULT_BIND_ADDRESS,
  isLoopbackAddress,
  isPrivateAddress,
  getPrivateIPv4Candidates,
  buildRelaunchArgs,
  resolveRuntimeConfig,
  describeRuntimeConfig,
  getHealthCheckAddress,
  isSuccessfulHttpStatus,
  isSuccessfulStatusResponse,
} = require("./runtime-config");
const { createServiceController } = require("./service-controller");
const { createServicePanelIPC, SERVICE_PANEL_STATE_CHANNEL } = require("./service-panel-ipc");
const { createNativeI18n, normalizeLanguage } = require("./native-i18n");
const { createServiceDiagnostics } = require("./service-diagnostics");

const APP_NAME = "MyAPI";
const CANONICAL_BINARY_NAME = process.platform === "win32" ? "my-api.exe" : "my-api";
const LEGACY_BINARY_NAME = process.platform === "win32" ? "new-api.exe" : "new-api";
const CANONICAL_DATABASE_NAME = "my-api.db";
const LEGACY_DATABASE_NAMES = ["new-api.db", "one-api.db"];
const DESKTOP_ARGS = process.argv.slice(1);
const DEV_FRONTEND_PORT = 5173;
const IS_DEV = process.env.NODE_ENV === "development";
const PANEL_FILE = path.join(__dirname, "service-panel.html");
const PANEL_URL = pathToFileURL(PANEL_FILE).href;
let mainWindow = null;
let serviceWindow = null;
let tray = null;
let controller = null;
let actionBusy = false;
let allowExit = false;
let notice = null;
let nativeReason = null;
let launchDiagnostics = createServiceDiagnostics(0);
let stateRevision = 0;
const developmentProbe = new AbortController();
let translator = createNativeI18n();
const t = (key, values) => translator.t(key, values);
let runtimeConfig;
try {
  runtimeConfig = resolveRuntimeConfig({ args: DESKTOP_ARGS, env: process.env });
} catch (error) {
  runtimeConfig = {
    bindAddress: DEFAULT_BIND_ADDRESS,
    port: 3000,
    allowLan: false,
    isLan: false,
    error,
  };
}
const PORT = runtimeConfig.port;
const BIND_ADDRESS = runtimeConfig.bindAddress;
const ALLOW_LAN = runtimeConfig.allowLan;

// Only these product strings enter the local renderer, never process.env,
// raw backend output, controller identities or session secret values.
const PANEL_KEYS = [
  "Application version",
  "Automatic updates are off.",
  "Local service",
  "Language",
  "Bundled service",
  "Open console",
  "Start bundled service",
  "Stop bundled service",
  "OpenAI-compatible Base URL",
  "Copy address",
  "Access scope",
  "This is the configured address, not proof of reachability or model availability.",
  "Public access is not configured by this panel and has not been verified. A separate setup is required.",
  "LAN status and help",
  "Public access help",
  "Change local access scope",
  "Changing the listener requires confirmation and an application restart.",
  "Restart with LAN access",
  "Restart with local access only",
  "Data and logs",
  "The data folder is not a complete backup guarantee. Explicit database paths and consistency requirements still apply.",
  "Save diagnostics and open logs",
  "Original diagnostic logs may use their original language. They are not displayed in this panel.",
  "Closing this window keeps MyAPI in the tray. Quitting stops its bundled service first.",
  "Quit MyAPI",
  "No configured address is available.",
  "Application data",
  "Default data folder",
  "Database location",
  "Session secret storage",
  "Logs folder",
  "Service detail",
  "Diagnostic guidance",
  "Not provided",
  "The action could not be completed. Reopen the service panel and review its state.",
  "The service panel bridge is unavailable. Reopen it from the tray.",
  "Service state unknown",
  "Saving diagnostics writes bounded original stderr to a private local file. Review it before sharing.",
];

function locations() {
  const userDataPath = app.getPath("userData");
  const dataDir = path.join(userDataPath, "data");
  const logsDir = path.join(userDataPath, "logs");
  const legacyDatabasePath = LEGACY_DATABASE_NAMES.map((name) => path.join(dataDir, name)).find(
    (candidate) => fs.existsSync(candidate),
  );
  const databasePath =
    process.env.MYAPI_SQLITE_PATH ||
    process.env.SQLITE_PATH ||
    legacyDatabasePath ||
    path.join(dataDir, CANONICAL_DATABASE_NAME);
  return { userDataPath, dataDir, logsDir, databasePath };
}

function recordNativeReason(code) {
  nativeReason = code;
  try {
    const { logsDir } = locations();
    fs.mkdirSync(logsDir, { recursive: true });
    // A fixed category is enough to inspect native startup/stop failures;
    // raw upstream payloads and environment values are not copied here.
    fs.appendFileSync(
      path.join(logsDir, "desktop-service.log"),
      `${new Date().toISOString()} ${code}\n`,
      { mode: 0o600 },
    );
  } catch {
    /* The panel still reports the operation result when logging fails. */
  }
}

function ensureSessionSecret(userDataPath) {
  const secretPath = path.join(userDataPath, "myapi-session-secret");
  try {
    if (fs.existsSync(secretPath)) {
      const existing = fs.readFileSync(secretPath, "utf8").trim();
      if (/^[a-f0-9]{64}$/i.test(existing)) return existing;
    }
    const generated = crypto.randomBytes(32).toString("hex");
    fs.writeFileSync(secretPath, `${generated}\n`, { mode: 0o600 });
    try {
      fs.chmodSync(secretPath, 0o600);
    } catch {
      /* Platform ACLs may apply. */
    }
    return generated;
  } catch {
    recordNativeReason("session-secret-write-failed");
    throw new Error(t("Unable to save the session secret."));
  }
}

function getBinaryPath() {
  if (IS_DEV) {
    const canonicalPath = path.join(__dirname, "..", CANONICAL_BINARY_NAME);
    return fs.existsSync(canonicalPath)
      ? canonicalPath
      : path.join(__dirname, "..", LEGACY_BINARY_NAME);
  }
  const canonicalPath = path.join(process.resourcesPath, "bin", CANONICAL_BINARY_NAME);
  return fs.existsSync(canonicalPath)
    ? canonicalPath
    : path.join(process.resourcesPath, "bin", LEGACY_BINARY_NAME);
}

// The controller owns cancellation. A late response cannot promote another
// generation, and an aborted request never schedules another retry.
function checkServerAvailability(
  port,
  maxRetries = 30,
  retryDelay = 1000,
  hostname = "127.0.0.1",
  requestPath = "/",
  { signal } = {},
) {
  return new Promise((resolve, reject) => {
    let attempts = 0;
    let settled = false;
    let request = null;
    let retryTimer = null;
    const finish = (error) => {
      if (settled) return;
      settled = true;
      if (retryTimer !== null) clearTimeout(retryTimer);
      signal?.removeEventListener("abort", abort);
      if (request) request.destroy();
      if (error) reject(error);
      else resolve();
    };
    const abort = () => finish(new Error("Service readiness check cancelled."));
    const tryConnect = () => {
      if (settled || signal?.aborted) {
        abort();
        return;
      }
      attempts++;
      let completed = false;
      const retry = (error) => {
        if (completed || settled) return;
        completed = true;
        if (request) request.destroy();
        if (attempts >= maxRetries) finish(error);
        else retryTimer = setTimeout(tryConnect, retryDelay);
      };
      request = http.get(
        { hostname: getHealthCheckAddress(hostname), port, path: requestPath, timeout: 10000 },
        (response) => {
          const statusCode = Number(response.statusCode || 0);
          let responseBody = "";
          if (requestPath === "/api/status") {
            response.setEncoding("utf8");
            response.on("data", (chunk) => {
              if (responseBody.length + chunk.length > 65536) {
                retry(new Error("Backend status response exceeded its limit."));
                return;
              }
              responseBody += chunk;
            });
          } else response.resume();
          response.on("error", retry);
          response.on("end", () => {
            if (completed || settled) return;
            const successful =
              requestPath === "/api/status"
                ? isSuccessfulStatusResponse(statusCode, responseBody)
                : isSuccessfulHttpStatus(statusCode);
            if (!successful) {
              retry(new Error(`Unexpected HTTP response from ${requestPath}`));
              return;
            }
            completed = true;
            finish();
          });
        },
      );
      request.on("error", retry);
      request.on("timeout", () => retry(new Error("Service readiness check timed out.")));
    };
    signal?.addEventListener("abort", abort, { once: true });
    tryConnect();
  });
}

// This factory preserves existing startup configuration; only the controller
// retains the returned child and decides whether a generation is still current.
function startServer({ signal, generation }) {
  const diagnostics = createServiceDiagnostics(generation);
  launchDiagnostics = diagnostics;
  try {
    if (IS_DEV) throw new Error("External development services are not owned.");
    if (runtimeConfig.error) throw new Error("Invalid desktop listener configuration.");
    if (!isLoopbackAddress(BIND_ADDRESS) && (!ALLOW_LAN || !isPrivateAddress(BIND_ADDRESS)))
      throw new Error("Invalid desktop LAN configuration.");
    const { userDataPath, dataDir, logsDir, databasePath } = locations();
    fs.mkdirSync(dataDir, { recursive: true });
    fs.mkdirSync(logsDir, { recursive: true });
    process.env.ELECTRON_DATA_DIR = dataDir;
    const env = {
      ...process.env,
      PORT: PORT.toString(),
      MYAPI_BIND_ADDRESS: BIND_ADDRESS,
      MYAPI_EDITION: "lite",
      SESSION_COOKIE_SECURE: "false",
      FULL_CONTENT_LOG_DIR: path.join(logsDir, "full-content"),
    };
    env.SESSION_SECRET = process.env.SESSION_SECRET || ensureSessionSecret(userDataPath);
    env.SQLITE_PATH = databasePath;
    const binaryPath = getBinaryPath();
    const child = spawn(binaryPath, ["--log-dir", logsDir], { env, cwd: process.resourcesPath });
    child.stdout?.on("data", (data) => {
      if (launchDiagnostics === diagnostics) console.log(`Server: ${data}`);
    });
    child.stderr?.on("data", (data) => {
      if (launchDiagnostics !== diagnostics) return;
      diagnostics.append(data);
      console.error(`Server Error: ${data}`);
    });
    child.on("error", (error) => {
      if (launchDiagnostics !== diagnostics) return;
      diagnostics.append(`${error.code || ""}: ${error.message || ""}\n`);
      recordNativeReason(error.code === "ENOENT" ? "binary-missing" : "spawn-failed");
    });
    const healthCheckHost = getHealthCheckAddress(BIND_ADDRESS);
    const ready = checkServerAvailability(PORT, 30, 1000, healthCheckHost, "/api/status", {
      signal,
    }).catch((error) => {
      if (!signal?.aborted && launchDiagnostics === diagnostics)
        recordNativeReason("readiness-failed");
      throw error;
    });
    return { child, ready };
  } catch (error) {
    diagnostics.append(`${error.code || ""}: ${error.stack || error.message || String(error)}\n`);
    recordNativeReason("launch-failed");
    throw error;
  }
}

function syncDockVisibility() {
  if (process.platform !== "darwin" || !app.dock) return;
  const visible = [serviceWindow, mainWindow].some(
    (window) => window && !window.isDestroyed() && window.isVisible(),
  );
  if (visible) Promise.resolve(app.dock.show()).catch(() => {});
  else app.dock.hide();
}

function closeToTray(window) {
  window.on("close", (event) => {
    if (allowExit) return;
    event.preventDefault();
    window.hide();
    syncDockVisibility();
  });
  window.on("show", syncDockVisibility);
  window.on("hide", syncDockVisibility);
}

function showServicePanel() {
  if (!serviceWindow || serviceWindow.isDestroyed()) {
    serviceWindow = new BrowserWindow({
      width: 900,
      height: 760,
      minWidth: 480,
      minHeight: 520,
      title: APP_NAME,
      icon: path.join(__dirname, "icon.png"),
      webPreferences: {
        preload: path.join(__dirname, "service-panel-preload.js"),
        nodeIntegration: false,
        contextIsolation: true,
        sandbox: true,
      },
    });
    const window = serviceWindow;
    closeToTray(window);
    window.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    window.webContents.on("will-navigate", (event, url) => {
      if (url !== PANEL_URL) event.preventDefault();
    });
    window.webContents.on("will-redirect", (event) => event.preventDefault());
    window.webContents.on("will-attach-webview", (event) => event.preventDefault());
    window.webContents.on("did-finish-load", publishState);
    window.on("closed", () => {
      if (serviceWindow === window) serviceWindow = null;
      syncDockVisibility();
    });
    void window.loadFile(PANEL_FILE).catch(() => {
      notice = {
        key: "The service panel could not be opened. Use the tray controls or inspect local logs.",
        tone: "warning",
      };
      recordNativeReason("panel-load-failed");
      rebuildTray();
    });
  }
  serviceWindow.show();
  serviceWindow.focus();
  syncDockVisibility();
}

function createWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.show();
    mainWindow.focus();
    syncDockVisibility();
    return;
  }
  const loadPort = IS_DEV ? DEV_FRONTEND_PORT : PORT;
  const loadHost = IS_DEV ? "127.0.0.1" : getHealthCheckAddress(BIND_ADDRESS);
  mainWindow = new BrowserWindow({
    width: 1080,
    height: 720,
    title: APP_NAME,
    icon: path.join(__dirname, "icon.png"),
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      nodeIntegration: false,
      contextIsolation: true,
    },
  });
  const window = mainWindow;
  closeToTray(window);
  window.on("closed", () => {
    if (mainWindow === window) mainWindow = null;
    syncDockVisibility();
  });
  void mainWindow.loadURL(`http://${loadHost}:${loadPort}`).catch(() => {
    notice = {
      key: "The console could not be opened. The service panel remains available.",
      tone: "warning",
    };
    showServicePanel();
    publishState();
  });
  syncDockVisibility();
}

function stateText(state) {
  switch (state) {
    case "starting":
      return [
        t("Starting"),
        t("The bundled process is starting. Its interface is not ready yet."),
        "neutral",
      ];
    case "running":
      return [
        t("Running"),
        t(
          "The owned bundled process is running and its local status endpoint responded. Model availability is not verified.",
        ),
        "success",
      ];
    case "stopping":
      return [t("Stopping"), t("Waiting for the owned bundled process to close."), "neutral"];
    case "failed":
      return [
        t("Needs attention"),
        t("The bundled service did not start or exited. Review local logs before trying again."),
        "warning",
      ];
    case "stop-unconfirmed":
      return [
        t("Stop not confirmed"),
        t(
          "The owned process has not confirmed closure. Keep MyAPI open; another process will not be started.",
        ),
        "warning",
      ];
    case "external":
      return [
        t("External development service"),
        t("Development servers are started outside MyAPI. This panel cannot start or stop them."),
        "neutral",
      ];
    default:
      return [
        t("Stopped"),
        t("The bundled service is stopped. MyAPI remains available in the tray."),
        "neutral",
      ];
  }
}

function reasonLabel(reason) {
  if (runtimeConfig.error)
    return t(
      "Startup settings are invalid. Restore local access or relaunch with valid private-network settings.",
    );
  if (nativeReason === "binary-missing")
    return t("The bundled program could not be found. Check the application package.");
  if (nativeReason === "session-secret-write-failed")
    return t(
      "The persistent session secret could not be saved. Check the application data folder permissions.",
    );
  switch (reason) {
    case "start-unverified":
      return t("No readiness evidence was returned. Stop the owned process before trying again.");
    case "start-failed":
      return t(
        "The local status check did not confirm readiness. Stop the owned process before trying again.",
      );
    case "launch-failed":
      return t("The bundled program could not be launched. Open local logs for details.");
    case "process-error":
      return t("The owned process reported a startup or process error.");
    case "process-exited":
      return t("The owned process exited. No automatic restart or request replay was performed.");
    case "stop-timeout":
      return t("Closure was not confirmed before the stop deadline. MyAPI will remain open.");
    case "externally-managed":
      return t("Development service lifecycle is managed outside MyAPI.");
    default:
      break;
  }
  return t("No classified service error is available.");
}

function diagnosticText() {
  switch (launchDiagnostics.category()) {
    case "port":
      return {
        summary: t("The configured port is unavailable."),
        guidance: t(
          "Check whether another instance or application uses the configured port. Do not stop an unrelated process from this panel.",
        ),
      };
    case "database":
      return {
        summary: t("The configured database needs attention."),
        guidance: t(
          "Check the actual database location and whether another instance is using it. Preserve database, WAL and SHM files and use a consistent backup before recovery.",
        ),
      };
    case "permission":
      return {
        summary: t("Application data access needs attention."),
        guidance: t(
          "Check the application, database and log folder permissions for your normal account. Ask the administrator if access is required.",
        ),
      };
    case "network":
      return {
        summary: t("A service network connection failed."),
        guidance: t(
          "Check the configured address, network and firewall. This panel does not change proxy or provider settings.",
        ),
      };
    case "configuration":
      return {
        summary: t("The service configuration needs attention."),
        guidance: t(
          "Review the original diagnostic log and saved configuration. Preserve existing files rather than deleting or replacing them as a recovery shortcut.",
        ),
      };
    case "memory":
      return {
        summary: t("The service reported insufficient memory."),
        guidance: t(
          "Review available system memory and other running applications before starting the service again.",
        ),
      };
    case "missing-file":
      return {
        summary: t("A required service file could not be found."),
        guidance: t(
          "Check the installed application package and configured file locations. Preserve existing data when repairing the installation.",
        ),
      };
    default:
      return {
        summary: t("No specific startup error category is available."),
        guidance: t(
          "Save and open the original diagnostic log to inspect the failure. Original text may contain private information and may use another language.",
        ),
      };
  }
}

function snapshot() {
  const service = controller?.snapshot() || {
    state: "stopped",
    generation: 0,
    owned: false,
    pending: false,
    reason: null,
  };
  const [stateLabel, stateDescription, stateTone] = runtimeConfig.error
    ? [
        t("Needs attention"),
        t(
          "Startup settings are invalid. Restore local access or relaunch with valid private-network settings.",
        ),
        "warning",
      ]
    : stateText(service.state);
  const { userDataPath, dataDir, logsDir, databasePath } = locations();
  const addresses = runtimeConfig.error
    ? []
    : describeRuntimeConfig(runtimeConfig, process.platform).endpoints.map(
        (endpoint) => `${endpoint}/v1`,
      );
  const explicitDatabase = Boolean(process.env.SQL_DSN && process.env.SQL_DSN !== "local");
  const diagnostic = diagnosticText();
  return {
    revision: stateRevision,
    applicationVersion: app.getVersion(),
    state: service.state,
    generation: service.generation,
    owned: service.owned,
    pending: service.pending,
    stateLabel,
    stateDescription:
      service.state === "failed" &&
      launchDiagnostics.category() !== "unknown" &&
      !runtimeConfig.error
        ? diagnostic.summary
        : stateDescription,
    stateTone,
    reasonLabel: reasonLabel(service.reason),
    diagnostic: {
      category: launchDiagnostics.category(),
      summary: diagnostic.summary,
      guidance: diagnostic.guidance,
      hasOriginal: launchDiagnostics.hasOriginal(),
      truncated: launchDiagnostics.isTruncated(),
    },
    language: translator.language,
    scopeLabel: runtimeConfig.error
      ? t("Access scope unknown")
      : runtimeConfig.isLan
        ? t("Private LAN")
        : t("This computer only"),
    addresses,
    canStart:
      !IS_DEV &&
      !actionBusy &&
      !runtimeConfig.error &&
      !service.owned &&
      ["stopped", "failed"].includes(service.state),
    canStop:
      !IS_DEV &&
      !actionBusy &&
      service.owned &&
      ["starting", "running", "failed"].includes(service.state),
    canQuit: !actionBusy && !["stopping", "stop-unconfirmed"].includes(service.state),
    canRestart: !IS_DEV && !actionBusy && !["stopping", "stop-unconfirmed"].includes(service.state),
    canOpenConsole: !actionBusy && (IS_DEV || service.state === "running"),
    messages: Object.fromEntries(PANEL_KEYS.map((key) => [key, t(key)])),
    notice: notice ? { message: t(notice.key, notice.values), tone: notice.tone } : null,
    locations: {
      userData: userDataPath,
      dataDirectory: dataDir,
      logsDirectory: logsDir,
      databaseLocationLabel: IS_DEV
        ? t("Development database configuration is managed outside MyAPI.")
        : explicitDatabase
          ? t("An explicit database connection is configured; connection details are hidden.")
          : databasePath,
      sessionSecretLabel:
        IS_DEV || process.env.SESSION_SECRET
          ? t("Configured outside this panel; value hidden.")
          : t("Stored in the application data folder; value hidden."),
    },
  };
}

function publishState() {
  if (allowExit) return;
  stateRevision++;
  rebuildTray();
  if (
    serviceWindow &&
    !serviceWindow.isDestroyed() &&
    !serviceWindow.webContents.isDestroyed() &&
    serviceWindow.webContents.getURL() === PANEL_URL
  ) {
    serviceWindow.webContents.send(SERVICE_PANEL_STATE_CHANNEL, snapshot());
  }
}

async function confirmAction(context, key, label, listener) {
  const identity = controller.identity();
  const options = {
    type: "question",
    title: APP_NAME,
    message: t(key),
    detail: [
      t(
        "Client requests may be interrupted. Configuration, database files and logs are not deleted or restored by this action.",
      ),
      listener ? t("Listener after restart: {{address}}:{{port}}", listener) : "",
    ]
      .filter(Boolean)
      .join("\n\n"),
    buttons: [t("Cancel"), t(label)],
    defaultId: 0,
    cancelId: 0,
    noLink: true,
  };
  const result =
    serviceWindow && !serviceWindow.isDestroyed()
      ? await dialog.showMessageBox(serviceWindow, options)
      : await dialog.showMessageBox(options);
  if (result.response !== 1) return false;
  if (!context.isTrusted() || identity !== controller.identity()) {
    notice = {
      key: "The service changed while confirmation was open. Review its current state.",
      tone: "warning",
    };
    return false;
  }
  return true;
}

async function selectPrivateAddress(context) {
  const identity = controller.identity();
  const candidates = getPrivateIPv4Candidates();
  if (candidates.length === 0) {
    notice = {
      key: "No private IPv4 address was found. Connect to a private network and try again.",
      tone: "warning",
    };
    return null;
  }
  const options = {
    type: "question",
    title: APP_NAME,
    message: t("Choose a private LAN address."),
    detail: t(
      "Only a listed private address can be selected. The listener changes after a separate restart confirmation.",
    ),
    buttons: [t("Cancel"), ...candidates],
    defaultId: 0,
    cancelId: 0,
    noLink: true,
  };
  const result =
    serviceWindow && !serviceWindow.isDestroyed()
      ? await dialog.showMessageBox(serviceWindow, options)
      : await dialog.showMessageBox(options);
  if (result.response === 0) return null;
  const selected = Number.isSafeInteger(result.response)
    ? candidates[result.response - 1]
    : undefined;
  if (!selected || !context.isTrusted() || identity !== controller.identity()) {
    notice = {
      key: "The service changed while confirmation was open. Review its current state.",
      tone: "warning",
    };
    return null;
  }
  if (!getPrivateIPv4Candidates().includes(selected)) {
    notice = {
      key: "The selected private address is no longer available. Select it again after checking the network.",
      tone: "warning",
    };
    return null;
  }
  return selected;
}

async function runControlledAction(name, context) {
  if (actionBusy) return snapshot();
  if (!context.isTrusted()) return snapshot();
  actionBusy = true;
  notice = null;
  publishState();
  try {
    await applyConfirmedServiceAction(name, context);
  } catch {
    recordNativeReason("action-failed");
    notice = {
      key: "The action could not be completed. Reopen the service panel and review its state.",
      tone: "warning",
    };
  } finally {
    actionBusy = false;
    publishState();
  }
  return snapshot();
}

async function applyConfirmedServiceAction(name, context) {
  const current = controller.snapshot();
  if (current.state === "stop-unconfirmed") {
    notice = {
      key: "Stop was not confirmed. MyAPI will stay open and will not start another process.",
      tone: "warning",
    };
    return;
  }
  if (name === "start") {
    if (
      IS_DEV ||
      runtimeConfig.error ||
      current.owned ||
      !["stopped", "failed"].includes(current.state)
    )
      return;
    if (!(await confirmAction(context, "Start the bundled service?", "Start bundled service")))
      return;
    nativeReason = null;
    void controller.start().catch(() => {
      recordNativeReason("start-failed");
      publishState();
    });
  } else if (name === "stop") {
    if (IS_DEV || !current.owned || current.state === "stopping") return;
    if (
      !(await confirmAction(
        context,
        "Stop only the bundled service and keep MyAPI open?",
        "Stop bundled service",
      ))
    )
      return;
    await controller.stop();
    if (controller.snapshot().owned)
      notice = {
        key: "Stop was not confirmed. MyAPI will stay open and will not start another process.",
        tone: "warning",
      };
  } else {
    const relaunch = name === "restartLAN" || name === "restartLoopback";
    if (IS_DEV && relaunch) return;
    let relaunchArgs;
    let selectedAddress;
    if (relaunch) {
      selectedAddress =
        name === "restartLAN" ? await selectPrivateAddress(context) : DEFAULT_BIND_ADDRESS;
      if (!selectedAddress) return;
      relaunchArgs = buildRelaunchArgs(DESKTOP_ARGS, {
        bindAddress: selectedAddress,
        port: PORT,
        allowLan: name === "restartLAN",
      });
      resolveRuntimeConfig({ args: relaunchArgs, env: {} });
    }
    const message = relaunch
      ? "Restart MyAPI with the selected private access scope?"
      : "Quit MyAPI and stop its bundled service?";
    if (
      !(await confirmAction(
        context,
        message,
        relaunch ? "Restart MyAPI" : "Quit MyAPI",
        relaunch ? { address: selectedAddress, port: PORT } : null,
      ))
    )
      return;
    if (name === "restartLAN" && !getPrivateIPv4Candidates().includes(selectedAddress)) {
      notice = {
        key: "The selected private address is no longer available. Select it again after checking the network.",
        tone: "warning",
      };
      return;
    }
    await controller.stop();
    if (!context.isTrusted()) return;
    const stopped = controller.snapshot();
    if (stopped.owned || ["starting", "stopping", "stop-unconfirmed"].includes(stopped.state)) {
      notice = {
        key: "Stop was not confirmed. MyAPI will stay open and will not start another process.",
        tone: "warning",
      };
      return;
    }
    if (name === "restartLAN" && !getPrivateIPv4Candidates().includes(selectedAddress)) {
      notice = {
        key: "The selected private address is no longer available. Select it again after checking the network.",
        tone: "warning",
      };
      return;
    }
    allowExit = true;
    app.isQuitting = true;
    developmentProbe.abort();
    if (relaunchArgs) app.relaunch({ args: relaunchArgs });
    app.quit();
  }
}

async function dispatch(name, context = { isTrusted: () => true }, argument) {
  if (!context.isTrusted()) return snapshot();
  if (["start", "stop", "quit", "restartLAN", "restartLoopback"].includes(name))
    return runControlledAction(name, context);
  try {
    if (name === "openConsole") {
      if (IS_DEV)
        await checkServerAvailability(DEV_FRONTEND_PORT, 1, 0, "127.0.0.1", "/", {
          signal: developmentProbe.signal,
        });
      else if (controller.snapshot().state !== "running") return snapshot();
      if (context.isTrusted() && !allowExit) createWindow();
    } else if (name === "setLanguage") {
      if (!["en", "zh"].includes(argument)) return snapshot();
      translator.setLanguage(argument);
      try {
        const preferencePath = path.join(app.getPath("userData"), "myapi-desktop-language.json");
        const temporary = `${preferencePath}.tmp`;
        fs.mkdirSync(app.getPath("userData"), { recursive: true });
        fs.writeFileSync(temporary, JSON.stringify({ language: argument }) + "\n", { mode: 0o600 });
        fs.renameSync(temporary, preferencePath);
        notice = null;
      } catch {
        notice = {
          key: "Language changed for this session; saving the preference failed.",
          tone: "warning",
        };
      }
    } else if (name === "copyKnownAddress") {
      if (typeof argument !== "string" || !snapshot().addresses.includes(argument)) {
        notice = {
          key: "The action could not be completed. Reopen the service panel and review its state.",
          tone: "warning",
        };
        publishState();
        return snapshot();
      }
      clipboard.writeText(argument);
      notice = { key: "Configured address copied.", tone: "neutral" };
    } else if (name === "openKnownLogs") {
      const { logsDir } = locations();
      const record = launchDiagnostics;
      fs.mkdirSync(logsDir, { recursive: true });
      const diagnosticPath = path.join(logsDir, "desktop-service-diagnostic.log");
      const descriptor = fs.openSync(
        diagnosticPath,
        fs.constants.O_WRONLY |
          fs.constants.O_CREAT |
          fs.constants.O_TRUNC |
          (fs.constants.O_NOFOLLOW || 0),
        0o600,
      );
      try {
        fs.fchmodSync(descriptor, 0o600);
        fs.writeFileSync(
          descriptor,
          `MyAPI Desktop\nLaunch generation: ${record.generation}\nOriginal stderr / startup error (not translated)\nTruncated: ${record.isTruncated()}\n\n${record.originalText() || "No original stderr was captured for this launch."}\n`,
        );
      } finally {
        fs.closeSync(descriptor);
      }
      const error = await shell.openPath(diagnosticPath);
      if (error) notice = { key: "The diagnostic log could not be opened.", tone: "warning" };
    } else if (name === "showLAN" || name === "showPublicHelp") {
      const lan = name === "showLAN";
      await dialog.showMessageBox({
        type: "info",
        title: t(lan ? "LAN status and help" : "Public access help"),
        message: lan
          ? snapshot().scopeLabel
          : t(
              "Public access is not configured by this panel and has not been verified. A separate setup is required.",
            ),
        detail: lan
          ? [
              snapshot().addresses.join("\n"),
              t(
                "LAN access requires a private network, an API Key and the operating system firewall permission. Listener changes require a confirmed restart.",
              ),
            ].join("\n\n")
          : t(
              "This panel keeps the existing local and private-LAN limits. It does not open a public listener or configure a tunnel.",
            ),
        buttons: [t("Close")],
        defaultId: 0,
        cancelId: 0,
      });
    }
  } catch {
    notice = {
      key: "The action could not be completed. Reopen the service panel and review its state.",
      tone: "warning",
    };
  }
  publishState();
  return snapshot();
}

function rebuildTray() {
  if (!tray || !controller || allowExit || tray.isDestroyed()) return;
  const current = snapshot();
  const nativeAction = (name) => () => {
    void dispatch(name).catch(() => {});
  };
  const menu = Menu.buildFromTemplate([
    { label: t("Show service panel"), click: showServicePanel },
    {
      label: t("Open console"),
      enabled: current.canOpenConsole,
      click: nativeAction("openConsole"),
    },
    { label: current.stateLabel, enabled: false },
    { label: current.scopeLabel, enabled: false },
    { type: "separator" },
    { label: t("Start bundled service"), enabled: current.canStart, click: nativeAction("start") },
    { label: t("Stop bundled service"), enabled: current.canStop, click: nativeAction("stop") },
    { label: t("LAN status and help"), click: nativeAction("showLAN") },
    {
      label: t("Restart with LAN access"),
      enabled: current.canRestart,
      click: nativeAction("restartLAN"),
    },
    {
      label: t("Restart with local access only"),
      enabled: current.canRestart,
      click: nativeAction("restartLoopback"),
    },
    { label: t("Public access help"), click: nativeAction("showPublicHelp") },
    {
      label: t("Language"),
      submenu: [
        {
          label: "中文",
          type: "radio",
          checked: translator.language === "zh",
          click: () => {
            void dispatch("setLanguage", { isTrusted: () => true }, "zh").catch(() => {});
          },
        },
        {
          label: "English",
          type: "radio",
          checked: translator.language === "en",
          click: () => {
            void dispatch("setLanguage", { isTrusted: () => true }, "en").catch(() => {});
          },
        },
      ],
    },
    { type: "separator" },
    { label: t("Quit MyAPI"), enabled: current.canQuit, click: nativeAction("quit") },
  ]);
  tray.setContextMenu(menu);
  tray.setToolTip(`${APP_NAME} · ${current.stateLabel}`);
}

function createTray() {
  const trayIconPath =
    process.platform === "darwin"
      ? path.join(__dirname, "tray-iconTemplate.png")
      : path.join(__dirname, "tray-icon-windows.png");
  tray = new Tray(trayIconPath);
  tray.on("click", showServicePanel);
  rebuildTray();
}

const hasSingleInstanceLock = app.requestSingleInstanceLock();
if (!hasSingleInstanceLock) app.quit();
else
  app.on("second-instance", () => {
    if (app.isReady()) showServicePanel();
  });

app
  .whenReady()
  .then(() => {
    if (!hasSingleInstanceLock) return;
    let language = normalizeLanguage(app.getLocale());
    try {
      const preference = JSON.parse(
        fs.readFileSync(path.join(app.getPath("userData"), "myapi-desktop-language.json"), "utf8"),
      );
      if (["en", "zh"].includes(preference.language)) language = preference.language;
    } catch {
      /* Missing preferences keep the operating system language. */
    }
    translator.setLanguage(language);
    process.env.ELECTRON_DATA_DIR = locations().dataDir;
    controller = createServiceController({
      launch: startServer,
      external: IS_DEV,
      onChange: () => publishState(),
    });
    const actions = {};
    for (const name of [
      "snapshot",
      "start",
      "stop",
      "quit",
      "openConsole",
      "setLanguage",
      "copyKnownAddress",
      "openKnownLogs",
      "showLAN",
      "showPublicHelp",
      "restartLAN",
      "restartLoopback",
    ]) {
      actions[name] = (context, argument) =>
        name === "snapshot" ? snapshot() : dispatch(name, context, argument);
    }
    createServicePanelIPC({
      ipcMain,
      getWindow: () => serviceWindow,
      panelURL: PANEL_URL,
      actions,
    });
    createTray();
    showServicePanel();
    if (runtimeConfig.error) {
      publishState();
      return;
    }
    if (IS_DEV) {
      void checkServerAvailability(DEV_FRONTEND_PORT, 30, 1000, "127.0.0.1", "/", {
        signal: developmentProbe.signal,
      }).catch(() => {
        if (allowExit) return;
        notice = {
          key: "The external development console is not ready. Start it outside MyAPI, then use Open console.",
          tone: "neutral",
        };
        publishState();
      });
    } else {
      void controller.start().catch(() => {
        recordNativeReason("start-failed");
        publishState();
      });
    }
  })
  .catch(() => {
    notice = {
      key: "The service panel could not be opened. Use the tray controls or inspect local logs.",
      tone: "warning",
    };
    recordNativeReason("desktop-initialization-failed");
    publishState();
  });

app.on("window-all-closed", () => {
  /* Windows close to the tray; never stop here. */
});
app.on("activate", () => {
  if (hasSingleInstanceLock) showServicePanel();
});
app.on("before-quit", (event) => {
  if (allowExit || !hasSingleInstanceLock) return;
  if (!controller) {
    allowExit = true;
    return;
  }
  event.preventDefault();
  void dispatch("quit").catch(() => {});
});
