package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
)

const requestJournalName = ".pending-assignment.json"

type requestJournal struct {
	RequestID uuid.UUID `json:"request_id"`
	Resolved  bool      `json:"resolved"`
}

func (w *WorkspaceLock) readRequestJournal() (*requestJournal, error) {
	root, err := os.OpenRoot(w.path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(requestJournalName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return nil, err
	}
	if len(data) > 4096 {
		return nil, errors.New("assignment request journal is too large")
	}
	var journal requestJournal
	if err := json.Unmarshal(data, &journal); err != nil || journal.RequestID == uuid.Nil {
		return nil, errors.New("assignment request journal is invalid")
	}
	return &journal, nil
}

func (w *WorkspaceLock) writeRequestJournal(journal requestJournal) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return writeAtomicContextFile(w.path, requestJournalName, data)
}

// PendingRequest persists a request ID before network transmission and reuses
// it after a lost response or process restart. The workspace lock must remain
// held until the response has been reconciled and durably recorded.
func (w *WorkspaceLock) PendingRequest() (uuid.UUID, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return uuid.Nil, errors.New("worker workspace lock is closed")
	}
	journal, err := w.readRequestJournal()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return uuid.Nil, fmt.Errorf("read assignment request journal: %w", err)
	}
	if err == nil && !journal.Resolved {
		return journal.RequestID, nil
	}
	request := requestJournal{RequestID: uuid.New()}
	if err := w.writeRequestJournal(request); err != nil {
		return uuid.Nil, fmt.Errorf("persist assignment request: %w", err)
	}
	return request.RequestID, nil
}

// ResolveRequest is safe only after an empty result or after the assignment and
// capability have been committed to the durable job context.
func (w *WorkspaceLock) ResolveRequest(id uuid.UUID) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return errors.New("worker workspace lock is closed")
	}
	journal, err := w.readRequestJournal()
	if err != nil {
		return fmt.Errorf("read assignment request journal: %w", err)
	}
	if id == uuid.Nil || journal.RequestID != id {
		return errors.New("assignment request journal identity mismatch")
	}
	if journal.Resolved {
		return nil
	}
	journal.Resolved = true
	if err := w.writeRequestJournal(*journal); err != nil {
		return fmt.Errorf("resolve assignment request journal: %w", err)
	}
	return nil
}
