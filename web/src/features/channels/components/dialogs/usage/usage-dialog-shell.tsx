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
import { AlertTriangle, RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import { RawResponseCollapsible } from './raw-response-collapsible'
import type { UsageErrorCopy } from './use-usage-dialog-state'

export type UsageDialogShellProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  errorCopy: UsageErrorCopy | null
  /** Loading content rendered instead of the body while a query runs. */
  loading?: ReactNode
  /** Optional info banner rendered above the error (e.g. Radeon Cloud). */
  info?: ReactNode
  showDegraded: boolean
  /** Body of the degraded alert; each provider supplies its own copy. */
  degradedBody?: ReactNode
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
  showRawPanel: boolean
  rawJsonText: string
  /** Override the raw panel trigger label, e.g. Codex uses "Raw JSON". */
  rawTriggerLabel?: string
  /** Extra content rendered above the raw JSON (e.g. a copy button). */
  rawHeader?: ReactNode
  contentClassName?: string
  titleClassName?: string
  bodyClassName?: string
  children: ReactNode
}

export function UsageDialogShell(props: UsageDialogShellProps) {
  const { t } = useTranslation()
  const [showRawJson, setShowRawJson] = useState(false)

  useEffect(() => {
    if (!props.open) {
      setShowRawJson(false)
    }
  }, [props.open])

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.title}
      contentHeight='auto'
      contentClassName={props.contentClassName}
      titleClassName={props.titleClassName}
      bodyClassName={props.bodyClassName ?? 'flex flex-col gap-4'}
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => props.onOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {props.loading}
        {props.info}
        {props.errorCopy ? (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{props.errorCopy.title}</AlertTitle>
            <AlertDescription>{props.errorCopy.body}</AlertDescription>
          </Alert>
        ) : null}
        {props.showDegraded ? (
          <Alert>
            <AlertTriangle />
            <AlertTitle>{t('Unable to identify usage data')}</AlertTitle>
            <AlertDescription>
              {props.degradedBody ??
                t(
                  'The upstream response did not include recognizable usage windows.'
                )}
            </AlertDescription>
          </Alert>
        ) : null}
        {props.onRefresh ? (
          <div className='flex justify-end'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={props.onRefresh}
              disabled={Boolean(props.isRefreshing)}
            >
              <RefreshCw data-icon='inline-start' />
              {t('Refresh usage')}
            </Button>
          </div>
        ) : null}
        {props.children}
        {props.showRawPanel ? (
          <RawResponseCollapsible
            open={showRawJson}
            onOpenChange={setShowRawJson}
            text={props.rawJsonText}
            triggerLabel={props.rawTriggerLabel}
            header={props.rawHeader}
          />
        ) : null}
      </div>
    </Dialog>
  )
}