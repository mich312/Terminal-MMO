package store

import "testing"

func TestUnwritableDBDegradesToNoop(t *testing.T) {
	s := Open("/proc/durst/nope/x.db") // /proc is read-only even for root
	if _, ok := s.(noopStore); !ok {
		t.Fatalf("expected noop fallback, got %T", s)
	}
	v := s.RecordVisit("anna")
	if v.VisitCount != 1 || !v.FirstVisit {
		t.Error("noop store should return sane defaults")
	}
	if err := s.SignGuestbook("anna", "hi"); err != nil {
		t.Error("noop sign should not error")
	}
}
