/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { create } from 'zustand'

/**
 * The object an open inspector shows. The console shell reads it so the
 * breadcrumb names the object on PC and the mobile top bar offers a back
 * control while the inspector fills the screen (design 5.1, 5.2).
 */
export type OpenInspector = {
  /** Accessible name of the object, e.g. "API Key #21". */
  title: string
  /** Closes the inspector, normally by removing its URL parameter. */
  close: () => void
}

type InspectorState = {
  current: OpenInspector | null
  setCurrent: (current: OpenInspector | null) => void
}

export const useInspectorStore = create<InspectorState>()((set) => ({
  current: null,
  setCurrent: (current) => set({ current }),
}))
