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
/// <reference types="@rsbuild/core/types" />

interface ImportMetaEnv {
  readonly VITE_MINIO_ENDPOINT?: string
  readonly VITE_MINIO_BUCKET?: string
  readonly VITE_MINIO_ACCESS_KEY?: string
  readonly VITE_MINIO_SECRET_KEY?: string
  readonly VITE_MINIO_PUBLIC_BASE_URL?: string
  readonly VITE_MINIO_REGION?: string
  /** Days until uploaded reference media should expire (default 7). */
  readonly VITE_MINIO_OBJECT_EXPIRE_DAYS?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '@visactor/react-vchart' {
  export const VChart: React.ComponentType<Record<string, unknown>>
}

declare module '@visactor/vchart-semi-theme' {
  export const initVChartSemiTheme: (opts?: Record<string, unknown>) => void
}
