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
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useAuthStore } from '@/stores/auth-store'

import { PlaygroundChat } from './components/chat/playground-chat'
import { PlaygroundInput } from './components/input/playground-input'
import {
  useChatHandler,
  usePlaygroundConversation,
  usePlaygroundOptions,
  usePlaygroundState,
} from './hooks'
import {
  applyUnsupportedParameterRestrictions,
  PLAYGROUND_PARAMETER_CONTROLS,
} from './lib/parameters/playground-parameters'
import {
  STRICT_CHAT_UNSUPPORTED_PARAMETERS,
  usesStrictChatParameters,
} from './lib/parameters/strict-chat-parameters'

export function Playground() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  return userId ? <PlaygroundSession key={userId} userId={userId} /> : null
}

function PlaygroundSession(props: { userId: number }) {
  const { t } = useTranslation()
  const {
    config,
    parameterEnabled,
    messages,
    isLoadingMessages,
    updateMessages,
    updateConfig,
    updateParameterEnabled,
    clearMessages,
  } = usePlaygroundState(props.userId)

  const {
    keys,
    selectedKey,
    models,
    keyNotice,
    isLoadingKeys,
    isLoadingModels,
    canSend,
    refreshKeys,
  } = usePlaygroundOptions({
    userId: props.userId,
    keyId: config.keyId,
    currentModel: config.model,
    updateConfig,
  })

  const selectedModel = useMemo(
    () => models.find((model) => model.value === config.model),
    [config.model, models]
  )
  const strictChatParameters = usesStrictChatParameters(config, selectedKey)
  const unsupportedProvider = strictChatParameters
    ? t('Strict Token budget')
    : selectedModel?.provider
  const unsupportedParameters = useMemo(
    () =>
      strictChatParameters
        ? STRICT_CHAT_UNSUPPORTED_PARAMETERS
        : (selectedModel?.unsupportedParameters ?? []),
    [selectedModel, strictChatParameters]
  )
  const effectiveParameterEnabled = useMemo(
    () =>
      applyUnsupportedParameterRestrictions(
        parameterEnabled,
        unsupportedParameters
      ),
    [parameterEnabled, unsupportedParameters]
  )
  const autoDisabledLabels = useMemo(
    () =>
      PLAYGROUND_PARAMETER_CONTROLS.filter(
        (control) =>
          unsupportedParameters.includes(control.key) &&
          parameterEnabled[control.key]
      ).map((control) => t(control.labelKey)),
    [parameterEnabled, t, unsupportedParameters]
  )

  useEffect(() => {
    if (autoDisabledLabels.length === 0) {
      return
    }

    toast.info(t('Unsupported parameters were turned off'), {
      description: t('{{provider}} does not support: {{parameters}}', {
        provider: unsupportedProvider || t('Current provider'),
        parameters: autoDisabledLabels.join(', '),
      }),
    })
  }, [autoDisabledLabels, config.model, unsupportedProvider, t])

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    selectedKey,
    canSend: canSend && !isLoadingMessages,
    onKeyRejected: () => {
      updateConfig('keyId', null)
      refreshKeys()
    },
    parameterEnabled: effectiveParameterEnabled,
    onMessageUpdate: updateMessages,
  })

  const {
    completedRetryDraft,
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  } = usePlaygroundConversation({
    messages,
    updateMessages,
    sendChat,
  })

  const handleClearMessages = () => {
    handleEditOpenChange(false)
    clearMessages()
  }

  return (
    <div className='relative flex size-full min-h-0 flex-col overflow-hidden'>
      {/* Full-width scroll container: scrolling works even over side whitespace */}
      <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
        <PlaygroundChat
          messages={messages}
          isLoadingMessages={isLoadingMessages}
          onRegenerateMessage={handleRegenerateMessage}
          onEditMessage={handleEditMessage}
          onDeleteMessage={handleDeleteMessage}
          onSelectPrompt={handleSendMessage}
          isGenerating={isGenerating}
          editingKey={editingMessageKey}
          onCancelEdit={handleEditOpenChange}
          onSaveEdit={(newContent) => applyEdit(newContent, false)}
          onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
        />
      </div>

      {/* Input area: center content and constrain to the same container width */}
      <div className='mx-auto w-full max-w-4xl'>
        <PlaygroundInput
          completedRetryDraft={completedRetryDraft}
          config={config}
          disabled={isGenerating}
          keys={keys}
          canSend={canSend && !isLoadingMessages}
          keyNotice={keyNotice}
          isLoadingKeys={isLoadingKeys}
          onRefreshKeys={refreshKeys}
          onKeyChange={(value) => updateConfig('keyId', value)}
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          modelValue={config.model}
          models={models}
          onConfigChange={updateConfig}
          onClearMessages={handleClearMessages}
          onModelChange={(value) => updateConfig('model', value)}
          onParameterEnabledChange={updateParameterEnabled}
          onStop={stopGeneration}
          onSubmit={handleSendMessage}
          parameterEnabled={effectiveParameterEnabled}
          strictChatParameters={strictChatParameters}
          unsupportedParameters={unsupportedParameters}
          unsupportedProvider={unsupportedProvider}
          hasMessages={messages.length > 0}
        />
      </div>
    </div>
  )
}
