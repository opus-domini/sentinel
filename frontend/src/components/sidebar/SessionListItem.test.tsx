// @vitest-environment jsdom
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import SessionListItem from './SessionListItem'

vi.mock('@/components/TooltipHelper', () => ({
  TooltipHelper: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/hooks/useDateFormat', () => ({
  useDateFormat: () => ({
    formatTimestamp: (value: string) => value,
  }),
}))

vi.mock('@/contexts/MetaContext', () => ({
  useMetaContext: () => ({
    processUser: 'hugo',
    isRoot: false,
    allowedUsers: ['postgres'],
  }),
}))

afterEach(() => {
  cleanup()
})

function SortableTestShell({ children }: { children: ReactNode }) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter}>
      <SortableContext items={['api']} strategy={verticalListSortingStrategy}>
        <ul>{children}</ul>
      </SortableContext>
    </DndContext>
  )
}

const baseSession = {
  name: 'api',
  windows: 2,
  panes: 3,
  attached: 0,
  createdAt: '2026-03-29T00:00:00Z',
  activityAt: '2026-03-29T00:00:00Z',
  command: 'node',
  hash: 'abcdef123456',
  lastContent: 'ready',
  icon: 'server',
}

describe('SessionListItem', () => {
  it('uses the same tone for icon and title on attached sessions', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={{ ...baseSession, attached: 1 }}
          isActive
          isPinned={false}
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    const title = screen.getByText('api')
    const icon = title.previousSibling as SVGElement | null

    expect(title.className).not.toContain('text-muted-foreground')
    expect(icon?.className.baseVal ?? '').not.toContain('text-primary-text')
  })

  it('dims the title to match the icon when the session is detached', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned={false}
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    const title = screen.getByText('api')
    const icon = title.previousSibling as SVGElement | null

    expect(title.className).toContain('text-muted-foreground')
    expect(icon?.className.baseVal ?? '').toContain('text-muted-foreground')
  })

  it('pins a session from the context menu', async () => {
    const onPinSession = vi.fn()

    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned={false}
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={onPinSession}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    fireEvent.contextMenu(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Pin Session')).toBeTruthy()
    })

    fireEvent.click(screen.getByText('Pin Session'))

    expect(onPinSession).toHaveBeenCalledWith('api')
  })

  it('shows only unpin action for pinned sessions', async () => {
    const onUnpinSession = vi.fn()

    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={onUnpinSession}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    fireEvent.contextMenu(screen.getByRole('button'))

    await waitFor(() => {
      expect(screen.getByText('Unpin Session')).toBeTruthy()
    })

    expect(screen.queryByText('Pin Session')).toBeNull()

    fireEvent.click(screen.getByText('Unpin Session'))

    expect(onUnpinSession).toHaveBeenCalledWith('api')
  })

  it('hides the content preview when rendered in compact density', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned={false}
          density="compact"
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    expect(screen.queryByText('ready')).toBeNull()
    expect(screen.getByText('abc…456')).toBeTruthy()
  })

  it('shows only icon, name, and time in minimal density', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned={false}
          density="minimal"
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    expect(screen.getByText('api')).toBeTruthy()
    expect(screen.queryByText('ready')).toBeNull()
    expect(screen.queryByText('abc…456')).toBeNull()
    expect(screen.queryByLabelText(/window/i)).toBeNull()
    expect(screen.queryByLabelText(/pane/i)).toBeNull()
  })

  it('shows a user indicator when session user differs from process user', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={{ ...baseSession, user: 'postgres' }}
          isActive={false}
          isPinned={false}
          density="compact"
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    expect(screen.getByText('postgres')).toBeTruthy()
  })

  it('hides the user indicator when session user matches process user', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={{ ...baseSession, user: 'hugo' }}
          isActive={false}
          isPinned={false}
          density="compact"
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    expect(screen.queryByText('hugo')).toBeNull()
  })

  it('uses touch pan-y when drag is disabled', () => {
    render(
      <SortableTestShell>
        <SessionListItem
          session={baseSession}
          isActive={false}
          isPinned={false}
          dragEnabled={false}
          onAttach={() => {}}
          onRename={() => {}}
          onDetach={() => {}}
          onKill={() => {}}
          onChangeIcon={() => {}}
          onPinSession={() => {}}
          onUnpinSession={() => {}}
          canDetach={false}
        />
      </SortableTestShell>,
    )

    expect(screen.getByRole('button').style.touchAction).toBe('pan-y')
  })
})
