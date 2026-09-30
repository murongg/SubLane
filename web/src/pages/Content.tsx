import { useId, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDown } from 'lucide-react'
import { authOptions } from '@/lib/auth'
import {
  contentOptions,
  contentModes,
  contentKinds,
  contentModeKeys,
  contentKindKeys,
  saveContent,
  testContent,
  contentError,
  type ContentState,
  type ContentRule,
} from '@/lib/content'
import { useAdminMutation } from '@/hooks/use-admin-mutation'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Status } from '@/components/Status'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/DropdownMenu'

export function Content() {
  const { data } = useQuery(authOptions())
  return data?.user?.role === 'admin' ? (
    <ContentSettings key={data.user.id} userID={data.user.id} />
  ) : null
}

function ContentSettings({ userID }: { userID: number }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(contentOptions(userID))
  const [editing, setEditing] = useState(false)
  const [saved, setSaved] = useState(false)
  const value = query.data
  return (
    <div className="max-w-3xl">
      <h1 className="page-title">{t('contentTitle')}</h1>
      <p className="page-description">{t('contentDescription')}</p>
      <div className="mt-8 border-y border-border py-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-medium">{t('contentWorkspacePolicy')}</h2>
          <div className="flex flex-wrap items-center gap-3">
            {value && !query.isError && (
              <Status kind={value.mode === 'block' ? 'warning' : 'neutral'}>
                {t(contentModeKeys[value.mode])}
              </Status>
            )}
            <Button
              type="button"
              variant="ghost"
              disabled={query.isFetching}
              onClick={() => {
                setEditing(false)
                setSaved(false)
                return query.refetch()
              }}
            >
              {t('refresh')}
            </Button>
          </div>
        </div>
        {query.isPending ? (
          <p className="mt-4 text-sm text-muted-foreground">{t('loading')}</p>
        ) : query.isError ? (
          <div className="mt-4 space-y-3">
            <p role="alert" className="text-sm text-error">
              {t('contentUnavailable')}
            </p>
          </div>
        ) : (
          value && (
            <>
              {editing ? (
                <ContentEditor
                  key={value.revision}
                  value={value}
                  userID={userID}
                  onCancel={() => setEditing(false)}
                  onSaved={async (result) => {
                    client.setQueryData(contentOptions(userID).queryKey, result)
                    setEditing(false)
                    setSaved(true)
                    await client.invalidateQueries({ queryKey: ['audit'] })
                  }}
                />
              ) : (
                <>
                  <p className="mt-3 text-sm leading-6 text-muted-foreground">
                    {t(
                      value.mode === 'observe'
                        ? 'contentObserveHint'
                        : value.mode === 'block'
                          ? 'contentBlockHint'
                          : 'contentOffHint',
                    )}
                  </p>
                  <div className="mt-5 divide-y divide-border">
                    {value.rules.length ? (
                      value.rules.map((rule) => (
                        <div
                          key={rule.id}
                          className="flex flex-wrap items-center justify-between gap-3 py-3"
                        >
                          <div className="min-w-0">
                            <p className="break-words text-sm font-medium">
                              {rule.name}
                            </p>
                            <p className="mt-1 break-all text-xs text-muted-foreground">
                              {rule.id} · {t(contentKindKeys[rule.kind])}
                            </p>
                          </div>
                          <Status kind="neutral">
                            {t(
                              rule.enabled
                                ? 'contentRuleEnabled'
                                : 'contentRuleDisabled',
                            )}
                          </Status>
                        </div>
                      ))
                    ) : (
                      <p className="py-3 text-sm text-muted-foreground">
                        {t('contentEmpty')}
                      </p>
                    )}
                  </div>
                  <Button
                    className="mt-5"
                    variant="outline"
                    onClick={() => {
                      setEditing(true)
                      setSaved(false)
                    }}
                  >
                    {t('contentConfigure')}
                  </Button>
                </>
              )}
            </>
          )
        )}
        {saved && (
          <p role="status" className="mt-4 text-sm text-success">
            {t('contentSaved')}
          </p>
        )}
      </div>
      <p className="mt-4 text-sm leading-6 text-muted-foreground">
        {t('contentScope')}
      </p>
    </div>
  )
}

function Choice({
  value,
  options,
  onChange,
  label,
  disabled,
}: {
  value: string
  options: readonly { value: string; label: string }[]
  onChange: (value: string) => void
  label: string
  disabled?: boolean
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={label}
        >
          {options.find((option) => option.value === value)?.label}
          <ChevronDown className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuRadioGroup value={value} onValueChange={onChange}>
          {options.map((option) => (
            <DropdownMenuRadioItem key={option.value} value={option.value}>
              {option.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

type Draft = ContentRule & { draftID: string; pattern: string }

function ruleInput(rule: Draft): ContentRule {
  // An omitted pattern retains the encrypted value; placeholders must never replace it.
  return {
    id: rule.id,
    name: rule.name,
    kind: rule.kind,
    enabled: rule.enabled,
    ...(rule.pattern ? { pattern: rule.pattern } : {}),
  }
}
function ContentEditor({
  value,
  userID,
  onCancel,
  onSaved,
}: {
  value: ContentState
  userID: number
  onCancel: () => void
  onSaved: (value: ContentState) => unknown
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState(value.mode)
  const [rules, setRules] = useState<Draft[]>(() =>
    value.rules.map((rule) => ({ ...rule, draftID: rule.id, pattern: '' })),
  )
  const save = useAdminMutation({
    userID,
    mutationFn: saveContent,
    onSuccess: onSaved,
  })
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (save.isPending) return
    save.mutate({
      mode,
      revision: value.revision,
      rules: rules.map(ruleInput),
    })
  }
  return (
    <form className="mt-5 space-y-5" onSubmit={submit}>
      <fieldset disabled={save.isPending} className="space-y-5">
        <div className="space-y-2">
          <p className="text-sm font-medium">{t('contentMode')}</p>
          <Choice
            value={mode}
            label={t(contentModeKeys[mode])}
            options={contentModes.map((value) => ({
              value,
              label: t(contentModeKeys[value]),
            }))}
            onChange={(value) => setMode(value as ContentState['mode'])}
          />
          <p className="text-sm leading-6 text-muted-foreground">
            {t(
              mode === 'observe'
                ? 'contentObserveHint'
                : mode === 'block'
                  ? 'contentBlockHint'
                  : 'contentOffHint',
            )}
          </p>
        </div>
        <div className="divide-y divide-border border-y border-border">
          {rules.map((rule) => (
            <RuleEditor
              key={rule.draftID}
              userID={userID}
              rule={rule}
              onChange={(next) =>
                setRules((current) =>
                  current.map((item) =>
                    item.draftID === rule.draftID ? next : item,
                  ),
                )
              }
              onRemove={() =>
                setRules((current) =>
                  current.filter((item) => item.draftID !== rule.draftID),
                )
              }
            />
          ))}
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={rules.length >= 50}
          onClick={() =>
            setRules((current) => [
              ...current,
              {
                draftID: crypto.randomUUID(),
                id: '',
                name: '',
                kind: 'text',
                enabled: true,
                pattern: '',
              },
            ])
          }
        >
          {t('contentAdd')}
        </Button>
        <p className="text-xs leading-5 text-muted-foreground">
          {t('contentLimits')}
        </p>
      </fieldset>
      {save.isError && (
        <p role="alert" className="text-sm text-error">
          {t(contentError(save.error))}
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        <Button type="submit" disabled={save.isPending}>
          {t(save.isPending ? 'alertsSaving' : 'saveChanges')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={save.isPending}
          onClick={onCancel}
        >
          {t('cancel')}
        </Button>
      </div>
    </form>
  )
}

function RuleEditor({
  userID,
  rule,
  onChange,
  onRemove,
}: {
  userID: number
  rule: Draft
  onChange: (rule: Draft) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [sample, setSample] = useState('')
  const [matched, setMatched] = useState<boolean | null>(null)
  const [testError, setTestError] = useState<Error | null>(null)
  const test = useAdminMutation({
    userID,
    mutationFn: testContent,
    onSuccess: (result) => {
      setSample('')
      setMatched(result.matched)
      test.reset()
    },
    onError: (error) => {
      setSample('')
      setTestError(error)
      test.reset()
    },
  })
  const change = (next: Draft) => {
    setMatched(null)
    setTestError(null)
    test.reset()
    onChange(next)
  }
  const testRule = () => {
    setMatched(null)
    setTestError(null)
    test.mutate({ rule: ruleInput(rule), sample })
  }
  return (
    <fieldset
      className="min-w-0 space-y-4 py-5"
      disabled={test.isPending}
      aria-label={rule.name || t('contentNewRule')}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label htmlFor={`${id}-name`} className="text-sm font-medium">
            {t('contentRuleName')}
          </label>
          <Input
            id={`${id}-name`}
            value={rule.name}
            maxLength={64}
            required
            onChange={(event) => change({ ...rule, name: event.target.value })}
          />
        </div>
        <div className="space-y-2">
          <p className="text-sm font-medium">{t('contentKind')}</p>
          <Choice
            value={rule.kind}
            label={t('contentKind')}
            options={contentKinds.map((value) => ({
              value,
              label: t(contentKindKeys[value]),
            }))}
            onChange={(value) =>
              change({ ...rule, kind: value as Draft['kind'] })
            }
          />
        </div>
      </div>
      <div className="space-y-2">
        <label htmlFor={`${id}-pattern`} className="text-sm font-medium">
          {t('contentPattern')}
        </label>
        <Input
          id={`${id}-pattern`}
          type="password"
          value={rule.pattern}
          maxLength={1024}
          required={!rule.id}
          autoComplete="off"
          spellCheck={false}
          placeholder={rule.id ? t('contentKeepPattern') : undefined}
          aria-describedby={`${id}-hint`}
          onChange={(event) => change({ ...rule, pattern: event.target.value })}
        />
        <p
          id={`${id}-hint`}
          className="text-xs leading-5 text-muted-foreground"
        >
          {t(rule.kind === 'text' ? 'contentTextHint' : 'contentRegexHint')}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={rule.enabled}
            onChange={(event) =>
              change({ ...rule, enabled: event.target.checked })
            }
            className="size-4 accent-primary"
          />
          {t('contentEnableRule')}
        </label>
        <Button type="button" variant="ghost" onClick={onRemove}>
          {t('contentRemove')}
        </Button>
      </div>
      <details className="text-sm">
        <summary className="min-h-11 cursor-pointer rounded-sm py-2 font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring">
          {t('contentTestTitle')}
        </summary>
        <div className="mt-3 space-y-3">
          <label htmlFor={`${id}-sample`} className="block text-sm font-medium">
            {t('contentSample')}
          </label>
          <textarea
            id={`${id}-sample`}
            value={sample}
            maxLength={65536}
            autoComplete="off"
            spellCheck={false}
            className="min-h-24 w-full rounded-md border border-input bg-transparent p-3 text-base outline-none focus-visible:ring-2 focus-visible:ring-ring sm:text-sm"
            onChange={(event) => {
              setSample(event.target.value)
              setMatched(null)
              setTestError(null)
              test.reset()
            }}
          />
          <p className="text-xs leading-5 text-muted-foreground">
            {t('contentSampleHint')}
          </p>
          <Button
            type="button"
            variant="outline"
            disabled={test.isPending || !sample || (!rule.id && !rule.pattern)}
            onClick={testRule}
          >
            {t('contentTest')}
          </Button>
          {matched !== null && (
            <p
              role="status"
              className={matched ? 'text-warning' : 'text-muted-foreground'}
            >
              {t(matched ? 'contentMatched' : 'contentNotMatched')}
            </p>
          )}
          {testError && (
            <p role="alert" className="text-error">
              {t(contentError(testError))}
            </p>
          )}
        </div>
      </details>
    </fieldset>
  )
}
