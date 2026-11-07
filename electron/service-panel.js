(() => {
  const bridge = window.myapiService;
  let snapshot = null;
  let addressSignature = "";
  const element = (id) => document.getElementById(id);
  const t = (key) => snapshot?.messages?.[key] || key;

  function render(next) {
    if (
      !next ||
      typeof next !== "object" ||
      !Number.isSafeInteger(next.revision) ||
      next.revision < 0 ||
      (snapshot && next.revision < snapshot.revision)
    )
      return;
    if (
      ![
        "stopped",
        "starting",
        "running",
        "stopping",
        "failed",
        "stop-unconfirmed",
        "external",
      ].includes(next.state)
    )
      return;
    snapshot = next;
    element("application-version").textContent =
      typeof next.applicationVersion === "string" && next.applicationVersion.trim()
        ? next.applicationVersion
        : t("Not provided");
    document.documentElement.lang = next.language === "zh" ? "zh-CN" : "en";
    document.querySelectorAll("[data-i18n]").forEach((node) => {
      node.textContent = t(node.dataset.i18n);
    });
    element("language").value = next.language;
    element("state").textContent = next.stateLabel;
    element("state").dataset.tone = next.stateTone;
    element("state-description").textContent = next.stateDescription;
    element("scope").textContent = next.scopeLabel;
    const notice = element("notice");
    notice.textContent = next.notice?.message || "";
    notice.dataset.tone = next.notice?.tone || "neutral";
    notice.hidden = !next.notice?.message;
    for (const [id, available] of [
      ["start", next.canStart],
      ["stop", next.canStop],
      ["quit", next.canQuit],
      ["open-console", next.canOpenConsole],
      ["restart-lan", next.canRestart],
      ["restart-loopback", next.canRestart],
    ]) {
      element(id).disabled = !available;
    }
    const addresses = Array.isArray(next.addresses) ? next.addresses : [];
    const signature = JSON.stringify([next.language, addresses]);
    if (signature !== addressSignature) {
      addressSignature = signature;
      const rows = addresses.map((address) => {
        const row = document.createElement("div");
        row.className = "address";
        const code = document.createElement("code");
        code.textContent = address;
        const copy = document.createElement("button");
        copy.type = "button";
        copy.textContent = t("Copy address");
        copy.addEventListener("click", () => run(() => bridge.copyKnownAddress(address)));
        row.append(code, copy);
        return row;
      });
      if (rows.length === 0) {
        const empty = document.createElement("p");
        empty.textContent = t("No configured address is available.");
        rows.push(empty);
      }
      element("addresses").replaceChildren(...rows);
    }
    const locations = [
      ["Application data", next.locations?.userData],
      ["Default data folder", next.locations?.dataDirectory],
      ["Database location", next.locations?.databaseLocationLabel],
      ["Session secret storage", next.locations?.sessionSecretLabel],
      ["Logs folder", next.locations?.logsDirectory],
      ["Service detail", next.reasonLabel],
      ["Diagnostic guidance", next.diagnostic?.guidance],
    ];
    element("locations").replaceChildren(
      ...locations.flatMap(([key, value]) => {
        const term = document.createElement("dt");
        const detail = document.createElement("dd");
        term.textContent = t(key);
        detail.textContent = typeof value === "string" ? value : t("Not provided");
        return [term, detail];
      }),
    );
  }

  async function run(action) {
    try {
      render(await action());
    } catch {
      element("notice").textContent = t(
        "The action could not be completed. Reopen the service panel and review its state.",
      );
      element("notice").dataset.tone = "warning";
      element("notice").hidden = false;
    }
  }
  if (!bridge) {
    element("state-description").textContent = t(
      "The service panel bridge is unavailable. Reopen it from the tray.",
    );
    document.querySelectorAll("button, select").forEach((node) => {
      node.disabled = true;
    });
    return;
  }
  element("start").addEventListener("click", () => run(() => bridge.start()));
  element("stop").addEventListener("click", () => run(() => bridge.stop()));
  element("quit").addEventListener("click", () => run(() => bridge.quit()));
  element("open-console").addEventListener("click", () => run(() => bridge.openConsole()));
  element("open-logs").addEventListener("click", () => run(() => bridge.openKnownLogs()));
  element("show-lan").addEventListener("click", () => run(() => bridge.showLAN()));
  element("show-public").addEventListener("click", () => run(() => bridge.showPublicHelp()));
  element("restart-lan").addEventListener("click", () => run(() => bridge.restartLAN()));
  element("restart-loopback").addEventListener("click", () => run(() => bridge.restartLoopback()));
  element("language").addEventListener("change", () =>
    run(() => bridge.setLanguage(element("language").value)),
  );
  const unsubscribe = bridge.onState(render);
  window.addEventListener("unload", unsubscribe, { once: true });
  run(() => bridge.snapshot());
})();
