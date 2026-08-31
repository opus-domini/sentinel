import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { TmuxIcon } from '@/lib/tmuxIcons'
import { cn } from '@/lib/utils'

type SortableLauncherRowProps = {
  id: string
  iconKey: string
  name: string
  description: string
  selected: boolean
  dragEnabled: boolean
  onSelect: (id: string) => void
}

/** One draggable row in the launcher list of the window and session launcher dialogs. */
export default function SortableLauncherRow({
  id,
  iconKey,
  name,
  description,
  selected,
  dragEnabled,
  onSelect,
}: SortableLauncherRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })
  return (
    <li
      ref={setNodeRef}
      className="min-w-0 shrink-0"
      style={{
        transform: dragEnabled ? CSS.Transform.toString(transform) : undefined,
        transition: dragEnabled ? transition : undefined,
        opacity: dragEnabled && isDragging ? 0.5 : undefined,
        zIndex: dragEnabled && isDragging ? 10 : undefined,
      }}
    >
      <button
        type="button"
        className={cn(
          'flex w-full cursor-pointer items-center gap-2 rounded-md border px-2 py-2 text-left transition-colors',
          selected
            ? 'border-primary/60 bg-surface-active-primary'
            : 'border-transparent hover:border-border-subtle hover:bg-surface-hover',
        )}
        onClick={() => onSelect(id)}
        style={{ touchAction: dragEnabled ? undefined : 'pan-y' }}
        {...(dragEnabled ? attributes : {})}
        {...(dragEnabled ? listeners : {})}
      >
        <TmuxIcon iconKey={iconKey} className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12px] font-semibold">{name}</span>
          <span className="block truncate text-[10px] text-muted-foreground">{description}</span>
        </span>
      </button>
    </li>
  )
}
