export const UPDATE_DISCOVERED_EVENT = 'mcm:update-discovered';

export function announceUpdateDiscovery<T>(result: T): T {
  window.dispatchEvent(new Event(UPDATE_DISCOVERED_EVENT));
  return result;
}
