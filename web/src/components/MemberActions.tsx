import { useRef, useState } from 'react'
import { MoreHorizontal, Gauge, KeyRound, UserMinus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Member } from '@/lib/members'
import { Button } from './ui/Button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/DropdownMenu'
import { MemberLimits } from './MemberLimits'
import { PasswordDialog } from './PasswordDialog'
import { RemoveMember } from './RemoveMember'

export function MemberActions({
  member,
  onPasswordReset,
  onRemoved,
}: {
  member: Member
  onPasswordReset?: () => void
  onRemoved?: () => Promise<void>
}) {
  const { t } = useTranslation()
  const [action, setAction] = useState<'limits' | 'password' | 'remove' | null>(
    null,
  )
  const trigger = useRef<HTMLButtonElement>(null)
  const focus = () => trigger.current?.focus()
  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button
            ref={trigger}
            variant="ghost"
            size="icon"
            aria-label={t('memberActionsFor', { username: member.username })}
          >
            <MoreHorizontal aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={() => setAction('limits')}>
            <Gauge aria-hidden="true" />
            {t('memberLimits')}
          </DropdownMenuItem>
          {onPasswordReset && (
            <DropdownMenuItem onSelect={() => setAction('password')}>
              <KeyRound aria-hidden="true" />
              {t('resetPassword')}
            </DropdownMenuItem>
          )}
          {member.can_remove && onRemoved && (
            <DropdownMenuItem
              variant="destructive"
              onSelect={() => setAction('remove')}
            >
              <UserMinus aria-hidden="true" />
              {t('memberRemove')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {action === 'limits' && (
        <MemberLimits
          member={member}
          onClose={() => setAction(null)}
          returnFocus={focus}
        />
      )}
      {action === 'password' && onPasswordReset && (
        <PasswordDialog
          member={member}
          onClose={() => setAction(null)}
          onSaved={onPasswordReset}
          returnFocus={focus}
        />
      )}
      {action === 'remove' && onRemoved && (
        <RemoveMember
          member={member}
          onClose={() => setAction(null)}
          onRemoved={onRemoved}
          restoreFocus={focus}
        />
      )}
    </>
  )
}
