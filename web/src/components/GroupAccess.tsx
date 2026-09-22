import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  groupOptions,
  memberGroupOptions,
  saveMemberGroups,
  type Group,
} from '@/lib/groups'
import type { Member } from '@/lib/members'
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

export function GroupAccess({ member }: { member: Member }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-label={t('memberGroupsNamed', { username: member.username })}
        >
          {t('groupAccess')}
        </Button>
      </DialogTrigger>
      {open && <AccessContent member={member} onClose={() => setOpen(false)} />}
    </Dialog>
  )
}
function AccessContent({
  member,
  onClose,
}: {
  member: Member
  onClose: () => void
}) {
  const { t } = useTranslation()
  const groups = useQuery(groupOptions)
  const grants = useQuery({ ...memberGroupOptions(member.id), staleTime: 0 })
  if (
    groups.data &&
    grants.data &&
    !groups.isFetching &&
    !grants.isFetching &&
    !groups.isError &&
    !grants.isError
  )
    return (
      <AccessForm
        member={member}
        groups={groups.data.groups}
        initial={grants.data.group_ids}
        onClose={onClose}
      />
    )
  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t('groupAccess')}</DialogTitle>
        <DialogDescription>
          {t('memberGroupsDescription', { username: member.username })}
        </DialogDescription>
      </DialogHeader>
      {groups.isError || grants.isError ? (
        <div role="alert" className="space-y-3">
          <p className="text-sm text-error">{t('groupsLoadFailed')}</p>
          <Button
            variant="outline"
            onClick={() => Promise.all([groups.refetch(), grants.refetch()])}
          >
            {t('reconnect')}
          </Button>
        </div>
      ) : (
        <p role="status" className="text-sm text-muted-foreground">
          {t('loadingGroups')}
        </p>
      )}
    </DialogContent>
  )
}
function AccessForm({
  member,
  groups,
  initial,
  onClose,
}: {
  member: Member
  groups: Group[]
  initial: number[]
  onClose: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [selected, setSelected] = useState(initial)
  const mutation = useMutation({
    mutationFn: saveMemberGroups,
    onSuccess: async () => {
      onClose()
      await Promise.all([
        client.invalidateQueries({ queryKey: ['groups'] }),
        client.invalidateQueries({ queryKey: ['member-groups', member.id] }),
        client.invalidateQueries({ queryKey: ['keys'] }),
        client.invalidateQueries({ queryKey: ['available-groups'] }),
      ])
    },
  })
  return (
    <DialogContent
      showCloseButton={false}
      className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      onInteractOutside={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
      onEscapeKeyDown={(event) => {
        if (mutation.isPending) event.preventDefault()
      }}
    >
      <DialogHeader>
        <DialogTitle>{t('groupAccess')}</DialogTitle>
        <DialogDescription>
          {t('memberGroupsDescription', { username: member.username })}
        </DialogDescription>
      </DialogHeader>
      <p className="text-sm leading-6 text-muted-foreground">
        {t('groupGrantRevocationHint')}
      </p>
      <fieldset
        disabled={mutation.isPending}
        className="max-h-72 divide-y divide-border overflow-y-auto rounded-lg border border-border"
        aria-label={t('accountGroups')}
      >
        {groups.map((group) => (
          <label
            key={group.id}
            className="flex min-h-12 cursor-pointer items-center gap-3 px-3 py-3 hover:bg-muted"
          >
            <input
              type="checkbox"
              checked={selected.includes(group.id)}
              onChange={(event) =>
                setSelected((ids) =>
                  event.target.checked
                    ? [...ids, group.id]
                    : ids.filter((id) => id !== group.id),
                )
              }
              className="size-4 shrink-0 accent-primary focus-visible:ring-2 focus-visible:ring-ring"
            />
            <span className="min-w-0 break-words text-sm">
              {group.name}
              {!group.enabled && (
                <span className="ml-2 text-xs text-muted-foreground">
                  {t('disabled')}
                </span>
              )}
            </span>
          </label>
        ))}
      </fieldset>
      {selected.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t('memberNoGroupsHint')}
        </p>
      )}
      {mutation.isError && (
        <p role="alert" className="text-sm text-error">
          {t('groupSaveFailed')}
        </p>
      )}
      <DialogFooter>
        <Button
          variant="outline"
          onClick={onClose}
          disabled={mutation.isPending}
        >
          {t('cancel')}
        </Button>
        <Button
          onClick={() =>
            mutation.mutate({ id: member.id, group_ids: selected })
          }
          disabled={mutation.isPending}
        >
          {mutation.isPending && (
            <LoaderCircle
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          )}
          {t('saveGroupAccess')}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}
