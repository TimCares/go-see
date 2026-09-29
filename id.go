package see

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// reservedRoot is the first id segment owned by this package.
const reservedRoot = "see"

// ID is the stable, machine readable name of an event.
//
// An ID can only be created through [NewID], which validates it and adds it to the
// process wide registry. The zero ID is invalid and is reported, not logged, by [Emit].
type ID struct {
	name string
}

// String returns the dot separated id, e.g. "storage.artifact.upload".
func (id ID) String() string {
	return id.name
}

// MarshalText lets an ID serialize to its bare string, e.g. as a payload field.
func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.name), nil
}

var registry = struct {
	sync.Mutex
	ids map[string]ID
}{ids: map[string]ID{}}

// NewID validates and registers an event id, and panics if it is not a valid one.
//
// It is meant to be called once per id in a package level var, like
// [regexp.MustCompile], so a broken vocabulary fails at startup instead of silently
// producing unqueryable records.
//
// An id is at least two segments joined by ".". Each segment is lowercase ascii
// letters, digits and underscores, and starts with a letter. No id may be a strict
// prefix of another id: hierarchical namespaces are encouraged, but all event ids
// should be leaves. Concrete success events should end in ".success", concrete
// failures in ".error".
func NewID(name string) ID {
	if root, _, _ := strings.Cut(name, "."); root == reservedRoot {
		panic(fmt.Sprintf("see: id %q uses the reserved root %q", name, reservedRoot))
	}
	return register(name)
}

// IDs returns every registered id, sorted.
//
// Useful to pre create every metric series at 0, so dashboards and alerts have data
// before the first occurrence.
func IDs() []ID {
	registry.Lock()
	defer registry.Unlock()

	ids := make([]ID, 0, len(registry.ids))
	for _, id := range registry.ids {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b ID) int { return strings.Compare(a.name, b.name) })
	return ids
}

// register adds an id to the registry without the reserved root check.
func register(name string) ID {
	if err := validate(name); err != nil {
		panic(err.Error())
	}

	registry.Lock()
	defer registry.Unlock()

	if _, ok := registry.ids[name]; ok {
		panic(fmt.Sprintf("see: id %q is already registered", name))
	}
	for existing := range registry.ids {
		if strings.HasPrefix(existing, name+".") || strings.HasPrefix(name, existing+".") {
			panic(fmt.Sprintf("see: id %q and %q are prefixes of one another, ids must be leaves", name, existing))
		}
	}

	id := ID{name: name}
	registry.ids[name] = id
	return id
}

func validate(name string) error {
	segments := strings.Split(name, ".")
	if len(segments) < 2 {
		return fmt.Errorf("see: id %q needs at least two dot separated segments", name)
	}
	for _, segment := range segments {
		if !validSegment(segment) {
			return fmt.Errorf(
				"see: id %q has the invalid segment %q, segments are lowercase ascii letters, digits and underscores, starting with a letter",
				name,
				segment,
			)
		}
	}
	return nil
}

func validSegment(segment string) bool {
	if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
		return false
	}
	for _, r := range segment {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
