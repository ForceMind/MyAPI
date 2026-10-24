/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useMediaQuery } from '@/hooks/use-media-query'
import {
  CONSOLE_INSPECTOR_SIDE_QUERY,
  CONSOLE_MOBILE_QUERY,
} from '@/lib/console-layout'

export type InspectorPresentation = 'page' | 'overlay' | 'side'

/**
 * How an inspector is presented at the current width (design 6.1): a
 * full-screen page below 1024px, an overlay up to 1279px, beside the list
 * from 1280px.
 */
export function useInspectorPresentation(): InspectorPresentation {
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const side = useMediaQuery(CONSOLE_INSPECTOR_SIDE_QUERY)
  if (mobile) return 'page'
  return side ? 'side' : 'overlay'
}
