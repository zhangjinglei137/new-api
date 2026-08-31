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
/* eslint-disable react-refresh/only-export-components */
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { MultiSelect } from '@/components/multi-select'
import { Combobox } from '@/components/ui/combobox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import {
  CAPABILITY_GROUP_NAME_PRESETS,
  CAPABILITY_MODALITY_PRESETS,
  REASONING_OPTION_PRESETS,
} from '../constants'

type CapabilityLimits = {
  context?: number
  output?: number
}

export type CapabilityGroup = {
  name: string
  input?: string[]
  output?: string[]
  reasoning_options?: string[]
  limits?: CapabilityLimits
}

/**
 * Legacy single-group shape: `{"modalities": {...}, "limits": {...}, ...}`.
 * Kept for reading capabilities written before the groups format existed.
 */
type LegacyCapabilitiesShape = {
  modalities?: {
    input?: string[]
    output?: string[]
  }
  reasoning_options?: string[]
  limits?: CapabilityLimits
}

function isLegacyCapabilitiesShape(value: unknown): value is LegacyCapabilitiesShape {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  return (
    !Array.isArray(record.groups) &&
    (record.modalities !== undefined ||
      record.limits !== undefined ||
      record.reasoning_options !== undefined)
  )
}

// ============================================================================
// Capabilities JSON <-> editor groups
// ============================================================================

/**
 * Coerce an unknown value into a string array, dropping any non-string
 * entries. Empty or all-invalid arrays become `undefined` so the key is
 * omitted from the generated JSON.
 */
function toStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const strings = value.filter(
    (item): item is string => typeof item === 'string' && item.trim() !== ''
  )
  return strings.length > 0 ? strings : undefined
}

/**
 * Coerce stored `reasoning_options` (array of objects and/or strings) into
 * the editor's string values. Upstream catalogs (e.g. opencode-go) store
 * object entries like `[{"type":"effort","values":["high","max"]}]`; their
 * `type` is kept so the option stays visible and editable. Rendering objects
 * directly would crash chip rendering (React error #31).
 */
function reasoningOptionsToStrings(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) return undefined
  const strings: string[] = []
  for (const item of value) {
    if (typeof item === 'string' && item.trim() !== '') {
      strings.push(item)
    } else if (item && typeof item === 'object') {
      const type = (item as Record<string, unknown>).type
      if (typeof type === 'string' && type.trim() !== '') {
        strings.push(type)
      }
    }
  }
  return strings.length > 0 ? strings : undefined
}

/**
 * Serialize the editor's string reasoning options into the persisted object
 * array shape `[{"type":"low"}]` required by the backend's rich model
 * metadata output (`reasoning_options: []map[string]any`).
 */
function reasoningOptionsToObjects(
  values: string[] | undefined
): Array<{ type: string }> | undefined {
  if (!values || values.length === 0) return undefined
  const types = values
    .map((value) => value.trim())
    .filter((value) => value !== '')
  return types.length > 0 ? types.map((type) => ({ type })) : undefined
}

/**
 * Normalize a parsed group into the editor shape: string-array modalities and
 * reasoning options, non-negative integer limits. Invalid entries are dropped.
 * Accepts both the nested shape (`modalities: { input, output }`, backend
 * contract) and the flat shape (`input`/`output` keys) written by earlier
 * editor versions.
 */
function normalizeCapabilityGroup(value: unknown): CapabilityGroup {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { name: '' }
  }
  const record = value as Record<string, unknown>
  const modalities =
    record.modalities && typeof record.modalities === 'object'
      ? (record.modalities as Record<string, unknown>)
      : undefined
  const normalized: CapabilityGroup = {
    name: typeof record.name === 'string' ? record.name : '',
  }
  const input = toStringArray(record.input ?? modalities?.input)
  const output = toStringArray(record.output ?? modalities?.output)
  const reasoningOptions = reasoningOptionsToStrings(record.reasoning_options)
  if (input) normalized.input = input
  if (output) normalized.output = output
  if (reasoningOptions) normalized.reasoning_options = reasoningOptions
  const limits = record.limits
  if (limits && typeof limits === 'object') {
    const rawLimits = limits as Record<string, unknown>
    const normalizedLimits: CapabilityLimits = {}
    for (const key of ['context', 'output'] as const) {
      const raw = rawLimits[key]
      if (
        typeof raw === 'number' &&
        Number.isInteger(raw) &&
        raw >= 0
      ) {
        normalizedLimits[key] = raw
      }
    }
    if (Object.keys(normalizedLimits).length > 0) {
      normalized.limits = normalizedLimits
    }
  }
  return normalized
}

/**
 * Parse a stored capabilities JSON string into editor groups.
 * - Empty string → one empty group.
 * - `{"groups":[...]}` (current backend contract) → each group mapped.
 * - Bare-array shape written by earlier editor versions → mapped as groups.
 * - Legacy single-group flat shape → normalized to a "chat" group.
 * - Invalid/unknown JSON → one empty group (kept for user inspection).
 */
export function parseCapabilitiesToGroups(capabilities: string): CapabilityGroup[] {
  if (!capabilities || capabilities.trim() === '') {
    return [{ name: '' }]
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(capabilities)
  } catch {
    return [{ name: '' }]
  }

  // Current backend contract: `{"groups":[...]}`.
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
    const groups = (parsed as Record<string, unknown>).groups
    if (Array.isArray(groups)) {
      const mapped = groups
        .map(normalizeCapabilityGroup)
        .filter((group) => group.name.trim() !== '')
      return mapped.length > 0 ? mapped : [{ name: '' }]
    }
  }

  if (Array.isArray(parsed)) {
    const groups = parsed
      .map(normalizeCapabilityGroup)
      .filter((group) => group.name.trim() !== '')
    return groups.length > 0 ? groups : [{ name: '' }]
  }

  if (isLegacyCapabilitiesShape(parsed)) {
    const group = normalizeCapabilityGroup(parsed)
    group.name = 'chat'
    return [group]
  }

  return [{ name: '' }]
}

/**
 * Serialize editor groups into the capabilities JSON string following the
 * backend contract: `{"groups":[{"name":"chat","modalities":{"input":[...],
 * "output":[...]},"limits":{...},"reasoning_options":[{"type":"low"}]}]}`.
 * - The payload is wrapped under a `groups` key: the backend validator
 *   (`controller/model_meta.go`) only accepts object shapes, so writing a
 *   bare array would be rejected with "capabilities 不是合法的 JSON".
 * - Empty modality arrays are not written.
 * - Empty limits values are not written.
 * - A group that only has a name still produces `{"name": "..."}`.
 * - Arrays are defensively coerced so objects can never leak into the JSON.
 */
export function serializeGroupsToCapabilities(groups: CapabilityGroup[]): string {
  const payload = groups
    .filter((group) => group.name.trim() !== '')
    .map((group) => {
      const entry: Record<string, unknown> = { name: group.name.trim() }
      const input = toStringArray(group.input)
      const output = toStringArray(group.output)
      if (input || output) {
        const modalities: Record<string, string[]> = {}
        if (input) modalities.input = input
        if (output) modalities.output = output
        entry.modalities = modalities
      }
      const reasoningOptions = reasoningOptionsToObjects(group.reasoning_options)
      if (reasoningOptions) entry.reasoning_options = reasoningOptions
      if (group.limits) {
        const limits: Record<string, number> = {}
        if (group.limits.context !== undefined && group.limits.context >= 0) {
          limits.context = group.limits.context
        }
        if (group.limits.output !== undefined && group.limits.output >= 0) {
          limits.output = group.limits.output
        }
        if (Object.keys(limits).length > 0) {
          entry.limits = limits
        }
      }
      return entry
    })
  if (payload.length === 0) return ''
  return JSON.stringify({ groups: payload })
}

/**
 * Normalize a raw limits value into a non-negative integer, or undefined.
 * Inputs that are not valid non-negative integers map to undefined (not
 * written to the JSON).
 */
export function parseLimitValue(raw: string): number | undefined {
  const trimmed = raw.trim()
  if (trimmed === '') return undefined
  if (!/^\d+$/.test(trimmed)) return undefined
  return Number.parseInt(trimmed, 10)
}

type CapabilityGroupCardProps = {
  group: CapabilityGroup
  onGroupChange: (group: CapabilityGroup) => void
  onRemove: () => void
  canRemove: boolean
}

function CapabilityGroupCard({
  group,
  onGroupChange,
  onRemove,
  canRemove,
}: CapabilityGroupCardProps) {
  const { t } = useTranslation()

  const nameOptions = useMemo(
    () =>
      CAPABILITY_GROUP_NAME_PRESETS.map((preset) => ({
        value: preset,
        label: preset,
      })),
    []
  )
  const modalityOptions = useMemo(
    () =>
      CAPABILITY_MODALITY_PRESETS.map((preset) => ({
        value: preset,
        label: preset,
      })),
    []
  )
  const reasoningOptions = useMemo(
    () =>
      REASONING_OPTION_PRESETS.map((preset) => ({
        value: preset,
        label: preset,
      })),
    []
  )

  return (
    <div className='border-border/60 space-y-3 rounded-lg border p-3'>
      <div className='flex items-start justify-between gap-3'>
        <div className='grid flex-1 gap-2 sm:grid-cols-2'>
          <div className='space-y-1.5'>
            <Label>{t('Group name')}</Label>
            <Combobox
              options={nameOptions}
              value={group.name}
              onValueChange={(value) =>
                onGroupChange({ ...group, name: value ?? '' })
              }
              placeholder={t('chat, image, video, ...')}
              allowCustomValue
              emptyText={t('No matching items')}
            />
          </div>

          <div className='space-y-1.5'>
            <Label>{t('Reasoning options')}</Label>
            <MultiSelect
              options={reasoningOptions}
              selected={group.reasoning_options || []}
              onChange={(values) =>
                onGroupChange({ ...group, reasoning_options: values })
              }
              placeholder={t('Select options...')}
              allowCreate
              maxVisibleChips={3}
            />
          </div>
        </div>

        <Button
          type='button'
          variant='ghost'
          size='icon'
          onClick={onRemove}
          disabled={!canRemove}
          className='shrink-0'
          aria-label={t('Remove group')}
        >
          <Trash2 className='h-4 w-4' />
        </Button>
      </div>

      <div className='grid gap-2 sm:grid-cols-2'>
        <div className='space-y-1.5'>
          <Label>{t('Input modalities')}</Label>
          <MultiSelect
            options={modalityOptions}
            selected={group.input || []}
            onChange={(values) => onGroupChange({ ...group, input: values })}
            placeholder={t('Select modalities...')}
            allowCreate
            maxVisibleChips={3}
          />
        </div>
        <div className='space-y-1.5'>
          <Label>{t('Output modalities')}</Label>
          <MultiSelect
            options={modalityOptions}
            selected={group.output || []}
            onChange={(values) => onGroupChange({ ...group, output: values })}
            placeholder={t('Select modalities...')}
            allowCreate
            maxVisibleChips={3}
          />
        </div>
      </div>

      <div className='grid grid-cols-2 gap-2'>
        <div className='space-y-1.5'>
          <Label>{t('Context limit')}</Label>
          <Input
            type='text'
            inputMode='numeric'
            value={group.limits?.context ?? ''}
            onChange={(event) =>
              onGroupChange({
                ...group,
                limits: { ...group.limits, context: parseLimitValue(event.target.value) },
              })
            }
            placeholder='0'
          />
        </div>
        <div className='space-y-1.5'>
          <Label>{t('Output limit')}</Label>
          <Input
            type='text'
            inputMode='numeric'
            value={group.limits?.output ?? ''}
            onChange={(event) =>
              onGroupChange({
                ...group,
                limits: { ...group.limits, output: parseLimitValue(event.target.value) },
              })
            }
            placeholder='0'
          />
        </div>
      </div>
    </div>
  )
}

type CapabilityGroupsEditorProps = {
  value: string
  onChange: (value: string) => void
}

/**
 * Visual editor for the capabilities JSON. Each named combination is a card;
 * the generated JSON is rendered read-only below so users can verify the
 * exact payload that will be saved.
 */
export function CapabilityGroupsEditor({
  value,
  onChange,
}: CapabilityGroupsEditorProps) {
  const { t } = useTranslation()
  const [groups, setGroups] = useState<CapabilityGroup[]>(() =>
    parseCapabilitiesToGroups(value)
  )
  const [previewOpen, setPreviewOpen] = useState(false)

  // Re-parse when the stored JSON changes externally (e.g. model loaded).
  useEffect(() => {
    setGroups(parseCapabilitiesToGroups(value))
  }, [value])

  const previewJson = useMemo(() => {
    const serialized = serializeGroupsToCapabilities(groups)
    return serialized === '' ? '' : JSON.stringify(JSON.parse(serialized), null, 2)
  }, [groups])

  const updateGroup = (index: number, next: CapabilityGroup) => {
    setGroups((previous) => {
      const nextGroups = previous.map((group, i) => (i === index ? next : group))
      onChange(serializeGroupsToCapabilities(nextGroups))
      return nextGroups
    })
  }

  const addGroup = () => {
    setGroups((previous) => {
      const nextGroups = [...previous, { name: '' }]
      onChange(serializeGroupsToCapabilities(nextGroups))
      return nextGroups
    })
  }

  const removeGroup = (index: number) => {
    setGroups((previous) => {
      const nextGroups = previous.filter((_, i) => i !== index)
      onChange(serializeGroupsToCapabilities(nextGroups))
      return nextGroups
    })
  }

  return (
    <div className='space-y-3'>
      <div className='space-y-3'>
        {groups.map((group, index) => (
          <CapabilityGroupCard
            key={group.name || `group-${index}`}
            group={group}
            onGroupChange={(next) => updateGroup(index, next)}
            onRemove={() => removeGroup(index)}
            canRemove={groups.length > 1}
          />
        ))}
      </div>

      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={addGroup}
        className='w-full'
      >
        <Plus className='mr-2 h-4 w-4' />
        {t('Add group')}
      </Button>

      <Collapsible open={previewOpen} onOpenChange={setPreviewOpen}>
        <CollapsibleTrigger
          render={
            <Button
              type='button'
              variant='outline'
              className='flex w-full items-center justify-between'
            />
          }
        >
          {t('View JSON')}
        </CollapsibleTrigger>
        <CollapsibleContent className='pt-2'>
          {previewJson ? (
            <JsonCodeEditor value={previewJson} disabled onChange={() => {}} />
          ) : (
            <p className='text-muted-foreground text-xs italic'>
              {t('No capabilities configured yet.')}
            </p>
          )}
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}

export function CapabilityGroupsEditorEmptyHint({ className }: { className?: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant='outline' className={cn('font-normal', className)}>
      {t('Empty')}
    </Badge>
  )
}
