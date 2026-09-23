import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'
import { MemberActions } from './MemberActions'

it('hides global password reset for workspace administrators', async () => {
  render(
    <MemberActions
      member={{
        id: 3,
        username: 'synthetic-member',
        role: 'member',
        enabled: true,
        created_at: 1,
      }}
    />,
  )
  await userEvent
    .setup()
    .click(
      screen.getByRole('button', { name: 'More actions for synthetic-member' }),
    )
  expect(screen.queryByRole('menuitem', { name: 'Reset password' })).toBeNull()
})
