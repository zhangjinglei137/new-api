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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Cable, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  getEndpointDefinitions,
  updateEndpointDefinitions,
} from '../../api'
import { endpointDefinitionsQueryKeys } from '../../lib'
import type { EndpointDefinition } from '../../types'

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'] as const
const HTTP_METHODS_SET = new Set<string>(HTTP_METHODS)

/**
 * Endpoint types whose path is provided at model level and may be empty in
 * the global endpoint definition.
 */
const PATH_OPTIONAL_ENDPOINT_TYPES = new Set<string>(['openai-video'])

type EndpointManagementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EndpointManagementDialog({
  open,
  onOpenChange,
}: EndpointManagementDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rows, setRows] = useState<EndpointDefinition[]>([])
  const [isSaving, setIsSaving] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: endpointDefinitionsQueryKeys.list(),
    queryFn: getEndpointDefinitions,
    enabled: open,
  })

  useEffect(() => {
    if (open && data?.data?.endpoints) {
      setRows(data.data.endpoints)
    }
  }, [open, data])

  const updateRow = (type: string, patch: Partial<EndpointDefinition>) => {
    setRows((previous) =>
      previous.map((row) => (row.type === type ? { ...row, ...patch } : row))
    )
  }

  const validate = (): string | null => {
    for (const row of rows) {
      if (!row.display_name || row.display_name.trim() === '') {
        return t('Display name is required for endpoint type "{{type}}"', {
          type: row.type,
        })
      }
      const isPathOptional = PATH_OPTIONAL_ENDPOINT_TYPES.has(row.type)
      if (!isPathOptional && (!row.path || row.path.trim() === '')) {
        return t('Path is required for endpoint type "{{type}}"', {
          type: row.type,
        })
      }
      if (row.method && !HTTP_METHODS_SET.has(row.method.toUpperCase())) {
        return t('Method must be one of GET, POST, PUT, DELETE, PATCH')
      }
    }
    return null
  }

  const handleSave = async () => {
    const validationMessage = validate()
    if (validationMessage) {
      toast.error(validationMessage)
      return
    }
    setIsSaving(true)
    try {
      const response = await updateEndpointDefinitions({ endpoints: rows })
      if (response.success) {
        toast.success(t('Endpoint definitions saved'))
        queryClient.invalidateQueries({
          queryKey: endpointDefinitionsQueryKeys.lists(),
        })
        onOpenChange(false)
      } else {
        toast.error(response.message || t('Failed to save endpoint definitions'))
      }
    } catch (err: unknown) {
      toast.error((err as Error)?.message || t('Failed to save endpoint definitions'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <>
          <Cable className='text-foreground/80 h-5 w-5' />
          {t('Endpoint Management')}
        </>
      }
      description={t(
        'Configure the endpoint definitions offered by the model editor template list.'
      )}
      contentClassName='w-[calc(100vw-2rem)] sm:max-w-[52rem]'
      contentHeight='auto'
      bodyClassName='space-y-3'
      footer={
        <>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isSaving}>
            {isSaving && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Save')}
          </Button>
        </>
      }
    >
      {error && (
        <div className='text-destructive text-sm'>
          {t('Unable to load endpoint definitions')}
        </div>
      )}

      {isLoading ? (
        <div className='flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Loading...')}
        </div>
      ) : (
        <>
          <div className='border-border/60 overflow-hidden rounded-lg border'>
            <StaticDataTable
              data={rows}
              getRowKey={(row) => row.type}
              tableClassName='min-w-[720px]'
              columns={[
                {
                  id: 'type',
                  header: t('Endpoint Type'),
                  className: 'w-[22%]',
                  cell: ({ type }) => (
                    <div className='flex flex-wrap items-center gap-1.5'>
                      <span className='font-mono text-xs font-medium'>
                        {type}
                      </span>
                      {PATH_OPTIONAL_ENDPOINT_TYPES.has(type) && (
                        <Badge
                          variant='warning'
                          className='font-normal'
                        >
                          {t('path in model endpoint config')}
                        </Badge>
                      )}
                    </div>
                  ),
                },
                {
                  id: 'display_name',
                  header: t('Display Name'),
                  className: 'w-[24%]',
                  cell: ({ type, display_name }) => (
                    <Input
                      value={display_name || ''}
                      onChange={(event) =>
                        updateRow(type, { display_name: event.target.value })
                      }
                      placeholder={t('Display name')}
                    />
                  ),
                },
                {
                  id: 'path',
                  header: t('Path'),
                  className: 'w-[26%]',
                  cell: ({ type, path }) => (
                    <Input
                      value={path || ''}
                      onChange={(event) =>
                        updateRow(type, { path: event.target.value })
                      }
                      placeholder='/v1/chat/completions'
                      className={cn(
                        'font-mono',
                        PATH_OPTIONAL_ENDPOINT_TYPES.has(type) &&
                          !path &&
                          'border-dashed'
                      )}
                    />
                  ),
                },
                {
                  id: 'method',
                  header: t('Method'),
                  className: 'w-[14%]',
                  cell: ({ type, method }) => (
                    <Input
                      value={method || ''}
                      onChange={(event) =>
                        updateRow(type, { method: event.target.value })
                      }
                      placeholder='POST'
                      className='font-mono'
                    />
                  ),
                },
                {
                  id: 'npm',
                  header: t('NPM'),
                  className: 'w-[14%]',
                  cell: ({ type, npm }) => (
                    <Input
                      value={npm || ''}
                      onChange={(event) =>
                        updateRow(type, { npm: event.target.value })
                      }
                      placeholder='@ai-sdk/openai'
                      className='font-mono'
                    />
                  ),
                },
              ]}
            />
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Endpoint types are fixed. Edit display name, path, method, and NPM package only.'
            )}
          </p>
        </>
      )}
    </Dialog>
  )
}
