// Copyright 2023 Google LLC.
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

// Package shtemplate is a drop-in-replacement for using text/template to produce shell scripts, that adds automatic detection for argument/command injection
package shtemplate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode/utf8"

	"github.com/google/safetext/common"

	"mvdan.cc/sh/v3/syntax"
	"github.com/pborman/uuid"
)

// ErrInvalidShTemplate indicates the requested template is not a valid script
var ErrInvalidShTemplate error = errors.New("Invalid Shell Template")

// ErrShInjection indicates the inputs resulted in shell injection.
var ErrShInjection error = errors.New("Shell Injection Detected")

// ExecError is the custom error type returned when Execute has an
// error evaluating its template. (If a write error occurs, the actual
// error is returned; it will not be of type ExecError.)
type ExecError = template.ExecError

// FuncMap is the type of the map defining the mapping from names to functions.
// Each function must have either a single return value, or two return values of
// which the second has type error. In that case, if the second (error)
// return value evaluates to non-nil during execution, execution terminates and
// Execute returns that error.
//
// Errors returned by Execute wrap the underlying error; call errors.As to
// uncover them.
//
// When template execution invokes a function with an argument list, that list
// must be assignable to the function's parameter types. Functions meant to
// apply to arguments of arbitrary type can use parameters of type any or
// of type reflect.Value. Similarly, functions meant to return a result of arbitrary
// type can return any or reflect.Value.
type FuncMap = template.FuncMap

// Template is the representation of a parsed template. The *parse.Tree
// field is exported only for use by html/template and should be treated
// as unexported by all other clients.
type Template struct {
	unsafeTemplate *template.Template
	uuid           string
}

// New allocates a new, undefined template with the given name.
func New(name string) *Template {
	id := uuid.New()
	funcMap := common.BuildTextTemplateFuncMap(id)
	return &Template{unsafeTemplate: template.New(name).Funcs(funcMap), uuid: id}
}

// Mutation algorithm
func mutateString(s string) string {
	out := make([]rune, utf8.RuneCountInString(s)*2)

	i := 0
	for _, r := range s {
		out[i] = r
		i++

		out[i] = r
		i++
	}

	return string(out[:i])
}

func paramExprMatch(a, b, c *syntax.ParamExp) bool {
	if b.Short != a.Short || c.Short != a.Short {
		return false
	}

	if b.Excl != a.Excl || c.Excl != a.Excl {
		return false
	}

	if b.Length != a.Length || c.Length != a.Length {
		return false
	}

	if b.Width != a.Width || c.Width != a.Width {
		return false
	}

	if (b.Param == nil) != (a.Param == nil) || (c.Param == nil) != (a.Param == nil) {
		return false
	}

	if a.Param != nil &&
		(b.Param.Value != a.Param.Value || c.Param.Value != a.Param.Value) {
		return false
	}

	if (b.Index == nil) != (a.Index == nil) || (c.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index, c.Index) {
		return false
	}

	if (b.Slice == nil) != (a.Slice == nil) || (c.Slice == nil) != (a.Slice == nil) {
		return false
	}

	if a.Slice != nil && !slicesMatch(a.Slice, b.Slice, c.Slice) {
		return false
	}

	if (b.Repl == nil) != (a.Repl == nil) || (c.Repl == nil) != (a.Repl == nil) {
		return false
	}

	if a.Repl != nil && !replaceMatch(a.Repl, b.Repl, c.Repl) {
		return false
	}

	if b.Names != a.Names || c.Names != a.Names {
		return false
	}

	if (b.Exp == nil) != (a.Exp == nil) || (c.Exp == nil) != (a.Exp == nil) {
		return false
	}

	if a.Exp != nil && !wordsMatch(a.Exp.Word.Parts, b.Exp.Word.Parts, c.Exp.Word.Parts, matchExactly) {
		return false
	}

	return true
}

func cmdSubstMatch(a, b, c *syntax.CmdSubst) bool {
	if b.Backquotes != a.Backquotes || c.Backquotes != a.Backquotes {
		return false
	}

	if b.TempFile != a.TempFile || c.TempFile != a.TempFile {
		return false
	}

	if b.ReplyVar != a.ReplyVar || c.ReplyVar != a.ReplyVar {
		return false
	}

	if !statementsArrayMatch(a.Stmts, b.Stmts, c.Stmts) {
		return false
	}

	return true
}

func slicesMatch(a, b, c *syntax.Slice) bool {
	return arithmExprMatch(a.Offset, b.Offset, c.Offset) && arithmExprMatch(a.Length, b.Length, c.Length)
}

func replaceMatch(a, b, c *syntax.Replace) bool {
	if b.All != a.All || c.All != a.All {
		return false
	}

	if !wordsMatch(a.Orig.Parts, b.Orig.Parts, c.Orig.Parts, justStructure) {
		return false
	}

	if !wordsMatch(a.With.Parts, b.With.Parts, c.With.Parts, justStructure) {
		return false
	}

	return true
}

func arithmExpMatch(a, b, c syntax.ArithmExp) bool {
	if b.Bracket != a.Bracket || c.Bracket != a.Bracket {
		return false
	}

	if b.Unsigned != a.Unsigned || c.Unsigned != a.Unsigned {
		return false
	}

	return arithmExprMatch(a.X, b.X, c.X)
}

func arithmExprArrayMatch(a, b, c []syntax.ArithmExpr) bool {
	if len(b) != len(a) || len(c) != len(a) {
		return false
	}

	for i := range a {
		if !arithmExprMatch(a[i], b[i], c[i]) {
			return false
		}
	}

	return true
}

func arithmExprMatch(a, b, c syntax.ArithmExpr) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) || reflect.TypeOf(c) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.BinaryArithm:
		if b.(*syntax.BinaryArithm).Op != a.Op || c.(*syntax.BinaryArithm).Op != a.Op {
			return false
		}
		if !arithmExprMatch(a.X, b.(*syntax.BinaryArithm).X, c.(*syntax.BinaryArithm).X) {
			return false
		}
		if !arithmExprMatch(a.Y, b.(*syntax.BinaryArithm).Y, c.(*syntax.BinaryArithm).Y) {
			return false
		}
	case *syntax.UnaryArithm:
		if b.(*syntax.UnaryArithm).Op != a.Op || c.(*syntax.UnaryArithm).Op != a.Op {
			return false
		}
		if b.(*syntax.UnaryArithm).Post != a.Post || c.(*syntax.UnaryArithm).Post != a.Post {
			return false
		}
		if !arithmExprMatch(a.X, b.(*syntax.UnaryArithm).X, c.(*syntax.UnaryArithm).X) {
			return false
		}
	case *syntax.ParenArithm:
		if !arithmExprMatch(a.X, b.(*syntax.ParenArithm).X, c.(*syntax.ParenArithm).X) {
			return false
		}
	case *syntax.Word:
		if !wordsMatch(a.Parts, b.(*syntax.Word).Parts, c.(*syntax.Word).Parts, justStructure) {
			return false
		}
	}

	return true
}

func assignArrayMatch(a, b, c []*syntax.Assign) bool {
	if len(b) != len(a) || len(c) != len(a) {
		return false
	}

	for i := range a {
		if !assignMatch(a[i], b[i], c[i]) {
			return false
		}
	}

	return true
}

func assignMatch(a, b, c *syntax.Assign) bool {
	if b.Append != a.Append || c.Append != a.Append {
		return false
	}

	if b.Naked != a.Naked || c.Naked != a.Naked {
		return false
	}

	if (b.Name == nil) != (a.Name == nil) || (c.Name == nil) != (a.Name == nil) {
		return false
	}

	if a.Name != nil &&
		(b.Name.Value != a.Name.Value || c.Name.Value != a.Name.Value) {
		return false
	}

	if (b.Index == nil) != (a.Index == nil) || (c.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index, c.Index) {
		return false
	}

	if (b.Value == nil) != (a.Value == nil) || (c.Value == nil) != (a.Value == nil) {
		return false
	}

	if a.Value != nil && !wordsMatch(a.Value.Parts, b.Value.Parts, c.Value.Parts, justStructure) {
		return false
	}

	if (b.Array == nil) != (a.Array == nil) || (c.Array == nil) != (a.Array == nil) {
		return false
	}

	if a.Array != nil && !arrayExprMatch(a.Array, b.Array, c.Array) {
		return false
	}

	return true
}

func arrayExprMatch(a, b, c *syntax.ArrayExpr) bool {
	if len(b.Elems) != len(a.Elems) || len(c.Elems) != len(a.Elems) {
		return false
	}

	for i := range a.Elems {
		if !arrayElemMatch(a.Elems[i], b.Elems[i], c.Elems[i]) {
			return false
		}
	}

	return true
}

func arrayElemMatch(a, b, c *syntax.ArrayElem) bool {
	if (b.Index == nil) != (a.Index == nil) || (c.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index, c.Index) {
		return false
	}

	if (b.Value == nil) != (a.Value == nil) || (c.Value == nil) != (a.Value == nil) {
		return false
	}

	if a.Value != nil && !wordsMatch(a.Value.Parts, b.Value.Parts, c.Value.Parts, justStructure) {
		return false
	}

	return true
}

func caseItemArrayMatch(a, b, c []*syntax.CaseItem) bool {
	if len(b) != len(a) || len(c) != len(a) {
		return false
	}

	for i := range a {
		if !caseItemMatch(a[i], b[i], c[i]) {
			return false
		}
	}

	return true
}

func caseItemMatch(a, b, c *syntax.CaseItem) bool {
	if b.Op != a.Op || c.Op != a.Op {
		return false
	}

	if !wordsArrayMatch(a.Patterns, b.Patterns, c.Patterns, justStructure) {
		return false
	}

	if !statementsArrayMatch(a.Stmts, b.Stmts, c.Stmts) {
		return false
	}

	return true
}

func loopMatch(a, b, c syntax.Loop) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) || reflect.TypeOf(c) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.WordIter:
		if (b.(*syntax.WordIter).Name == nil) != (a.Name == nil) || (c.(*syntax.WordIter).Name == nil) != (a.Name == nil) {
			return false
		}

		if a.Name != nil &&
			(b.(*syntax.WordIter).Name.Value != a.Name.Value || c.(*syntax.WordIter).Name.Value != a.Name.Value) {
			return false
		}

		if !wordsArrayMatch(a.Items, b.(*syntax.WordIter).Items, c.(*syntax.WordIter).Items, justStructure) {
			return false
		}
	case *syntax.CStyleLoop:
		if (b.(*syntax.CStyleLoop).Init == nil) != (a.Init == nil) || (c.(*syntax.CStyleLoop).Init == nil) != (a.Init == nil) {
			return false
		}

		if a.Init != nil && !arithmExprMatch(a.Init, b.(*syntax.CStyleLoop).Init, c.(*syntax.CStyleLoop).Init) {
			return false
		}

		if (b.(*syntax.CStyleLoop).Cond == nil) != (a.Cond == nil) || (c.(*syntax.CStyleLoop).Cond == nil) != (a.Cond == nil) {
			return false
		}

		if a.Cond != nil && !arithmExprMatch(a.Cond, b.(*syntax.CStyleLoop).Cond, c.(*syntax.CStyleLoop).Cond) {
			return false
		}

		if (b.(*syntax.CStyleLoop).Post == nil) != (a.Post == nil) || (c.(*syntax.CStyleLoop).Post == nil) != (a.Post == nil) {
			return false
		}

		if a.Post != nil && !arithmExprMatch(a.Post, b.(*syntax.CStyleLoop).Post, c.(*syntax.CStyleLoop).Post) {
			return false
		}
	}

	return true
}

func testExprMatch(a, b, c syntax.TestExpr) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) || reflect.TypeOf(c) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.BinaryTest:
		if b.(*syntax.BinaryTest).Op != a.Op || c.(*syntax.BinaryTest).Op != a.Op {
			return false
		}

		if !testExprMatch(a.X, b.(*syntax.BinaryTest).X, c.(*syntax.BinaryTest).X) {
			return false
		}

		if !testExprMatch(a.Y, b.(*syntax.BinaryTest).Y, c.(*syntax.BinaryTest).Y) {
			return false
		}
	case *syntax.UnaryTest:
		if b.(*syntax.UnaryTest).Op != a.Op || c.(*syntax.UnaryTest).Op != a.Op {
			return false
		}

		if !testExprMatch(a.X, b.(*syntax.UnaryTest).X, c.(*syntax.UnaryTest).X) {
			return false
		}
	case *syntax.ParenTest:
		if !testExprMatch(a.X, b.(*syntax.ParenTest).X, c.(*syntax.ParenTest).X) {
			return false
		}
	case *syntax.Word:
		if !wordsMatch(a.Parts, b.(*syntax.Word).Parts, c.(*syntax.Word).Parts, justStructure) {
			return false
		}
	}

	return true
}

type wordComparisonContext int

const (
	matchExactly        wordComparisonContext = 0
	forbidFlagInjection wordComparisonContext = 1
	justStructure       wordComparisonContext = 2
)

func wordsArrayMatch(a, b, c []*syntax.Word, ctx wordComparisonContext) bool {
	if len(b) != len(a) || len(c) != len(a) {
		return false
	}

	for i := range a {
		if !wordsMatch(a[i].Parts, b[i].Parts, c[i].Parts, ctx) {
			return false
		}
	}

	return true
}

func wordsMatch(a, b, c []syntax.WordPart, ctx wordComparisonContext) bool {
	if len(b) != len(a) || len(c) != len(a) {
		return false
	}

	for i, p := range a {
		if !wordPartsMatch(p, b[i], c[i], ctx) {
			return false
		}
	}

	return true
}

func verifyStrings(a, b, c string, ctx wordComparisonContext) bool {
	if ctx == matchExactly {
		if b != a || c != a {
			return false
		}
	} else if ctx == forbidFlagInjection {
		if flagInjected(a, b, c) {
			return false
		}
	}

	return true
}

func flagInjected(a, b, c string) bool {
	return !strings.HasPrefix(a, "-") && (strings.HasPrefix(b, "-") || strings.HasPrefix(c, "-"))
}

// Check for introduction or removal of ? * + @ ! characters
func literalInjection(a, b, c string) bool {
	for _, specialChar := range []string{"?", "*", "+", "@", "!"} {
		count := strings.Count(a, specialChar)

		if strings.Count(b, specialChar) != count || strings.Count(c, specialChar) != count {
			return true
		}
	}

	return false
}

func wordPartsMatch(a, b, c syntax.WordPart, ctx wordComparisonContext) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) || reflect.TypeOf(c) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.Lit:
		if !verifyStrings(a.Value, b.(*syntax.Lit).Value, c.(*syntax.Lit).Value, ctx) {
			return false
		}

		if literalInjection(a.Value, b.(*syntax.Lit).Value, c.(*syntax.Lit).Value) {
			return false
		}
	case *syntax.SglQuoted:
		if !verifyStrings(a.Value, b.(*syntax.SglQuoted).Value, c.(*syntax.SglQuoted).Value, ctx) {
			return false
		}
	case *syntax.DblQuoted:
		if !wordsMatch(a.Parts, b.(*syntax.DblQuoted).Parts, c.(*syntax.DblQuoted).Parts, ctx) {
			return false
		}
	case *syntax.ParamExp:
		if !paramExprMatch(a, b.(*syntax.ParamExp), c.(*syntax.ParamExp)) {
			return false
		}
	case *syntax.CmdSubst:
		if !cmdSubstMatch(a, b.(*syntax.CmdSubst), c.(*syntax.CmdSubst)) {
			return false
		}
	case *syntax.ArithmExp:
		if !arithmExpMatch(*a, *b.(*syntax.ArithmExp), *c.(*syntax.ArithmExp)) {
			return false
		}
	case *syntax.ProcSubst:
		if b.(*syntax.ProcSubst).Op != a.Op || c.(*syntax.ProcSubst).Op != a.Op {
			return false
		}

		if !statementsArrayMatch(a.Stmts, b.(*syntax.ProcSubst).Stmts, c.(*syntax.ProcSubst).Stmts) {
			return false
		}
	case *syntax.ExtGlob:
		if b.(*syntax.ExtGlob).Op != a.Op || c.(*syntax.ExtGlob).Op != a.Op {
			return false
		}

		if b.(*syntax.ExtGlob).Pattern.Value != a.Pattern.Value || c.(*syntax.ExtGlob).Pattern.Value != a.Pattern.Value {
			return false
		}
	case *syntax.BraceExp:
		if !wordsArrayMatch(a.Elems, b.(*syntax.BraceExp).Elems, c.(*syntax.BraceExp).Elems, ctx) {
			return false
		}
	}

	return true
}

func commandsMatch(a, b, c syntax.Command) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) || reflect.TypeOf(c) != reflect.TypeOf(a) {
		return false
	}

	switch cmd := a.(type) {
	case *syntax.CallExpr:
		if !assignArrayMatch(cmd.Assigns, b.(*syntax.CallExpr).Assigns, c.(*syntax.CallExpr).Assigns) {
			return false
		}

		if len(b.(*syntax.CallExpr).Args) != len(cmd.Args) || len(c.(*syntax.CallExpr).Args) != len(cmd.Args) {
			return false
		}

		// First "argument" is command name
		context := matchExactly
		for i := 0; i < len(cmd.Args); i++ {
			if !wordsMatch(cmd.Args[i].Parts, b.(*syntax.CallExpr).Args[i].Parts, c.(*syntax.CallExpr).Args[i].Parts, context) {
				return false
			}

			// For subsequent arguments, just check for argument inject
			context = forbidFlagInjection
		}

	case *syntax.IfClause:
		if !statementsArrayMatch(cmd.Cond, b.(*syntax.IfClause).Cond, c.(*syntax.IfClause).Cond) {
			return false
		}

		if !statementsArrayMatch(cmd.Then, b.(*syntax.IfClause).Then, c.(*syntax.IfClause).Then) {
			return false
		}

		if (b.(*syntax.IfClause).Else == nil) != (cmd.Else == nil) || (c.(*syntax.IfClause).Else == nil) != (cmd.Else == nil) {
			return false
		}

		if cmd.Else != nil && !commandsMatch(cmd.Else, b.(*syntax.IfClause).Else, c.(*syntax.IfClause).Else) {
			return false
		}
	case *syntax.WhileClause:
		if b.(*syntax.WhileClause).Until != cmd.Until || c.(*syntax.WhileClause).Until != cmd.Until {
			return false
		}

		if !statementsArrayMatch(cmd.Cond, b.(*syntax.WhileClause).Cond, c.(*syntax.WhileClause).Cond) {
			return false
		}

		if !statementsArrayMatch(cmd.Do, b.(*syntax.WhileClause).Do, c.(*syntax.WhileClause).Do) {
			return false
		}
	case *syntax.ForClause:
		if b.(*syntax.ForClause).Select != cmd.Select || c.(*syntax.ForClause).Select != cmd.Select {
			return false
		}

		if !loopMatch(cmd.Loop, b.(*syntax.ForClause).Loop, c.(*syntax.ForClause).Loop) {
			return false
		}

		if !statementsArrayMatch(cmd.Do, b.(*syntax.ForClause).Do, c.(*syntax.ForClause).Do) {
			return false
		}
	case *syntax.CaseClause:
		if !wordsMatch(cmd.Word.Parts, b.(*syntax.CaseClause).Word.Parts, c.(*syntax.CaseClause).Word.Parts, justStructure) {
			return false
		}

		if !caseItemArrayMatch(cmd.Items, b.(*syntax.CaseClause).Items, c.(*syntax.CaseClause).Items) {
			return false
		}
	case *syntax.Block:
		if !statementsArrayMatch(cmd.Stmts, b.(*syntax.Block).Stmts, c.(*syntax.Block).Stmts) {
			return false
		}
	case *syntax.Subshell:
		if !statementsArrayMatch(cmd.Stmts, b.(*syntax.Subshell).Stmts, c.(*syntax.Subshell).Stmts) {
			return false
		}
	case *syntax.BinaryCmd:
		if b.(*syntax.BinaryCmd).Op != cmd.Op || c.(*syntax.BinaryCmd).Op != cmd.Op {
			return false
		}

		if !statementsMatch(cmd.X, b.(*syntax.BinaryCmd).X, c.(*syntax.BinaryCmd).X) {
			return false
		}

		if !statementsMatch(cmd.Y, b.(*syntax.BinaryCmd).Y, c.(*syntax.BinaryCmd).Y) {
			return false
		}

	case *syntax.FuncDecl:
		if b.(*syntax.FuncDecl).RsrvWord != cmd.RsrvWord || c.(*syntax.FuncDecl).RsrvWord != cmd.RsrvWord {
			return false
		}

		if b.(*syntax.FuncDecl).Name.Value != cmd.Name.Value || c.(*syntax.FuncDecl).Name.Value != cmd.Name.Value {
			return false
		}

		if !statementsMatch(cmd.Body, b.(*syntax.FuncDecl).Body, c.(*syntax.FuncDecl).Body) {
			return false
		}
	case *syntax.ArithmCmd:
		if b.(*syntax.ArithmCmd).Unsigned != cmd.Unsigned || c.(*syntax.ArithmCmd).Unsigned != cmd.Unsigned {
			return false
		}

		if !arithmExprMatch(cmd.X, b.(*syntax.ArithmCmd).X, c.(*syntax.ArithmCmd).X) {
			return false
		}
	case *syntax.TestClause:
		if !testExprMatch(cmd.X, b.(*syntax.TestClause).X, c.(*syntax.TestClause).X) {
			return false
		}
	case *syntax.DeclClause:
		if (b.(*syntax.DeclClause).Variant == nil) != (cmd.Variant == nil) || (c.(*syntax.DeclClause).Variant == nil) != (cmd.Variant == nil) {
			return false
		}

		if cmd.Variant != nil &&
			(b.(*syntax.DeclClause).Variant.Value != cmd.Variant.Value || c.(*syntax.DeclClause).Variant.Value != cmd.Variant.Value) {
			return false
		}

		if !assignArrayMatch(cmd.Args, b.(*syntax.DeclClause).Args, c.(*syntax.DeclClause).Args) {
			return false
		}
	case *syntax.LetClause:
		if !arithmExprArrayMatch(cmd.Exprs, b.(*syntax.LetClause).Exprs, c.(*syntax.LetClause).Exprs) {
			return false
		}
	case *syntax.TimeClause:
		if !statementsMatch(cmd.Stmt, b.(*syntax.TimeClause).Stmt, c.(*syntax.TimeClause).Stmt) {
			return false
		}
	case *syntax.CoprocClause:
		if !wordsMatch(cmd.Name.Parts, b.(*syntax.CoprocClause).Name.Parts, c.(*syntax.CoprocClause).Name.Parts, justStructure) {
			return false
		}

		if !statementsMatch(cmd.Stmt, b.(*syntax.CoprocClause).Stmt, c.(*syntax.CoprocClause).Stmt) {
			return false
		}
	}

	return true
}

func redirectsMatch(a, b, c *syntax.Redirect) bool {
	if b.Op != a.Op || c.Op != a.Op {
		return false
	}

	if (b.N == nil) != (a.N == nil) || (c.N == nil) != (a.N == nil) {
		return false
	}

	if a.N != nil &&
		(b.N.Value != a.N.Value || c.N.Value != a.N.Value) {
		return false
	}

	if (b.Word == nil) != (a.Word == nil) || (c.Word == nil) != (a.Word == nil) {
		return false
	}

	if a.Word != nil && !wordsMatch(a.Word.Parts, b.Word.Parts, c.Word.Parts, matchExactly) {
		return false
	}

	if (b.Hdoc == nil) != (a.Hdoc == nil) || (c.Hdoc == nil) != (a.Hdoc == nil) {
		return false
	}

	if a.Hdoc != nil && !wordsMatch(a.Hdoc.Parts, b.Hdoc.Parts, c.Hdoc.Parts, matchExactly) {
		return false
	}

	return true
}

func redirectsArrayMatch(a, b, c []*syntax.Redirect) bool {
	for i := range a {
		if !redirectsMatch(a[i], b[i], c[i]) {
			return false
		}
	}

	return true
}

func statementsMatch(a, b, c *syntax.Stmt) bool {
	if b.Negated != a.Negated || c.Negated != a.Negated {
		return false
	}

	if b.Background != a.Background || c.Background != a.Background {
		return false
	}

	if b.Coprocess != a.Coprocess || c.Coprocess != a.Coprocess {
		return false
	}

	if !redirectsArrayMatch(a.Redirs, b.Redirs, c.Redirs) {
		return false
	}

	if (b.Cmd == nil) != (a.Cmd == nil) || (c.Cmd == nil) != (a.Cmd == nil) {
		return false
	}

	if a.Cmd != nil && !commandsMatch(a.Cmd, b.Cmd, c.Cmd) {
		return false
	}

	return true
}

func statementsArrayMatch(a, b, c []*syntax.Stmt) bool {
	for i := range a {
		if !statementsMatch(a[i], b[i], c[i]) {
			return false
		}
	}

	return true
}

func scriptsMatch(a, b, c *syntax.File) bool {
	if len(b.Stmts) != len(a.Stmts) || len(c.Stmts) != len(a.Stmts) {
		return false
	}

	if !statementsArrayMatch(a.Stmts, b.Stmts, c.Stmts) {
		return false
	}

	return true
}

func prefixDash(in string) string {
	return "-" + in
}

// Execute applies a parsed template to the specified data object,
// and writes the output to wr.
// If an error occurs executing the template or writing its output,
// execution stops, but partial results may already have been written to
// the output writer.
// A template may be executed safely in parallel, although if parallel
// executions share a Writer the output may be interleaved.
//
// If data is a reflect.Value, the template applies to the concrete
// value that the reflect.Value holds, as in fmt.Print.
func (t *Template) Execute(wr io.Writer, data any) (err error) {
	if data == nil {
		return t.unsafeTemplate.Execute(wr, data)
	}

	// An attacker may be able to cause type confusion or nil dereference panic at some stage of comparison
	defer func() {
		if r := recover(); r != nil {
			err = ErrShInjection
		}
	}()

	// Calculate requested result first
	var requestedResult bytes.Buffer

	if err := common.ExecuteWithShCallback(t.unsafeTemplate, t.uuid, common.EchoString, common.EchoString, &requestedResult, data); err != nil {
		return err
	}

	walked, err := t.unsafeTemplate.Clone()
	if err != nil {
		return err
	}
	walked.Tree = walked.Tree.Copy()

	common.WalkApplyFuncToNonDeclaractiveActions(walked, walked.Tree.Root)

	// Get baseline
	var baselineResult bytes.Buffer
	if err = common.ExecuteWithShCallback(walked, t.uuid, common.BaselineString, prefixDash, &baselineResult, data); err != nil {
		return err
	}

	parsedBaselineResult, err := syntax.NewParser().Parse(strings.NewReader(baselineResult.String()), "template.sh")
	if err != nil {
		return ErrInvalidShTemplate
	}

	// If baseline was valid, request must also be valid YAML for no injection to have occurred
	parsedRequestedResult, err := syntax.NewParser().Parse(strings.NewReader(requestedResult.String()), "template.sh")
	if err != nil {
		return ErrShInjection
	}

	// Mutate the input
	var mutatedResult bytes.Buffer
	if err = common.ExecuteWithShCallback(walked, t.uuid, mutateString, prefixDash, &mutatedResult, data); err != nil {
		return err
	}

	parsedMutatedResult, err := syntax.NewParser().Parse(strings.NewReader(mutatedResult.String()), "template.sh")
	if err != nil {
		return ErrShInjection
	}

	// Compare results
	if !scriptsMatch(parsedBaselineResult, parsedRequestedResult, parsedMutatedResult) {
		return ErrShInjection
	}

	requestedResult.WriteTo(wr)
	return nil
}

// Name returns the name of the template.
func (t *Template) Name() string {
	return t.unsafeTemplate.Name()
}

// New allocates a new, undefined template associated with the given one and with the same
// delimiters. The association, which is transitive, allows one template to
// invoke another with a {{template}} action.
//
// Because associated templates share underlying data, template construction
// cannot be done safely in parallel. Once the templates are constructed, they
// can be executed in parallel.
func (t *Template) New(name string) *Template {
	id := uuid.New()
	funcMap := common.BuildTextTemplateFuncMap(id)
	return &Template{unsafeTemplate: t.unsafeTemplate.New(name).Funcs(funcMap), uuid: id}
}

// Clone returns a duplicate of the template, including all associated
// templates. The actual representation is not copied, but the name space of
// associated templates is, so further calls to Parse in the copy will add
// templates to the copy but not to the original. Clone can be used to prepare
// common templates and use them with variant definitions for other templates
// by adding the variants after the clone is made.
func (t *Template) Clone() (*Template, error) {
	id := uuid.New()
	nt, err := t.unsafeTemplate.Clone()
	return &Template{unsafeTemplate: nt, uuid: id}, err
}

// AddParseTree associates the argument parse tree with the template t, giving
// it the specified name. If the template has not been defined, this tree becomes
// its definition. If it has been defined and already has that name, the existing
// definition is replaced; otherwise a new template is created, defined, and returned.
func (t *Template) AddParseTree(name string, tree *parse.Tree) (*Template, error) {
	nt, err := t.unsafeTemplate.AddParseTree(name, tree)

	if nt != t.unsafeTemplate {
		id := uuid.New()
		return &Template{unsafeTemplate: nt, uuid: id}, err
	}
	return t, err
}

// Option sets options for the template. Options are described by
// strings, either a simple string or "key=value". There can be at
// most one equals sign in an option string. If the option string
// is unrecognized or otherwise invalid, Option panics.
//
// Known options:
//
// missingkey: Control the behavior during execution if a map is
// indexed with a key that is not present in the map.
//
//	"missingkey=default" or "missingkey=invalid"
//		The default behavior: Do nothing and continue execution.
//		If printed, the result of the index operation is the string
//		"<no value>".
//	"missingkey=zero"
//		The operation returns the zero value for the map type's element.
//	"missingkey=error"
//		Execution stops immediately with an error.
func (t *Template) Option(opt ...string) *Template {
	for _, s := range opt {
		t.unsafeTemplate.Option(s)
	}
	return t
}

// Templates returns a slice of defined templates associated with t.
func (t *Template) Templates() []*Template {
	s := t.unsafeTemplate.Templates()
	var id string

	var ns []*Template
	for _, nt := range s {
		id = uuid.New()
		ns = append(ns, &Template{unsafeTemplate: nt, uuid: id})
	}

	return ns
}

// ExecuteTemplate applies the template associated with t that has the given name
// to the specified data object and writes the output to wr.
// If an error occurs executing the template or writing its output,
// execution stops, but partial results may already have been written to
// the output writer.
// A template may be executed safely in parallel, although if parallel
// executions share a Writer the output may be interleaved.
func (t *Template) ExecuteTemplate(wr io.Writer, name string, data any) error {
	tmpl := t.Lookup(name)
	if tmpl == nil {
		return fmt.Errorf("template: no template %q associated with template %q", name, t.Name())
	}
	return tmpl.Execute(wr, data)
}

// Delims sets the action delimiters to the specified strings, to be used in
// subsequent calls to Parse, ParseFiles, or ParseGlob. Nested template
// definitions will inherit the settings. An empty delimiter stands for the
// corresponding default: {{ or }}.
// The return value is the template, so calls can be chained.
func (t *Template) Delims(left, right string) *Template {
	t.unsafeTemplate.Delims(left, right)
	return t
}

// DefinedTemplates returns a string listing the defined templates,
// prefixed by the string "; defined templates are: ". If there are none,
// it returns the empty string. For generating an error message here
// and in html/template.
func (t *Template) DefinedTemplates() string {
	return t.unsafeTemplate.DefinedTemplates()
}

// Funcs adds the elements of the argument map to the template's function map.
// It must be called before the template is parsed.
// It panics if a value in the map is not a function with appropriate return
// type or if the name cannot be used syntactically as a function in a template.
// It is legal to overwrite elements of the map. The return value is the template,
// so calls can be chained.
func (t *Template) Funcs(funcMap FuncMap) *Template {
	t.unsafeTemplate.Funcs(funcMap)
	return t
}

// Lookup returns the template with the given name that is associated with t.
// It returns nil if there is no such template or the template has no definition.
func (t *Template) Lookup(name string) *Template {
	nt := t.unsafeTemplate.Lookup(name)

	if nt == nil {
		return nil
	}

	if nt != t.unsafeTemplate {
		id := uuid.New()
		return &Template{unsafeTemplate: nt, uuid: id}
	}

	return t
}

// Parse parses text as a template body for t.
// Named template definitions ({{define ...}} or {{block ...}} statements) in text
// define additional templates associated with t and are removed from the
// definition of t itself.
//
// Templates can be redefined in successive calls to Parse.
// A template definition with a body containing only white space and comments
// is considered empty and will not replace an existing template's body.
// This allows using Parse to add new named template definitions without
// overwriting the main template body.
func (t *Template) Parse(text string) (*Template, error) {
	nt, err := t.unsafeTemplate.Parse(text)

	if nt != t.unsafeTemplate {
		id := uuid.New()
		return &Template{unsafeTemplate: nt, uuid: id}, err
	}

	return t, err
}

// Must is a helper that wraps a call to a function returning (*Template, error)
// and panics if the error is non-nil. It is intended for use in variable
// initializations such as
//
//	var t = template.Must(template.New("name").Parse("text"))
func Must(t *Template, err error) *Template {
	if err != nil {
		panic(err)
	}
	return t
}

func readFileOS(file string) (name string, b []byte, err error) {
	name = filepath.Base(file)
	b, err = os.ReadFile(file)
	return
}

func readFileFS(fsys fs.FS) func(string) (string, []byte, error) {
	return func(file string) (name string, b []byte, err error) {
		name = path.Base(file)
		b, err = fs.ReadFile(fsys, file)
		return
	}
}

func parseFiles(t *Template, readFile func(string) (string, []byte, error), filenames ...string) (*Template, error) {
	if len(filenames) == 0 {
		// Not really a problem, but be consistent.
		return nil, fmt.Errorf("template: no files named in call to ParseFiles")
	}
	for _, filename := range filenames {
		name, b, err := readFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		// First template becomes return value if not already defined,
		// and we use that one for subsequent New calls to associate
		// all the templates together. Also, if this file has the same name
		// as t, this file becomes the contents of t, so
		//  t, err := New(name).Funcs(xxx).ParseFiles(name)
		// works. Otherwise we create a new template associated with t.
		var tmpl *Template
		if t == nil {
			t = New(name)
		}
		if name == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		_, err = tmpl.Parse(s)
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// parseGlob is the implementation of the function and method ParseGlob.
func parseGlob(t *Template, pattern string) (*Template, error) {
	filenames, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(filenames) == 0 {
		return nil, fmt.Errorf("template: pattern matches no files: %#q", pattern)
	}
	return parseFiles(t, readFileOS, filenames...)
}

func parseFS(t *Template, fsys fs.FS, patterns []string) (*Template, error) {
	var filenames []string
	for _, pattern := range patterns {
		list, err := fs.Glob(fsys, pattern)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("template: pattern matches no files: %#q", pattern)
		}
		filenames = append(filenames, list...)
	}
	return parseFiles(t, readFileFS(fsys), filenames...)
}

// ParseFiles creates a new Template and parses the template definitions from
// the named files. The returned template's name will have the base name and
// parsed contents of the first file. There must be at least one file.
// If an error occurs, parsing stops and the returned *Template is nil.
//
// When parsing multiple files with the same name in different directories,
// the last one mentioned will be the one that results.
// For instance, ParseFiles("a/foo", "b/foo") stores "b/foo" as the template
// named "foo", while "a/foo" is unavailable.
func ParseFiles(filenames ...string) (*Template, error) {
	return parseFiles(nil, readFileOS, filenames...)
}

// ParseFiles parses the named files and associates the resulting templates with
// t. If an error occurs, parsing stops and the returned template is nil;
// otherwise it is t. There must be at least one file.
// Since the templates created by ParseFiles are named by the base
// names of the argument files, t should usually have the name of one
// of the (base) names of the files. If it does not, depending on t's
// contents before calling ParseFiles, t.Execute may fail. In that
// case use t.ExecuteTemplate to execute a valid template.
//
// When parsing multiple files with the same name in different directories,
// the last one mentioned will be the one that results.
func (t *Template) ParseFiles(filenames ...string) (*Template, error) {
	// Ensure template is inited
	t.Option()

	return parseFiles(t, readFileOS, filenames...)
}

// ParseGlob creates a new Template and parses the template definitions from
// the files identified by the pattern. The files are matched according to the
// semantics of filepath.Match, and the pattern must match at least one file.
// The returned template will have the (base) name and (parsed) contents of the
// first file matched by the pattern. ParseGlob is equivalent to calling
// ParseFiles with the list of files matched by the pattern.
//
// When parsing multiple files with the same name in different directories,
// the last one mentioned will be the one that results.
func ParseGlob(pattern string) (*Template, error) {
	return parseGlob(nil, pattern)
}

// ParseGlob parses the template definitions in the files identified by the
// pattern and associates the resulting templates with t. The files are matched
// according to the semantics of filepath.Match, and the pattern must match at
// least one file. ParseGlob is equivalent to calling t.ParseFiles with the
// list of files matched by the pattern.
//
// When parsing multiple files with the same name in different directories,
// the last one mentioned will be the one that results.
func (t *Template) ParseGlob(pattern string) (*Template, error) {
	// Ensure template is inited
	t.Option()

	return parseGlob(t, pattern)
}

// ParseFS is like ParseFiles or ParseGlob but reads from the file system fsys
// instead of the host operating system's file system.
// It accepts a list of glob patterns.
// (Note that most file names serve as glob patterns matching only themselves.)
func ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	return parseFS(nil, fsys, patterns)
}

// ParseFS is like ParseFiles or ParseGlob but reads from the file system fsys
// instead of the host operating system's file system.
// It accepts a list of glob patterns.
// (Note that most file names serve as glob patterns matching only themselves.)
func (t *Template) ParseFS(fsys fs.FS, patterns ...string) (*Template, error) {
	// Ensure template is inited
	t.Option()

	return parseFS(t, fsys, patterns)
}

// HTMLEscape writes to w the escaped HTML equivalent of the plain text data b.
func HTMLEscape(w io.Writer, b []byte) {
	template.HTMLEscape(w, b)
}

// HTMLEscapeString returns the escaped HTML equivalent of the plain text data s.
func HTMLEscapeString(s string) string {
	return template.HTMLEscapeString(s)
}

// HTMLEscaper returns the escaped HTML equivalent of the textual
// representation of its arguments.
func HTMLEscaper(args ...any) string {
	return template.HTMLEscaper(args)
}

// IsTrue reports whether the value is 'true', in the sense of not the zero of its type,
// and whether the value has a meaningful truth value. This is the definition of
// truth used by if and other such actions.
func IsTrue(val any) (truth, ok bool) {
	return template.IsTrue(val)
}

// JSEscape writes to w the escaped JavaScript equivalent of the plain text data b.
func JSEscape(w io.Writer, b []byte) {
	template.JSEscape(w, b)
}

// JSEscapeString returns the escaped JavaScript equivalent of the plain text data s.
func JSEscapeString(s string) string {
	return template.JSEscapeString(s)
}

// JSEscaper returns the escaped JavaScript equivalent of the textual
// representation of its arguments.
func JSEscaper(args ...any) string {
	return template.JSEscaper(args)
}

// URLQueryEscaper returns the escaped value of the textual representation of
// its arguments in a form suitable for embedding in a URL query.
func URLQueryEscaper(args ...any) string {
	return template.URLQueryEscaper(args)
}
