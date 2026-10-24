/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { VariantProps } from 'class-variance-authority'

import { buttonVariants } from '@/components/ui/button'

import { cn } from './utils'

/**
 * Classes for a navigation link drawn as a button. It stays a link for
 * assistive technology (a Button rendering a link reports itself as a
 * button); give it `data-myapi-button=''` so touch screens size it to 44px.
 */
export function linkButtonClassName(
  options: VariantProps<typeof buttonVariants> & { className?: string } = {}
) {
  return cn(
    buttonVariants({ variant: options.variant, size: options.size }),
    'h-auto min-h-9 whitespace-normal',
    options.className
  )
}
