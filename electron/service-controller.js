'use strict';

/** Owns exactly the child returned by launch; Electron and its confirmations stay outside. */
function createServiceController(options) {
  if (!options || (!options.external && typeof options.launch !== 'function')) {
    throw new TypeError('A desktop service launcher is required');
  }
  const timers = options.timers || { setTimeout, clearTimeout };
  const gracefulTimeoutMs = options.gracefulTimeoutMs ?? 5000;
  if (!Number.isFinite(gracefulTimeoutMs) || gracefulTimeoutMs < 0) {
    throw new TypeError('The desktop stop deadline must be finite and nonnegative');
  }
  let state = options.external ? 'external' : 'stopped';
  let reason = options.external ? 'externally-managed' : null;
  let generation = 0;
  let operation = 0;
  let owner = null;
  let starting = null;
  let stopping = null;
  let identity = Object.freeze({ generation, operation, child: null });

  function snapshot() {
    return Object.freeze({ state, generation, operation, owned: owner !== null, pending: state === 'starting' || state === 'stopping', reason });
  }

  function publish(nextState, nextReason = null) {
    state = nextState;
    reason = nextReason;
    operation += 1;
    identity = Object.freeze({ generation, operation, child: owner?.child ?? null });
    // A presentation callback cannot interrupt ownership or shutdown bookkeeping.
    try { options.onChange?.(snapshot()); } catch { /* the controller remains authoritative */ }
  }

  function finishStart(record) {
    if (!record || record.finished) return;
    record.finished = true;
    if (starting === record) starting = null;
    record.resolve(snapshot());
  }

  function finishStop(record) {
    if (!record || record.finished) return;
    timers.clearTimeout(record.timer);
    record.finished = true;
    if (stopping === record) stopping = null;
    record.resolve(snapshot());
  }

  function start() {
    if (starting) return starting.promise;
    if (options.external || owner || stopping) return Promise.resolve(snapshot());
    generation += 1;
    const abort = new AbortController();
    const record = { generation, abort, finished: false, resolve: null, promise: null };
    record.promise = new Promise((resolve) => { record.resolve = resolve; });
    starting = record;
    let launched;
    try {
      launched = options.launch({ generation, signal: abort.signal });
      if (!launched?.child || typeof launched.child.once !== 'function' || typeof launched.child.kill !== 'function') {
        throw new TypeError('Launcher did not return an owned child');
      }
    } catch {
      abort.abort();
      publish('failed', 'launch-failed');
      finishStart(record);
      return record.promise;
    }
    const current = { child: launched.child, stopRequested: false, abort, requestShutdown: launched.requestShutdown };
    owner = current;
    current.child.once('close', (code, signal) => {
      if (owner !== current) return;
      owner = null;
      abort.abort();
      const stop = stopping;
      if (current.stopRequested) {
        if (code === 0 && signal == null) publish('stopped');
        else publish('failed', 'shutdown-failed');
      } else if (code === 0) {
        publish('stopped', 'process-exited');
      } else {
        publish('failed', 'process-exited');
      }
      finishStart(record);
      finishStop(stop);
    });
    current.child.once('error', () => {
      if (owner !== current) return;
      abort.abort();
      if (!current.stopRequested) publish('failed', 'process-error');
      finishStart(record);
    });
    publish('starting');
    if (!launched.ready || typeof launched.ready.then !== 'function') {
      abort.abort();
      publish('failed', 'start-unverified');
      finishStart(record);
      return record.promise;
    }
    Promise.resolve(launched.ready).then(() => {
      if (owner === current && starting === record && !abort.signal.aborted && state === 'starting') {
        publish('running');
      }
      finishStart(record);
    }, () => {
      if (owner === current && starting === record && !abort.signal.aborted && state === 'starting') {
        publish('failed', 'start-failed');
        abort.abort();
      }
      finishStart(record);
    });
    return record.promise;
  }

  function stop() {
    if (stopping) return stopping.promise;
    if (options.external) return Promise.resolve(snapshot());
    if (!owner || state === 'stop-unconfirmed') return Promise.resolve(snapshot());
    const current = owner;
    current.stopRequested = true;
    const record = { timer: null, finished: false, resolve: null, promise: null };
    record.promise = new Promise((resolve) => { record.resolve = resolve; });
    stopping = record;
    publish('stopping');
    current.abort.abort();
    finishStart(starting);
    record.timer = timers.setTimeout(() => {
      if (owner !== current || stopping !== record || record.finished) return;
      publish('stop-unconfirmed', 'stop-timeout');
      finishStop(record);
    }, gracefulTimeoutMs);
    // The owned process can still be draining requests when the UI deadline expires.
    try {
      if (current.requestShutdown) current.requestShutdown();
      else current.child.kill('SIGTERM');
    } catch { /* still await close and the bounded deadline */ }
    return record.promise;
  }

  return { start, stop, snapshot, identity: () => identity };
}

module.exports = { createServiceController };
