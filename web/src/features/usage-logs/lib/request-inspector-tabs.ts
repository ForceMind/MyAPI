/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

/** Tabs of the request inspector; the active one is kept in the URL. */
export const REQUEST_INSPECTOR_TABS = ['diagnosis', 'body', 'sse'] as const
export type RequestInspectorTab = (typeof REQUEST_INSPECTOR_TABS)[number]
export const requestInspectorTabSchema = z
  .enum(REQUEST_INSPECTOR_TABS)
  .optional()
  .catch(undefined)
