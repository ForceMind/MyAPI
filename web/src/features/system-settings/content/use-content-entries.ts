import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { useAuthStore } from '@/stores/auth-store'
import { useSettingsReloadScope } from '../components/settings-page-scope'

import { parseContentEntries, type ContentEntry } from './collection-json'

/** The four content collections edit local drafts before their page Save. */
export function useContentEntries<T extends ContentEntry>(source: string, validate?: (entry: T) => boolean) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const scope = useSettingsReloadScope()
  const parsed = useMemo(() => parseContentEntries<T>(source), [source])
  const sourceIncompatible = parsed.incompatible || Boolean(validate && parsed.entries.some((entry) => !validate(entry)))
  const [entries, setEntries] = useState(parsed.entries)
  const [baseline, setBaseline] = useState(() => JSON.stringify(parsed.entries))
  const [changed, setChanged] = useState(false)
  const [incompatible, setIncompatible] = useState(sourceIncompatible)
  const currentSource = useRef(source)
  const actor = useRef(identity)
  const mounted = useRef(true)
  const serialized = useMemo(() => JSON.stringify(entries), [entries])
  const isDirty = changed || serialized !== baseline
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => {
    const actorChanged = actor.current !== identity
    if (!actorChanged && currentSource.current === source) return
    currentSource.current = source
    actor.current = identity
    setIncompatible(sourceIncompatible)
    if (!actorChanged && isDirty) return
    setEntries(parsed.entries)
    setBaseline(JSON.stringify(parsed.entries))
    setChanged(false)
  }, [source, parsed.entries, sourceIncompatible, identity, isDirty])

  const isCurrent = useCallback(() => {
    const { auth } = useAuthStore.getState()
    return mounted.current && scope.isActive() && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
  }, [identity, scope])
  const acknowledge = useCallback((saved: T[]) => {
    if (!isCurrent()) return
    setBaseline(JSON.stringify(saved))
    setChanged(false)
  }, [isCurrent])
  const canApplyRaw = useCallback((raw: string) => {
    const result = parseContentEntries<T>(raw)
    return !result.incompatible && (!validate || result.entries.every(validate))
  }, [validate])
  const applyRaw = useCallback((raw: string) => {
    const result = parseContentEntries<T>(raw)
    if (result.incompatible || (validate && !result.entries.every(validate)) || !isCurrent()) return false
    setEntries(result.entries)
    setIncompatible(false)
    setChanged(true)
    return true
  }, [validate, isCurrent])

  return { entries: incompatible ? [] : entries, setEntries, isDirty, markChanged: setChanged, incompatible, source, isCurrent, acknowledge, canApplyRaw, applyRaw }
}
