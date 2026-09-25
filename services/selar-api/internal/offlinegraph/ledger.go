// Package offlinegraph models reviewed, source-scoped graph assertions entirely in memory.
// It does not read or write the live graph, learner state, or study data.
package offlinegraph

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalid        = errors.New("invalid assertion or action")
	ErrStale          = errors.New("stale graph revision")
	ErrNotActive      = errors.New("assertion not active")
	ErrDuplicate      = errors.New("assertion ID already used")
	ErrInvalidPreview = errors.New("invalid correction preview")
)

type Support struct{ DocumentID, ChunkID string }

type Assertion struct {
	ID, SourceID, From, Relation, To string
	FromSupport, ToSupport           Support
}

type EventKind string

const (
	Corrected  EventKind = "corrected"
	Retracted  EventKind = "retracted"
	RolledBack EventKind = "rolled_back"
)

type Event struct {
	Revision       uint64
	Kind           EventKind
	Before, After  Assertion
	TargetRevision uint64
	Reason         string
}

type Snapshot struct {
	Revision uint64
	Active   []Assertion
	History  []Event
}

type Preview struct {
	BaseRevision  uint64
	Before, After Assertion
	seal          [32]byte
}

type Ledger struct {
	key     [32]byte
	initial []Assertion
	active  map[string]Assertion
	history []Event
	used    map[string]bool
}

func validate(a Assertion) error {
	for _, value := range []string{a.ID, a.SourceID, a.From, a.To, a.Relation, a.FromSupport.DocumentID, a.FromSupport.ChunkID, a.ToSupport.DocumentID, a.ToSupport.ChunkID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: missing identifier or source support", ErrInvalid)
		}
	}
	return nil
}

func NewLedger(initial []Assertion) (*Ledger, error) {
	l := &Ledger{active: make(map[string]Assertion), used: make(map[string]bool)}
	if _, err := rand.Read(l.key[:]); err != nil {
		return nil, err
	}
	for _, a := range initial {
		if err := validate(a); err != nil {
			return nil, err
		}
		if l.used[a.ID] {
			return nil, ErrDuplicate
		}
		l.initial = append(l.initial, a)
		l.active[a.ID] = a
		l.used[a.ID] = true
	}
	return l, nil
}

func (l *Ledger) Snapshot() Snapshot {
	s := Snapshot{Revision: uint64(len(l.history)), Active: make([]Assertion, 0, len(l.active)), History: append([]Event{}, l.history...)}
	for _, a := range l.active {
		s.Active = append(s.Active, a)
	}
	sort.Slice(s.Active, func(i, j int) bool { return s.Active[i].ID < s.Active[j].ID })
	return s
}

func (l *Ledger) PreviewCorrection(id string, replacement Assertion) (Preview, error) {
	before, ok := l.active[id]
	if !ok {
		return Preview{}, ErrNotActive
	}
	if err := validate(replacement); err != nil {
		return Preview{}, err
	}
	if replacement.SourceID != before.SourceID {
		return Preview{}, fmt.Errorf("%w: source scope changed", ErrInvalid)
	}
	if l.used[replacement.ID] {
		return Preview{}, ErrDuplicate
	}
	p := Preview{BaseRevision: uint64(len(l.history)), Before: before, After: replacement}
	p.seal = l.sealPreview(p)
	return p, nil
}

func (l *Ledger) sealPreview(p Preview) [32]byte {
	data, _ := json.Marshal(struct {
		Revision      uint64
		Before, After Assertion
	}{p.BaseRevision, p.Before, p.After})
	mac := hmac.New(sha256.New, l.key[:])
	_, _ = mac.Write(data)
	var seal [32]byte
	copy(seal[:], mac.Sum(nil))
	return seal
}

func (l *Ledger) Retract(id string, expectedRevision uint64, reason string) error {
	if expectedRevision != uint64(len(l.history)) {
		return ErrStale
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: retraction needs a reason", ErrInvalid)
	}
	before, ok := l.active[id]
	if !ok {
		return ErrNotActive
	}
	delete(l.active, id)
	l.history = append(l.history, Event{Revision: expectedRevision + 1, Kind: Retracted, Before: before, Reason: reason})
	return nil
}

// SnapshotAt reduces the immutable action log to a historical revision.
func (l *Ledger) SnapshotAt(revision uint64) (Snapshot, error) {
	if revision > uint64(len(l.history)) {
		return Snapshot{}, fmt.Errorf("%w: unknown revision", ErrInvalid)
	}
	states := make([]map[string]Assertion, revision+1)
	states[0] = make(map[string]Assertion, len(l.initial))
	for _, a := range l.initial {
		states[0][a.ID] = a
	}
	for i := uint64(0); i < revision; i++ {
		event := l.history[i]
		previous := states[i]
		if event.Kind == RolledBack {
			previous = states[event.TargetRevision]
		}
		current := make(map[string]Assertion, len(previous))
		for id, a := range previous {
			current[id] = a
		}
		switch event.Kind {
		case Corrected:
			delete(current, event.Before.ID)
			current[event.After.ID] = event.After
		case Retracted:
			delete(current, event.Before.ID)
		case RolledBack:
		default:
			return Snapshot{}, fmt.Errorf("%w: unknown event", ErrInvalid)
		}
		states[i+1] = current
	}
	s := Snapshot{Revision: revision, Active: make([]Assertion, 0, len(states[revision])), History: append([]Event{}, l.history[:revision]...)}
	for _, a := range states[revision] {
		s.Active = append(s.Active, a)
	}
	sort.Slice(s.Active, func(i, j int) bool { return s.Active[i].ID < s.Active[j].ID })
	return s, nil
}

// Rollback appends a compensating action; neither source assertions nor past events are edited.
func (l *Ledger) Rollback(targetRevision, expectedRevision uint64, reason string) error {
	if expectedRevision != uint64(len(l.history)) {
		return ErrStale
	}
	if targetRevision >= expectedRevision || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: rollback needs an earlier revision and reason", ErrInvalid)
	}
	target, err := l.SnapshotAt(targetRevision)
	if err != nil {
		return err
	}
	l.active = make(map[string]Assertion, len(target.Active))
	for _, a := range target.Active {
		l.active[a.ID] = a
	}
	l.history = append(l.history, Event{Revision: expectedRevision + 1, Kind: RolledBack, TargetRevision: targetRevision, Reason: reason})
	return nil
}

func (l *Ledger) ConfirmCorrection(preview Preview, expectedRevision uint64) error {
	if expectedRevision != uint64(len(l.history)) || preview.BaseRevision != expectedRevision {
		return ErrStale
	}
	if preview.seal != l.sealPreview(preview) {
		return ErrInvalidPreview
	}
	current, err := l.PreviewCorrection(preview.Before.ID, preview.After)
	if err != nil {
		return err
	}
	if current != preview {
		return ErrInvalidPreview
	}
	delete(l.active, preview.Before.ID)
	l.active[preview.After.ID] = preview.After
	l.used[preview.After.ID] = true
	l.history = append(l.history, Event{Revision: expectedRevision + 1, Kind: Corrected, Before: preview.Before, After: preview.After})
	return nil
}
