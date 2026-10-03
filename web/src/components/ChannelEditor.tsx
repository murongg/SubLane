import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { channelErrorKey, saveChannel, type Channel } from '@/lib/channels'
import { proxyOptions } from '@/lib/proxies'
import { ProxyPicker } from './ProxyPicker'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function ChannelEditor({
  channel,
  onClose,
  onSaved,
  restoreFocus,
}: {
  channel?: Channel
  onClose: () => void
  onSaved: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(channel?.name ?? '')
  const [baseURL, setBaseURL] = useState(
    channel?.base_url ?? 'https://api.openai.com/v1',
  )
  const [key, setKey] = useState('')
  const [proxyID, setProxyID] = useState(channel?.proxy_id ?? '')
  const [invalid, setInvalid] = useState<'name' | 'key' | 'endpoint' | null>(
    null,
  )
  const proxies = useQuery({ ...proxyOptions, enabled: !channel })
  const save = useMutation({
    mutationFn: saveChannel,
    gcTime: 0,
    onSuccess: async () => {
      setKey('')
      await onSaved()
      onClose()
    },
  })
  const close = () => {
    if (!save.isPending) onClose()
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (save.isPending) return
    setInvalid(null)
    if (
      Array.from(name.trim()).length < 1 ||
      Array.from(name.trim()).length > 64 ||
      /\p{Cc}/u.test(name)
    ) {
      setInvalid('name')
      return
    }
    if (
      !key ||
      new TextEncoder().encode(key).length > 16384 ||
      /[\s\p{Cc}]/u.test(key)
    ) {
      setInvalid('key')
      return
    }
    try {
      const url = new URL(baseURL.trim())
      if (
        !['https:', 'http:'].includes(url.protocol) ||
        url.username ||
        url.password ||
        url.search ||
        url.hash ||
        new TextEncoder().encode(baseURL.trim()).length > 2048
      ) {
        setInvalid('endpoint')
        return
      }
    } catch {
      setInvalid('endpoint')
      return
    }
    save.mutate({
      id: channel?.id,
      name: name.trim(),
      api_key: key,
      base_url: baseURL.trim(),
      proxy_id: !channel && proxyID ? proxyID : undefined,
    })
  }
  const fieldErrors = {
    name: 'channelNameInvalid',
    key: 'upstreamAPIKeyInvalid',
    endpoint: 'upstreamBaseURLInvalid',
  } as const
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto [@media(pointer:coarse)]:[&_button]:min-h-11"
        onInteractOutside={(event) => {
          if (save.isPending) event.preventDefault()
        }}
        onEscapeKeyDown={(event) => {
          if (save.isPending) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          restoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(channel ? 'replaceAPIKey' : 'addChannel')}
          </DialogTitle>
          <DialogDescription>{t('apiUpstreamDescription')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} noValidate className="space-y-5">
          <div className="space-y-2">
            <label htmlFor="channel-name" className="text-sm font-medium">
              {t('channelName')}
            </label>
            <Input
              id="channel-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              readOnly={Boolean(channel)}
              disabled={save.isPending}
              maxLength={128}
              aria-invalid={invalid === 'name'}
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="channel-base-url" className="text-sm font-medium">
              {t('upstreamBaseURL')}
            </label>
            <Input
              id="channel-base-url"
              type="url"
              value={baseURL}
              onChange={(event) => setBaseURL(event.target.value)}
              readOnly={Boolean(channel)}
              disabled={save.isPending}
              autoComplete="off"
              spellCheck={false}
              aria-invalid={invalid === 'endpoint'}
              aria-describedby="channel-endpoint-hint"
            />
            <p
              id="channel-endpoint-hint"
              className="text-xs leading-5 text-muted-foreground"
            >
              {t(channel ? 'upstreamEndpointLocked' : 'upstreamBaseURLHint')}
            </p>
          </div>
          <div className="space-y-2">
            <label htmlFor="channel-key" className="text-sm font-medium">
              {t('upstreamAPIKey')}
            </label>
            <Input
              id="channel-key"
              type="password"
              value={key}
              onChange={(event) => setKey(event.target.value)}
              disabled={save.isPending}
              autoComplete="new-password"
              spellCheck={false}
              maxLength={16384}
              aria-invalid={invalid === 'key'}
              aria-describedby="channel-key-hint"
            />
            <p
              id="channel-key-hint"
              className="text-xs leading-5 text-muted-foreground"
            >
              {t('channelKeyHint')}
            </p>
          </div>
          {!channel && (
            <div className="space-y-2">
              {proxies.isPending ? (
                <p role="status" className="text-sm text-muted-foreground">
                  {t('loadingProxies')}
                </p>
              ) : proxies.isError ? (
                <p role="alert" className="text-sm text-error">
                  {t('proxiesLoadFailed')}
                </p>
              ) : (
                <ProxyPicker
                  value={proxyID}
                  onChange={setProxyID}
                  proxies={proxies.data.proxies}
                  disabled={save.isPending}
                />
              )}
              <p className="text-xs leading-5 text-muted-foreground">
                {t('apiUpstreamProxyHint')}
              </p>
            </div>
          )}
          {(invalid || save.isError) && (
            <p role="alert" className="text-sm text-error">
              {t(invalid ? fieldErrors[invalid] : channelErrorKey(save.error))}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={close}
              disabled={save.isPending}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t(channel ? 'replaceAPIKey' : 'connectAPI')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
