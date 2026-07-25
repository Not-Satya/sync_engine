import { useState } from 'react';

export default function NotesList({ notes, onAddNote }) {
  const [draft, setDraft] = useState('');

  const submit = (e) => {
    e.preventDefault();
    if (!draft.trim()) return;
    onAddNote(draft.trim());
    setDraft('');
  };

  return (
    <div className="notes-list">
      <form onSubmit={submit}>
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="New note..."
        />
        <button type="submit">Add</button>
      </form>
      <ul>
        {notes.map((n) => (
          <li key={n.id}>{n.text}</li>
        ))}
      </ul>
    </div>
  );
}
