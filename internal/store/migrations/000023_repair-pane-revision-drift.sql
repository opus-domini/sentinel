-- ============================================================
-- watchtower: repair panes whose revision fell below seen_revision
-- ============================================================
-- The pane upsert clamped seen_revision but not revision, so a session rename
-- (which made the collector treat an existing pane as new, restarting its
-- revision at 1) could leave revision permanently below seen_revision. Unread
-- is revision > seen_revision, so those panes could never report unread again.
--
-- The clamp is fixed in code, but it only prevents new drift: an already
-- inverted pane would need as many changes as the gap to climb back. Level the
-- two counters so the pane resumes from "seen", which is the honest reading —
-- the user did see this pane, and everything since is what the collector will
-- pick up on its next tick.

UPDATE wt_panes
   SET revision = seen_revision
 WHERE seen_revision > revision;

UPDATE wt_windows
   SET unread_panes = 0,
       has_unread = 0
 WHERE has_unread = 1
   AND NOT EXISTS (
         SELECT 1 FROM wt_panes
          WHERE wt_panes.session_name = wt_windows.session_name
            AND wt_panes.window_index = wt_windows.window_index
            AND wt_panes.revision > wt_panes.seen_revision
       );
