// Cross-tab auth invalidation via localStorage `storage` events.
//
// Cookies do not fire `storage` events, so we use a dedicated
// localStorage key as the signaling channel. Each app owns its own
// channel name (keyed off audience) so a user-frontend logout does not
// log out an admin-frontend tab on the same origin (dev/codespaces) or
// vice versa.

export interface CrossTabChannel {
  // Fire an invalidation to peer tabs. Value is timestamped so repeated
  // broadcasts still trigger the `storage` event (identical values are
  // coalesced).
  broadcastLogout: () => void;

  // Subscribe to peer-tab logouts. The callback runs with the raw
  // StorageEvent so the caller can inspect `e.key === null` (full
  // localStorage.clear()). Returns an unsubscribe fn.
  onRemoteLogout: (cb: (event: StorageEvent) => void) => () => void;

  // Whether the storage event indicates an invalidation for THIS
  // channel. Exposed so callers can keep their own listeners (they
  // often want to do their own work in the same handler).
  isInvalidationEvent: (event: StorageEvent) => boolean;
}

export function createCrossTabChannel(channelKey: string): CrossTabChannel {
  function broadcastLogout(): void {
    try {
      localStorage.setItem(channelKey, `logout:${Date.now()}`);
    } catch {
      // Private mode / storage quota — peer tabs recover on their next
      // API call when it 401s.
    }
  }

  function isInvalidationEvent(event: StorageEvent): boolean {
    // e.key === null covers localStorage.clear() across tabs.
    return (
      (event.key === channelKey && event.newValue !== null) ||
      event.key === null
    );
  }

  function onRemoteLogout(cb: (event: StorageEvent) => void): () => void {
    if (typeof window === 'undefined') {
      return () => {
        /* noop for SSR */
      };
    }
    const handler = (event: StorageEvent): void => {
      if (isInvalidationEvent(event)) {
        cb(event);
      }
    };
    window.addEventListener('storage', handler);
    return () => {
      window.removeEventListener('storage', handler);
    };
  }

  return { broadcastLogout, onRemoteLogout, isInvalidationEvent };
}
