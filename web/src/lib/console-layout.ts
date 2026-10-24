// Keep in sync with personal-console.css desktop media query.
export const CONSOLE_MOBILE_QUERY = '(max-width: 1023px)'
// From this width an inspector sits beside its list (design 6.1); between
// the mobile breakpoint and this width it overlays the list.
export const CONSOLE_INSPECTOR_SIDE_QUERY = '(min-width: 1280px)'
// Inspector tabs share the panel width. Long labels (ru, fr) wrap inside
// their tab instead of pushing the first tab past the panel edge at 320px.
export const INSPECTOR_TABS_LIST_CLASS =
  'w-full group-data-horizontal/tabs:h-auto [&>[data-slot=tabs-trigger]]:min-w-0 [&>[data-slot=tabs-trigger]]:whitespace-normal [&>[data-slot=tabs-trigger]]:leading-tight [&>[data-slot=tabs-trigger]]:hyphens-auto'
