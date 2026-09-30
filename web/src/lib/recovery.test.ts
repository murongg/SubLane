import { expect, it } from 'vitest'
import { requestRecovery } from './recovery'

it('gives administrators the relevant repair destination', () => {
  expect(requestRecovery('auth_required', true).to).toBe('/accounts')
  expect(requestRecovery('allocation_model_unpriced', true).to).toBe(
    '/admin/allocations',
  )
  expect(requestRecovery('group_unavailable', true).to).toBe('/groups')
  expect(requestRecovery('upstream_unavailable', true).to).toBe('/accounts')
  expect(requestRecovery('model_not_allowed', false).to).toBe('/keys')
})
it('never offers administrator actions to members or unknown errors', () => {
  expect(requestRecovery('auth_required', false).to).toBeUndefined()
  expect(requestRecovery('member_rate_limited', false).hint).toBe(
    'recoveryWait',
  )
  expect(requestRecovery('unknown', true).to).toBeUndefined()
})

it('routes content failures to workspace rules only for administrators', () => {
  expect(requestRecovery('content_policy_blocked', true).to).toBe(
    '/admin/content',
  )
  expect(requestRecovery('content_policy_blocked', false).to).toBeUndefined()
  expect(requestRecovery('content_check_unavailable', false).hint).toBe(
    'recoveryContentUnavailable',
  )
})
