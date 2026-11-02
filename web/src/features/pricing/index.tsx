/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { LogPagination } from '@/components/log-pagination'
import { useRememberReturnFocus, useReturnFocus } from '@/components/object-page/return-focus'
import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { PageTransition } from '@/components/page-transition'

import {
  LoadingSkeleton,
  EmptyState,
  SearchBar,
  PricingTable,
  PricingSidebar,
  PricingToolbar,
  ModelCardGrid,
  ModelDetailsDrawer,
} from './components'
import { findModelCatalogAction, modelCatalogFocusKey } from './catalog-focus'
import { EXCLUDED_GROUPS, VIEW_MODES } from './constants'
import { useFilters } from './hooks/use-filters'
import { usePricingData } from './hooks/use-pricing-data'

export function Pricing() {
  const { t } = useTranslation()
  const [selectedModelName, setSelectedModelName] = useState<string | null>(
    null
  )

  const {
    models,
    vendors,
    groupRatio,
    usableGroup,
    endpointMap,
    autoGroups,
    isLoading,
    error,
    refetch,
    isFetching,
    priceRate,
    usdExchangeRate,
  } = usePricingData()

  const {
    search: catalogSearch,
    page,
    pageSize,
    setPage,
    setPageSize,
    searchInput,
    sortBy,
    vendorFilter,
    groupFilter,
    quotaTypeFilter,
    endpointTypeFilter,
    tagFilter,
    tokenUnit,
    viewMode,
    showRechargePrice,
    setSearchInput,
    setSortBy,
    setVendorFilter,
    setGroupFilter,
    setQuotaTypeFilter,
    setEndpointTypeFilter,
    setTagFilter,
    setTokenUnit,
    setViewMode,
    setShowRechargePrice,
    filteredModels,
    hasActiveFilters,
    activeFilterCount,
    availableTags,
    clearFilters,
    clearSearch,
    clearAll,
  } = useFilters(models || [])

  const rememberFocus = useRememberReturnFocus()
  const headingRef = useRef<HTMLHeadingElement>(null)
  const openerRef = useRef<HTMLElement | null>(null)
  const historyKey = useLocation({ select: (location) => location.state.__TSR_key ?? location.state.key })
  const requestedModel = catalogSearch.focusModel
  const focusRows = useMemo(() => ({ models: filteredModels, requestedModel }), [filteredModels, requestedModel])
  // Bridge name-only pricing identities into the existing one-shot history focus.
  // Register only on entry/request changes, so saved-data refreshes cannot steal focus.
  useEffect(() => {
    if (historyKey && requestedModel) rememberFocus('data-pricing-model', modelCatalogFocusKey(requestedModel))
  }, [historyKey, requestedModel, rememberFocus])
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filteredModels.length / pageSize)))
  useReturnFocus({ attribute: 'data-pricing-model', settled: !isLoading, rows: focusRows, fallback: headingRef })
  const handleModelClick = useCallback((modelName: string) => {
    const active = document.activeElement instanceof HTMLElement ? document.activeElement : null
    openerRef.current = active?.getAttribute('data-pricing-model') === modelCatalogFocusKey(modelName)
      ? active
      : findModelCatalogAction(modelName)
    setSelectedModelName(modelName)
  }, [])

  const selectedModel = useMemo(
    () =>
      selectedModelName
        ? (models || []).find(
            (model) => model.model_name === selectedModelName
          ) || null
        : null,
    [models, selectedModelName]
  )

  const availableGroups = useMemo(
    () =>
      Object.keys(usableGroup || {}).filter(
        (g) => !EXCLUDED_GROUPS.includes(g)
      ),
    [usableGroup]
  )

  const handleClearAll = useCallback(() => {
    clearAll()
  }, [clearAll])

  const renderPricingContent = () => {
    if (filteredModels.length === 0) {
      return (
        <EmptyState
          emptyCatalog={models.length === 0}
          searchQuery={searchInput}
          hasActiveFilters={hasActiveFilters}
          onClearFilters={handleClearAll}
        />
      )
    }

    if (viewMode === VIEW_MODES.CARD) {
      return (
        <ModelCardGrid
          models={filteredModels}
          page={currentPage}
          pageSize={pageSize}
          onModelClick={handleModelClick}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          tokenUnit={tokenUnit}
          showRechargePrice={showRechargePrice}
          selectedGroup={groupFilter}
        />
      )
    }

    return (
      <PricingTable
        models={filteredModels}
        page={currentPage}
        pageSize={pageSize}
        priceRate={priceRate}
        usdExchangeRate={usdExchangeRate}
        tokenUnit={tokenUnit}
        showRechargePrice={showRechargePrice}
        selectedGroup={groupFilter}
        onModelClick={handleModelClick}
      />
    )
  }

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='mx-auto w-full max-w-[1200px] min-w-0 px-4 py-6 sm:py-8 lg:px-6'>
          <LoadingSkeleton viewMode={viewMode} />
        </div>
      </PublicLayout>
    )
  }

  if (error) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='mx-auto w-full max-w-[1200px] min-w-0 px-4 py-6 lg:px-6'>
          <h1 data-pricing-heading ref={headingRef} tabIndex={-1} className='mb-4 text-2xl font-semibold'>{t('Model Square')}</h1>
          <Alert variant='destructive'>
            <AlertTitle>{t('Failed to load models')}</AlertTitle>
            <Button className='mt-3 min-h-11' disabled={isFetching} onClick={() => { void refetch() }}>{t('Retry')}</Button>
          </Alert>
        </main>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      {/* The console's 1200px column and page header (design direction 3). */}
      <PageTransition className='mx-auto w-full max-w-[1200px] min-w-0 px-4 py-6 sm:py-8 lg:px-6'>
        <header className='mb-5 flex min-w-0 flex-col gap-1.5 sm:mb-6'>
          <h1 data-pricing-heading ref={headingRef} tabIndex={-1} className='text-2xl leading-8 font-semibold tracking-tight wrap-break-word'>
            {t('Model Square')}
          </h1>
          <p className='text-muted-foreground text-sm leading-relaxed'>
            {t('This site currently has {{count}} models enabled', {
              count: models?.length || 0,
            })}
            {' · '}
            {t(
              'Discover curated AI models, compare pricing and capabilities, and choose the right model for every scenario.'
            )}
          </p>
          <SearchBar
            value={searchInput}
            onChange={setSearchInput}
            onClear={clearSearch}
            placeholder={t('Search model name, provider, endpoint, or tag...')}
            className='mt-3 max-w-2xl'
          />
        </header>

        <dl className='mb-5 grid grid-cols-1 gap-3 sm:grid-cols-3'>
          {[
            [t('Returned models'), models.length],
            [t('Matching models'), filteredModels.length],
            [t('Providers in returned models'), models.length > 0 && models.some((model) => model.vendor_id == null) ? t('Unknown') : new Set(models.map((model) => model.vendor_id)).size],
          ].map(([label, value]) => <div key={label} className='bg-card rounded-xl border p-4'><dt className='text-muted-foreground text-sm'>{label}</dt><dd className='mt-1 text-2xl font-semibold tabular-nums'>{value}</dd></div>)}
        </dl>
        <p className='text-muted-foreground mb-4 text-xs'>{t('Counts and filters cover models returned by this catalog read.')}</p>
        <div className='grid gap-4 xl:grid-cols-[280px_minmax(0,1fr)]'>
          <PricingSidebar
            quotaTypeFilter={quotaTypeFilter}
            endpointTypeFilter={endpointTypeFilter}
            vendorFilter={vendorFilter}
            groupFilter={groupFilter}
            tagFilter={tagFilter}
            onQuotaTypeChange={setQuotaTypeFilter}
            onEndpointTypeChange={setEndpointTypeFilter}
            onVendorChange={setVendorFilter}
            onGroupChange={setGroupFilter}
            onTagChange={setTagFilter}
            vendors={vendors || []}
            groups={availableGroups}
            groupRatios={groupRatio}
            tags={availableTags}
            models={models || []}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={clearFilters}
            className='hover-scrollbar sticky top-[4.5rem] hidden max-h-[calc(100dvh-5.5rem)] self-start overflow-y-auto xl:block'
          />

          <main className='min-w-0 space-y-4'>
            <PricingToolbar
              filteredCount={filteredModels.length}
              totalCount={models?.length}
              sortBy={sortBy}
              onSortChange={setSortBy}
              tokenUnit={tokenUnit}
              onTokenUnitChange={setTokenUnit}
              showRechargePrice={showRechargePrice}
              onRechargePriceChange={setShowRechargePrice}
              viewMode={viewMode}
              onViewModeChange={setViewMode}
              quotaTypeFilter={quotaTypeFilter}
              endpointTypeFilter={endpointTypeFilter}
              vendorFilter={vendorFilter}
              groupFilter={groupFilter}
              tagFilter={tagFilter}
              onQuotaTypeChange={setQuotaTypeFilter}
              onEndpointTypeChange={setEndpointTypeFilter}
              onVendorChange={setVendorFilter}
              onGroupChange={setGroupFilter}
              onTagChange={setTagFilter}
              vendors={vendors || []}
              groups={availableGroups}
              groupRatios={groupRatio}
              tags={availableTags}
              models={models || []}
              hasActiveFilters={hasActiveFilters}
              activeFilterCount={activeFilterCount}
              onClearFilters={clearFilters}
            />

            {renderPricingContent()}
            <LogPagination label={t('Model catalog pagination')} page={currentPage} pageSize={pageSize} total={models.length === 0 ? 0 : filteredModels.length} busy={false} onPageChange={setPage} onPageSizeChange={setPageSize} />
          </main>
        </div>

        {selectedModel && (
          <ModelDetailsDrawer
            key={selectedModel.model_name}
            returnSearch={{ ...catalogSearch, page: currentPage, pageSize, focusModel: selectedModel.model_name }}
            returnFocus={openerRef}
            onOpenPage={() => rememberFocus('data-pricing-model', modelCatalogFocusKey(selectedModel.model_name))}
            open={Boolean(selectedModel)}
            onOpenChange={(open) => {
              if (!open) setSelectedModelName(null)
            }}
            model={selectedModel}
            groupRatio={groupRatio || {}}
            usableGroup={usableGroup || {}}
            endpointMap={
              (endpointMap as Record<
                string,
                { path?: string; method?: string }
              >) || {}
            }
            autoGroups={autoGroups || []}
            priceRate={priceRate ?? 1}
            usdExchangeRate={usdExchangeRate ?? 1}
            tokenUnit={tokenUnit}
            showRechargePrice={showRechargePrice}
          />
        )}
      </PageTransition>
    </PublicLayout>
  )
}
