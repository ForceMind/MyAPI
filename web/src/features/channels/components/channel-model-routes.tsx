/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
  modelRouteSchema,
  validateModelRoutes,
  type ModelRoute,
} from '../lib/model-routes'

export function ChannelModelRoutes(props: {
  settings: string
  type: number
  disabled?: boolean
  onChange: (settings: string) => void
}) {
  const { t } = useTranslation()
  const rowIds = useRef<number[]>([])
  const nextRowId = useRef(0)
  let settings: Record<string, unknown>
  try {
    settings = JSON.parse(props.settings || '{}')
  } catch {
    return <p role='alert'>{t('Invalid JSON')}</p>
  }
  if (!settings || typeof settings !== 'object' || Array.isArray(settings)) {
    return <p role='alert'>{t('Invalid JSON')}</p>
  }
  const routes: ModelRoute[] = []
  if (
    settings.model_routes !== undefined &&
    !Array.isArray(settings.model_routes)
  ) {
    return <p role='alert'>{t('Invalid model route configuration')}</p>
  }
  if (Array.isArray(settings.model_routes)) {
    for (const item of settings.model_routes) {
      const parsed = modelRouteSchema.safeParse(item)
      // Empty strings are editable drafts, not silently dropped invalid configuration.
      if (parsed.success) routes.push(parsed.data)
      else if (
        item &&
        typeof item === 'object' &&
        typeof item.public_model === 'string' &&
        typeof item.upstream_model === 'string'
      ) {
        routes.push(item as ModelRoute)
      } else return <p role='alert'>{t('Invalid model route configuration')}</p>
    }
  }
  while (rowIds.current.length < routes.length) {
    rowIds.current.push(nextRowId.current++)
  }
  const error = validateModelRoutes(props.settings, props.type)
  const update = (next: ModelRoute[]) =>
    props.onChange(JSON.stringify({ ...settings, model_routes: next }))
  return (
    <section
      className='space-y-3 rounded-lg border p-4'
      aria-label={t('Explicit model routes')}
    >
      <h3 className='font-medium'>{t('Explicit model routes')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Enable concrete public names in Models above. Routes never enable models automatically. Existing Model Mapping takes precedence. Prefix targets are literal upstream IDs.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Save changes, then use Routing Preview and the channel test action to check the saved route.'
        )}
      </p>
      {routes.length > 0 && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Automatic upstream model additions are paused while explicit routes exist.'
          )}
        </p>
      )}
      {error && (
        <p role='alert' className='text-destructive text-sm'>
          {t(error)}
        </p>
      )}
      {routes.map((route, index) => (
        <fieldset
          key={rowIds.current[index]}
          disabled={props.disabled}
          className='grid min-w-0 gap-2 rounded border p-3 sm:grid-cols-2'
        >
          <legend className='px-1 text-sm'>
            {t('Route')} {index + 1}
          </legend>
          <label className='space-y-1 text-sm'>
            {t('Public model')}
            <Input
              value={route.public_model}
              onChange={(event) =>
                update(
                  routes.map((item, i) =>
                    i === index
                      ? { ...item, public_model: event.target.value }
                      : item
                  )
                )
              }
            />
          </label>
          <label className='space-y-1 text-sm'>
            {t('Upstream model')}
            <Input
              value={route.upstream_model}
              onChange={(event) =>
                update(
                  routes.map((item, i) =>
                    i === index
                      ? { ...item, upstream_model: event.target.value }
                      : item
                  )
                )
              }
            />
          </label>
          <label className='space-y-1 text-sm'>
            {t('Endpoint')}
            <select
              className='bg-background block h-9 w-full rounded border px-2'
              value={route.endpoint}
              onChange={(event) =>
                update(
                  routes.map((item, i) =>
                    i === index
                      ? {
                          ...item,
                          endpoint: event.target
                            .value as ModelRoute['endpoint'],
                        }
                      : item
                  )
                )
              }
            >
              <option value=''>{t('Any supported endpoint')}</option>
              <option value='/v1/responses'>/v1/responses</option>
              {props.type !== 57 && (
                <option value='/v1/chat/completions'>
                  /v1/chat/completions
                </option>
              )}
            </select>
          </label>
          <label className='space-y-1 text-sm'>
            {t('Match mode')}
            <select
              className='bg-background block h-9 w-full rounded border px-2'
              value={route.match}
              onChange={(event) =>
                update(
                  routes.map((item, i) =>
                    i === index
                      ? {
                          ...item,
                          match: event.target.value as ModelRoute['match'],
                        }
                      : item
                  )
                )
              }
            >
              <option value='exact'>{t('Exact')}</option>
              <option value='prefix'>{t('Prefix')}</option>
            </select>
          </label>
          <label className='space-y-1 text-sm'>
            {t('Priority')}
            <Input
              type='number'
              step='1'
              value={route.priority}
              onChange={(event) =>
                update(
                  routes.map((item, i) =>
                    i === index
                      ? { ...item, priority: event.target.valueAsNumber || 0 }
                      : item
                  )
                )
              }
            />
          </label>
          <Button
            type='button'
            variant='outline'
            className='self-end'
            onClick={() => {
              rowIds.current.splice(index, 1)
              update(routes.filter((_, i) => i !== index))
            }}
          >
            {t('Remove route')}
          </Button>
        </fieldset>
      ))}
      <Button
        type='button'
        variant='outline'
        disabled={props.disabled}
        onClick={() =>
          update([
            ...routes,
            {
              public_model: '',
              upstream_model: '',
              endpoint: '',
              match: 'exact',
              priority: 0,
            },
          ])
        }
      >
        {t('Add model route')}
      </Button>
    </section>
  )
}
