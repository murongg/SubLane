import { useState } from 'react'
import { Bird, ExternalLink, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { APIKey } from '@/lib/keys'
import { magpieLink, openMagpie } from '@/lib/magpie'
import { ApiError } from '@/lib/request'
import { useKeySecret } from '@/hooks/use-key-secret'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/Dialog'

export function MagpieImport({
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
          size="sm"
          className="h-auto flex-col gap-1 py-1.5"
          disabled={!eligible}
          aria-label={t('magpieImportNamed', { name: value.name })}
          title={t(
            !value.copyable && value.revoked_at === null
              ? 'keyLegacyCopy'
              : eligible
                ? 'magpieImport'
                : 'magpieKeyUnavailable',
          )}
        >
          <Bird aria-hidden="true" />
          <span className="text-xs font-normal">{t('magpieImport')}</span>
        </Button>
      </DialogTrigger>
      {open && (
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t('magpieImport')}</DialogTitle>
            <DialogDescription>
              {t(eligible ? 'magpieDescription' : 'magpieKeyUnavailable')}
            </DialogDescription>
          </DialogHeader>
          {eligible ? (
            <ImportContent
              value={value}
              userID={userID}
              onClose={() => setOpen(false)}
            />
          ) : (
            <DialogFooter>
              <Button onClick={() => setOpen(false)}>{t('done')}</Button>
            </DialogFooter>
          )}
        </DialogContent>
      )}
    </Dialog>
  )
}

function ImportContent({
  value,
  userID,
  onClose,
}: {
  value: APIKey
  userID: number
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [prepared, setPrepared] = useState<string | null>(null)
  const [opened, setOpened] = useState(false)
  const [openFailed, setOpenFailed] = useState(false)
  const origin = window.location.origin
  const { mutation, isCurrentOwner } = useKeySecret(
    userID,
    value.id,
    ({ secret }) => {
      setPrepared(magpieLink(origin, value.name, secret))
    },
  )
  const launch = () => {
    if (!prepared || !isCurrentOwner()) return
    try {
      // A second click preserves browser user activation after the asynchronous secret lookup.
      openMagpie(prepared)
      setOpened(true)
      setOpenFailed(false)
    } catch {
      setOpenFailed(true)
    }
  }
  return (
    <>
      <dl className="space-y-3 text-sm">
        <div>
          <dt className="text-muted-foreground">{t('clientEndpoint')}</dt>
          <dd className="mt-1 break-all font-mono text-xs">{origin}</dd>
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
          {t('magpieOpenFailed')}
        </p>
      )}
      <p
        className="text-xs leading-5 text-muted-foreground"
        role={prepared ? 'status' : undefined}
      >
        {t(
          opened ? 'magpieOpened' : prepared ? 'magpieReady' : 'magpiePrivacy',
        )}
      </p>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          {t(opened ? 'done' : 'cancel')}
        </Button>
        {prepared ? (
          <Button onClick={launch}>
            <ExternalLink aria-hidden="true" />
            {t('magpieOpen')}
          </Button>
        ) : (
          <Button
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending && (
              <LoaderCircle
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            )}
            {t(mutation.isPending ? 'retrievingKey' : 'magpiePrepare')}
          </Button>
        )}
      </DialogFooter>
    </>
  )
}
