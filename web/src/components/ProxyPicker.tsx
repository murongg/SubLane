import { useTranslation } from 'react-i18next'
import type { Proxy } from '@/lib/proxies'
import { SearchPicker } from './SearchPicker'

export function ProxyPicker({
  value,
  onChange,
  proxies,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  proxies: Proxy[]
  disabled?: boolean
}) {
  const { t } = useTranslation()
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t('accountProxy')}</legend>
      <SearchPicker
        label={t('accountProxy')}
        value={value}
        onChange={onChange}
        options={[
          { value: '', label: t('directConnection') },
          ...proxies.map((proxy) => ({ value: proxy.id, label: proxy.name })),
        ]}
        searchLabel={t('proxySearch')}
        emptyLabel={t('proxyNoMatch')}
        disabled={disabled}
      />
    </fieldset>
  )
}
