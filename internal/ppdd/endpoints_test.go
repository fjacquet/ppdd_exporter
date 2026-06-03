package ppdd

import "testing"

func TestMTreeStatsPath(t *testing.T) {
	got := mtreeStatsPath("%2Fdata%2Fcol1%2Fbackup1")
	want := "/rest/v2.0/dd-systems/0/mtrees/%2Fdata%2Fcol1%2Fbackup1/stats/capacity"
	if got != want {
		t.Fatalf("mtreeStatsPath = %q, want %q", got, want)
	}
}
