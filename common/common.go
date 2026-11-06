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

// Package common implements common functionality for dealing with text/template.
package common

import (
	"io"
	"reflect"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"
	"unicode/utf8"

	"github.com/google/safetext/lockedcallbacks"
	"mvdan.cc/sh/v3/syntax"
)

const textTemplateRemediationFuncName = "ApplyInjectionDetection"

// EchoString is a nop string callback
func EchoString(in string) string {
	return in
}

// BaselineString is a string callback function that just returns a constant string,
// used to get a baseline of how the resultant YAML is structured.
func BaselineString(string) string {
	return "baseline"
}

// The safetext library is executing each template multiple times. The goal is to see differences
// when user's input are used and detect modifications to the structure (e.g. YAML keys).
// text/template have a FuncMap that can be used to add functions called during the execution of
// templates. Yet, those functions cannot be updated once the template has been parsed. As safetext
// is modifying a specific function (to detect injection) during executions, a wrapper was
// introduced and a function pointer is updated for each template.
// statesmap hold those function pointers for each template that is processed by the library
// instance.
var statesmap = lockedcallbacks.New()

// BuildTextTemplateFuncMap generates a per template (using its name as identifier) FuncMap.
// A Virtual callback is created as once Template.Parse() is called this is not something we can
// update. However, in its current design, safetext requires changing the methods as templates are
// executed multiple times and outputs are diffed to detect injection
func BuildTextTemplateFuncMap(safeTmplUUID string) map[string]any {
	templateRemediationFunc := statesmap.BuildTextTemplateRemediationFunc(safeTmplUUID, DeepCopyMutateStrings)
	templateFuncMap := map[string]any{
		textTemplateRemediationFuncName: templateRemediationFunc,
		"StructuralData":                func(data any) any { return data },

		// Sh specific callbacks
		"AllowFlags": statesmap.BuildAllowFlagsCallbackFunc(safeTmplUUID, DeepCopyMutateStrings),
	}
	return templateFuncMap
}

// ExecuteWithCallback performs an execution on a callback-applied template
// (WalkApplyFuncToNonDeclaractiveActions) with a specified callback.
func ExecuteWithCallback(tmpl *template.Template, safeTmplUUID string, cb func(string) string, result io.Writer, data any) error {
	return statesmap.SetAndExecuteWithCallback(tmpl, safeTmplUUID, cb, result, data)
}

// ExecuteWithShCallback is like ExecuteWithCallback, but with the additional sh-specific callbacks specified.
func ExecuteWithShCallback(tmpl *template.Template, safeTmplUUID string, cb func(string) string, allowFlagsCb func(string) string, result io.Writer, data any) error {
	return statesmap.SetAndExecuteWithShCallback(tmpl, safeTmplUUID, cb, allowFlagsCb, result, data)
}

func makePointer(data any) any {
	rtype := reflect.New(reflect.TypeOf(data))
	rtype.Elem().Set(reflect.ValueOf(data))
	return rtype.Interface()
}

func dereference(data any) any {
	return reflect.ValueOf(data).Elem().Interface()
}

// DeepCopyMutateStrings performs a deep copy, but mutates any strings according to the mutation callback.
func DeepCopyMutateStrings(data any, mutateF func(string) string) any {
	var r any

	if data == nil {
		return nil
	}

	value := reflect.ValueOf(data)
	valueType := reflect.TypeOf(data)

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			r = data
		} else {
			c := DeepCopyMutateStrings(dereference(data), mutateF)
			r = makePointer(c)

			// Sometimes we accidentally introduce one too many layers of indirection (seems related to protobuf generated fields like ReleaseNamespace *ReleaseNamespace `... reflect:"unexport"`)
			if reflect.TypeOf(r) != valueType {
				r = c
			}
		}
	case reflect.String:
		mutated := mutateF(value.String())
		return reflect.ValueOf(mutated).Convert(valueType).Interface()
	case reflect.Slice, reflect.Array:
		var rc reflect.Value
		if value.Kind() == reflect.Slice {
			if value.IsNil() {
				return data
			}
			rc = reflect.MakeSlice(valueType, value.Len(), value.Len())
		} else {
			rc = reflect.New(valueType).Elem()
		}
		for i := range value.Len() {
			rc.Index(i).Set(reflect.ValueOf(DeepCopyMutateStrings(value.Index(i).Interface(), mutateF)))
		}
		r = rc.Interface()
	case reflect.Map:
		rc := reflect.MakeMap(valueType)
		dataIter := value.MapRange()
		for dataIter.Next() {
			rc.SetMapIndex(dataIter.Key(), reflect.ValueOf(DeepCopyMutateStrings(dataIter.Value().Interface(), mutateF)))
		}
		r = rc.Interface()
	case reflect.Struct:
		s := reflect.New(valueType)

		n := value.NumField()
		for i := range n {
			r, _ := utf8.DecodeRuneInString(valueType.Field(i).Name)

			// Don't copy unexported fields
			if unicode.IsUpper(r) {
				reflect.Indirect(s).Field(i).Set(
					reflect.ValueOf(DeepCopyMutateStrings(value.Field(i).Interface(), mutateF)),
				)
			}
		}

		r = reflect.Indirect(s).Interface()
	default:
		// No other types need special handling (int, bool, etc)
		r = data
	}

	return r
}

func applyPipeCmds(cmds []*parse.CommandNode) {
	for _, c := range cmds {
		newArgs := make([]parse.Node, 0)
		for i, a := range c.Args {
			switch a := a.(type) {
			case *parse.DotNode, *parse.FieldNode, *parse.VariableNode, *parse.ChainNode:
				if i == 0 && len(c.Args) > 1 {
					// If this is the first "argument" of multiple, then it is really a function
					newArgs = append(newArgs, a)
				} else {
					// If this node is an argument to a call to "StructuralData", then pass it through as-is
					switch identifier := c.Args[0].(type) {
					case *parse.IdentifierNode:
						if identifier.Ident == "StructuralData" {
							newArgs = append(newArgs, a)
							continue
						}
					}

					newPipe := &parse.PipeNode{NodeType: parse.NodePipe, Decl: nil}
					newPipe.Cmds = []*parse.CommandNode{
						&parse.CommandNode{NodeType: parse.NodeCommand, Args: []parse.Node{a}},
						&parse.CommandNode{NodeType: parse.NodeCommand, Args: []parse.Node{
							&parse.IdentifierNode{
								NodeType: parse.NodeIdentifier,
								Ident:    textTemplateRemediationFuncName,
							},
						}},
					}
					newArgs = append(newArgs, newPipe)
				}
			case *parse.PipeNode:
				applyPipeCmds(a.Cmds)
				newArgs = append(newArgs, a)
			default:
				newArgs = append(newArgs, a)
			}
		}

		c.Args = newArgs
	}
}

func branchNode(node parse.Node) *parse.BranchNode {
	switch node := node.(type) {
	case *parse.IfNode:
		return &node.BranchNode
	case *parse.RangeNode:
		return &node.BranchNode
	case *parse.WithNode:
		return &node.BranchNode
	}

	return nil
}

// WalkApplyFuncToNonDeclaractiveActions walks the AST, applying a pipeline function to any "paste" nodes (non-declarative action nodes)
func WalkApplyFuncToNonDeclaractiveActions(template *template.Template, node parse.Node) {
	switch node := node.(type) {
	case *parse.ActionNode:
		// Non-declarative actions are paste actions
		if len(node.Pipe.Decl) == 0 {
			applyPipeCmds(node.Pipe.Cmds)
		}

	case *parse.IfNode, *parse.RangeNode, *parse.WithNode:
		nodeBranch := branchNode(node)
		WalkApplyFuncToNonDeclaractiveActions(template, nodeBranch.List)
		if nodeBranch.ElseList != nil {
			WalkApplyFuncToNonDeclaractiveActions(template, nodeBranch.ElseList)
		}
	case *parse.ListNode:
		for _, node := range node.Nodes {
			WalkApplyFuncToNonDeclaractiveActions(template, node)
		}
	case *parse.TemplateNode:
		tmpl := template.Lookup(node.Name)
		if tmpl != nil {
			treeCopy := tmpl.Tree.Copy()
			WalkApplyFuncToNonDeclaractiveActions(tmpl, treeCopy.Root)
			template.AddParseTree(node.Name, treeCopy)
		}
	}
}

// removeSingleBackslash removes the backslash escaping that bash strips from an
// unquoted literal during quote removal (e.g. `\-rf` becomes `-rf`).
func removeSingleBackslash(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}

	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		sb.WriteByte(s[i])
	}

	return sb.String()
}

// SplitBracesInPlace rewrites the brace expansions (e.g. `{a,b}`) of words in
// AST positions where bash performs brace expansion into BraceExp nodes, so
// that they are compared structurally. The parser leaves them as literals
// otherwise.
func SplitBracesInPlace(f *syntax.File) {
	// syntax.Walk does not support BraceExp nodes, so collect all words before
	// modifying any of them.
	var words []*syntax.Word
	syntax.Walk(f, func(node syntax.Node) bool {
		switch x := node.(type) {
		case *syntax.CallExpr:
			words = append(words, x.Args...)
		case *syntax.WordIter:
			words = append(words, x.Items...)
		case *syntax.ArrayElem:
			if x.Index == nil && x.Value != nil {
				words = append(words, x.Value)
			}
		case *syntax.DeclClause:
			for _, a := range x.Args {
				if a.Value != nil {
					words = append(words, a.Value)
				}
			}
		case *syntax.Redirect:
			if x.Word != nil && x.Op != syntax.Hdoc && x.Op != syntax.DashHdoc && x.Op != syntax.WordHdoc {
				words = append(words, x.Word)
			}
		}
		return true
	})

	for _, w := range words {
		if syntax.SplitBraces(w) {
			w.Parts = mergeAdjacentLits(w.Parts)
		}
	}
}

// mergeAdjacentLits merges consecutive literals. syntax.SplitBraces splits
// literals around braces even when they do not expand (e.g. `{foo}` becomes
// `{`, `foo`, `}`), which would otherwise change the number of word parts.
func mergeAdjacentLits(parts []syntax.WordPart) []syntax.WordPart {
	var merged []syntax.WordPart
	for _, p := range parts {
		if be, ok := p.(*syntax.BraceExp); ok {
			for _, elem := range be.Elems {
				elem.Parts = mergeAdjacentLits(elem.Parts)
			}
		}

		lit, ok := p.(*syntax.Lit)
		if !ok || len(merged) == 0 {
			merged = append(merged, p)
			continue
		}

		if prev, ok := merged[len(merged)-1].(*syntax.Lit); ok {
			merged[len(merged)-1] = &syntax.Lit{ValuePos: prev.ValuePos, ValueEnd: lit.ValueEnd, Value: prev.Value + lit.Value}
		} else {
			merged = append(merged, p)
		}
	}

	return merged
}

// CanStartWithDash reports whether any of the arguments bash produces from a
// word (after brace expansion and quote removal) may start with '-'.
func CanStartWithDash(parts []syntax.WordPart) bool {
	_, dash := leadingDash(parts, false)
	return dash
}

// leadingDash determines whether the first character of the given word parts
// may be '-'. decided is false if all parts may expand to the empty string, in
// which case the first character is determined by whatever follows.
func leadingDash(parts []syntax.WordPart, quoted bool) (decided, dash bool) {
	for i, p := range parts {
		switch p := p.(type) {
		case *syntax.Lit:
			v := p.Value
			if !quoted {
				// Bash strips backslashes from unquoted literals, so `\-rf` is passed as `-rf`.
				v = removeSingleBackslash(v)
			}
			if v != "" {
				return true, v[0] == '-'
			}
		case *syntax.SglQuoted:
			if p.Value != "" {
				// In $'...' strings, escape sequences such as `\x2d` may produce a '-'.
				return true, p.Value[0] == '-' || (p.Dollar && p.Value[0] == '\\')
			}
		case *syntax.DblQuoted:
			if decided, dash := leadingDash(p.Parts, true); decided {
				return true, dash
			}
		case *syntax.BraceExp:
			// Each element starts a separate argument, followed by the rest of the word.
			rest := parts[i+1:]
			for _, elem := range p.Elems {
				if _, dash := leadingDash(slices.Concat(elem.Parts, rest), quoted); dash {
					return true, true
				}
			}
			return true, false
		default:
			// Parameter expansions, command substitutions etc. are defined by the
			// template, but may expand to the empty string, so whatever follows may
			// end up at the start of the argument.
		}
	}

	return false, false
}
