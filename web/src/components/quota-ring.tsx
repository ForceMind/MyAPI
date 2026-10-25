/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { cn } from '@/lib/utils'

const RADIUS = 18
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

/**
 * Account window ring (design v2 7.2 `QuotaRing`): the fill is the used share
 * on a same-ramp track, coloured by severity (>=60% warning, >=80%
 * critical), and the number is always written inside, so the state never
 * depends on colour alone.
 */
export function QuotaRing(props: {
  used: number
  label: string
  className?: string
}) {
  const used = Math.min(100, Math.max(0, props.used))
  let severity = 'normal'
  if (used >= 80) severity = 'critical'
  else if (used >= 60) severity = 'warning'
  return (
    <svg
      viewBox='0 0 44 44'
      role='img'
      aria-label={props.label}
      data-severity={severity}
      className={cn('size-11 shrink-0', props.className)}
    >
      <circle
        cx='22'
        cy='22'
        r={RADIUS}
        fill='none'
        strokeWidth='5'
        className='stroke-meter-track'
      />
      <circle
        cx='22'
        cy='22'
        r={RADIUS}
        fill='none'
        strokeWidth='5'
        strokeLinecap='round'
        strokeDasharray={`${((used / 100) * CIRCUMFERENCE).toFixed(1)} ${CIRCUMFERENCE.toFixed(1)}`}
        transform='rotate(-90 22 22)'
        className={cn(
          severity === 'normal' && 'stroke-meter',
          severity === 'warning' && 'stroke-meter-warning',
          severity === 'critical' && 'stroke-meter-critical'
        )}
      />
      <text
        x='22'
        y='22'
        textAnchor='middle'
        dominantBaseline='central'
        className='fill-foreground text-[11px] font-semibold tabular-nums'
        aria-hidden='true'
      >
        {Math.round(props.used)}%
      </text>
    </svg>
  )
}
