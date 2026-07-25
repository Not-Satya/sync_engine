import { useCallback, useEffect, useState } from 'react';
import { syncClient } from './lib/syncClient';
import { useSyncEvents } from './hooks/useSyncEvents';
import NotesList from './components/NotesList';

export default function App() {
  const [notes, setNotes] = useState([]);
  const [status, setStatus] = useState('idle');

  const refresh = useCallback(() => {
    syncClient.listNotes().then(setNotes);
    syncClient.getStatus().then((s) => setStatus(s.state));
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // This is the entire "realtime sync" wiring on the frontend: one
  // subscription, re-fetch on change. No polling interval anywhere.
  useSyncEvents(refresh);

  const addNote = async (text) => {
    // Returns as soon as SQLite has it — not after a server round trip.
    const note = await syncClient.createNote(text);
    setNotes((prev) => [...prev, note]);
  };

  return (
    <div className="app">
      <header className="app__header">
        <h1>Notes</h1>
        <span className={`status status--${status}`}>{status}</span>
      </header>
      <NotesList notes={notes} onAddNote={addNote} />
    </div>
  );
}
