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
import { findModelAction } from '../model-action-focus'

import { DescriptionDialog } from './dialogs/description-dialog'
import { MissingModelsDialog } from './dialogs/missing-models-dialog'
import { PrefillGroupManagement } from './dialogs/prefill-group-management'
import { SyncWizardDialog } from './dialogs/sync-wizard-dialog'
import { UpstreamConflictDialog } from './dialogs/upstream-conflict-dialog'
import { VendorMutateDialog } from './dialogs/vendor-mutate-dialog'
import { ModelMutateDrawer } from './drawers/model-mutate-drawer'
import { useModels } from './models-provider'

export function ModelsDialogs() {
  const {
    open,
    returnFocus,
    setOpen,
    currentRow,
    setCurrentRow,
    missingReturnTarget,
    setMissingReturnTarget,
    currentVendor,
    descriptionData,
    setDescriptionData,
  } = useModels()

  return (
    <>
      {/* Model Create/Update Drawer */}
      <ModelMutateDrawer
        open={open === 'create-model' || open === 'update-model'}
        onOpenChange={(v) => {
          if (v) return
          setCurrentRow(null)
          setOpen(missingReturnTarget ? 'missing-models' : null)
        }}
        currentRow={currentRow}
        returnFocus={() => missingReturnTarget ? findModelAction(missingReturnTarget) : returnFocus()}
      />

      {/* Vendor Create/Update Dialog */}
      <VendorMutateDialog
        open={open === 'create-vendor' || open === 'update-vendor'}
        onOpenChange={(v) => !v && setOpen(null)}
        currentVendor={open === 'update-vendor' ? currentVendor : null}
        returnFocus={returnFocus}
      />

      {/* Missing Models Dialog */}
      <MissingModelsDialog
        open={open === 'missing-models'}
        onOpenChange={(v) => {
          if (v) return
          setMissingReturnTarget(null)
          setOpen(null)
        }}
        returnFocus={returnFocus}
      />

      {/* Sync Wizard Dialog */}
      <SyncWizardDialog
        returnFocus={returnFocus}
        open={open === 'sync-wizard'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* Upstream Conflict Dialog */}
      <UpstreamConflictDialog
        returnFocus={returnFocus}
        open={open === 'upstream-conflict'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* Prefill Groups Management */}
      <PrefillGroupManagement
        returnFocus={returnFocus}
        open={open === 'prefill-groups'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* Description Dialog */}
      <DescriptionDialog
        open={open === 'description'}
        onOpenChange={(v) => {
          if (!v) {
            setOpen(null)
            setDescriptionData(null)
          }
        }}
        returnFocus={returnFocus}
        modelName={descriptionData?.modelName || ''}
        description={descriptionData?.description || ''}
      />
    </>
  )
}
