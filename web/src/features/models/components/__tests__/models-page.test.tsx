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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getVendors, searchVendors } from '../../api'
import { Models } from '../../index'

// 页面级测试需要最小的路由与翻译上下文：
// - getRouteApi().useParams() 固定返回 vendors section，激活 Vendors tab
// - useNavigate 仅在 tab 切换时被调用，测试不触发
vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useParams: () => ({ section: 'vendors' }) }),
  useNavigate: () => vi.fn(),
}))

// Identity translation：按钮文本即 i18n 键，按键名定位按钮。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

// 本测试只关心 tab 内容区的操作行结构；弹窗/抽屉与页面级测试无关，渲染为 null。
vi.mock('../../components/models-dialogs', () => ({
  ModelsDialogs: () => null,
}))
// models-table 的列渲染会经 @/lib/lobe-icon 加载 @lobehub/icons 全量 barrel，
// 其依赖 @lobehub/fluent-emoji 的目录导入在 Vitest 的 ESM 解析下不可用。
// 图标是纯展示叶子模块，与按钮结构断言无关，mock 为 null。
vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))
vi.mock('../../components/dialogs/create-deployment-drawer', () => ({
  CreateDeploymentDrawer: () => null,
}))

// VendorsTabContent 走真实组件（其工具栏的 Add Vendor 是唯一保留入口），
// 仅 mock 其 API 数据源返回空列表。
vi.mock('../../api', () => ({
  getVendors: vi.fn(),
  searchVendors: vi.fn(),
  deleteVendor: vi.fn(),
}))

function renderVendorsPage() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <Models />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  const emptyPage = {
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 10 },
  }
  vi.mocked(getVendors).mockResolvedValue(emptyPage)
  vi.mocked(searchVendors).mockResolvedValue(emptyPage)
})

describe('models page vendors section actions contract', () => {
  test('vendors 页面级操作区不渲染 Add Vendor，只保留工具栏唯一入口', async () => {
    renderVendorsPage()

    // 等表格数据稳定（空态），避免异步查询产生未清理的状态更新。
    await screen.findByText('No vendors found')

    // 页面级操作区（标题行右侧 SectionPageLayout.Actions 容器）整体置空：
    // vendors 分支页面级按钮随 actions=null 一起移除，容器不再渲染。
    // 修复前页面级按钮存在，容器照常渲染，此断言失败（RED）。
    const pageActions = [...document.querySelectorAll('div')].find(
      (element) =>
        element.className.includes('shrink-0') &&
        element.className.includes('justify-end')
    )
    expect(pageActions).toBeUndefined()

    // 全页只剩 VendorsTabContent 工具栏一个 Add Vendor：修复前页面级按钮
    // 与工具栏按钮并存，同一视图出现两个相同按钮，此断言失败（RED）。
    const addVendorButtons = screen.getAllByRole('button', {
      name: 'Add Vendor',
    })
    expect(addVendorButtons).toHaveLength(1)
    expect(addVendorButtons[0]).toBeEnabled()
  })
})
