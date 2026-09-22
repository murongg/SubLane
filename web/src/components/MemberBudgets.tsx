import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { budgetOptions, type Budget } from '@/lib/budgets'
import type { Member } from '@/lib/members'
import { Button } from './ui/Button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog'
import { BudgetList } from './BudgetList'
import { BudgetForm } from './BudgetForm'
import { BudgetSettlement } from './BudgetSettlement'

export function MemberBudgets({
  member,
  onClose,
  returnFocus,
}: {
  member: Member
  onClose: () => void
  returnFocus: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(budgetOptions(member.id))
  const [editing, setEditing] = useState<Budget | null | undefined>()
  const [busyCount, setBusyCount] = useState(0)
  const busy = busyCount > 0
  const setBusy = (value: boolean) =>
    setBusyCount((count) => Math.max(0, count + (value ? 1 : -1)))
  const addButton = useRef<HTMLButtonElement>(null)
  const refresh = () =>
    Promise.all([
      client.invalidateQueries({ queryKey: ['member-budgets', member.id] }),
      client.invalidateQueries({ queryKey: ['own-budgets', member.id] }),
    ])
  function closeForm() {
    setEditing(undefined)
    requestAnimationFrame(() => addButton.current?.focus())
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
    >
      <DialogContent
        className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl"
        showCloseButton={!busy}
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          returnFocus()
        }}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault()
        }}
        onInteractOutside={(event) => {
          if (busy) event.preventDefault()
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {t('budgetsFor', { username: member.username })}
          </DialogTitle>
          <DialogDescription>{t('budgetsDescription')}</DialogDescription>
        </DialogHeader>
        {query.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            {t('budgetsLoading')}
          </p>
        ) : query.isError ? (
          <div role="alert" className="space-y-3">
            <p className="text-sm text-error">{t('budgetsLoadFailed')}</p>
            <Button variant="outline" onClick={() => query.refetch()}>
              {t('reconnect')}
            </Button>
          </div>
        ) : (
          <>
            <BudgetList
              rules={query.data.rules}
              onEdit={editing === undefined && !busy ? setEditing : undefined}
            />
            {editing === undefined ? (
              <Button
                ref={addButton}
                variant="outline"
                disabled={busy || query.data.rules.length >= 64}
                onClick={() => setEditing(null)}
              >
                {t('budgetAdd')}
              </Button>
            ) : (
              <BudgetForm
                key={editing?.id ?? 'new'}
                userID={member.id}
                rule={editing}
                onBusy={setBusy}
                onCancel={closeForm}
                onSaved={async () => {
                  await refresh()
                  closeForm()
                }}
              />
            )}
            {query.data.pending.length > 0 && (
              <section className="space-y-3 pt-3">
                <h3 className="font-medium">{t('budgetPending')}</h3>
                <p className="text-sm leading-6 text-muted-foreground">
                  {t('budgetSettlementHint')}
                </p>
                {query.data.pending.map((pending) => (
                  <BudgetSettlement
                    key={pending.request_id}
                    userID={member.id}
                    pending={pending}
                    onSaved={refresh}
                    onBusy={setBusy}
                  />
                ))}
              </section>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
