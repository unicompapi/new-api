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

import { useCallback, useState } from 'react'
import {
  addSeedanceHistoryEntry,
  clearSeedanceHistory,
  loadSeedanceHistory,
  removeSeedanceHistoryEntry,
  updateSeedanceHistoryStatus,
} from '../lib/history-storage'
import type { SeedanceHistoryEntry } from '../types'

export function useSeedanceHistory() {
  const [entries, setEntries] = useState<SeedanceHistoryEntry[]>(() =>
    loadSeedanceHistory()
  )

  const refresh = useCallback(() => {
    setEntries(loadSeedanceHistory())
  }, [])

  const addEntry = useCallback((entry: SeedanceHistoryEntry) => {
    setEntries(addSeedanceHistoryEntry(entry))
  }, [])

  const updateStatus = useCallback((taskId: string, status: string) => {
    setEntries(updateSeedanceHistoryStatus(taskId, status))
  }, [])

  const removeEntry = useCallback((taskId: string) => {
    setEntries(removeSeedanceHistoryEntry(taskId))
  }, [])

  const clearAll = useCallback(() => {
    setEntries(clearSeedanceHistory())
  }, [])

  return {
    entries,
    addEntry,
    updateStatus,
    removeEntry,
    clearAll,
    refresh,
  }
}
