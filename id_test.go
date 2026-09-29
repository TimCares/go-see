package see

import (
	"slices"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", name)
		}
	}()
	fn()
}

func TestNewIDAcceptsTheGrammar(t *testing.T) {
	for _, name := range []string{
		"test.id.valid",
		"test.id.with_underscore",
		"test.id.v2",
	} {
		if got := NewID(name).String(); got != name {
			t.Errorf("NewID(%q).String() = %q", name, got)
		}
	}
}

func TestNewIDRejectsInvalidIDs(t *testing.T) {
	for _, name := range []string{
		"",
		"single",
		"test.Upper",
		"test..empty",
		"test.trailing.",
		".test.leading",
		"test.1digit",
		"test.with-dash",
		"test.with space",
		"test.ümlaut",
		"see.reserved",
	} {
		mustPanic(t, "NewID("+name+")", func() { NewID(name) })
	}
}

func TestNewIDRejectsDuplicates(t *testing.T) {
	NewID("test.id.duplicate")
	mustPanic(t, "registering twice", func() { NewID("test.id.duplicate") })
}

func TestNewIDRejectsPrefixes(t *testing.T) {
	NewID("test.prefix.leaf")

	mustPanic(t, "registering a parent", func() { NewID("test.prefix") })
	mustPanic(t, "registering a child", func() { NewID("test.prefix.leaf.child") })

	// Sharing a string prefix without sharing a segment is fine.
	NewID("test.prefix.leaf_sibling")
}

func TestIDsIsSortedAndComplete(t *testing.T) {
	id := NewID("test.ids.listed")

	ids := IDs()
	if !slices.Contains(ids, id) {
		t.Errorf("IDs() is missing %s", id)
	}
	if !slices.Contains(ids, measureErrorID) {
		t.Errorf("IDs() is missing the reserved %s", measureErrorID)
	}
	if !slices.IsSortedFunc(ids, func(a, b ID) int { return strings.Compare(a.name, b.name) }) {
		t.Errorf("IDs() is not sorted: %v", ids)
	}
}
