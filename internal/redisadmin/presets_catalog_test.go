package redisadmin

import (
	"strings"
	"testing"
)

func TestNamedPresetsCatalogOrderAndCommands(t *testing.T) {
	got := NamedPresets()
	if got == nil {
		t.Fatal("NamedPresets returned nil")
	}
	want := []NamedPreset{
		{Preset: PresetCacheReadWrite, Commands: inspectCacheReadWrite},
		{Preset: PresetReadOnly, Commands: inspectReadOnly},
		{Preset: PresetQueueWorker, QueueKind: QueueLists, Commands: inspectQueueLists},
		{Preset: PresetQueueWorker, QueueKind: QueueStreams, Commands: inspectQueueStreams},
		{Preset: PresetQueueWorker, QueueKind: QueueSortedSets, Commands: inspectQueueSortedSets},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Preset != want[i].Preset || got[i].QueueKind != want[i].QueueKind {
			t.Fatalf("[%d] labels = %#v want %#v", i, got[i], want[i])
		}
		if got[i].Preset == PresetCustom {
			t.Fatalf("[%d] custom in catalog", i)
		}
		if got[i].Commands == nil {
			t.Fatalf("[%d] commands nil", i)
		}
		if !equalSet(got[i].Commands, want[i].Commands) {
			t.Fatalf("[%d] commands mismatch", i)
		}
		for j := 1; j < len(got[i].Commands); j++ {
			if got[i].Commands[j-1] >= got[i].Commands[j] {
				t.Fatalf("[%d] commands not unique-sorted: %#v", i, got[i].Commands)
			}
			if got[i].Commands[j] != strings.ToLower(got[i].Commands[j]) {
				t.Fatalf("[%d] command not lowercase: %q", i, got[i].Commands[j])
			}
		}
	}
}
