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
import { ChevronDown, ChevronUp } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ScrollArea } from '@/components/ui/scroll-area'

export type RawResponseCollapsibleProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  text: string
  /** Override the trigger label, e.g. Codex uses "Raw JSON". */
  triggerLabel?: string
  /** Extra content rendered above the JSON, e.g. a copy button. */
  header?: ReactNode
}

export function RawResponseCollapsible(props: RawResponseCollapsibleProps) {
  const { t } = useTranslation()
  const label = props.triggerLabel ?? t('Show raw upstream response')

  return (
    <Collapsible
      open={props.open}
      onOpenChange={props.onOpenChange}
      className='rounded-lg border'
    >
      <CollapsibleTrigger
        render={
          <button
            type='button'
            className='hover:bg-muted/40 flex w-full items-center justify-between gap-2 p-3 text-left transition-colors'
            aria-expanded={props.open}
          />
        }
      >
        <div className='text-sm font-medium'>{label}</div>
        {props.open ? (
          <ChevronUp className='text-muted-foreground h-4 w-4' />
        ) : (
          <ChevronDown className='text-muted-foreground h-4 w-4' />
        )}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {props.header}
        <ScrollArea className='max-h-[50vh]'>
          <pre className='bg-muted/30 m-0 p-3 text-xs break-words whitespace-pre-wrap'>
            {props.text}
          </pre>
        </ScrollArea>
      </CollapsibleContent>
    </Collapsible>
  )
}