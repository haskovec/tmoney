package config

import (
	v1 "encoding/json"
	v2 "encoding/json/v2"
	"testing"
)

// TestConfigSurvivesJSONV2 pins Config to the same bytes under the v1 and the
// v2 encoder.
//
// Config is persisted: Save writes it to config.json in the user's config
// directory and Load reads it back, so a change in which keys get emitted is a
// change to a file already on users' disks. Go 1.27 implements encoding/json on
// top of the v2 engine but keeps v1 semantics, so the two agree today only
// because no omitempty tag here sits on a type where they can diverge.
//
// The divergence to guard against is omitempty on a bool, number, or pointer.
// v1 asks whether the Go value is empty and so drops false and 0; v2 asks
// whether the encoded JSON is an empty JSON value, and false and 0 are not, so
// it keeps them. `omitzero` means what v1 meant and behaves the same in both.
//
// The RecentFiles cases below are the reason that field keeps omitempty rather
// than following the bools to omitzero: a non-nil empty slice is empty but is
// not a slice's zero value, so omitzero would start writing "recent_files": []
// for a user who had cleared the list.
func TestConfigSurvivesJSONV2(t *testing.T) {
	cases := []struct {
		name  string
		value Config
	}{
		{"zero", Config{}},
		{"toggles off", Config{ShowClosedPositions: false, ValueAdjustmentNoticeShown: false}},
		{"toggles on", Config{ShowClosedPositions: true, ValueAdjustmentNoticeShown: true}},
		{"populated", Config{
			DefaultFile: "/home/u/personal.tdb",
			RecentFiles: []string{"/home/u/personal.tdb", "/home/u/old.tdb"},
			LastFile:    "/home/u/personal.tdb",
			Theme:       "turbo-vision",
		}},
		{"nil recent files", Config{RecentFiles: nil}},
		{"empty non-nil recent files", Config{RecentFiles: []string{}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := v1.Marshal(tc.value)
			if err != nil {
				t.Fatalf("v1 marshal: %v", err)
			}
			got, err := v2.Marshal(tc.value)
			if err != nil {
				t.Fatalf("v2 marshal: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("encoder disagreement\n  v1: %s\n  v2: %s", want, got)
			}
		})
	}
}
