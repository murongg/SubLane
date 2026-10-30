import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ChannelForm } from '@/components/ChannelForm'
import { Button } from '@/components/ui/Button'

export function ChannelCreate() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const client = useQueryClient()
  const [pending, setPending] = useState(false)
  const back = () => navigate({ to: '/channels' })
  return (
    <div className="max-w-3xl space-y-6">
      <div className="flex items-center gap-3">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t('channelBack')}
          disabled={pending}
          onClick={back}
        >
          <ArrowLeft aria-hidden="true" />
        </Button>
        <h1 className="page-title">{t('addChannel')}</h1>
      </div>
      <p className="text-sm leading-6 text-muted-foreground">
        {t('apiUpstreamDescription')}
      </p>
      <div className="border-t border-border pt-6">
        <ChannelForm
          onCancel={back}
          onPendingChange={setPending}
          onSaved={async () => {
            await client.invalidateQueries({ queryKey: ['channels'] })
            await back()
          }}
        />
      </div>
    </div>
  )
}
