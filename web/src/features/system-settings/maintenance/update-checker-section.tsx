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
import { ExternalLinkIcon, RefreshCcwIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Markdown } from '@/components/ui/markdown'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'

import { checkUpdate, updateSystemOption } from '../api'
import { SettingsSection } from '../components/settings-section'
import type { OperationsSettings } from '../types'

type ReleaseInfo = {
  tag_name: string
  name?: string
  body?: string
  html_url?: string
  published_at?: string
}

type UpdateCheckerSectionProps = {
  settings: OperationsSettings
  currentVersion?: string | null
  startTime?: number | null
}

export function UpdateCheckerSection({
  settings,
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [savingProxy, setSavingProxy] = useState(false)
  const [proxy, setProxy] = useState(settings.UpdateCheckProxy ?? '')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [release, setRelease] = useState<ReleaseInfo | null>(null)

  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const version = currentVersion || t('Unknown')

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

  const handleCheckUpdates = async () => {
    setChecking(true)
    try {
      const payload = await checkUpdate()
      if (!payload.success || !payload.data?.tag_name) {
        throw new Error(payload.message || t('Failed to check for updates'))
      }

      if (currentVersion && payload.data.tag_name === currentVersion) {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: payload.data.tag_name,
          })
        )
        return
      }

      setRelease(payload.data)
      setDialogOpen(true)
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to check for updates')
      toast.error(message)
    } finally {
      setChecking(false)
    }
  }

  const goToRelease = () => {
    if (release?.html_url) {
      window.open(release.html_url, '_blank', 'noopener,noreferrer')
    }
  }

  return (
    <>
      <SettingsSection title={t('System maintenance')}>
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Current version')}
              </div>
              <div className='text-lg font-semibold'>{version}</div>
            </div>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Uptime since')}
              </div>
              <div className='text-lg font-semibold'>{uptime}</div>
            </div>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='update-check-proxy'>
              {t('Update check proxy')}
            </Label>
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

          <Button onClick={handleCheckUpdates} disabled={checking}>
            {checking ? (
              t('Checking updates...')
            ) : (
              <>
                <RefreshCcwIcon className='me-2 h-4 w-4' />
                {t('Check for updates')}
              </>
            )}
          </Button>
        </div>
      </SettingsSection>

      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={
          release?.tag_name
            ? t('New version available: {{version}}', {
                version: release.tag_name,
              })
            : t('Release details')
        }
        description={
          release?.published_at
            ? `${t('Published')} ${formatTimestampToDate(
                new Date(release.published_at).getTime(),
                'milliseconds'
              )}`
            : undefined
        }
        contentClassName='max-h-[80vh] overflow-y-auto'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setDialogOpen(false)}
            >
              {t('Close')}
            </Button>
            {release?.html_url && (
              <Button type='button' onClick={goToRelease}>
                <ExternalLinkIcon className='me-2 h-4 w-4' />
                {t('Open release')}
              </Button>
            )}
          </>
        }
      >
        <div className='space-y-4'>
          {release?.body ? (
            <Markdown>{release.body}</Markdown>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('No release notes provided.')}
            </p>
          )}
        </div>
      </Dialog>
    </>
  )
}