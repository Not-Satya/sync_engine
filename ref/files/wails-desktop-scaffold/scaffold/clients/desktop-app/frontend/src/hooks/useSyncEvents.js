import { useEffect } from 'react';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

// Subscribes to the "sync:changed" event emitted from app_events.go.
// Fires onChange(entityIds) whenever a mutation lands — whether it
// originated on this device (you typed something) or arrived from a
// remote one over the sync server. Components just re-fetch; there's
// no separate "remote update" code path to maintain.
export function useSyncEvents(onChange) {
  useEffect(() => {
    EventsOn('sync:changed', onChange);
    return () => EventsOff('sync:changed');
  }, [onChange]);
}
