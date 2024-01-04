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

// Package shsprintf is a drop-in-replacement for using sprintf
// to produce shell scripts, that adds automatic detection for command / argument injection.
package shsprintf

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/safetext/common"
	"mvdan.cc/sh/v3/syntax"
)

// ErrInvalidShTemplate indicates the requested template is not a valid script
var ErrInvalidShTemplate error = errors.New("Invalid Shell Template")

// ErrShInjection indicates the inputs resulted in shell injection.
var ErrShInjection error = errors.New("Shell Injection Detected")

const replaceableIndicator string = "REPLACEABLE"

const (
	specialChars = "\\'\"`${[|&;<>()*?!+@" +
		// extraSpecialChars
		" \t\r\n" +
		// prefixChars
		"~"
)

// EscapeDefaultContext escapes special characters in a context where there is no existing quoting, such as --arg=%s
func EscapeDefaultContext(in string) string {
	var buf bytes.Buffer

	cur := in
	for len(cur) > 0 {
		c, l := utf8.DecodeRuneInString(cur)
		cur = cur[l:]

		if strings.ContainsRune(specialChars, c) {
			buf.WriteByte('\\')
		}

		buf.WriteRune(c)
	}

	return buf.String()
}

func commentsArrayMatch(a, b []syntax.Comment) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !verifyStrings(a[i].Text, b[i].Text, matchStructure) {
			return false
		}
	}

	return true
}

func paramExprMatch(a, b *syntax.ParamExp) bool {
	if b.Short != a.Short {
		return false
	}

	if b.Excl != a.Excl {
		return false
	}

	if b.Length != a.Length {
		return false
	}

	if b.Width != a.Width {
		return false
	}

	if (b.Param == nil) != (a.Param == nil) {
		return false
	}

	if a.Param != nil &&
		(b.Param.Value != a.Param.Value) {
		return false
	}

	if (b.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index) {
		return false
	}

	if (b.Slice == nil) != (a.Slice == nil) {
		return false
	}

	if a.Slice != nil && !slicesMatch(a.Slice, b.Slice) {
		return false
	}

	if (b.Repl == nil) != (a.Repl == nil) {
		return false
	}

	if a.Repl != nil && !replaceMatch(a.Repl, b.Repl) {
		return false
	}

	if b.Names != a.Names {
		return false
	}

	if (b.Exp == nil) != (a.Exp == nil) {
		return false
	}

	if a.Exp != nil && !wordsMatch(a.Exp.Word.Parts, b.Exp.Word.Parts, matchStructure) {
		return false
	}

	return true
}

func cmdSubstMatch(a, b *syntax.CmdSubst) bool {
	if b.Backquotes != a.Backquotes {
		return false
	}

	if b.TempFile != a.TempFile {
		return false
	}

	if b.ReplyVar != a.ReplyVar {
		return false
	}

	if !statementsArrayMatch(a.Stmts, b.Stmts) {
		return false
	}

	if !commentsArrayMatch(a.Last, b.Last) {
		return false
	}

	return true
}

func slicesMatch(a, b *syntax.Slice) bool {
	return arithmExprMatch(a.Offset, b.Offset) && arithmExprMatch(a.Length, b.Length)
}

func replaceMatch(a, b *syntax.Replace) bool {
	if b.All != a.All {
		return false
	}

	if !wordsMatch(a.Orig.Parts, b.Orig.Parts, matchStructure) {
		return false
	}

	if !wordsMatch(a.With.Parts, b.With.Parts, matchStructure) {
		return false
	}

	return true
}

func arithmExpMatch(a, b syntax.ArithmExp) bool {
	if b.Bracket != a.Bracket {
		return false
	}

	if b.Unsigned != a.Unsigned {
		return false
	}

	return arithmExprMatch(a.X, b.X)
}

func arithmExprArrayMatch(a, b []syntax.ArithmExpr) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !arithmExprMatch(a[i], b[i]) {
			return false
		}
	}

	return true
}

func arithmExprMatch(a, b syntax.ArithmExpr) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.BinaryArithm:
		if b.(*syntax.BinaryArithm).Op != a.Op {
			return false
		}
		if !arithmExprMatch(a.X, b.(*syntax.BinaryArithm).X) {
			return false
		}
		if !arithmExprMatch(a.Y, b.(*syntax.BinaryArithm).Y) {
			return false
		}
	case *syntax.UnaryArithm:
		if b.(*syntax.UnaryArithm).Op != a.Op {
			return false
		}
		if b.(*syntax.UnaryArithm).Post != a.Post {
			return false
		}
		if !arithmExprMatch(a.X, b.(*syntax.UnaryArithm).X) {
			return false
		}
	case *syntax.ParenArithm:
		if !arithmExprMatch(a.X, b.(*syntax.ParenArithm).X) {
			return false
		}
	case *syntax.Word:
		if !wordsMatch(a.Parts, b.(*syntax.Word).Parts, matchStructure) {
			return false
		}
	}

	return true
}

func assignArrayMatch(a, b []*syntax.Assign) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !assignMatch(a[i], b[i]) {
			return false
		}
	}

	return true
}

func assignMatch(a, b *syntax.Assign) bool {
	if b.Append != a.Append {
		return false
	}

	if b.Naked != a.Naked {
		return false
	}

	if (b.Name == nil) != (a.Name == nil) {
		return false
	}

	if a.Name != nil && !verifyStrings(a.Name.Value, b.Name.Value, matchStructure) {
		return false
	}

	if (b.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index) {
		return false
	}

	if (b.Value == nil) != (a.Value == nil) {
		return false
	}

	if a.Value != nil && !wordsMatch(a.Value.Parts, b.Value.Parts, matchStructure) {
		return false
	}

	if (b.Array == nil) != (a.Array == nil) {
		return false
	}

	if a.Array != nil && !arrayExprMatch(a.Array, b.Array) {
		return false
	}

	return true
}

func arrayExprMatch(a, b *syntax.ArrayExpr) bool {
	if len(b.Elems) != len(a.Elems) {
		return false
	}

	for i := range a.Elems {
		if !arrayElemMatch(a.Elems[i], b.Elems[i]) {
			return false
		}
	}

	if !commentsArrayMatch(a.Last, b.Last) {
		return false
	}

	return true
}

func arrayElemMatch(a, b *syntax.ArrayElem) bool {
	if (b.Index == nil) != (a.Index == nil) {
		return false
	}

	if a.Index != nil && !arithmExprMatch(a.Index, b.Index) {
		return false
	}

	if (b.Value == nil) != (a.Value == nil) {
		return false
	}

	if a.Value != nil && !wordsMatch(a.Value.Parts, b.Value.Parts, matchStructure) {
		return false
	}

	if !commentsArrayMatch(a.Comments, b.Comments) {
		return false
	}

	return true
}

func caseItemArrayMatch(a, b []*syntax.CaseItem) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !caseItemMatch(a[i], b[i]) {
			return false
		}
	}

	return true
}

func caseItemMatch(a, b *syntax.CaseItem) bool {
	if b.Op != a.Op {
		return false
	}

	if !wordsArrayMatch(a.Patterns, b.Patterns, matchStructure) {
		return false
	}

	if !statementsArrayMatch(a.Stmts, b.Stmts) {
		return false
	}

	if !commentsArrayMatch(a.Comments, b.Comments) {
		return false
	}

	if !commentsArrayMatch(a.Last, b.Last) {
		return false
	}

	return true
}

func loopMatch(a, b syntax.Loop) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.WordIter:
		if (b.(*syntax.WordIter).Name == nil) != (a.Name == nil) {
			return false
		}

		if a.Name != nil && !verifyStrings(a.Name.Value, b.(*syntax.WordIter).Name.Value, matchStructure) {
			return false
		}

		if !wordsArrayMatch(a.Items, b.(*syntax.WordIter).Items, matchStructure) {
			return false
		}
	case *syntax.CStyleLoop:
		if (b.(*syntax.CStyleLoop).Init == nil) != (a.Init == nil) {
			return false
		}

		if a.Init != nil && !arithmExprMatch(a.Init, b.(*syntax.CStyleLoop).Init) {
			return false
		}

		if (b.(*syntax.CStyleLoop).Cond == nil) != (a.Cond == nil) {
			return false
		}

		if a.Cond != nil && !arithmExprMatch(a.Cond, b.(*syntax.CStyleLoop).Cond) {
			return false
		}

		if (b.(*syntax.CStyleLoop).Post == nil) != (a.Post == nil) {
			return false
		}

		if a.Post != nil && !arithmExprMatch(a.Post, b.(*syntax.CStyleLoop).Post) {
			return false
		}
	}

	return true
}

func testExprMatch(a, b syntax.TestExpr) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.BinaryTest:
		if b.(*syntax.BinaryTest).Op != a.Op {
			return false
		}

		if !testExprMatch(a.X, b.(*syntax.BinaryTest).X) {
			return false
		}

		if !testExprMatch(a.Y, b.(*syntax.BinaryTest).Y) {
			return false
		}
	case *syntax.UnaryTest:
		if b.(*syntax.UnaryTest).Op != a.Op {
			return false
		}

		if !testExprMatch(a.X, b.(*syntax.UnaryTest).X) {
			return false
		}
	case *syntax.ParenTest:
		if !testExprMatch(a.X, b.(*syntax.ParenTest).X) {
			return false
		}
	case *syntax.Word:
		if !wordsMatch(a.Parts, b.(*syntax.Word).Parts, matchStructure) {
			return false
		}
	}

	return true
}

type wordComparisonContext int

const (
	matchStructure      wordComparisonContext = 0
	forbidFlagInjection wordComparisonContext = 1 << 0
	literal             wordComparisonContext = 1 << 1
)

func wordsArrayMatch(a, b []*syntax.Word, ctx wordComparisonContext) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !wordsMatch(a[i].Parts, b[i].Parts, ctx) {
			return false
		}
	}

	return true
}

func wordsMatch(a, b []syntax.WordPart, ctx wordComparisonContext) bool {
	if len(b) != len(a) {
		return false
	}

	for i, p := range a {
		if !wordPartsMatch(p, b[i], ctx) {
			return false
		}
	}

	return true
}

func verifyStrings(a, b string, ctx wordComparisonContext) bool {
	aPat := "(?s)^" + strings.Replace(regexp.QuoteMeta(a), replaceableIndicator, "(.*)", -1) + "$"

	matched, err := regexp.Match(aPat, []byte(b))
	if err != nil {
		return false
	}

	if !matched {
		return false
	}

	if ctx&forbidFlagInjection != 0 {
		if flagInjected(a, b) {
			return false
		}
	}

	if ctx&literal != 0 {
		// Repeat matching to capture subgroups (untrusted insertions)
		re := regexp.MustCompile(aPat)
		insertedContent := re.FindStringSubmatch(b)[1:]

		for _, g := range insertedContent {
			if literalInjection(g) {
				return false
			}
		}
	}

	return true
}

func flagInjected(a, b string) bool {
	return !strings.HasPrefix(a, "-") && strings.HasPrefix(b, "-")
}

// Check for unescaped ? * + @ ! characters
func literalInjection(a string) bool {
	literalSpecials := map[rune]struct{}{'?': {}, '*': {}, '+': {}, '@': {}, '!': {}}

	escaped := false
	for _, c := range a {
		if !escaped {
			if c == '\\' {
				escaped = true
			} else if _, special := literalSpecials[c]; special {
				return true
			}
		} else {
			escaped = false
		}
	}

	// Unpaired trailing \ character would be incorrect escaping
	return escaped
}

func wordPartsMatch(a, b syntax.WordPart, ctx wordComparisonContext) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		return false
	}

	switch a := a.(type) {
	case *syntax.Lit:
		if !verifyStrings(a.Value, b.(*syntax.Lit).Value, ctx|literal) {
			return false
		}
	case *syntax.SglQuoted:
		if !verifyStrings(a.Value, b.(*syntax.SglQuoted).Value, ctx) {
			return false
		}
	case *syntax.DblQuoted:
		if !wordsMatch(a.Parts, b.(*syntax.DblQuoted).Parts, ctx) {
			return false
		}
	case *syntax.ParamExp:
		if !paramExprMatch(a, b.(*syntax.ParamExp)) {
			return false
		}
	case *syntax.CmdSubst:
		if !cmdSubstMatch(a, b.(*syntax.CmdSubst)) {
			return false
		}
	case *syntax.ArithmExp:
		if !arithmExpMatch(*a, *b.(*syntax.ArithmExp)) {
			return false
		}
	case *syntax.ProcSubst:
		if b.(*syntax.ProcSubst).Op != a.Op {
			return false
		}

		if !statementsArrayMatch(a.Stmts, b.(*syntax.ProcSubst).Stmts) {
			return false
		}

		if !commentsArrayMatch(a.Last, b.(*syntax.ProcSubst).Last) {
			return false
		}
	case *syntax.ExtGlob:
		if b.(*syntax.ExtGlob).Op != a.Op {
			return false
		}

		if b.(*syntax.ExtGlob).Pattern.Value != a.Pattern.Value {
			return false
		}
	case *syntax.BraceExp:
		if !wordsArrayMatch(a.Elems, b.(*syntax.BraceExp).Elems, ctx) {
			return false
		}
	}

	return true
}

func commandsMatch(a, b syntax.Command) bool {
	if reflect.TypeOf(b) != reflect.TypeOf(a) {
		return false
	}

	switch cmd := a.(type) {
	case *syntax.CallExpr:
		if !assignArrayMatch(cmd.Assigns, b.(*syntax.CallExpr).Assigns) {
			return false
		}

		if len(b.(*syntax.CallExpr).Args) != len(cmd.Args) {
			return false
		}

		// First "argument" is command name
		context := matchStructure
		for i := 0; i < len(cmd.Args); i++ {
			if !wordsMatch(cmd.Args[i].Parts, b.(*syntax.CallExpr).Args[i].Parts, context) {
				return false
			}

			// For subsequent arguments, also check for flag injection
			context = forbidFlagInjection
		}
	case *syntax.IfClause:
		if !statementsArrayMatch(cmd.Cond, b.(*syntax.IfClause).Cond) {
			return false
		}

		if !statementsArrayMatch(cmd.Then, b.(*syntax.IfClause).Then) {
			return false
		}

		if (b.(*syntax.IfClause).Else == nil) != (cmd.Else == nil) {
			return false
		}

		if cmd.Else != nil && !commandsMatch(cmd.Else, b.(*syntax.IfClause).Else) {
			return false
		}

		if !commentsArrayMatch(cmd.CondLast, b.(*syntax.IfClause).CondLast) {
			return false
		}

		if !commentsArrayMatch(cmd.ThenLast, b.(*syntax.IfClause).ThenLast) {
			return false
		}

		if !commentsArrayMatch(cmd.Last, b.(*syntax.IfClause).Last) {
			return false
		}
	case *syntax.WhileClause:
		if b.(*syntax.WhileClause).Until != cmd.Until {
			return false
		}

		if !statementsArrayMatch(cmd.Cond, b.(*syntax.WhileClause).Cond) {
			return false
		}

		if !statementsArrayMatch(cmd.Do, b.(*syntax.WhileClause).Do) {
			return false
		}

		if !commentsArrayMatch(cmd.CondLast, b.(*syntax.WhileClause).CondLast) {
			return false
		}

		if !commentsArrayMatch(cmd.DoLast, b.(*syntax.WhileClause).DoLast) {
			return false
		}
	case *syntax.ForClause:
		if b.(*syntax.ForClause).Select != cmd.Select {
			return false
		}

		if !loopMatch(cmd.Loop, b.(*syntax.ForClause).Loop) {
			return false
		}

		if !statementsArrayMatch(cmd.Do, b.(*syntax.ForClause).Do) {
			return false
		}

		if !commentsArrayMatch(cmd.DoLast, b.(*syntax.ForClause).DoLast) {
			return false
		}
	case *syntax.CaseClause:
		if !wordsMatch(cmd.Word.Parts, b.(*syntax.CaseClause).Word.Parts, matchStructure) {
			return false
		}

		if !caseItemArrayMatch(cmd.Items, b.(*syntax.CaseClause).Items) {
			return false
		}

		if !commentsArrayMatch(cmd.Last, b.(*syntax.CaseClause).Last) {
			return false
		}
	case *syntax.Block:
		if !statementsArrayMatch(cmd.Stmts, b.(*syntax.Block).Stmts) {
			return false
		}

		if !commentsArrayMatch(cmd.Last, b.(*syntax.Block).Last) {
			return false
		}
	case *syntax.Subshell:
		if !statementsArrayMatch(cmd.Stmts, b.(*syntax.Subshell).Stmts) {
			return false
		}

		if !commentsArrayMatch(cmd.Last, b.(*syntax.Subshell).Last) {
			return false
		}
	case *syntax.BinaryCmd:
		if b.(*syntax.BinaryCmd).Op != cmd.Op {
			return false
		}

		if !statementsMatch(cmd.X, b.(*syntax.BinaryCmd).X) {
			return false
		}

		if !statementsMatch(cmd.Y, b.(*syntax.BinaryCmd).Y) {
			return false
		}

	case *syntax.FuncDecl:
		if b.(*syntax.FuncDecl).RsrvWord != cmd.RsrvWord {
			return false
		}

		if (cmd.Name == nil) != (b.(*syntax.FuncDecl).Name != nil) {
			return false
		}

		if cmd.Name != nil && !verifyStrings(cmd.Name.Value, b.(*syntax.FuncDecl).Name.Value, matchStructure) {
			return false
		}

		if !statementsMatch(cmd.Body, b.(*syntax.FuncDecl).Body) {
			return false
		}
	case *syntax.ArithmCmd:
		if b.(*syntax.ArithmCmd).Unsigned != cmd.Unsigned {
			return false
		}

		if !arithmExprMatch(cmd.X, b.(*syntax.ArithmCmd).X) {
			return false
		}
	case *syntax.TestClause:
		if !testExprMatch(cmd.X, b.(*syntax.TestClause).X) {
			return false
		}
	case *syntax.DeclClause:
		if (b.(*syntax.DeclClause).Variant == nil) != (cmd.Variant == nil) {
			return false
		}

		if cmd.Variant != nil &&
			(b.(*syntax.DeclClause).Variant.Value != cmd.Variant.Value) {
			return false
		}

		if !assignArrayMatch(cmd.Args, b.(*syntax.DeclClause).Args) {
			return false
		}
	case *syntax.LetClause:
		if !arithmExprArrayMatch(cmd.Exprs, b.(*syntax.LetClause).Exprs) {
			return false
		}
	case *syntax.TimeClause:
		if !statementsMatch(cmd.Stmt, b.(*syntax.TimeClause).Stmt) {
			return false
		}
	case *syntax.CoprocClause:
		if !wordsMatch(cmd.Name.Parts, b.(*syntax.CoprocClause).Name.Parts, matchStructure) {
			return false
		}

		if !statementsMatch(cmd.Stmt, b.(*syntax.CoprocClause).Stmt) {
			return false
		}
	}

	return true
}

func redirectsMatch(a, b *syntax.Redirect) bool {
	if b.Op != a.Op {
		return false
	}

	if (b.N == nil) != (a.N == nil) {
		return false
	}

	if a.N != nil && !verifyStrings(a.N.Value, b.N.Value, matchStructure) {
		return false
	}

	if (b.Word == nil) != (a.Word == nil) {
		return false
	}

	if a.Word != nil && !wordsMatch(a.Word.Parts, b.Word.Parts, matchStructure) {
		return false
	}

	if (b.Hdoc == nil) != (a.Hdoc == nil) {
		return false
	}

	if a.Hdoc != nil && !wordsMatch(a.Hdoc.Parts, b.Hdoc.Parts, matchStructure) {
		return false
	}

	return true
}

func redirectsArrayMatch(a, b []*syntax.Redirect) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !redirectsMatch(a[i], b[i]) {
			return false
		}
	}

	return true
}

func statementsMatch(a, b *syntax.Stmt) bool {
	if b.Negated != a.Negated {
		return false
	}

	if b.Background != a.Background {
		return false
	}

	if b.Coprocess != a.Coprocess {
		return false
	}

	if !redirectsArrayMatch(a.Redirs, b.Redirs) {
		return false
	}

	if (b.Cmd == nil) != (a.Cmd == nil) {
		return false
	}

	if a.Cmd != nil && !commandsMatch(a.Cmd, b.Cmd) {
		return false
	}

	if !commentsArrayMatch(a.Comments, b.Comments) {
		return false
	}

	return true
}

func statementsArrayMatch(a, b []*syntax.Stmt) bool {
	if len(b) != len(a) {
		return false
	}

	for i := range a {
		if !statementsMatch(a[i], b[i]) {
			return false
		}
	}

	return true
}

func scriptsMatch(a, b *syntax.File) bool {
	if !statementsArrayMatch(a.Stmts, b.Stmts) {
		return false
	}

	if !commentsArrayMatch(a.Last, b.Last) {
		return false
	}

	return true
}

var truncationSpecifierPattern = regexp.MustCompile(`([^%]|^)%([+-0 #]*)(\d*\.\d*)(s|v)`)

// Sprintf is a fmt.Sprintf replacement specific for generating bash scripts, with protection against any string substitution from escaping its context, or from injecting new flag arguments.
func Sprintf(format string, a ...any) (string, error) {
	return SprintfLang(format, syntax.LangBash, a...)
}

// MustSprintf is a wrapper to Sprintf that panics upon error.
func MustSprintf(format string, a ...any) string {
	res, err := Sprintf(format, a...)
	if err != nil {
		panic(err)
	}
	return res
}

// SprintfLang is a fmt.Sprintf replacement specific for generating shell scripts, with protection against any string substitution from escaping its context, or from injecting new flag arguments.
func SprintfLang(format string, lang syntax.LangVariant, a ...any) (string, error) {
	baselineArgs := common.DeepCopyMutateStrings(a, func(in string) string { return replaceableIndicator })

	// Disable string truncation so that strings are always directly substituted
	baselineFormat := string(truncationSpecifierPattern.ReplaceAll([]byte(format), []byte("${1}%${2}${4}")))
	baselineResult := fmt.Sprintf(baselineFormat, baselineArgs.([]any)...)

	parsedBaselineResult, err := syntax.NewParser(syntax.Variant(lang), syntax.KeepComments(true)).Parse(strings.NewReader(baselineResult), "template.sh")
	if err != nil {
		return "", ErrInvalidShTemplate
	}

	requestedResult := fmt.Sprintf(format, a...)
	parsedRequestedResult, err := syntax.NewParser(syntax.Variant(lang), syntax.KeepComments(true)).Parse(strings.NewReader(requestedResult), "template.sh")
	if err != nil {
		return "", ErrShInjection
	}

	if !scriptsMatch(parsedBaselineResult, parsedRequestedResult) {
		return "", ErrShInjection
	}

	return requestedResult, nil
}
