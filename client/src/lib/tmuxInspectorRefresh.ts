export function shouldSkipInspectorRefresh(
  background: boolean,
  inspectorLoading: boolean,
): boolean {
  return background && inspectorLoading
}
