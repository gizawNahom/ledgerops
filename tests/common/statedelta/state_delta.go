// Package statedelta is the Go port of the nWave state-delta assertion
// contract. Bootstrapped 2026-08-18 during the first DISTILL wave in this
// project, per nw-distill § Polyglot bootstrap (apply-if-absent).
//
// The contract is identical across languages: every state-mutating test at
// layers 1-3 calls AssertStateDelta(before, after, universe, expected).
// The universe declares the observable, port-exposed names the test is willing
// to reason about. Anything in the universe that expected does not mention MUST
// be unchanged — the assertion is fail-closed, so an unforeseen side effect is
// a failure rather than a silence.
//
// Universe keys are port-exposed observable names ("account.wallet_a.balance",
// "entries.count"), never internal struct fields. A universe naming private
// state couples the test to implementation and reds on a rename.
package statedelta

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Snapshot is a capture of the observable universe at one point in time.
// Keys are port-exposed names; values are whatever the port exposes.
type Snapshot map[string]any

// Predicate decides whether the transition from before to after is the one the
// test expected for a single universe key. It returns a human-readable reason
// when it is not.
type Predicate interface {
	Check(before, after any) (ok bool, reason string)
	Describe() string
}

type predicate struct {
	describe string
	check    func(before, after any) (bool, string)
}

func (p predicate) Check(before, after any) (bool, string) { return p.check(before, after) }
func (p predicate) Describe() string                       { return p.describe }

// SetTo asserts the key holds want after the action, whatever it held before.
func SetTo(want any) Predicate {
	return predicate{
		describe: fmt.Sprintf("set to %v", want),
		check: func(_, after any) (bool, string) {
			if equal(after, want) {
				return true, ""
			}
			return false, fmt.Sprintf("expected %v, got %v", want, after)
		},
	}
}

// Unchanged asserts the key holds the same value it held before.
func Unchanged() Predicate {
	return predicate{
		describe: "unchanged",
		check: func(before, after any) (bool, string) {
			if equal(before, after) {
				return true, ""
			}
			return false, fmt.Sprintf("expected it to stay %v, but it became %v", before, after)
		},
	}
}

// AppendedWith asserts the key is a slice that gained exactly want, in order,
// at the end, and lost nothing.
func AppendedWith(want ...any) Predicate {
	return predicate{
		describe: fmt.Sprintf("appended with %v", want),
		check: func(before, after any) (bool, string) {
			b, okB := toSlice(before)
			a, okA := toSlice(after)
			if !okB || !okA {
				return false, fmt.Sprintf("appendedWith needs slices, got %T then %T", before, after)
			}
			if len(a) != len(b)+len(want) {
				return false, fmt.Sprintf("expected %d items, got %d", len(b)+len(want), len(a))
			}
			for i := range b {
				if !equal(a[i], b[i]) {
					return false, fmt.Sprintf("existing item %d changed from %v to %v", i, b[i], a[i])
				}
			}
			for i, w := range want {
				if !equal(a[len(b)+i], w) {
					return false, fmt.Sprintf("appended item %d: expected %v, got %v", i, w, a[len(b)+i])
				}
			}
			return true, ""
		},
	}
}

// PrependedWith asserts the key is a slice that gained exactly want, in order,
// at the front, and lost nothing.
func PrependedWith(want ...any) Predicate {
	return predicate{
		describe: fmt.Sprintf("prepended with %v", want),
		check: func(before, after any) (bool, string) {
			b, okB := toSlice(before)
			a, okA := toSlice(after)
			if !okB || !okA {
				return false, fmt.Sprintf("prependedWith needs slices, got %T then %T", before, after)
			}
			if len(a) != len(b)+len(want) {
				return false, fmt.Sprintf("expected %d items, got %d", len(b)+len(want), len(a))
			}
			for i, w := range want {
				if !equal(a[i], w) {
					return false, fmt.Sprintf("prepended item %d: expected %v, got %v", i, w, a[i])
				}
			}
			for i := range b {
				if !equal(a[len(want)+i], b[i]) {
					return false, fmt.Sprintf("existing item %d changed from %v to %v", i, b[i], a[len(want)+i])
				}
			}
			return true, ""
		},
	}
}

// Containing asserts the key's value contains want — substring for strings,
// membership for slices.
func Containing(want any) Predicate {
	return predicate{
		describe: fmt.Sprintf("containing %v", want),
		check: func(_, after any) (bool, string) {
			if s, ok := after.(string); ok {
				w, isStr := want.(string)
				if isStr && strings.Contains(s, w) {
					return true, ""
				}
				return false, fmt.Sprintf("%q does not contain %v", s, want)
			}
			items, ok := toSlice(after)
			if !ok {
				return false, fmt.Sprintf("containing needs a string or slice, got %T", after)
			}
			for _, item := range items {
				if equal(item, want) {
					return true, ""
				}
			}
			return false, fmt.Sprintf("%v does not contain %v", after, want)
		},
	}
}

// NormalizedTo asserts the key was rewritten into a canonical form: the value
// after equals want, and it differed before. Use when the point of the action
// is the normalization itself.
func NormalizedTo(want any) Predicate {
	return predicate{
		describe: fmt.Sprintf("normalized to %v", want),
		check: func(before, after any) (bool, string) {
			if !equal(after, want) {
				return false, fmt.Sprintf("expected normalized %v, got %v", want, after)
			}
			if equal(before, after) {
				return false, fmt.Sprintf("value was already %v — nothing was normalized", want)
			}
			return true, ""
		},
	}
}

// IdempotentAfter asserts the key already held want before the action and still
// does. This is the shape a replay assertion takes: the second application
// changed nothing precisely because the first had already done the work.
func IdempotentAfter(want any) Predicate {
	return predicate{
		describe: fmt.Sprintf("idempotent after %v", want),
		check: func(before, after any) (bool, string) {
			if !equal(before, want) {
				return false, fmt.Sprintf("expected %v before the repeat, got %v", want, before)
			}
			if !equal(after, want) {
				return false, fmt.Sprintf("repeat changed it from %v to %v", want, after)
			}
			return true, ""
		},
	}
}

// LegacyHealed asserts a value that was in a known-bad state before is in the
// wanted state after. Distinct from SetTo because it also asserts the bad
// starting point — a test that never had the defect proves nothing about a fix.
func LegacyHealed(from, to any) Predicate {
	return predicate{
		describe: fmt.Sprintf("legacy healed from %v to %v", from, to),
		check: func(before, after any) (bool, string) {
			if !equal(before, from) {
				return false, fmt.Sprintf("expected the legacy value %v before, got %v", from, before)
			}
			if !equal(after, to) {
				return false, fmt.Sprintf("expected %v after healing, got %v", to, after)
			}
			return true, ""
		},
	}
}

// AssertStateDelta is the universe guard. It fails when:
//   - expected names a key outside the universe (the test is asserting on
//     something it did not declare it would observe);
//   - a universe key is missing from either snapshot;
//   - an expected predicate does not hold;
//   - a universe key with no expectation changed anyway (fail-closed).
func AssertStateDelta(t testing.TB, before, after Snapshot, universe []string, expected map[string]Predicate) {
	t.Helper()

	declared := make(map[string]bool, len(universe))
	for _, key := range universe {
		declared[key] = true
	}

	var failures []string

	for key := range expected {
		if !declared[key] {
			failures = append(failures, fmt.Sprintf("%s: asserted on but not declared in the universe", key))
		}
	}

	for _, key := range universe {
		valueBefore, okBefore := before[key]
		valueAfter, okAfter := after[key]
		if !okBefore || !okAfter {
			failures = append(failures, fmt.Sprintf("%s: declared in the universe but missing from the %s snapshot", key, missingSide(okBefore, okAfter)))
			continue
		}
		want, asserted := expected[key]
		if !asserted {
			want = Unchanged()
		}
		if ok, reason := want.Check(valueBefore, valueAfter); !ok {
			failures = append(failures, fmt.Sprintf("%s: expected %s — %s", key, want.Describe(), reason))
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		t.Fatalf("state delta violated:\n  %s", strings.Join(failures, "\n  "))
	}
}

func missingSide(okBefore, okAfter bool) string {
	switch {
	case !okBefore && !okAfter:
		return "before and after"
	case !okBefore:
		return "before"
	default:
		return "after"
	}
}

func equal(a, b any) bool { return reflect.DeepEqual(a, b) }

func toSlice(v any) ([]any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}
