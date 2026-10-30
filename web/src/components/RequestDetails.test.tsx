import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { authKey } from '@/lib/auth'
import { memberAuthenticated } from '@/test/fixtures'
import type { RequestRecord } from '@/lib/requests'
import { RequestDetails } from './RequestDetails'

const record: RequestRecord = {
  id: 1,
  user_id: 2,
  key_id: 1,
  group_id: 1,
  account_id: '',
  account_name: '',
  provider: 'codex',
  model: 'synthetic-model',
  reasoning_effort: '',
  transport: 'http',
  operation: 'responses',
  started_at: 1900000000,
  duration_ms: 50,
  request_id: 'req_synthetic',
  first_token_ms: null,
  outcome: 'success',
  error_code: '',
  upstream_status: 200,
  input_tokens: 3,
  output_tokens: 2,
  cached_tokens: 0,
  estimated_cost_micro_usd: null,
  username: 'synthetic-member',
  key_name: 'Synthetic key',
  group_name: 'Synthetic group',
}

function renderDetails(value: RequestRecord) {
  const client = new QueryClient()
  client.setQueryData(authKey, memberAuthenticated)
  render(
    <QueryClientProvider client={client}>
      <RequestDetails
        value={value}
        onClose={() => {}}
        onRestoreFocus={() => {}}
      />
    </QueryClientProvider>,
  )
}

it('shows the captured strategy and aggregate exclusions in request details', () => {
  renderDetails({
    ...record,
    route: {
      version: 1,
      strategy: 'rotation',
      result: 'selected',
      candidates: 2,
      parent_fallback: true,
      exclusions: {
        model: 3,
        policy: 0,
        quota: 1,
        cooldown: 0,
        concurrency: 0,
        unavailable: 0,
      },
    },
  })
  const section = screen.getByRole('region', { name: 'Routing explanation' })
  expect(
    within(section).getByText(
      'Used rotation within the preferred eligible tier.',
    ),
  ).toBeTruthy()
  expect(
    within(section).getByText('Model support unknown or unsupported'),
  ).toBeTruthy()
  expect(
    within(section).getByText('Upstream quota or member allowance'),
  ).toBeTruthy()
  expect(
    within(section).getByText(
      'The parent preference was unavailable; normal scheduling applied.',
    ),
  ).toBeTruthy()
  expect(within(section).queryByText('Concurrency capacity')).toBeNull()
})

it('shows unavailable historical explanation for legacy records', () => {
  renderDetails(record)
  expect(
    screen.getByText(
      'No historical routing explanation is available for this request.',
    ),
  ).toBeTruthy()
})

it.each([
  ['affinity', 'Kept the conversation’s bound resource.'],
  ['parent', 'Preferred the parent conversation’s eligible resource.'],
  [
    'quota_weighted',
    'Used quota-weighted rotation within the preferred eligible tier.',
  ],
  ['none', 'No resource was selected.'],
] as const)(
  'explains %s routing without inferring from the request outcome',
  (strategy, message) => {
    renderDetails({
      ...record,
      route: {
        version: 1,
        strategy,
        result: 'rejected',
        candidates: 0,
        parent_fallback: false,
        exclusions: {
          model: 0,
          policy: 0,
          quota: 0,
          cooldown: 1,
          concurrency: 0,
          unavailable: 0,
        },
      },
    })
    expect(screen.getByText(message)).toBeTruthy()
    if (strategy !== 'none')
      expect(
        screen.getByText('Resource admission did not succeed.'),
      ).toBeTruthy()
  },
)
