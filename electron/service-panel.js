(() => {
  const bridge = window.myapiService;
  let snapshot = null;
  let addressSignature = "";
  const publicConfig = window.MyAPIPublicAccess;
  let publicView = null;
  let publicStep = 1;
  let proxyLocation = "";
  let prerequisites = "unknown";
  let draftInitialized = false;
  let draftEdited = false;
  let draftRevision = null;
  let editSequence = 0;
  let stepSequence = 0;
  let publicPending = false;
  let publicFeedback = "";
  let guideRequest = 0;
  let proxySignature = "";
  let savedSignature = "";
  let recoverySignature = "";
  let proxyCopyButtons = [];
  let savedButtons = [];
  let recoveryButton = null;
  const element = (id) => document.getElementById(id);
  const t = (key) => snapshot?.messages?.[key] || key;

  function publicRecord(next) {
    const value = next.publicAccess;
    if (
      !publicConfig ||
      typeof publicConfig.validatePublicEntry !== "function" ||
      typeof publicConfig.validateProxyTarget !== "function" ||
      !Number.isSafeInteger(next.publicGuideRequest) ||
      next.publicGuideRequest < 0 ||
      !value ||
      typeof value !== "object" ||
      !Number.isSafeInteger(value.recordRevision) ||
      value.recordRevision < 0 ||
      value.verification !== "unknown" ||
      !["local", "lan", "unavailable"].includes(value.listenerScope) ||
      typeof value.busy !== "boolean" ||
      !["not-configured", "unverified", "unreadable"].includes(value.status) ||
      !Array.isArray(value.proxyTargets) ||
      !["savePublicEntry", "clearPublicEntry", "copyPublicBaseURL", "copyProxyTarget"].every(
        (name) => typeof bridge?.[name] === "function",
      )
    )
      return null;
    try {
      if (!value.proxyTargets.every((target) => publicConfig.validateProxyTarget(target)))
        return null;
      if (value.status === "unverified") {
        const saved = publicConfig.validatePublicEntry(value.savedOrigin);
        if (!saved.ok || saved.origin !== value.savedOrigin || saved.baseURL !== value.savedBaseURL)
          return null;
      } else if (value.savedOrigin !== null || value.savedBaseURL !== null) return null;
    } catch {
      return null;
    }
    return value;
  }

  function validateDraft() {
    if (!publicConfig || typeof publicConfig.validatePublicEntry !== "function")
      return { ok: false, code: "invalid" };
    return publicConfig.validatePublicEntry(element("public-origin").value);
  }

  function publicReady() {
    return Boolean(publicView && !publicView.busy && !publicPending);
  }

  function setPublicStep(step, focus = true) {
    publicStep = step;
    stepSequence++;
    updatePublicControls();
    if (focus) {
      const heading = element(`public-heading-${step}`);
      heading.focus({ preventScroll: true });
      heading.scrollIntoView({ block: "nearest" });
    }
  }

  function updatePublicControls() {
    const ready = publicReady();
    const editable = ready && publicView.status !== "unreadable";
    const validation = validateDraft();
    const listenerReady = Boolean(
      publicView?.proxyTargets.length &&
      ((proxyLocation === "same" && publicView.listenerScope !== "unavailable") ||
        (proxyLocation === "lan" && publicView.listenerScope === "lan")),
    );
    const prerequisitesReady = listenerReady && prerequisites === "ready";
    for (let step = 1; step <= 4; step++) {
      const button = element(`public-step-${step}`);
      button.disabled = !editable;
      if (step === publicStep) button.setAttribute("aria-current", "step");
      else button.removeAttribute("aria-current");
      element(`public-page-${step}`).hidden = step !== publicStep;
    }
    for (const id of [
      "public-proxy-same",
      "public-proxy-lan",
      "public-prerequisites",
      "public-origin",
      "public-back-2",
      "public-back-3",
      "public-back-4",
      "public-review-lan",
    ])
      element(id).disabled = !editable;
    element("public-next-1").disabled = !editable || !listenerReady;
    element("public-next-2").disabled = !editable || !prerequisitesReady;
    element("public-save").disabled = !editable || !prerequisitesReady || !validation.ok;
    element("public-open-console").disabled =
      !editable || publicView.status !== "unverified" || !snapshot?.canOpenConsole;
    element("public-lan-note").hidden = proxyLocation !== "lan";
    element("public-prerequisite-required").hidden = prerequisitesReady;
    element("public-prerequisite-note").textContent =
      prerequisites === "ready"
        ? t("You confirmed these prerequisites. MyAPI has not checked the external reverse proxy.")
        : t(
            "External prerequisites are not confirmed. Configure your reverse proxy outside MyAPI before continuing.",
          );
    const messages = {
      valid: "Address format accepted. External access is still unverified.",
      empty: "Enter the existing HTTPS root origin.",
      "too-long": "The HTTPS origin is too long.",
      "control-character": "Control characters are not allowed in the HTTPS origin.",
      backslash: "Backslashes are not allowed in the HTTPS origin.",
      scheme: "Use an HTTPS origin.",
      credentials: "Do not include credentials in the HTTPS origin.",
      "query-fragment": "Query strings and fragments are not supported.",
      path: "Path prefixes are not supported. Enter only the HTTPS root origin.",
      "local-or-lan": "Enter a public HTTPS origin, not a local or private-LAN address.",
      invalid: "The HTTPS root origin is invalid.",
    };
    element("public-origin-feedback").textContent = t(
      messages[validation.code] || messages.invalid,
    );
    element("public-origin").setAttribute("aria-invalid", String(draftEdited && !validation.ok));
    element("public-preview").textContent = validation.ok ? validation.baseURL : t("Not provided");
    element("public-draft-note").hidden =
      !draftEdited || !publicView || draftRevision === publicView.recordRevision;
    element("public-action-feedback").textContent = publicFeedback ? t(publicFeedback) : "";
    for (const button of proxyCopyButtons) button.disabled = !editable;
    for (const button of savedButtons) button.disabled = !editable;
    if (recoveryButton) recoveryButton.disabled = !ready;
  }

  async function runPublic(action, savedDraft, clearingRecord) {
    if (!publicReady()) return;
    publicPending = true;
    publicFeedback = "";
    updatePublicControls();
    try {
      const response = await action();
      const accepted = render(response);
      if (savedDraft) {
        const record = accepted ? publicView : null;
        const result = response?.publicSaveResult;
        const boundResult =
          accepted &&
          result &&
          ["saved", "cancelled", "rejected", "failed", "unconfirmed"].includes(result.outcome) &&
          result.origin === savedDraft.origin &&
          result.expectedRevision === savedDraft.revision &&
          Number.isSafeInteger(result.recordRevision) &&
          result.recordRevision >= 0 &&
          typeof result.unchanged === "boolean";
        if (
          boundResult &&
          result.outcome === "saved" &&
          !result.unchanged &&
          result.recordRevision === savedDraft.revision + 1 &&
          record?.status === "unverified" &&
          record.savedOrigin === savedDraft.origin &&
          record.recordRevision === result.recordRevision
        ) {
          publicFeedback = "Entry saved on this computer. It remains unverified.";
          if (editSequence === savedDraft.edit && stepSequence === savedDraft.step) {
            draftEdited = false;
            draftRevision = record.recordRevision;
            setPublicStep(4);
          }
        } else if (
          boundResult &&
          ["cancelled", "rejected", "failed"].includes(result.outcome) &&
          result.unchanged &&
          record &&
          record.status !== "unreadable" &&
          record.recordRevision === result.recordRevision
        ) {
          publicFeedback = "The saved entry did not change. Your draft is preserved.";
        } else if (boundResult && ["cancelled", "rejected"].includes(result.outcome)) {
          publicFeedback =
            "The save was cancelled or rejected. Your draft is preserved; review the current saved entry.";
        } else if (boundResult && result.outcome === "failed") {
          publicFeedback =
            "Public entry action failed. Your draft is preserved; review the saved entry before retrying.";
        } else {
          publicFeedback =
            "Save result is unconfirmed. Your draft is preserved; review the saved entry.";
        }
      } else if (clearingRecord) {
        const record = accepted ? publicView : null;
        const result = response?.publicClearResult;
        const boundResult =
          accepted &&
          result &&
          ["cleared", "cancelled", "rejected", "failed", "unconfirmed"].includes(result.outcome) &&
          result.origin === clearingRecord.origin &&
          result.expectedRevision === clearingRecord.revision &&
          Number.isSafeInteger(result.recordRevision) &&
          result.recordRevision >= 0 &&
          typeof result.unchanged === "boolean";
        if (
          boundResult &&
          result.outcome === "cleared" &&
          !result.unchanged &&
          result.recordRevision === clearingRecord.revision + 1 &&
          record?.status === "not-configured" &&
          record.savedOrigin === null &&
          record.savedBaseURL === null &&
          record.recordRevision === result.recordRevision
        ) {
          publicFeedback =
            "The saved entry on this computer was cleared. External configuration is unchanged.";
        } else if (
          boundResult &&
          ["cancelled", "rejected", "failed"].includes(result.outcome) &&
          result.unchanged &&
          result.recordRevision === clearingRecord.revision &&
          record &&
          record.savedOrigin === clearingRecord.origin &&
          record.recordRevision === result.recordRevision
        ) {
          publicFeedback = "The saved entry did not change. Your draft is preserved.";
        } else if (boundResult && ["cancelled", "rejected"].includes(result.outcome)) {
          publicFeedback =
            "The saved entry changed. Review the current record before trying again.";
        } else if (boundResult && result.outcome === "failed") {
          publicFeedback =
            "Public entry action failed. Your draft is preserved; review the saved entry before retrying.";
        } else {
          publicFeedback =
            "The write result could not be confirmed. Read the saved entry again before taking another action.";
        }
      }
    } catch {
      publicFeedback =
        "Public entry action failed. Your draft is preserved; review the saved entry before retrying.";
    } finally {
      publicPending = false;
      updatePublicControls();
    }
  }

  function savePublicEntry() {
    if (element("public-save").disabled || !publicReady() || publicView.status === "unreadable")
      return;
    const draft = validateDraft();
    if (!draft.ok) return;
    const expected = publicView.recordRevision;
    return runPublic(() => bridge.savePublicEntry(draft.origin, expected), {
      origin: draft.origin,
      revision: expected,
      edit: editSequence,
      step: stepSequence,
    });
  }

  function renderPublicGuide(next) {
    publicView = publicRecord(next);
    const view = publicView;
    if (view && view.status !== "unreadable" && !draftInitialized) {
      draftInitialized = true;
      if (!draftEdited) element("public-origin").value = view.savedOrigin || "";
      draftRevision = view.recordRevision;
    }
    element("public-step-navigation").setAttribute("aria-label", t("Public access guide steps"));
    if (Number.isSafeInteger(publicConfig?.MAX_PUBLIC_ORIGIN_LENGTH))
      element("public-origin").maxLength = publicConfig.MAX_PUBLIC_ORIGIN_LENGTH;
    let status = "Public entry status is unknown.";
    if (view?.status === "not-configured") status = "No public entry is saved on this computer.";
    else if (view?.status === "unreadable") status = "The saved public entry is unreadable.";
    else if (view?.status === "unverified") status = "Saved on this computer · unverified";
    element("public-record-status").textContent = t(status);
    element("public-summary-status").textContent = t(status);
    let listenerStatus = "The configured listener is unavailable.";
    if (view?.listenerScope === "lan") listenerStatus = "Configured listener: private LAN.";
    else if (view?.listenerScope === "local")
      listenerStatus = "Configured listener: this computer only.";
    element("public-listener-scope").textContent = t(listenerStatus);
    const targets = JSON.stringify([view?.proxyTargets ?? null, next.language]);
    if (targets !== proxySignature) {
      proxySignature = targets;
      proxyCopyButtons = [];
      const rows = (view?.proxyTargets || []).map((address) => {
        const row = document.createElement("div");
        row.className = "address";
        const code = document.createElement("code");
        code.textContent = address;
        const copy = document.createElement("button");
        copy.type = "button";
        copy.textContent = t("Copy proxy target");
        copy.addEventListener(
          "click",
          () =>
            publicReady() &&
            publicView.status !== "unreadable" &&
            runPublic(() => bridge.copyProxyTarget(address)),
        );
        proxyCopyButtons.push(copy);
        row.append(code, copy);
        return row;
      });
      if (!rows.length) {
        const empty = document.createElement("p");
        empty.textContent = t("No configured proxy target is available.");
        rows.push(empty);
      }
      element("public-proxy-targets").replaceChildren(...rows);
    }
    const saved = JSON.stringify([
      view?.status ?? null,
      view?.savedOrigin,
      view?.savedBaseURL,
      view?.recordRevision,
      next.language,
    ]);
    if (saved !== savedSignature) {
      savedSignature = saved;
      savedButtons = [];
      const rows = [];
      if (view?.status === "unverified") {
        const baseURL = view.savedBaseURL,
          origin = view.savedOrigin,
          revision = view.recordRevision;
        const row = document.createElement("div");
        row.className = "address";
        const code = document.createElement("code");
        code.textContent = baseURL;
        const copy = document.createElement("button");
        copy.type = "button";
        copy.textContent = t("Copy saved public Base URL");
        copy.addEventListener(
          "click",
          () =>
            publicReady() &&
            publicView.status === "unverified" &&
            runPublic(() => bridge.copyPublicBaseURL(baseURL, revision)),
        );
        const clear = document.createElement("button");
        clear.type = "button";
        clear.textContent = t("Clear this application's saved entry");
        clear.addEventListener("click", () =>
          runPublic(() => bridge.clearPublicEntry(origin, revision), null, { origin, revision }),
        );
        savedButtons.push(copy, clear);
        row.append(code, copy);
        rows.push(row, clear);
      } else {
        const empty = document.createElement("p");
        empty.textContent = t(status);
        rows.push(empty);
      }
      element("public-saved-entry").replaceChildren(...rows);
    }
    element("public-recovery").hidden = view?.status !== "unreadable";
    const recovery = JSON.stringify([view?.status, view?.recordRevision, next.language]);
    if (recovery !== recoverySignature) {
      recoverySignature = recovery;
      recoveryButton = null;
      if (view?.status === "unreadable") {
        const revision = view.recordRevision;
        recoveryButton = document.createElement("button");
        recoveryButton.type = "button";
        recoveryButton.textContent = t("Clear the unreadable entry record");
        recoveryButton.addEventListener("click", () =>
          runPublic(() => bridge.clearPublicEntry(null, revision), null, {
            origin: null,
            revision,
          }),
        );
        element("public-recovery-actions").replaceChildren(recoveryButton);
      } else element("public-recovery-actions").replaceChildren();
    }
    updatePublicControls();
    if (Number.isSafeInteger(next.publicGuideRequest) && next.publicGuideRequest > guideRequest) {
      guideRequest = next.publicGuideRequest;
      element("public-guide").open = true;
      element("public-guide-summary").focus({ preventScroll: true });
      element("public-guide-summary").scrollIntoView({ block: "start" });
    }
  }

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
    renderPublicGuide(next);
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
    return true;
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
    document.querySelectorAll("button, select, input").forEach((node) => {
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
  for (let step = 1; step <= 4; step++)
    element(`public-step-${step}`).addEventListener("click", () => {
      if (!element(`public-step-${step}`).disabled) setPublicStep(step);
    });
  for (const [id, step] of [
    ["public-next-1", 2],
    ["public-next-2", 3],
    ["public-back-2", 1],
    ["public-back-3", 2],
    ["public-back-4", 3],
  ])
    element(id).addEventListener("click", () => {
      if (!element(id).disabled) setPublicStep(step);
    });
  for (const [id, location] of [
    ["public-proxy-same", "same"],
    ["public-proxy-lan", "lan"],
  ])
    element(id).addEventListener("change", () => {
      if (!element(id).disabled && element(id).checked) {
        proxyLocation = location;
        updatePublicControls();
      }
    });
  element("public-prerequisites").addEventListener("change", () => {
    if (element("public-prerequisites").disabled) return;
    prerequisites = element("public-prerequisites").value;
    updatePublicControls();
  });
  element("public-origin").addEventListener("input", () => {
    draftEdited = true;
    editSequence++;
    publicFeedback = "";
    updatePublicControls();
  });
  element("public-origin").addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !element("public-save").disabled) {
      event.preventDefault();
      void savePublicEntry();
    }
  });
  element("public-save").addEventListener("click", savePublicEntry);
  element("public-review-lan").addEventListener("click", () => {
    if (element("public-review-lan").disabled) return;
    element("local-access-controls").open = true;
    element("local-access-summary").focus({ preventScroll: true });
  });
  element("public-open-console").addEventListener("click", () => {
    if (!element("public-open-console").disabled) return run(() => bridge.openConsole());
  });
  const unsubscribe = bridge.onState(render);
  window.addEventListener("unload", unsubscribe, { once: true });
  run(() => bridge.snapshot());
})();
