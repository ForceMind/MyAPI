/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, it } from 'vitest'

import { managementKeyFormSchema, parseManagementResourceIDs } from '../lib/management-key-form'

it('keeps an empty ID scope unrestricted and deduplicates valid resource IDs', () => {
  expect(parseManagementResourceIDs('  ')).toEqual([])
  expect(parseManagementResourceIDs('12, 9,12')).toEqual([12, 9])
})

it.each(['0', '-1', '1.5', '1e2', '1,,2', '1,', 'bad', '9007199254740992'])('rejects invalid ID input %s without silently widening scope', (input) => {
  expect(parseManagementResourceIDs(input)).toBeNull()
})

it.each(['0', '91', '1.5', '-1', ''])('rejects validity %s outside the whole-day bounds', (days) => {
  expect(managementKeyFormSchema.safeParse({ name: 'audit', scopes: ['system.read'], days, channelIDs: '', tokenIDs: '' }).success).toBe(false)
})
