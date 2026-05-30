package ppdd

import "testing"

func TestSnapshotStoreLoadEmpty(t *testing.T) {
	st := NewSnapshotStore()
	if st.Load() == nil {
		t.Fatal("Load() on fresh store must return a non-nil empty snapshot")
	}
	if n := len(st.Load().Systems); n != 0 {
		t.Fatalf("fresh snapshot has %d systems, want 0", n)
	}
}

func TestSnapshotStoreStoreThenLoad(t *testing.T) {
	st := NewSnapshotStore()
	snap := &Snapshot{Systems: []*SystemSnapshot{{System: "dd01", OK: true}}}
	st.Store(snap)
	got := st.Load()
	if len(got.Systems) != 1 || got.Systems[0].System != "dd01" {
		t.Fatalf("Load() = %+v, want one system dd01", got)
	}
}
