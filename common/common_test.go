// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		name  string
		input any
		want  any
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "simple string",
			input: "hello",
			want:  "HELLO",
		},
		{
			name:  "slice of strings",
			input: []string{"foo", "bar"},
			want:  []string{"FOO", "BAR"},
		},
		{
			name:  "array of strings",
			input: [3]string{"a", "b", "c"},
			want:  [3]string{"A", "B", "C"},
		},
		{
			name:  "array of bytes",
			input: [4]byte{1, 2, 3, 4},
			want:  [4]byte{1, 2, 3, 4},
		},
		{
			name:  "array of ints",
			input: [3]int{10, 20, 30},
			want:  [3]int{10, 20, 30},
		},
		{
			name: "map with strings and arrays",
			input: map[string][2]string{
				"key1": {"val1", "val2"},
			},
			want: map[string][2]string{
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
			want: sampleStruct{
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
			want: []sampleStruct{
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
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DeepCopyMutateStrings(%v) = %v; want %v", tc.input, got, tc.want)
			}
		})
	}
}
