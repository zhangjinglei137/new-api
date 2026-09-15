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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SystemUpdateAction } from '@/features/system-update/system-update-action'
import { useStatus } from '@/hooks/use-status'
import { formatTimestamp } from '@/lib/format'

import { updateSystemOption } from '../api'
import { SettingsSection } from '../components/settings-section'
import type { OperationsSettings } from '../types'

type UpdateCheckerSectionProps = {
  settings?: OperationsSettings
  currentVersion?: string | null
  startTime?: number | null
}

export function UpdateCheckerSection(props: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [savingProxy, setSavingProxy] = useState(false)
  const [proxy, setProxy] = useState(props.settings?.UpdateCheckProxy ?? '')

  const uptime = props.startTime
    ? formatTimestamp(props.startTime)
    : t('Unknown')
  const version = status?.version || props.currentVersion || t('Unknown')

  const saveProxy = async () => {
    setSavingProxy(true)
    try {
      await updateSystemOption({ key: 'UpdateCheckProxy', value: proxy.trim() })
      toast.success(t('Proxy saved'))
    } catch {
      toast.error(t('Failed to save proxy'))
    } finally {
      setSavingProxy(false)
    }
  }

  return (
    <SettingsSection title={t('System maintenance')}>
      <div className='space-y-6'>
        <div className='grid gap-4 md:grid-cols-2'>
          <div className='rounded-lg border p-4'>
            <div className='text-muted-foreground text-sm'>
              {t('Current version')}
            </div>
            <div className='text-lg font-semibold break-all'>{version}</div>
          </div>
          <div className='rounded-lg border p-4'>
            <div className='text-muted-foreground text-sm'>
              {t('Uptime since')}
            </div>
            <div className='text-lg font-semibold'>{uptime}</div>
          </div>
        </div>
        <SystemUpdateAction compact={false} />
        <div className='space-y-2'>
          <Label htmlFor='update-check-proxy'>{t('Update check proxy')}</Label>
          <div className='flex gap-2'>
            <Input
              id='update-check-proxy'
              value={proxy}
              onChange={(event) => setProxy(event.target.value)}
              placeholder='http://127.0.0.1:7890'
            />
            <Button
              type='button'
              variant='secondary'
              onClick={saveProxy}
              disabled={savingProxy}
            >
              {savingProxy ? t('Saving...') : t('Save')}
            </Button>
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Optional outbound proxy for checking updates. Leave empty to connect directly with a domestic mirror fallback.'
            )}
          </p>
        </div>
      </div>
    </SettingsSection>
  )
}
