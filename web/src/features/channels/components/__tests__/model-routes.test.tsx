/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToUpdatePayload,
  transformChannelToFormDefaults,
} from '../../lib/channel-form'
import { isModelRouteConfigChanged } from '../../lib/channel-form-errors'
import { validateModelRoutes } from '../../lib/model-routes'
import { channelSchema } from '../../types'
import { ChannelModelRoutes } from '../channel-model-routes'

const route = {
  public_model: 'public',
  upstream_model: 'upstream',
  match: 'exact',
  endpoint: '/v1/responses',
  priority: 0,
}
function Editor(props: {
  initial?: string
  disabled?: boolean
  type?: number
}) {
  const [settings, setSettings] = useState(props.initial || '{}')
  return (
    <ChannelModelRoutes
      settings={settings}
      onChange={setSettings}
      disabled={props.disabled}
      type={props.type || 1}
    />
  )
}
describe('explicit model route editor', () => {
  test('adding a route exposes editable fields and invalid drafts remain visible', async () => {
    render(<Editor />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Add model route' })
    )
    expect(screen.getByRole('alert')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Public model'), 'public')
    await userEvent.type(screen.getByLabelText('Upstream model'), 'upstream')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Remove route' }))
    expect(screen.queryByLabelText('Public model')).not.toBeInTheDocument()
  })
  test('equal match priority conflicts are visible and block form validation', () => {
    const settings = JSON.stringify({
      model_routes: [route, { ...route, upstream_model: 'different' }],
    })
    render(<Editor initial={settings} />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Conflicting model routes'
    )
    expect(
      channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'test',
        models: 'public',
        settings,
      }).success
    ).toBe(false)
  })
  test('saving routes preserves unrelated settings and never enables models', () => {
    const payload = transformFormDataToUpdatePayload(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        models: 'explicit-only',
        settings: JSON.stringify({ custom: 'keep', model_routes: [route] }),
      },
      1
    )
    expect(payload.models).toBe('explicit-only')
    expect(JSON.parse(payload.settings || '{}').custom).toBe('keep')
    expect(JSON.parse(payload.settings || '{}').model_routes).toEqual([route])
  })
  test('disabled editor prevents additions and field edits', () => {
    render(
      <Editor initial={JSON.stringify({ model_routes: [route] })} disabled />
    )
    expect(screen.getByLabelText('Public model')).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Add model route' })
    ).toBeDisabled()
  })
  test('Codex does not offer Chat and rejects existing Chat rules', () => {
    render(
      <Editor type={57} initial={JSON.stringify({ model_routes: [route] })} />
    )
    expect(
      screen.queryByRole('option', { name: '/v1/chat/completions' })
    ).not.toBeInTheDocument()
    expect(
      validateModelRoutes(
        JSON.stringify({
          model_routes: [{ ...route, endpoint: '/v1/chat/completions' }],
        }),
        57
      )
    ).toBe('Codex routes require Responses.')
  })
  test('editing and removing routes retains the original detail digest in the update', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'test',
      created_time: 1,
      test_time: 1,
      response_time: 1,
      balance_updated_time: 1,
      routing_config_digest: 'original-digest',
      settings: JSON.stringify({ model_routes: [route] }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    const payload = transformFormDataToUpdatePayload(
      { ...defaults, settings: JSON.stringify({ model_routes: [] }) },
      1
    )
    expect(payload.expected_routing_config).toBe('original-digest')
    expect(payload).not.toHaveProperty('routing_config_digest')
  })
  test('only the explicit configuration conflict is classified for reload and review', () => {
    const error = new AxiosError('conflict')
    error.response = {
      status: 409,
      data: { code: 'model_route_config_changed' },
    } as typeof error.response
    expect(isModelRouteConfigChanged(error)).toBe(true)
    expect(isModelRouteConfigChanged(new Error('generic failure'))).toBe(false)
  })
  test('existing explicit routes explain that automatic model additions are paused', () => {
    render(<Editor initial={JSON.stringify({ model_routes: [route] })} />)
    expect(
      screen.getByText(
        'Automatic upstream model additions are paused while explicit routes exist.'
      )
    ).toBeInTheDocument()
  })
  test('endpoint and match selectors have exact accessible names and change route values', async () => {
    render(<Editor initial={JSON.stringify({ model_routes: [route] })} />)
    const endpoint = screen.getByLabelText('Endpoint', { exact: true })
    const match = screen.getByLabelText('Match mode', { exact: true })
    expect(endpoint).toHaveAccessibleName('Endpoint')
    expect(match).toHaveAccessibleName('Match mode')
    await userEvent.selectOptions(endpoint, '/v1/chat/completions')
    await userEvent.selectOptions(match, 'prefix')
    expect(endpoint).toHaveValue('/v1/chat/completions')
    expect(match).toHaveValue('prefix')
  })

  test('select labels remain unique and correctly associated after route removal and insertion', async () => {
    render(
      <Editor
        initial={JSON.stringify({
          model_routes: [route, { ...route, public_model: 'other' }],
        })}
      />
    )
    const initialEndpoints = screen.getAllByLabelText('Endpoint', {
      exact: true,
    }) as HTMLSelectElement[]
    const initialMatches = screen.getAllByLabelText('Match mode', {
      exact: true,
    }) as HTMLSelectElement[]
    const retainedEndpointId = initialEndpoints[1].id
    const retainedMatchId = initialMatches[1].id
    expect(
      new Set(
        [...initialEndpoints, ...initialMatches].map((select) => select.id)
      ).size
    ).toBe(4)
    for (const select of initialEndpoints) {
      expect(select.labels?.[0]?.textContent).toBe('Endpoint')
      expect(select.labels?.[0]?.htmlFor).toBe(select.id)
    }
    for (const select of initialMatches) {
      expect(select.labels?.[0]?.textContent).toBe('Match mode')
      expect(select.labels?.[0]?.htmlFor).toBe(select.id)
    }
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Remove route' })[0]
    )
    expect(screen.getByLabelText('Endpoint', { exact: true })).toHaveAttribute(
      'id',
      retainedEndpointId
    )
    expect(
      screen.getByLabelText('Match mode', { exact: true })
    ).toHaveAttribute('id', retainedMatchId)
    await userEvent.click(
      screen.getByRole('button', { name: 'Add model route' })
    )
    const endpoints = screen.getAllByLabelText('Endpoint', {
      exact: true,
    }) as HTMLSelectElement[]
    expect(endpoints[0].id).toBe(retainedEndpointId)
    expect(endpoints[1].id).not.toBe(initialEndpoints[0].id)
    expect(endpoints[1].id).not.toBe(retainedEndpointId)
    expect(endpoints[1].labels?.[0]?.htmlFor).toBe(endpoints[1].id)
    await userEvent.selectOptions(endpoints[1], '/v1/chat/completions')
    expect(endpoints[0]).toHaveValue('/v1/responses')
    expect(endpoints[1]).toHaveValue('/v1/chat/completions')
  })
})
