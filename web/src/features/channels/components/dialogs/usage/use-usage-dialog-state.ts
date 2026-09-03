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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

/**
 * The minimal response shape every usage dialog consumes. Concrete provider
 * response types are structurally compatible thanks to optional fields.
 */
export type UsageResponseLike = {
  success?: boolean
  message?: string
  error_code?: string
} | null

export type UsageErrorCopy = {
  title: string
  body: string
}

/**
 * Uniform `error_code -> copy` mapping shared by usage dialogs. Providers that
 * need wider classification (e.g. Radeon Cloud) supply their own function with
 * the same signature.
 */
export function getUsageErrorCopy(
  errorCode: string | undefined,
  message: string | undefined,
  t: (key: string) => string
): UsageErrorCopy {
  if (
    errorCode === 'credentials_not_configured' ||
    errorCode === 'credentials_expired'
  ) {
    return {
      title: t('Usage credentials not configured'),
      body: message?.trim() || t('Failed to fetch usage'),
    }
  }
  return {
    title: t('Unable to identify usage data'),
    body: message?.trim() || t('Failed to fetch usage'),
  }
}

/**
 * Derive the shared dialog state from a usage response: error copy, degraded
 * flag, the raw JSON string and whether the raw panel should be offered.
 * `hasUsageData` is provided by the caller because each provider decides
 * which fields count as usable data.
 */
export function useUsageDialogState(props: {
  response: UsageResponseLike
  hasUsageData: boolean
  getUsageErrorCopy?: typeof getUsageErrorCopy
}) {
  const { t } = useTranslation()
  const getErrorCopy = props.getUsageErrorCopy ?? getUsageErrorCopy

  const errorCopy = useMemo(() => {
    if (props.response?.success === false) {
      return getErrorCopy(
        props.response.error_code,
        props.response.message,
        t
      )
    }
    return null
  }, [props.response, getErrorCopy, t])

  const rawJsonText = useMemo(
    () => (props.response ? JSON.stringify(props.response, null, 2) : ''),
    [props.response]
  )

  const showRawPanel = Boolean(
    props.response && props.response.success !== false && rawJsonText
  )
  const showDegraded =
    props.response != null &&
    props.response.success !== false &&
    !props.hasUsageData

  return {
    hasUsageData: props.hasUsageData,
    errorCopy,
    showDegraded,
    rawJsonText,
    showRawPanel,
  }
}