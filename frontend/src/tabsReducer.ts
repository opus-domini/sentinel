export type TabsState = {
  openTabs: Array<string>
  activeSession: string
  activeEpoch: number
}

export type TabsAction =
  | { type: 'activate'; session: string }
  | { type: 'close'; session: string }
  | { type: 'rename'; oldName: string; newName: string }
  | { type: 'reorder'; from: number; to: number }
  | { type: 'sync'; sessions: Array<string> }
  | { type: 'clear' }

const STORAGE_KEY = 'sentinel_tabs'

export const initialTabsState: TabsState = {
  openTabs: [],
  activeSession: '',
  activeEpoch: 0,
}

export function loadPersistedTabs(): TabsState {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return initialTabsState
    const parsed = JSON.parse(raw) as {
      openTabs?: Array<string>
      activeSession?: string
    }
    if (!Array.isArray(parsed.openTabs)) return initialTabsState
    return {
      openTabs: parsed.openTabs.filter(
        (t) => typeof t === 'string' && t !== '',
      ),
      activeSession:
        typeof parsed.activeSession === 'string' ? parsed.activeSession : '',
      activeEpoch: 0,
    }
  } catch {
    return initialTabsState
  }
}

export function persistTabs(state: TabsState): void {
  try {
    sessionStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        openTabs: state.openTabs,
        activeSession: state.activeSession,
      }),
    )
  } catch {
    // sessionStorage full or unavailable — ignore.
  }
}

export function tabsReducer(state: TabsState, action: TabsAction): TabsState {
  switch (action.type) {
    case 'activate': {
      if (action.session === '') {
        return state
      }
      const openTabs = dedupe(
        state.openTabs.includes(action.session)
          ? state.openTabs
          : [...state.openTabs, action.session],
      )

      return {
        openTabs,
        activeSession: action.session,
        activeEpoch: state.activeEpoch + 1,
      }
    }
    case 'close': {
      if (action.session === '') {
        return state
      }

      const openTabs = state.openTabs.filter(
        (tabName) => tabName !== action.session,
      )
      if (state.activeSession !== action.session) {
        return { ...state, openTabs }
      }

      return {
        openTabs,
        activeSession: openTabs[openTabs.length - 1] ?? '',
        activeEpoch: state.activeEpoch + 1,
      }
    }
    case 'rename': {
      if (
        action.oldName === '' ||
        action.newName === '' ||
        action.oldName === action.newName
      ) {
        return state
      }

      const openTabs = dedupe(
        state.openTabs.map((tabName) =>
          tabName === action.oldName ? action.newName : tabName,
        ),
      )
      const activeSession =
        state.activeSession === action.oldName
          ? action.newName
          : state.activeSession

      return {
        ...state,
        openTabs,
        activeSession,
      }
    }
    case 'reorder': {
      const { from, to } = action
      if (
        from === to ||
        from < 0 ||
        to < 0 ||
        from >= state.openTabs.length ||
        to >= state.openTabs.length
      ) {
        return state
      }
      const tabs = [...state.openTabs]
      const [moved] = tabs.splice(from, 1)
      tabs.splice(to, 0, moved)
      return { ...state, openTabs: tabs }
    }
    case 'sync': {
      const allowed = new Set(action.sessions)
      const openTabs = dedupe(
        state.openTabs.filter((tabName) => allowed.has(tabName)),
      )
      let activeSession = allowed.has(state.activeSession)
        ? state.activeSession
        : ''

      if (activeSession === '' && openTabs.length > 0) {
        activeSession = openTabs[openTabs.length - 1]
      }

      return {
        openTabs,
        activeSession,
        activeEpoch:
          activeSession === state.activeSession
            ? state.activeEpoch
            : state.activeEpoch + 1,
      }
    }
    case 'clear': {
      return {
        openTabs: [],
        activeSession: '',
        activeEpoch: state.activeEpoch + 1,
      }
    }
    default: {
      return state
    }
  }
}

function dedupe(values: Array<string>): Array<string> {
  const seen = new Set<string>()
  const unique: Array<string> = []
  for (const value of values) {
    if (value === '' || seen.has(value)) {
      continue
    }
    seen.add(value)
    unique.push(value)
  }
  return unique
}
