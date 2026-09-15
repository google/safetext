package common

import (
	"reflect"
	"strings"
	"testing"
)

type sampleStruct struct {
	Name    string
	ID      [4]byte
	Values  [3]string
	Numbers []int
	Nested  *nestedStruct
}

type nestedStruct struct {
	Tag   string
	Flags [2]bool
}

func TestDeepCopyMutateStrings(t *testing.T) {
	mutateUpper := func(s string) string {
		return strings.ToUpper(s)
	}

	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "simple string",
			input:    "hello",
			expected: "HELLO",
		},
		{
			name:     "slice of strings",
			input:    []string{"foo", "bar"},
			expected: []string{"FOO", "BAR"},
		},
		{
			name:     "array of strings",
			input:    [3]string{"a", "b", "c"},
			expected: [3]string{"A", "B", "C"},
		},
		{
			name:     "array of bytes",
			input:    [4]byte{1, 2, 3, 4},
			expected: [4]byte{1, 2, 3, 4},
		},
		{
			name:     "array of ints",
			input:    [3]int{10, 20, 30},
			expected: [3]int{10, 20, 30},
		},
		{
			name: "map with strings and arrays",
			input: map[string][2]string{
				"key1": {"val1", "val2"},
			},
			expected: map[string][2]string{
				"key1": {"VAL1", "VAL2"},
			},
		},
		{
			name: "struct with array and slice fields",
			input: sampleStruct{
				Name:    "test-name",
				ID:      [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
				Values:  [3]string{"first", "second", "third"},
				Numbers: []int{1, 2, 3},
				Nested: &nestedStruct{
					Tag:   "nested-tag",
					Flags: [2]bool{true, false},
				},
			},
			expected: sampleStruct{
				Name:    "TEST-NAME",
				ID:      [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
				Values:  [3]string{"FIRST", "SECOND", "THIRD"},
				Numbers: []int{1, 2, 3},
				Nested: &nestedStruct{
					Tag:   "NESTED-TAG",
					Flags: [2]bool{true, false},
				},
			},
		},
		{
			name: "slice of structs with fixed-size arrays",
			input: []sampleStruct{
				{
					Name:   "item-1",
					ID:     [4]byte{1, 2, 3, 4},
					Values: [3]string{"x", "y", "z"},
				},
			},
			expected: []sampleStruct{
				{
					Name:   "ITEM-1",
					ID:     [4]byte{1, 2, 3, 4},
					Values: [3]string{"X", "Y", "Z"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeepCopyMutateStrings(tc.input, mutateUpper)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("DeepCopyMutateStrings(%v) = %v; want %v", tc.input, got, tc.expected)
			}
		})
	}
}
