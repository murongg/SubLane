import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { connectionOptions } from '@/lib/connection'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import { Status } from './Status'

export function ClientGuide({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(connectionOptions(client, userID))
  const [model, setModel] = useState('')
  const [copied, setCopied] = useState(false)
  const [copyFailed, setCopyFailed] = useState(false)
  const endpoint = `${window.location.origin}/v1`
  const configuration = `${model.trim() ? `model = ${JSON.stringify(model.trim())}\n` : ''}model_provider = "sublane"\n\n[model_providers.sublane]\nname = "SubLane"\nbase_url = ${JSON.stringify(endpoint)}\nenv_key = "SUBLANE_API_KEY"\nwire_api = "responses"\nrequires_openai_auth = false\nsupports_websockets = true`
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(configuration)
      setCopied(true)
      setCopyFailed(false)
    } catch {
      setCopyFailed(true)
      setCopied(false)
    }
  }
  return (
    <section
      className="space-y-5 border-t border-border pt-6"
      aria-labelledby="client-guide-title"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="client-guide-title" className="font-semibold">
          {t('clientGuideTitle')}
        </h2>
        <Status
          kind={
            query.isError || query.isPending
              ? 'neutral'
              : query.data?.status === 'ready'
                ? 'success'
                : 'warning'
          }
        >
          {t(
            query.isPending
              ? 'gatewayChecking'
              : query.isError
                ? 'gatewayStatusUnknown'
                : query.data?.status === 'ready'
                  ? 'ready'
                  : query.data?.status === 'needs_attention'
                    ? 'gatewayNeedsAttention'
                    : 'notConfigured',
          )}
        </Status>
      </div>
      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
        {t('clientGuideDescription')}
      </p>
      {query.isError ? (
        <p className="text-sm text-muted-foreground">
          {t('connectionStatusError')}
        </p>
      ) : query.data && query.data.status !== 'ready' ? (
        <p className="text-sm text-muted-foreground">
          {t('connectionNotConfigured')}
        </p>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label htmlFor="client-endpoint" className="text-sm font-medium">
            {t('clientEndpoint')}
          </label>
          <Input
            id="client-endpoint"
            value={endpoint}
            readOnly
            className="font-mono text-sm"
          />
        </div>
        <div className="space-y-2">
          <label htmlFor="client-model" className="text-sm font-medium">
            {t('clientModel')}
          </label>
          <Input
            id="client-model"
            value={model}
            onChange={(event) => {
              setModel(event.target.value)
              setCopied(false)
            }}
            maxLength={128}
            aria-describedby="client-model-hint"
          />
          <p id="client-model-hint" className="text-xs text-muted-foreground">
            {t('clientModelHint')}
          </p>
        </div>
      </div>
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-medium">{t('clientConfig')}</h3>
          <Button variant="ghost" size="sm" onClick={copy}>
            <Copy aria-hidden="true" />
            {t(copied ? 'copied' : 'copyClientConfig')}
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          {t('clientConfigLocation')}
        </p>
        <pre className="overflow-x-auto rounded-lg bg-muted p-4 text-xs leading-6">
          <code>{configuration}</code>
        </pre>
      </div>
      {copyFailed && (
        <p role="alert" className="text-sm text-error">
          {t('copyKeyFailed')}
        </p>
      )}
      <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
        {t('clientGuideNote')}
      </p>
    </section>
  )
}
