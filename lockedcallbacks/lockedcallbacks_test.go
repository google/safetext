// Copyright 2024 Google LLC.
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

package lockedcallbacks

import (
	"io"
	"testing"
	"time"
	"text/template"
)

func TestLockedCallbacksBuildFunc(t *testing.T) {
	wrapper := func(data any, mutateF func(string) string) any {
		return mutateF(data.(string))
	}
	statesmap := New()
	remediationFunc := statesmap.BuildTextTemplateRemediationFunc("foobar", wrapper)
	if remediationFunc == nil {
		t.Errorf("BuildTextTemplateRemediationFunc() error = nil")
	}
}

// Regression test for deadlock in SetAndExecuteWithCallback
func TestSetAndExecuteWithCallback_NoDeadlockOnError(t *testing.T) {
    statesmap := New()
    tmpl := template.New("test").Option("missingkey=error")
    tmpl, err := tmpl.Parse("foo: {{.NoSuchField}}")
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }

    done := make(chan error, 1)
    go func() {
        err := statesmap.SetAndExecuteWithCallback(tmpl, "uuid1", func(s string) string { return s }, io.Discard, map[string]string{"foo": "bar"})
        done <- err
    }()

    select {
    case err := <-done:
        if err == nil {
            t.Errorf("Expected error due to missing key, got nil")
        }
    case <-time.After(3 * time.Second):
        t.Fatal("Deadlock detected: execution did not return")
    }
}

// Regression test for deadlock in SetAndExecuteWithShCallback
func TestSetAndExecuteWithShCallback_NoDeadlockOnError(t *testing.T) {
    statesmap := New()
    tmpl := template.New("test").Option("missingkey=error")
    tmpl, err := tmpl.Parse("foo: {{.NoSuchField}}")
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }

    done := make(chan error, 1)
    go func() {
        err := statesmap.SetAndExecuteWithShCallback(tmpl, "uuid2", func(s string) string { return s }, func(s string) string { return s }, io.Discard, map[string]string{"foo": "bar"})
        done <- err
    }()

    select {
    case err := <-done:
        if err == nil {
            t.Errorf("Expected error due to missing key, got nil")
        }
    case <-time.After(3 * time.Second):
        t.Fatal("Deadlock detected: execution did not return")
    }
}

// TODO(b/297302763)
// Add more tests to validate locking mechanism w/o needing global presubmit TAP to run.
