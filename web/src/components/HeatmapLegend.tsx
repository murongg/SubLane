import { useTranslation } from 'react-i18next'
import { heatmapTones } from '@/lib/heatmap'

export function HeatmapLegend() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
      <span>{t('activityLess')}</span>
      {heatmapTones.map((tone) => (
        <span
          key={tone}
          aria-hidden="true"
          className={'size-3 rounded-sm ' + tone}
        />
      ))}
      <span>{t('activityMore')}</span>
      <span aria-hidden="true" className="ml-2 size-3 rounded-sm bg-muted/40" />
      <span>{t('activityUnknown')}</span>
    </div>
  )
}
