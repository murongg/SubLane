import { queryOptions } from '@tanstack/react-query'
import { z } from 'zod'
import type { en } from '@/locales/en'
import { request } from './request'
export const auditActions: Record<string, keyof typeof en> = {
  'backup.export': 'auditBackupExport',
  'backup.prepare': 'auditBackupPrepare',
  'backup.verify': 'auditBackupVerify',
  'settings.update': 'auditSettingsUpdate',
  'key.reveal': 'auditKeyReveal',
  'key.create': 'auditKeyCreate',
  'key.update': 'auditKeyUpdate',
  'key.revoke': 'auditKeyRevoke',
  'group.create': 'auditGroupCreate',
  'group.update': 'auditGroupUpdate',
  'member.create': 'auditMemberCreate',
  'member.update': 'auditMemberUpdate',
  'member.groups': 'auditMemberGroups',
  'member.budget': 'auditMemberBudget',
  'member.budget_settle': 'auditMemberBudgetSettle',
  'member.limits': 'auditMemberLimits',
  'member.password': 'auditMemberPassword',
  'user.password': 'auditUserPassword',
  'user.recover': 'auditUserRecover',
  'account.create': 'auditAccountCreate',
  'account.authorize': 'auditAccountAuthorize',
  'account.update': 'auditAccountUpdate',
  'account.delete': 'auditAccountDelete',
  'account.concurrency': 'auditAccountConcurrency',
  'account.resume': 'auditAccountResume',
}
export const auditResources = {
  backup: 'backupTitle',
  settings: 'auditSettings',
  key: 'apiKey',
  group: 'keyGroup',
  member: 'auditMember',
  user: 'auditUser',
  account: 'auditAccount',
} as const
export type AuditResource = '' | keyof typeof auditResources
export type AuditOutcome = '' | 'success' | 'failure'
const eventSchema = z.object({
  id: z.number().int().positive(),
  actor_id: z.number().int().nonnegative(),
  actor_name: z.string().max(64),
  actor_role: z.enum(['admin', 'member']),
  source: z.enum(['user', 'local']),
  action: z.string().max(64),
  resource: z.enum([
    'key',
    'group',
    'member',
    'user',
    'account',
    'settings',
    'backup',
  ]),
  resource_id: z.string().max(32),
  outcome: z.enum(['success', 'failure']),
  http_status: z.number().int().nullable(),
  created_at: z.number().int(),
})
export function auditOptions(
  cursor: number,
  resource: AuditResource,
  outcome: AuditOutcome,
) {
  const params = new URLSearchParams({
    cursor: String(cursor),
    resource,
    outcome,
  })
  return queryOptions({
    queryKey: ['audit', cursor, resource, outcome],
    queryFn: ({ signal }) =>
      request(
        `/api/audit?${params}`,
        z.object({
          events: z.array(eventSchema).max(50),
          next_cursor: z.number().int().nonnegative(),
        }),
        { signal },
      ),
  })
}
