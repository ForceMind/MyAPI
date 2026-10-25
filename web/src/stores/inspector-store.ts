/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { create } from 'zustand'

/**
 * The object an open inspector or object page shows. The console shell
 * reads it so the breadcrumb names the object on PC and the mobile top bar
 * offers a back control while the object fills the screen (design 5.1, 5.2).
 */
export type OpenInspector = {
  /** Accessible name of the object, e.g. "API Key #21". */
  title: string
  /** Steps back one level, normally to where the object was opened from. */
  close: () => void
  /**
   * Objects between the page and this one, outermost first; on a request
   * page inside a Key, the Key. Each crumb opens its object.
   */
  parents?: { title: string; open: () => void }[]
  /** Leaves every object for the page's list; defaults to `close`. */
  closeAll?: () => void
  /**
   * A full object page rather than an inspector beside a list: the
   * breadcrumb is its only way up on PC, so it is always shown.
   */
  page?: boolean
}

type InspectorState = {
  current: OpenInspector | null
  setCurrent: (current: OpenInspector | null) => void
}

export const useInspectorStore = create<InspectorState>()((set) => ({
  current: null,
  setCurrent: (current) => set({ current }),
}))
