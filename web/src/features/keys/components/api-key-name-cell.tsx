/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { Row } from '@tanstack/react-table'

import type { ApiKey } from '../types'
import { useApiKeys } from './api-keys-provider'
import { DataTableRowActions } from './data-table-row-actions'

/**
 * Key name and ID; on the Keys page it opens the Key in the inspector.
 * Table cells are rendered as components, so this keeps a stable type: an
 * inline cell function would remount the button on every table render and
 * drop its focus.
 */
export function ApiKeyNameCell(props: { row: Row<ApiKey> }) {
  const { openKey, inspectedKeyId } = useApiKeys()
  const apiKey = props.row.original
  if (!openKey) {
    return <span className='font-medium'>{apiKey.name}</span>
  }
  return (
    <button
      type='button'
      data-inspect-id={apiKey.id}
      aria-expanded={inspectedKeyId === apiKey.id}
      onClick={() => openKey(apiKey)}
      className='hover:text-primary flex min-h-9 max-w-full min-w-0 flex-col items-start text-left focus-visible:outline-2 focus-visible:outline-offset-2 pointer-coarse:min-h-11'
    >
      <span className='max-w-full truncate font-medium'>{apiKey.name}</span>
      <span className='text-caption text-muted-foreground font-mono'>
        #{apiKey.id}
      </span>
    </button>
  )
}

/** Row actions reduced to the "more" menu beside an open inspector. */
export function CompactApiKeyRowActions(props: { row: Row<ApiKey> }) {
  return <DataTableRowActions row={props.row} menuOnly />
}
