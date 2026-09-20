import { useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from './ui/Input'
import { cn } from '@/lib/cn'

export function ModelPicker({
  id,
  value,
  onChange,
  models,
  disabled = false,
}: {
  id: string
  value: string
  onChange: (value: string) => void
  models: string[]
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const choices = models
    .filter((model) => model.toLowerCase().includes(value.toLowerCase()))
    .slice(0, 100)
  const listID = `${id}-options`
  const choose = (model: string) => {
    onChange(model)
    setOpen(false)
    setActive(-1)
  }
  const activeIndex = active >= 0 && active < choices.length ? active : -1
  return (
    <div className="space-y-2">
      <label htmlFor={id} className="text-sm font-medium">
        {t('clientModel')}
      </label>
      <div
        className="relative"
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
        }}
      >
        <Input
          id={id}
          role="combobox"
          autoComplete="off"
          spellCheck={false}
          value={value}
          disabled={disabled}
          aria-autocomplete="list"
          aria-expanded={open && !disabled}
          aria-controls={open && !disabled ? listID : undefined}
          aria-activedescendant={
            open && activeIndex >= 0 ? `${listID}-${activeIndex}` : undefined
          }
          aria-describedby={`${id}-hint`}
          className="pr-9"
          onFocus={() => setOpen(true)}
          onChange={(event) => {
            onChange(event.target.value)
            setOpen(true)
            setActive(-1)
          }}
          onKeyDown={(event) => {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              setOpen(true)
              if (choices.length)
                setActive(
                  event.key === 'ArrowDown'
                    ? (activeIndex + 1) % choices.length
                    : activeIndex <= 0
                      ? choices.length - 1
                      : activeIndex - 1,
                )
            } else if (event.key === 'Enter' && open && activeIndex >= 0) {
              event.preventDefault()
              choose(choices[activeIndex])
            } else if (event.key === 'Escape' && open) {
              event.preventDefault()
              event.stopPropagation()
              setOpen(false)
            }
          }}
        />
        <ChevronDown
          aria-hidden="true"
          className="pointer-events-none absolute right-3 top-2.5 size-4 text-muted-foreground"
        />
        {open && !disabled && (
          <div className="absolute z-50 mt-1 max-h-52 w-full overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md">
            <ul
              id={listID}
              role="listbox"
              aria-label={t('catalogAvailableModels')}
            >
              {choices.map((model, index) => (
                <li key={model} role="presentation">
                  <button
                    type="button"
                    id={`${listID}-${index}`}
                    role="option"
                    aria-selected={value === model}
                    tabIndex={-1}
                    onPointerDown={(event) => event.preventDefault()}
                    onClick={() => choose(model)}
                    className={cn(
                      'flex w-full items-center justify-between gap-3 rounded-sm px-2 py-2 text-left text-sm break-all hover:bg-accent',
                      activeIndex === index && 'bg-accent',
                    )}
                  >
                    <span>{model}</span>
                    {value === model && (
                      <Check aria-hidden="true" className="size-4 shrink-0" />
                    )}
                  </button>
                </li>
              ))}
            </ul>
            {!choices.length && (
              <p className="px-2 py-3 text-sm text-muted-foreground">
                {t('catalogNoMatch')}
              </p>
            )}
          </div>
        )}
      </div>
      <p id={`${id}-hint`} className="text-xs leading-5 text-muted-foreground">
        {t('catalogPickerHint')}
      </p>
    </div>
  )
}
