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
import { Building2, Loader2, Plus, SearchIcon, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { DataTablePagination } from '@/components/data-table/core/pagination'
import { Dialog } from '@/components/dialog'
import { ReactIconByName } from '@/components/react-icon-by-name'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { getVendors, searchVendors, deleteVendor } from '../../api'
import { vendorsQueryKeys, modelsQueryKeys } from '../../lib'
import type { Vendor } from '../../types'
import { VendorMutateDialog } from './vendor-mutate-dialog'

const VENDOR_PAGE_SIZE = 10

type VendorManagementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type VendorRow = {
  vendor: Vendor
  referenceCount: number
}

function getVendorCount(
  vendor: Vendor,
  counts: Record<string, number> | undefined
): number {
  if (!counts) return 0
  const count = counts[String(vendor.id)]
  return typeof count === 'number' && count > 0 ? count : 0
}

export function VendorManagementDialog({
  open,
  onOpenChange,
}: VendorManagementDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(VENDOR_PAGE_SIZE)
  const [mutateState, setMutateState] = useState<{
    open: boolean
    vendor: Vendor | null
  }>({ open: false, vendor: null })
  const [deleteState, setDeleteState] = useState<{
    open: boolean
    vendor: Vendor | null
  }>({ open: false, vendor: null })
  const [isDeleting, setIsDeleting] = useState(false)

  const shouldSearch = keyword.trim() !== ''

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: vendorsQueryKeys.list({ keyword, p: page, page_size: pageSize }),
    queryFn: () =>
      shouldSearch
        ? searchVendors({ keyword: keyword.trim(), p: page, page_size: pageSize })
        : getVendors({ p: page, page_size: pageSize }),
    enabled: open,
  })

  const rows = useMemo<VendorRow[]>(
    () =>
      (data?.data?.items || []).map((vendor) => ({
        vendor,
        referenceCount: getVendorCount(vendor, data?.data?.vendor_counts),
      })),
    [data]
  )

  const total = data?.data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const handlePageSizeChange = (size: number) => {
    setPageSize(size)
    setPage(1)
  }

  const openCreate = () => setMutateState({ open: true, vendor: null })
  const openEdit = (vendor: Vendor) => setMutateState({ open: true, vendor })
  const openDelete = (vendor: Vendor) => setDeleteState({ open: true, vendor })

  const handleDeleteConfirm = async () => {
    if (!deleteState.vendor) return
    setIsDeleting(true)
    try {
      const response = await deleteVendor(deleteState.vendor.id)
      if (response.success) {
        toast.success(t('Vendor deleted successfully'))
        queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
        queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
        setDeleteState({ open: false, vendor: null })
      } else {
        // Backend refuses deletion while models still reference this vendor
        // and reports the reference count in the message.
        toast.error(response.message || t('Failed to delete vendor'))
      }
    } catch (err: unknown) {
      toast.error((err as Error)?.message || t('Failed to delete vendor'))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={
          <>
            <Building2 className='text-foreground/80 h-5 w-5' />
            {t('Vendor Management')}
          </>
        }
        description={t(
          'Create, edit, and remove the vendors that models reference.'
        )}
        contentClassName='w-[calc(100vw-2rem)] sm:max-w-[52rem]'
        contentHeight='auto'
        bodyClassName='space-y-3'
      >
        <div className='flex flex-wrap items-center gap-2'>
          <div className='relative min-w-0 flex-1'>
            <SearchIcon className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
            <Input
              value={keyword}
              onChange={(event) => {
                setKeyword(event.target.value)
                setPage(1)
              }}
              placeholder={t('Search vendors')}
              className='pl-8 pr-8'
            />
            {keyword && (
              <Button
                type='button'
                variant='ghost'
                size='icon-xs'
                onClick={() => {
                  setKeyword('')
                  setPage(1)
                }}
                className='absolute top-1/2 right-1 -translate-y-1/2'
                aria-label={t('Clear search')}
              >
                <X className='size-3.5' />
              </Button>
            )}
          </div>
          <Button size='sm' onClick={openCreate}>
            <Plus className='mr-2 h-4 w-4' />
            {t('Add Vendor')}
          </Button>
          <Button
            size='sm'
            variant='ghost'
            onClick={() => refetch()}
            disabled={isFetching}
          >
            {isFetching && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Refresh')}
          </Button>
        </div>

        {error && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Unable to load vendors')}</AlertTitle>
            <AlertDescription>
              {(error as Error).message || t('Please retry or refresh the page.')}
            </AlertDescription>
          </Alert>
        )}

        <div className='border-border/60 overflow-hidden rounded-lg border'>
          <StaticDataTable
            data={rows}
            getRowKey={({ vendor }) => vendor.id}
            tableClassName='min-w-[640px]'
            emptyContent={
              <div className='text-muted-foreground py-8 text-center text-sm'>
                {t('No vendors found')}
              </div>
            }
            columns={[
              {
                id: 'vendor',
                header: t('Vendor'),
                className: 'w-[26%]',
                cell: ({ vendor }) => (
                  <div className='flex items-center gap-2'>
                    <ReactIconByName
                      name={vendor.icon}
                      className='text-muted-foreground size-5 shrink-0'
                    />
                    <span className='truncate font-medium'>{vendor.name}</span>
                    <TableId value={vendor.id} />
                  </div>
                ),
              },
              {
                id: 'description',
                header: t('Description'),
                className: 'w-[30%]',
                cellClassName: 'whitespace-normal',
                cell: ({ vendor }) => (
                  <span className='text-muted-foreground line-clamp-2 text-xs'>
                    {vendor.description || (
                      <span className='italic'>{t('No description')}</span>
                    )}
                  </span>
                ),
              },
              {
                id: 'references',
                header: t('Model references'),
                className: 'w-[14%]',
                cell: ({ referenceCount }) => (
                  <StatusBadge
                    label={String(referenceCount)}
                    variant={referenceCount > 0 ? 'info' : 'neutral'}
                    size='sm'
                    copyable={false}
                  />
                ),
              },
              {
                id: 'status',
                header: t('Status'),
                className: 'w-[12%]',
                cell: ({ vendor }) => (
                  <StatusBadge
                    label={vendor.status === 1 ? t('Enabled') : t('Disabled')}
                    variant={vendor.status === 1 ? 'success' : 'neutral'}
                    size='sm'
                    copyable={false}
                  />
                ),
              },
              {
                id: 'actions',
                header: t('Actions'),
                className: 'w-[18%] text-right',
                cell: ({ vendor }) => (
                  <StaticRowActions
                    editLabel={t('Edit vendor')}
                    deleteLabel={t('Delete vendor')}
                    menuLabel={t('Open menu')}
                    onEdit={() => openEdit(vendor)}
                    onDelete={() => openDelete(vendor)}
                    deleteDisabled={false}
                  />
                ),
              },
            ]}
          />
        </div>

        <div className='flex items-center justify-between gap-2'>
          <StatusBadge
            label={`${total} ${t('vendors')}`}
            variant='neutral'
            size='sm'
            copyable={false}
          />
          <DataTablePagination
            table={
              {
                getState: () => ({ pagination: { pageIndex: page - 1, pageSize } }),
                getPageCount: () => totalPages,
                getRowCount: () => total,
                setPageSize: (size: number) => handlePageSizeChange(size),
                setPageIndex: (index: number) => setPage(index + 1),
                previousPage: () => setPage((p) => Math.max(1, p - 1)),
                nextPage: () => setPage((p) => Math.min(totalPages, p + 1)),
                getCanPreviousPage: () => page > 1,
                getCanNextPage: () => page < totalPages,
              } as unknown as Parameters<typeof DataTablePagination>[0]['table']
            }
          />
        </div>

        {isLoading && (
          <div className='flex items-center justify-center gap-2 py-4 text-sm text-muted-foreground'>
            <Loader2 className='h-4 w-4 animate-spin' />
            {t('Loading...')}
          </div>
        )}
      </Dialog>

      <VendorMutateDialog
        open={mutateState.open}
        onOpenChange={(next) => {
          if (!next) {
            setMutateState({ open: false, vendor: null })
          }
        }}
        currentVendor={mutateState.vendor}
      />

      <ConfirmDialog
        open={deleteState.open}
        onOpenChange={(next) => {
          if (!next) setDeleteState({ open: false, vendor: null })
        }}
        title={t('Delete vendor')}
        desc={
          <p>
            {t('Are you sure you want to delete vendor "{{name}}"?', {
              name: deleteState.vendor?.name ?? '',
            })}
          </p>
        }
        destructive
        confirmText={isDeleting ? t('Deleting...') : t('Delete')}
        isLoading={isDeleting}
        handleConfirm={handleDeleteConfirm}
      />
    </>
  )
}
