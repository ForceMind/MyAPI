/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { createContext } from 'react'

import type { ConnectionReturn } from './connection-state'

/** The table's name links and mobile cards return to the same inventory. */
export const ConnectionListContext = createContext<ConnectionReturn | undefined>(undefined)
