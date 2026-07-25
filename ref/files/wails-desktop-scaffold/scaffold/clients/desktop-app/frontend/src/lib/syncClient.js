// Wraps the auto-generated bindings in wailsjs/go/main/App.
//
// That file doesn't exist until you've run `wails dev` at least once —
// Wails inspects app.go's exported methods and generates a matching JS
// function for each one. Nothing here is a network call: it's an
// in-process call into the same Go binary this UI is running inside.
import {
  CreateNote as _CreateNote,
  ListNotes as _ListNotes,
  TriggerSync as _TriggerSync,
  GetStatus as _GetStatus,
} from '../../wailsjs/go/main/App';

export const syncClient = {
  createNote: (text) => _CreateNote(text),
  listNotes: () => _ListNotes(),
  triggerSync: () => _TriggerSync(),
  getStatus: () => _GetStatus(),
};
