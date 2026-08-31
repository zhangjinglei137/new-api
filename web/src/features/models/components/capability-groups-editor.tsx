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

function isCapabilityGroupShape(value: unknown): value is CapabilityGroup {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  return typeof record.name === 'string' && record.name.trim() !== ''
}

// ============================================================================
// Capabilities JSON <-> editor groups
// ============================================================================

/**
 * Parse a stored capabilities JSON string into editor groups.
 * - Empty string → one empty group.
 * - Legacy single-group shape is normalized to a "chat" group.
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

  if (Array.isArray(parsed)) {
    const groups = parsed.filter(isCapabilityGroupShape)
    return groups.length > 0 ? groups : [{ name: '' }]
  }

  if (isLegacyCapabilitiesShape(parsed)) {
    const group: CapabilityGroup = { name: 'chat' }
    if (parsed.modalities?.input?.length) {
      group.input = parsed.modalities.input
    }
    if (parsed.modalities?.output?.length) {
      group.output = parsed.modalities.output
    }
    if (parsed.reasoning_options?.length) {
      group.reasoning_options = parsed.reasoning_options
    }
    if (parsed.limits?.context !== undefined) {
      group.limits = { ...group.limits, context: parsed.limits.context }
    }
    if (parsed.limits?.output !== undefined) {
      group.limits = { ...group.limits, output: parsed.limits.output }
    }
    return [group]
  }

  return [{ name: '' }]
}

/**
 * Serialize editor groups into the capabilities JSON string.
 * - Omitted (empty) modality arrays are not written.
 * - Empty limits values are not written.
 * - A group that only has a name still produces `{"name": "..."}`.
 */
export function serializeGroupsToCapabilities(groups: CapabilityGroup[]): string {
  const payload = groups
    .filter((group) => group.name.trim() !== '')
    .map((group) => {
      const entry: Record<string, unknown> = { name: group.name.trim() }
      if (group.input && group.input.length > 0) {
        entry.input = group.input
      }
      if (group.output && group.output.length > 0) {
        entry.output = group.output
      }
      if (group.reasoning_options && group.reasoning_options.length > 0) {
        entry.reasoning_options = group.reasoning_options
      }
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
  return JSON.stringify(payload)
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
