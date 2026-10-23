import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Fingerprint, LoaderCircle, ShieldCheck } from 'lucide-react'
import {
  addPasskey,
  beginTOTP,
  confirmTOTP,
  deletePasskey,
  disableTOTP,
  passkeySupported,
  regenerateRecoveryCodes,
  securityError,
  securityOptions,
  type Proof,
  type TOTPSetup,
} from '@/lib/security'
import { ApiError } from '@/lib/request'
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

type Action =
  | { type: 'enable' | 'disable' | 'recovery' | 'add' }
  | { type: 'delete'; id: number; name: string }
const titles = {
  enable: 'totpEnableTitle',
  disable: 'totpDisable',
  recovery: 'recoveryRegenerate',
  add: 'passkeyAdd',
  delete: 'passkeyDelete',
} as const

export function Security() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const security = useQuery(securityOptions(client))
  const [action, setAction] = useState<Action | null>(null)
  const [saved, setSaved] = useState(false)
  const trigger = useRef<HTMLElement | null>(null)
  const open = (next: Action) => {
    trigger.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    setSaved(false)
    setAction(next)
  }
  return (
    <>
      <section className="space-y-5 py-6">
        <div>
          <h2 className="flex items-center gap-2 font-medium">
            <ShieldCheck className="size-4" aria-hidden="true" />
            {t('totpTitle')}
          </h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('totpDescription')}
          </p>
        </div>
        {security.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            {t('loading')}
          </p>
        ) : security.isError ? (
          <div className="space-y-3">
            <p role="alert" className="text-sm text-error">
              {t('unavailable')}
            </p>
            <Button variant="outline" onClick={() => security.refetch()}>
              {t('refresh')}
            </Button>
          </div>
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm">
                {t(security.data.totp_enabled ? 'totpEnabled' : 'totpDisabled')}
              </p>
              <div className="flex flex-wrap gap-2">
                {security.data.totp_enabled && (
                  <Button
                    variant="outline"
                    onClick={() => open({ type: 'recovery' })}
                  >
                    {t('recoveryRegenerate')}
                  </Button>
                )}
                <Button
                  variant="outline"
                  onClick={() =>
                    open({
                      type: security.data.totp_enabled ? 'disable' : 'enable',
                    })
                  }
                >
                  {t(security.data.totp_enabled ? 'totpDisable' : 'totpEnable')}
                </Button>
              </div>
            </div>
            {security.data.totp_enabled && (
              <p className="text-sm text-muted-foreground">
                {t('recoveryRemaining', {
                  remaining: security.data.recovery_codes,
                })}
              </p>
            )}
          </>
        )}
      </section>
      <section className="space-y-5 py-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-lg">
            <h2 className="flex items-center gap-2 font-medium">
              <Fingerprint className="size-4" aria-hidden="true" />
              {t('passkeysTitle')}
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {t('passkeysDescription')}
            </p>
          </div>
          <Button
            variant="outline"
            disabled={
              !security.data ||
              security.data.passkeys.length >= 10 ||
              !passkeySupported() ||
              security.isError
            }
            onClick={() => open({ type: 'add' })}
          >
            {t('passkeyAdd')}
          </Button>
        </div>
        {!passkeySupported() && (
          <p className="text-sm leading-6 text-muted-foreground">
            {t('passkeyUnavailable')}
          </p>
        )}
        {security.data &&
          (security.data.passkeys.length ? (
            <ul className="divide-y divide-border">
              {security.data.passkeys.map((key) => (
                <li
                  key={key.id}
                  className="flex items-center justify-between gap-3 py-3"
                >
                  <span className="min-w-0 break-words text-sm">
                    {key.name}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t('passkeyDeleteNamed', { name: key.name })}
                    onClick={() =>
                      open({ type: 'delete', id: key.id, name: key.name })
                    }
                  >
                    {t('passkeyDelete')}
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('passkeysEmpty')}
            </p>
          ))}
      </section>
      {saved && (
        <p role="status" className="py-4 text-sm text-success">
          {t('securitySaved')}
        </p>
      )}
      {action && security.data && (
        <SecurityDialog
          key={action.type}
          action={action}
          totpEnabled={security.data.totp_enabled}
          onClose={() => setAction(null)}
          returnFocus={() => trigger.current?.focus()}
          onSaved={async () => {
            setSaved(true)
            await client.invalidateQueries({ queryKey: ['security'] })
          }}
        />
      )}
    </>
  )
}

function SecurityDialog({
  action,
  totpEnabled,
  onClose,
  onSaved,
  returnFocus,
}: {
  action: Action
  totpEnabled: boolean
  onClose: () => void
  onSaved: () => Promise<unknown>
  returnFocus: () => void
}) {
  const { t } = useTranslation()
  const [setup, setSetup] = useState<TOTPSetup | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const form = useRef<HTMLFormElement>(null)
  const mutation = useMutation({
    gcTime: 0,
    mutationFn: async (input: { proof: Proof; name: string; code: string }) => {
      switch (action.type) {
        case 'enable':
          return setup ? confirmTOTP(input.code) : beginTOTP(input.proof)
        case 'disable':
          return disableTOTP(input.proof)
        case 'recovery':
          return regenerateRecoveryCodes(input.proof)
        case 'add':
          return addPasskey(input.name, input.proof)
        case 'delete':
          return deletePasskey(action.id, input.proof)
      }
    },
    onSuccess: async (result) => {
      form.current?.reset()
      mutation.reset()
      if (result && 'secret' in result) {
        setSetup(result)
        return
      }
      setSetup(null)
      if (result && 'recovery_codes' in result) setCodes(result.recovery_codes)
      else onClose()
      await onSaved()
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending) return
    const data = new FormData(event.currentTarget)
    mutation.mutate({
      proof: {
        current_password: String(data.get('password') ?? ''),
        code: String(data.get('factor') ?? '').trim(),
      },
      name: String(data.get('name') ?? '').trim(),
      code: String(data.get('code') ?? '').trim(),
    })
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose()
      }}
    >
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        showCloseButton={!mutation.isPending}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          returnFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t(codes ? 'recoveryCodesTitle' : titles[action.type])}
          </DialogTitle>
          <DialogDescription>
            {t(
              codes
                ? 'recoveryDescription'
                : setup
                  ? 'totpScan'
                  : 'securityProofHint',
            )}
          </DialogDescription>
        </DialogHeader>
        {codes ? (
          <div className="space-y-5">
            <ul
              className="grid grid-cols-1 gap-2 rounded-md bg-muted p-4 text-center font-mono text-sm tabular-nums min-[400px]:grid-cols-2"
              dir="ltr"
            >
              {codes.map((code) => (
                <li key={code}>{code}</li>
              ))}
            </ul>
            <DialogFooter>
              <Button variant="outline" asChild>
                <a
                  download="sublane-recovery-codes.txt"
                  href={
                    'data:text/plain;charset=utf-8,' +
                    encodeURIComponent(codes.join('\n') + '\n')
                  }
                >
                  {t('recoveryDownload')}
                </a>
              </Button>
              <Button onClick={onClose}>{t('recoverySaved')}</Button>
            </DialogFooter>
          </div>
        ) : (
          <form ref={form} onSubmit={submit} className="space-y-4">
            {setup ? (
              <>
                <img
                  src={setup.qr}
                  alt={t('totpQR')}
                  width={240}
                  height={240}
                  className="mx-auto max-w-full rounded-md"
                />
                <div className="space-y-2">
                  <label htmlFor="totp-secret" className="text-sm font-medium">
                    {t('totpSecret')}
                  </label>
                  <Input
                    id="totp-secret"
                    value={setup.secret}
                    readOnly
                    className="font-mono"
                    dir="ltr"
                  />
                </div>
                <div className="space-y-2">
                  <label htmlFor="totp-code" className="text-sm font-medium">
                    {t('totpCode')}
                  </label>
                  <Input
                    key="totp-confirm"
                    id="totp-code"
                    name="code"
                    autoComplete="one-time-code"
                    inputMode="numeric"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    required
                    disabled={mutation.isPending}
                  />
                </div>
              </>
            ) : (
              <>
                {action.type === 'delete' && (
                  <p className="break-words text-sm">{action.name}</p>
                )}
                {action.type === 'add' && (
                  <div className="space-y-2">
                    <label
                      htmlFor="passkey-name"
                      className="text-sm font-medium"
                    >
                      {t('passkeyName')}
                    </label>
                    <Input
                      id="passkey-name"
                      name="name"
                      maxLength={64}
                      required
                      disabled={mutation.isPending}
                      autoComplete="off"
                    />
                  </div>
                )}
                <div className="space-y-2">
                  <label
                    htmlFor="security-password"
                    className="text-sm font-medium"
                  >
                    {t('currentPassword')}
                  </label>
                  <Input
                    id="security-password"
                    name="password"
                    type="password"
                    autoComplete="current-password"
                    maxLength={256}
                    required
                    disabled={mutation.isPending}
                  />
                </div>
                {totpEnabled && (
                  <div className="space-y-2">
                    <label
                      htmlFor="security-factor"
                      className="text-sm font-medium"
                    >
                      {t('factorCode')}
                    </label>
                    <Input
                      id="security-factor"
                      name="factor"
                      autoComplete="one-time-code"
                      maxLength={64}
                      required
                      disabled={mutation.isPending}
                    />
                  </div>
                )}
              </>
            )}
            {mutation.isError && (
              <p role="alert" className="text-sm leading-6 text-error">
                {t(securityError(mutation.error), {
                  seconds:
                    mutation.error instanceof ApiError
                      ? mutation.error.retryAfter || 60
                      : 60,
                })}
              </p>
            )}
            <DialogFooter>
              <Button
                variant="outline"
                type="button"
                disabled={mutation.isPending}
                onClick={onClose}
              >
                {t('cancel')}
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending && (
                  <LoaderCircle
                    className="motion-safe:animate-spin"
                    aria-hidden="true"
                  />
                )}
                {t(
                  setup
                    ? 'totpConfirm'
                    : action.type === 'enable'
                      ? 'securityContinue'
                      : titles[action.type],
                )}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
