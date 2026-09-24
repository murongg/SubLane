import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { importProxies, proxyImportErrorKey } from '@/lib/proxies'
import { Button } from './ui/Button'
import { Textarea } from './ui/Textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'

export function ProxyImport({
  onClose,
  onImported,
  restoreFocus,
}: {
  onClose: () => void
  onImported: () => Promise<void>
  restoreFocus: () => void
}) {
  const { t } = useTranslation()
  const [content, setContent] = useState('')
  const [invalid, setInvalid] = useState(false)
  const importBatch = useMutation({
    mutationFn: importProxies,
    gcTime: 0,
    onSuccess: async () => {
      setContent('')
      await onImported()
      onClose()
    },
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const valid =
      content.trim().length > 0 &&
      new TextEncoder().encode(content).length <= 140 * 1024
    setInvalid(!valid)
    if (valid) importBatch.mutate(content)
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !importBatch.isPending) onClose()
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
        onInteractOutside={(event) => {
          if (importBatch.isPending) event.preventDefault()
        }}
        onEscapeKeyDown={(event) => {
          if (importBatch.isPending) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          restoreFocus()
        }}
      >
        <DialogHeader>
          <DialogTitle>{t('importProxies')}</DialogTitle>
          <DialogDescription>{t('proxyImportHint')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5" noValidate>
          <div className="space-y-2">
            <label htmlFor="proxy-list" className="text-sm font-medium">
              {t('proxyList')}
            </label>
            <Textarea
              id="proxy-list"
              rows={8}
              value={content}
              onChange={(event) => setContent(event.target.value)}
              spellCheck={false}
              autoComplete="off"
              disabled={importBatch.isPending}
              className="resize-y font-mono"
            />
            <p className="text-xs leading-5 text-muted-foreground">
              {t('proxyImportFormat')}
            </p>
          </div>
          {invalid && (
            <p role="alert" className="text-sm text-error">
              {t('proxyImportEmpty')}
            </p>
          )}
          {importBatch.isError && (
            <p role="alert" className="text-sm text-error">
              {t(proxyImportErrorKey(importBatch.error))}
            </p>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={importBatch.isPending}
            >
              {t('cancel')}
            </Button>
            <Button type="submit" disabled={importBatch.isPending}>
              {importBatch.isPending && (
                <LoaderCircle
                  className="motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {t('importProxies')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
