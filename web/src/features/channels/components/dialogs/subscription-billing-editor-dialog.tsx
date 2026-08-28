/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  CalendarClock,
  Check,
  Code2,
  ListPlus,
  ListTree,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import {
  type ReactNode,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { JsonCodeEditor } from '@/components/json-code-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ComboboxInput, type ComboboxInputOption } from '@/components/ui/combobox-input'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { cn } from '@/lib/utils'

import {
  getSubscriptionBilling,
  getSubscriptionUsage,
  updateSubscriptionBilling,
} from '../../api'
import type {
  SubscriptionBillingConfig,
  SubscriptionBillingMode,
  SubscriptionBillingModelTier,
  SubscriptionUsageResponse,
} from '../../types'
import {
  SUBSCRIPTION_BILLING_MODE_OPTIONS,
  SUBSCRIPTION_MONTHLY_RATIO_PERCENT,
  createSubscriptionBillingConfig,
  deriveFiveHourUsd,
  deriveWeeklyUsd,
  formatSubscriptionUsageUpdatedAt,
  normalizeSubscriptionBillingConfig,
  parseSubscriptionBillingConfig,
  stringifySubscriptionBillingConfig,
  validateSubscriptionBillingConfig,
} from '../../lib/subscription-billing'
import { SubscriptionWindowCards } from './subscription-usage-dialog'

type SubscriptionBillingEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId?: number
  channelName?: string
  /** 当前渠道配置的模型（逗号分隔），用于模型额度下拉与一键添加。 */
  channelModels?: string
  onSaved?: () => void
}

type SubscriptionBillingEditorTab =
  | 'overview'
  | 'model_quotas'
  | 'preview'
  | 'json'

const subscriptionBillingTabs = new Set<SubscriptionBillingEditorTab>([
  'overview',
  'model_quotas',
  'preview',
  'json',
])

/**
 * Decimal-capable controlled number input. Keeps a local text draft while
 * typing, commits the parsed value, and only syncs from the external numeric
 * value when the input is not being edited (e.g. after the JSON tab applies
 * changes).
 */
function NumberField({
  value,
  onChange,
  min,
  max,
  step,
  placeholder,
  className,
}: {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  step?: number
  placeholder?: string
  className?: string
}) {
  const [text, setText] = useState(String(value))
  const editingRef = useRef(false)

  useEffect(() => {
    if (editingRef.current) {
      return
    }
    const parsed = Number.parseFloat(text)
    if (!(Number.isFinite(parsed) && parsed === value)) {
      setText(String(value))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])

  const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const raw = event.target.value
    editingRef.current = true
    setText(raw)
    const parsed = Number.parseFloat(raw)
    if (Number.isFinite(parsed)) {
      onChange(parsed)
    }
  }

  const handleBlur = () => {
    editingRef.current = false
    const parsed = Number.parseFloat(text)
    let next = Number.isFinite(parsed) ? parsed : value
    if (min !== undefined) next = Math.max(min, next)
    if (max !== undefined) next = Math.min(max, next)
    setText(String(next))
    onChange(next)
  }

  return (
    <Input
      type='text'
      inputMode='decimal'
      value={text}
      step={step}
      onChange={handleChange}
      onBlur={handleBlur}
      placeholder={placeholder}
      className={className}
    />
  )
}

function FieldBlock({
  label,
  className,
  children,
}: {
  label: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-2', className)}>
      <span className='text-sm font-medium'>{label}</span>
      {children}
    </div>
  )
}

function SectionHeading(props: {
  title: string
  description?: string
  children?: ReactNode
}) {
  return (
    <div className='flex flex-wrap items-start justify-between gap-3'>
      <div className='min-w-0'>
        <div className='text-sm font-semibold'>{props.title}</div>
        {props.description ? (
          <div className='text-muted-foreground mt-1 text-xs leading-5'>
            {props.description}
          </div>
        ) : null}
      </div>
      {props.children ? (
        <div className='flex shrink-0 flex-wrap items-center gap-2'>
          {props.children}
        </div>
      ) : null}
    </div>
  )
}

export function SubscriptionBillingEditorDialog({
  open,
  onOpenChange,
  channelId,
  channelName,
  channelModels,
  onSaved,
}: SubscriptionBillingEditorDialogProps) {
  const { t } = useTranslation()
  const [config, setConfig] = useState<SubscriptionBillingConfig | null>(null)
  const [tierKeys, setTierKeys] = useState<string[]>([])
  const tierKeyCounterRef = useRef(0)
  const [isLoading, setIsLoading] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [activeTab, setActiveTab] =
    useState<SubscriptionBillingEditorTab>('overview')
  const [jsonText, setJsonText] = useState('')
  const [jsonError, setJsonError] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [usage, setUsage] = useState<SubscriptionUsageResponse | null>(null)
  const [isUsageLoading, setIsUsageLoading] = useState(false)
  const [usageError, setUsageError] = useState('')

  // 当前渠道配置的模型（逗号分隔），作为模型额度下拉与一键添加的来源。
  const channelModelNames = useMemo(() => {
    return channelModels
      ? channelModels
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean)
      : []
  }, [channelModels])
  const modelOptions = useMemo<ComboboxInputOption[]>(
    () => channelModelNames.map((name) => ({ value: name, label: name })),
    [channelModelNames]
  )

  const createTierKey = () => {
    tierKeyCounterRef.current += 1
    return `subscription-tier-${tierKeyCounterRef.current}`
  }

  const createTierKeys = (count: number) =>
    Array.from({ length: count }, () => createTierKey())

  const applyConfig = (nextConfig: SubscriptionBillingConfig) => {
    setConfig(nextConfig)
    setTierKeys(createTierKeys(nextConfig.model_tiers.length))
  }

  // Load the backend configuration every time the dialog opens.
  useEffect(() => {
    if (!open) {
      return
    }
    setActiveTab('overview')
    setJsonError('')
    setSaveError('')
    setUsage(null)
    setUsageError('')
    setIsUsageLoading(false)
    if (!channelId) {
      setLoadError(t('Channel ID is required'))
      applyConfig(createSubscriptionBillingConfig())
      setIsLoading(false)
      return
    }
    setConfig(null)
    setTierKeys([])
    setIsLoading(true)
    setLoadError('')
    let cancelled = false
    const load = async () => {
      try {
        const res = await getSubscriptionBilling(channelId)
        if (cancelled) return
        if (!res.success || !res.data) {
          setLoadError(
            res.message?.trim() ||
              t('Failed to load subscription billing configuration')
          )
          applyConfig(createSubscriptionBillingConfig())
          return
        }
        const normalized = normalizeSubscriptionBillingConfig(res.data)
        applyConfig(normalized)
        setJsonText(stringifySubscriptionBillingConfig(normalized))
      } catch (error) {
        if (cancelled) return
        setLoadError(
          error instanceof Error
            ? error.message
            : t('Failed to load subscription billing configuration')
        )
        applyConfig(createSubscriptionBillingConfig())
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, channelId])

  // Fetch usage when the preview tab becomes active for the first time.
  useEffect(() => {
    if (
      !open ||
      !channelId ||
      activeTab !== 'preview' ||
      usage ||
      isUsageLoading ||
      usageError
    ) {
      return
    }
    let cancelled = false
    const load = async () => {
      setIsUsageLoading(true)
      setUsageError('')
      try {
        const res = await getSubscriptionUsage(channelId)
        if (cancelled) return
        if (!res.success) {
          setUsageError(res.message?.trim() || t('Failed to fetch usage'))
          return
        }
        setUsage(res)
      } catch (error) {
        if (cancelled) return
        setUsageError(
          error instanceof Error ? error.message : t('Failed to fetch usage')
        )
      } finally {
        if (!cancelled) setIsUsageLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [open, channelId, activeTab, usage, isUsageLoading, usageError, t])

  const updateConfig = (patch: Partial<SubscriptionBillingConfig>) => {
    setConfig((current) => (current ? { ...current, ...patch } : current))
  }

  const updateTier = (
    index: number,
    patch: Partial<SubscriptionBillingModelTier>
  ) => {
    setConfig((current) => {
      if (!current) return current
      const nextTiers = current.model_tiers.map((tier, tierIndex) =>
        tierIndex === index ? { ...tier, ...patch } : tier
      )
      return { ...current, model_tiers: nextTiers }
    })
  }

  const addTier = () => {
    setConfig((current) =>
      current
        ? {
            ...current,
            model_tiers: [
              ...current.model_tiers,
              { model: '', monthly_usd: 0 },
            ],
          }
        : current
    )
    setTierKeys((current) => [...current, createTierKey()])
  }

  const removeTier = (index: number) => {
    setConfig((current) =>
      current
        ? {
            ...current,
            model_tiers: current.model_tiers.filter(
              (_, tierIndex) => tierIndex !== index
            ),
          }
        : current
    )
    setTierKeys((current) => current.filter((_, tierIndex) => tierIndex !== index))
  }

  // 一键添加：把当前渠道配置的全部模型批量加入额度档（跳过已存在的）。
  const addAllChannelModels = () => {
    if (!config || channelModelNames.length === 0) {
      return
    }
    const existing = new Set(
      config.model_tiers.map((tier) => tier.model.trim())
    )
    const missing = channelModelNames.filter(
      (name) => name !== '' && !existing.has(name)
    )
    if (missing.length === 0) {
      return
    }
    setConfig((current) =>
      current
        ? {
            ...current,
            model_tiers: [
              ...current.model_tiers,
              ...missing.map((model) => ({ model, monthly_usd: 0 })),
            ],
          }
        : current
    )
    setTierKeys((current) => [
      ...current,
      ...missing.map(() => createTierKey()),
    ])
  }

  const parseJsonEditorConfig = (): SubscriptionBillingConfig | null => {
    const parsed = parseSubscriptionBillingConfig(jsonText)
    if (!parsed) {
      setJsonError(t('Invalid JSON'))
      return null
    }
    const error = validateSubscriptionBillingConfig(parsed)
    if (error) {
      setJsonError(t(error))
      return null
    }
    setJsonError('')
    return parsed
  }

  const switchTab = (nextTab: SubscriptionBillingEditorTab) => {
    if (!subscriptionBillingTabs.has(nextTab) || !config) {
      return
    }
    if (activeTab !== 'json') {
      if (nextTab === 'json') {
        setJsonText(stringifySubscriptionBillingConfig(config))
        setJsonError('')
      }
      setActiveTab(nextTab)
      return
    }

    const parsed = parseJsonEditorConfig()
    if (!parsed) return
    applyConfig(parsed)
    setActiveTab(nextTab)
  }

  const loadUsage = (force: boolean) => {
    if (!channelId || isUsageLoading) {
      return
    }
    if (!force && usage) {
      return
    }
    const load = async () => {
      setIsUsageLoading(true)
      setUsageError('')
      try {
        const res = await getSubscriptionUsage(channelId)
        if (!res.success) {
          setUsageError(res.message?.trim() || t('Failed to fetch usage'))
          return
        }
        setUsage(res)
      } catch (error) {
        setUsageError(
          error instanceof Error ? error.message : t('Failed to fetch usage')
        )
      } finally {
        setIsUsageLoading(false)
      }
    }
    void load()
  }

  const handleJsonChange = (nextValue: string) => {
    setJsonText(nextValue)
    if (jsonError) setJsonError('')
  }

  const saveConfig = async () => {
    if (!channelId || !config || isSaving) {
      return
    }
    let payload = config
    if (activeTab === 'json') {
      const parsed = parseJsonEditorConfig()
      if (!parsed) {
        toast.error(t('Please fix JSON errors before saving'))
        return
      }
      payload = parsed
    } else {
      const error = validateSubscriptionBillingConfig(config)
      if (error) {
        toast.error(t(error))
        return
      }
    }

    setIsSaving(true)
    setSaveError('')
    try {
      const res = await updateSubscriptionBilling(channelId, payload)
      if (!res.success) {
        throw new Error(
          res.message || t('Failed to save subscription billing configuration')
        )
      }
      toast.success(t('Saved'))
      onOpenChange(false)
      onSaved?.()
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to save subscription billing configuration')
      setSaveError(message)
      toast.error(message)
    } finally {
      setIsSaving(false)
    }
  }

  const monthlyTotalUsd = config?.monthly_total_usd ?? 0
  const fiveHourRatioPercent = config?.five_hour_ratio_percent ?? 0
  const weeklyRatioPercent = config?.weekly_ratio_percent ?? 0
  const tiers = config?.model_tiers ?? []

  const derivedLimitFields = useMemo(
    () => [
      {
        label: t('5-hour limit:'),
        value: config
          ? formatBillingCurrencyFromUSD(deriveFiveHourUsd(config))
          : '-',
      },
      {
        label: t('Weekly limit:'),
        value: config
          ? formatBillingCurrencyFromUSD(deriveWeeklyUsd(config))
          : '-',
      },
      {
        label: t('Monthly limit:'),
        value: config
          ? formatBillingCurrencyFromUSD(config.monthly_total_usd)
          : '-',
      },
    ],
    [config, t]
  )

  let body: ReactNode
  if (isLoading) {
    body = (
      <div className='flex flex-col gap-4 p-4'>
        <Skeleton className='h-10 w-full' />
        <Skeleton className='h-10 w-full' />
        <Skeleton className='h-40 w-full' />
      </div>
    )
  } else if (!config) {
    body = (
      <div className='flex flex-col gap-4 p-4'>
        <Alert variant='destructive'>
          <AlertDescription>{loadError || t('Loading...')}</AlertDescription>
        </Alert>
      </div>
    )
  } else {
    body = (
      <Tabs
        value={activeTab}
        onValueChange={(value) =>
          switchTab(value as SubscriptionBillingEditorTab)
        }
        className='min-w-0 gap-0'
      >
        <div className='border-b px-4 py-3'>
          <TabsList className='grid h-auto w-full grid-cols-2 gap-1 sm:grid-cols-4'>
            <TabsTrigger value='overview'>
              <CalendarClock className='size-4' aria-hidden='true' />
              {t('Overview')}
            </TabsTrigger>
            <TabsTrigger value='model_quotas'>
              <ListTree className='size-4' aria-hidden='true' />
              {t('Model Quotas')}
              <span className='text-muted-foreground text-xs'>
                {tiers.length}
              </span>
            </TabsTrigger>
            <TabsTrigger value='preview'>
              <Check className='size-4' aria-hidden='true' />
              {t('Preview')}
            </TabsTrigger>
            <TabsTrigger value='json'>
              <Code2 className='size-4' aria-hidden='true' />
              {t('Full JSON')}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value='overview' className='flex flex-col gap-4 p-4'>
          <div className='grid gap-4 md:grid-cols-2'>
            <FieldBlock label={t('Billing Mode')}>
              <Select
                items={SUBSCRIPTION_BILLING_MODE_OPTIONS}
                value={config.billing_mode}
                onValueChange={(value) =>
                  updateConfig({
                    billing_mode: value as SubscriptionBillingMode,
                  })
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {SUBSCRIPTION_BILLING_MODE_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {t(option.label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </FieldBlock>

            <FieldBlock label={t('Monthly Total (USD)')}>
              <NumberField
                value={monthlyTotalUsd}
                onChange={(value) => updateConfig({ monthly_total_usd: value })}
                min={0}
                step={1}
              />
            </FieldBlock>

            <FieldBlock label={t('Five-Hour Ratio (%)')}>
              <NumberField
                value={fiveHourRatioPercent}
                onChange={(value) =>
                  updateConfig({ five_hour_ratio_percent: value })
                }
                min={0}
                max={100}
                step={1}
              />
            </FieldBlock>

            <FieldBlock label={t('Weekly Ratio (%)')}>
              <NumberField
                value={weeklyRatioPercent}
                onChange={(value) =>
                  updateConfig({ weekly_ratio_percent: value })
                }
                min={0}
                max={100}
                step={1}
              />
            </FieldBlock>

            <FieldBlock label={t('Monthly Ratio (%)')}>
              <Input
                value={String(SUBSCRIPTION_MONTHLY_RATIO_PERCENT)}
                readOnly
                disabled
              />
            </FieldBlock>
          </div>

          <div className='bg-muted/30 rounded-lg border p-3'>
            <div className='text-sm font-semibold'>{t('Derived Limits')}</div>
            <div className='text-muted-foreground mt-1 text-xs leading-5'>
              {t(
                'Derived limits are computed from the monthly total and the ratios above. The monthly window always uses 100%.'
              )}
            </div>
            <div className='mt-3 grid grid-cols-1 gap-2 text-sm sm:grid-cols-3'>
              {derivedLimitFields.map((field) => (
                <div key={field.label} className='min-w-0'>
                  <div className='text-muted-foreground text-[11px]'>
                    {field.label}
                  </div>
                  <div className='mt-0.5 font-medium tabular-nums'>
                    {field.value}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </TabsContent>

        <TabsContent value='model_quotas' className='flex flex-col gap-4 p-4'>
          <SectionHeading
            title={t('Model Quotas')}
            description={t(
              'Set a monthly quota per model. Use "*" as the model name to apply a fallback to all other models.'
            )}
          >
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={addAllChannelModels}
              disabled={channelModelNames.length === 0}
            >
              <ListPlus data-icon='inline-start' />
              {t('Add all channel models')}
            </Button>
            <Button type='button' variant='outline' size='sm' onClick={addTier}>
              <Plus data-icon='inline-start' />
              {t('Add model quota')}
            </Button>
          </SectionHeading>

          {tiers.length === 0 ? (
            <Empty className='min-h-32 border'>
              <EmptyHeader>
                <EmptyTitle>{t('No model quotas')}</EmptyTitle>
                <EmptyDescription>
                  {t(
                    'No per-model quotas are configured. All models share the monthly total limit.'
                  )}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className='flex flex-col gap-2'>
              {tiers.map((tier, index) => (
                <div
                  key={tierKeys[index] || `subscription-tier-${index}`}
                  className='grid grid-cols-1 gap-3 rounded-lg border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end'
                >
                  <FieldBlock label={t('Model')}>
                    <ComboboxInput
                      options={modelOptions}
                      value={tier.model}
                      onValueChange={(value) =>
                        updateTier(index, { model: value })
                      }
                      placeholder='*'
                      allowCustomValue
                      emptyText={t('No matching model')}
                    />
                  </FieldBlock>
                  <FieldBlock label={t('Monthly Quota (USD)')}>
                    <NumberField
                      value={tier.monthly_usd}
                      onChange={(value) =>
                        updateTier(index, { monthly_usd: value })
                      }
                      min={0}
                      step={1}
                    />
                  </FieldBlock>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={() => removeTier(index)}
                  >
                    <Trash2 data-icon='inline-start' />
                    {t('Delete')}
                  </Button>
                </div>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value='preview' className='flex flex-col gap-4 p-4'>
          <SectionHeading
            title={t('Usage Preview')}
            description={t(
              'Read-only preview of the current usage windows for this channel.'
            )}
          >
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => loadUsage(true)}
              disabled={isUsageLoading}
            >
              <RefreshCw data-icon='inline-start' />
              {t('Refresh now')}
            </Button>
          </SectionHeading>

          {usageError ? (
            <div className='border-destructive/40 bg-destructive/10 text-destructive rounded-lg border px-3 py-2 text-sm'>
              {usageError}
            </div>
          ) : null}

          {isUsageLoading ? (
            <div className='grid grid-cols-1 gap-3 md:grid-cols-3'>
              <Skeleton className='h-36 w-full' />
              <Skeleton className='h-36 w-full' />
              <Skeleton className='h-36 w-full' />
            </div>
          ) : (
            <SubscriptionWindowCards windows={usage?.data?.windows} />
          )}

          <div className='text-muted-foreground text-xs'>
            {t('Last updated:')}{' '}
            {formatSubscriptionUsageUpdatedAt(usage?.data?.updated_at)}
          </div>
        </TabsContent>

        <TabsContent value='json' className='flex flex-col gap-3 p-4'>
          <JsonCodeEditor
            value={jsonText}
            onChange={handleJsonChange}
            placeholder={stringifySubscriptionBillingConfig(
              createSubscriptionBillingConfig()
            )}
            heightClassName='h-[420px] min-h-[420px] max-h-[420px]'
            aria-invalid={Boolean(jsonError)}
            ariaLabel={t('Subscription billing configuration JSON')}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Edit JSON text directly. Format will be validated on save.')}
          </p>
          {jsonError ? (
            <p className='text-destructive text-xs'>{jsonError}</p>
          ) : null}
        </TabsContent>
      </Tabs>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Subscription Billing Configuration')}
      description={channelName?.trim() || undefined}
      contentClassName='flex max-h-[90vh] flex-col gap-0 p-0 sm:max-w-3xl'
      headerClassName='border-b px-6 py-4'
      footerClassName='border-t px-6 py-4'
      contentHeight='70vh'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSaving}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={() => void saveConfig()}
            disabled={isLoading || isSaving || !config}
          >
            {isSaving ? (
              <span className='mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent' />
            ) : (
              <Check data-icon='inline-start' />
            )}
            {t('Save changes')}
          </Button>
        </>
      }
    >
      {body}

      {saveError ? (
        <div className='border-t px-6 py-3'>
          <Alert variant='destructive'>
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        </div>
      ) : null}
    </Dialog>
  )
}
