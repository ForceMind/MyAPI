import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'

export type PageSectionLink = { id: string; label: string }

/** Scroll between already-mounted task regions without discarding their state. */
export function PageSectionLinks(props: {
  sections: readonly PageSectionLink[]
}) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t('On this page')} className='flex flex-wrap gap-2'>
      {props.sections.map((section) => (
        <a
          key={section.id}
          href={`#${section.id}`}
          data-slot='button'
          data-size='sm'
          data-variant='outline'
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
          onClick={(event) => {
            const target = document.getElementById(section.id)
            if (!target) return
            event.preventDefault()
            target.focus({ preventScroll: true })
            target.scrollIntoView({ block: 'start', behavior: 'auto' })
          }}
        >
          {section.label}
        </a>
      ))}
    </nav>
  )
}
