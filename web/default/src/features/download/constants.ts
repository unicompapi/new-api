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
export const UNICOMP_AI_VERSION = '1.0.0'

export const UNICOMP_AI_INSTALLER_NAME = 'UniComp AI Setup 1.0.0.exe'

export const UNICOMP_AI_EXECUTABLE = 'UniComp-AI'

export const UNICOMP_AI_DOWNLOAD_ACTION = '/api/download/windows'

export const UNICOMP_DESKTOP_VERSION = '1.0.0'

export const UNICOMP_DESKTOP_INSTALLER_NAME = 'UniComp-1.0.0-setup-x64.exe'

export const UNICOMP_DESKTOP_DOWNLOAD_ACTION = '/api/download/unicomp/windows'

export const UNICOMP_AI_TAB_ID = 'unicomp-ai'

export const UNICOMP_DESKTOP_TAB_ID = 'unicomp-desktop'

export const DOWNLOAD_TAB_IDS = [
  UNICOMP_AI_TAB_ID,
  UNICOMP_DESKTOP_TAB_ID,
] as const

export type DownloadTabId = (typeof DOWNLOAD_TAB_IDS)[number]

export const DOWNLOAD_DEFAULT_TAB_ID: DownloadTabId = UNICOMP_AI_TAB_ID
