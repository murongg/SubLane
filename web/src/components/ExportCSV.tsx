import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Download, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { authKey, type AuthState } from '@/lib/auth'
import { encodeCSV } from '@/lib/csv'
import { selectedWorkspace } from '@/lib/workspace'
import type { en } from '@/locales/en'
import { Button } from '@/components/ui/Button'

export function ExportCSV({
  load,
  filename,
  disabled,
  userID,
  failureKey,
}: {
  load: (signal: AbortSignal) => Promise<string[][]>
  filename: string
  disabled: boolean
  userID: number
  failureKey?: (error: unknown) => keyof typeof en | undefined
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const pending = useRef<AbortController | null>(null)
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState<keyof typeof en | null>(null)
  useEffect(() => () => pending.current?.abort(), [])
  const download = async () => {
    if (pending.current) return
    const controller = new AbortController()
    pending.current = controller
    const workspace = selectedWorkspace()
    setBusy(true)
    setFailure(null)
    try {
      const rows = await load(controller.signal)
      if (
        controller.signal.aborted ||
        selectedWorkspace() !== workspace ||
        client.getQueryData<AuthState>(authKey)?.user?.id !== userID
      )
        return
      const url = URL.createObjectURL(
        new Blob([encodeCSV(rows)], { type: 'text/csv;charset=utf-8' }),
      )
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      if (!controller.signal.aborted)
        setFailure(failureKey?.(error) ?? 'exportCSVFailed')
    } finally {
      pending.current = null
      if (!controller.signal.aborted) setBusy(false)
    }
  }
  return (
    <div className="flex flex-col items-end gap-2">
      <Button variant="outline" disabled={disabled || busy} onClick={download}>
        {busy ? (
          <LoaderCircle
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : (
          <Download aria-hidden="true" />
        )}
        {t(busy ? 'exportCSVLoading' : 'exportCSV')}
      </Button>
      {failure && (
        <p role="alert" className="text-sm text-destructive">
          {t(failure)}
        </p>
      )}
    </div>
  )
}
