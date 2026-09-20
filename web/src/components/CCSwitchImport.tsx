import { useState, type FormEvent } from 'react'
import { ExternalLink, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  ccSwitchLink,
  openCCSwitch,
  validImportSettings,
  type ImportSettings,
} from '@/lib/ccswitch'
import type { APIKey } from '@/lib/keys'
import { ApiError } from '@/lib/request'
import { useKeySecret } from '@/hooks/use-key-secret'
import { Button } from './ui/Button'
import { Input } from './ui/Input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'

export function CCSwitchImport({
  value,
  userID,
  usable,
}: {
  value: APIKey
  userID: number
  usable: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const eligible =
    usable &&
    value.copyable &&
    value.enabled &&
    value.revoked_at === null &&
    value.group_access === 'allowed'
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          disabled={!eligible}
          aria-label={t('ccSwitchImportNamed', { name: value.name })}
          title={t(
            !value.copyable && value.revoked_at === null
              ? 'keyLegacyCopy'
              : eligible
                ? 'ccSwitchImport'
                : 'ccSwitchKeyUnavailable',
          )}
        >
          <ExternalLink aria-hidden="true" />
        </Button>
      </DialogTrigger>
      {open &&
        (eligible ? (
          <ImportForm
            value={value}
            userID={userID}
            onClose={() => setOpen(false)}
          />
        ) : (
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('ccSwitchImport')}</DialogTitle>
              <DialogDescription>
                {t('ccSwitchKeyUnavailable')}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button onClick={() => setOpen(false)}>{t('done')}</Button>
            </DialogFooter>
          </DialogContent>
        ))}
    </Dialog>
  )
}

function ImportForm({
  value,
  userID,
  onClose,
}: {
  value: APIKey
  userID: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(`SubLane · ${value.name}`)
  const [model, setModel] = useState('')
  const [invalid, setInvalid] = useState(false)
  const [prepared, setPrepared] = useState<string | null>(null)
  const [opened, setOpened] = useState(false)
  const [openFailed, setOpenFailed] = useState(false)
  const origin = window.location.origin
  const { mutation, isCurrentOwner } = useKeySecret<ImportSettings>(
    userID,
    value.id,
    ({ secret, input }) => {
      setPrepared(ccSwitchLink({ ...input, secret }))
    },
  )
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (mutation.isPending || prepared) return
    const input = { origin, name, model }
    const valid = validImportSettings(input)
    setInvalid(!valid)
    if (valid) mutation.mutate(input)
  }
  const launch = () => {
    if (!prepared || !isCurrentOwner()) return
    try {
      // A separate click preserves browser user activation after the asynchronous secret lookup.
      openCCSwitch(prepared)
      setOpened(true)
      setOpenFailed(false)
    } catch {
      setOpenFailed(true)
    }
  }
  const edit = () => {
    setPrepared(null)
    setOpened(false)
    setOpenFailed(false)
    mutation.reset()
  }
  return (
    <DialogContent
      showCloseButton={false}
      className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
    >
      <DialogHeader>
        <DialogTitle>{t('ccSwitchImport')}</DialogTitle>
        <DialogDescription>{t('ccSwitchDescription')}</DialogDescription>
      </DialogHeader>
      <form onSubmit={submit} noValidate className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span className="font-medium">Codex</span>
          <span className="text-muted-foreground">
            {t('keyGroupName', {
              name: value.group_id === 1 ? t('defaultGroup') : value.group_name,
            })}
          </span>
        </div>
        <div className="space-y-2">
          <label htmlFor="cc-switch-name" className="text-sm font-medium">
            {t('ccSwitchName')}
          </label>
          <Input
            id="cc-switch-name"
            value={name}
            maxLength={256}
            disabled={mutation.isPending || prepared !== null}
            onChange={(event) => setName(event.target.value)}
            aria-invalid={invalid}
          />
        </div>
        <div className="space-y-2">
          <label htmlFor="cc-switch-model" className="text-sm font-medium">
            {t('clientModel')}
          </label>
          <Input
            id="cc-switch-model"
            value={model}
            maxLength={160}
            autoComplete="off"
            spellCheck={false}
            disabled={mutation.isPending || prepared !== null}
            onChange={(event) => setModel(event.target.value)}
            aria-describedby="cc-switch-model-hint"
            aria-invalid={invalid}
          />
          <p
            id="cc-switch-model-hint"
            className="text-xs leading-5 text-muted-foreground"
          >
            {t('ccSwitchModelHint')}
          </p>
        </div>
        <dl className="space-y-3 text-sm">
          <div>
            <dt className="text-muted-foreground">{t('clientEndpoint')}</dt>
            <dd className="mt-1 break-all font-mono text-xs">{origin}/v1</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">{t('apiKey')}</dt>
            <dd className="mt-1 break-words">
              {value.name}{' '}
              <code className="text-xs text-muted-foreground">
                {value.prefix}…
              </code>
            </dd>
          </div>
        </dl>
        {invalid && (
          <p role="alert" className="text-sm text-error">
            {t('ccSwitchInputInvalid')}
          </p>
        )}
        {mutation.isError && (
          <p role="alert" className="text-sm text-error">
            {t(
              mutation.error instanceof ApiError &&
                mutation.error.code === 'api_key_not_copyable'
                ? 'keyLegacyCopy'
                : mutation.error instanceof ApiError &&
                    mutation.error.code === 'api_key_revoked'
                  ? 'keyAlreadyRevoked'
                  : 'keyRetrieveFailed',
            )}
          </p>
        )}
        {openFailed && (
          <p role="alert" className="text-sm text-error">
            {t('ccSwitchOpenFailed')}
          </p>
        )}
        <p
          className="text-xs leading-5 text-muted-foreground"
          role={prepared ? 'status' : undefined}
        >
          {t(
            opened
              ? 'ccSwitchOpened'
              : prepared
                ? 'ccSwitchReady'
                : 'ccSwitchPrivacy',
          )}
        </p>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t(opened ? 'done' : 'cancel')}
          </Button>
          {prepared ? (
            <>
              <Button type="button" variant="ghost" onClick={edit}>
                {t('ccSwitchEdit')}
              </Button>
              <Button type="button" onClick={launch}>
                <ExternalLink aria-hidden="true" />
                {t('ccSwitchOpen')}
              </Button>
            </>
          ) : (
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending && (
                <LoaderCircle
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              )}
              {t(mutation.isPending ? 'retrievingKey' : 'ccSwitchPrepare')}
            </Button>
          )}
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
