// Mostly brought in as-is from otel-collector-contrib with the following changes:
// - GetExprEnv includes severity_text and severity_number
// - o11yExprPatcher rewrites like/ilike AST nodes for compile-time pattern compilation

package o11ystanzahelper

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
	o11ystanzaentry "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/entry"
	"github.com/hanzoai/otel-collector/utils"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
)

var envPool = sync.Pool{
	New: func() any {
		return map[string]any{
			"os_env_func": os.Getenv,
			"isJSON": func(v any) bool {
				return utils.IsJSON(v)
			},
			"unquote": func(v any) string {
				return utils.Unquote(v.(string))
			},
		}
	},
}

// RunWithExprEnv builds an expression environment for the given entry, calls fn
// with it, then returns the map to the pool. compiledPatterns contains
// pre-compiled like/ilike matchers produced by ExprCompile; pass nil if the
// expression has none.
func RunWithExprEnv(e *entry.Entry, forExprWithBodyFieldRef bool, compiledPatterns map[string]func(s string) bool, fn func(map[string]any) error) error {
	env := getExprEnv(e, forExprWithBodyFieldRef, compiledPatterns)
	defer putExprEnv(env, compiledPatterns)
	return fn(env)
}

func getExprEnv(e *entry.Entry, forExprWithBodyFieldRef bool, compiledPatterns map[string]func(s string) bool) map[string]any {
	env := envPool.Get().(map[string]any)
	env["$"] = e.Body
	env["body"] = e.Body
	env["attributes"] = e.Attributes
	env["resource"] = e.Resource
	env["timestamp"] = e.Timestamp
	env["severity_text"] = e.SeverityText
	env["severity_number"] = int(e.Severity)
	env["trace_id"] = e.TraceID
	env["span_id"] = e.SpanID
	env["trace_flags"] = e.TraceFlags

	if forExprWithBodyFieldRef {
		env["body_map"] = o11ystanzaentry.ParseBodyJson(e)
	}

	for k, v := range compiledPatterns {
		env[k] = v
	}

	return env
}

func putExprEnv(e map[string]any, compiledPatterns map[string]func(s string) bool) {
	for k := range compiledPatterns {
		delete(e, k)
	}
	envPool.Put(e)
}

// ExprCompile compiles an expression and returns the program, whether it
// references body fields, a map of pre-compiled like/ilike pattern matchers
// keyed by their env slot names, and any compilation error.
func ExprCompile(input string) (
	program *vm.Program, hasBodyFieldRef bool, compiledPatterns map[string]func(s string) bool, err error,
) {
	patcher := &o11yExprPatcher{}
	program, err = expr.Compile(
		input, expr.AllowUndefinedVariables(), expr.Patch(patcher),
	)
	if err != nil {
		return nil, false, nil, err
	}

	return program, patcher.foundBodyFieldRef, patcher.compiledPatterns, patcher.error()
}

// ExprCompileBool is like ExprCompile but additionally requires the expression
// to evaluate to a boolean.
func ExprCompileBool(input string) (
	program *vm.Program, hasBodyFieldRef bool, compiledPatterns map[string]func(s string) bool, err error,
) {
	patcher := &o11yExprPatcher{}
	program, err = expr.Compile(
		input, expr.AllowUndefinedVariables(), expr.Patch(patcher), expr.AsBool(),
	)
	if err != nil {
		return nil, false, nil, err
	}

	return program, patcher.foundBodyFieldRef, patcher.compiledPatterns, patcher.error()
}

type o11yExprPatcher struct {
	// set to true if the patcher encounters a reference to a body field
	// (like `body.request.id`) while compiling an expression
	foundBodyFieldRef bool

	// compiledPatterns is populated by the like/ilike rewrite (see Visit).
	// Keys are env slot names (e.g. "__like_3f8a1c2d"); values are the
	// corresponding pre-compiled matcher functions. This map is returned to
	// the caller by ExprCompile/ExprCompileBool and must be injected into the
	// expr env before every vm.Run call so the rewritten bytecode can resolve
	// the slot-name identifiers.
	compiledPatterns map[string]func(s string) bool

	// errs holds compile-time errors detected during the AST walk, for
	// example a like/ilike call with the wrong number of arguments or a
	// non-string literal pattern. expr.Compile itself cannot surface these
	// because AllowUndefinedVariables suppresses unknown-function errors; the
	// patcher catches them explicitly so callers get a meaningful error at
	// pipeline startup rather than at log-entry evaluation time.
	errs []error
}

// error aggregates the compile-time errors collected during the AST walk into a
// single error, or returns nil if none were encountered.
func (p *o11yExprPatcher) error() error {
	return errors.Join(p.errs...)
}

func (p *o11yExprPatcher) Visit(node *ast.Node) {
	// Change all references to fields inside body (eg: body.request.id)
	// to refer inside body_map instead (eg: body_map.request.id)
	//
	// expr represents `body.request.id` as:
	//   MemberNode { Node: IdentifierNode{"body"}, Property: ... }
	//
	// We rename the root identifier from "body" to "body_map" so the
	// expression reads from the JSON-parsed body map at runtime instead of
	// the raw body string. body_map is populated by GetExprEnv when
	// foundBodyFieldRef is true.
	if memberNode, ok := (*node).(*ast.MemberNode); ok {
		if id, ok := memberNode.Node.(*ast.IdentifierNode); ok && id.Value == "body" {
			id.Value = "body_map"
			p.foundBodyFieldRef = true
		}
	}

	// The remaining rewrites only apply to function call nodes.
	n, ok := (*node).(*ast.CallNode)
	if !ok {
		return
	}
	c, ok := n.Callee.(*ast.IdentifierNode)
	if !ok {
		return
	}

	// ── Rewrite 2a: env() → os_env_func() ────────────────────────────────────
	//
	// The expr built-in `env` conflicts with the pipeline keyword, so we
	// renamed it to os_env_func in the env map. Mirror that here.
	if c.Value == "env" {
		c.Value = "os_env_func"
		return
	}

	// ── Rewrite 2b: like/ilike pre-compilation ────────────────────────────────
	if c.Value == "like" || c.Value == "ilike" {
		p.rewriteLikeCall(n, c)
	}
}

// rewriteLikeCall pre-compiles a like/ilike call node at expression compile
// time so the pattern is not recompiled on every log entry.
//
// Pipeline expressions always use constant patterns, e.g.:
//
//	like(body, "%error%")
//
// Steps:
//
//	1 — detect:  second argument must be a StringNode (compile-time constant).
//	2 — compile: turn the LIKE pattern into a *regexp.Regexp wrapped as func(string) bool.
//	3 — store:   save the matcher under a stable slot name derived from the
//	             pattern (fnv64a hash), e.g. "__like_3f8a1c2d", so callers can
//	             inject it into the env before vm.Run.
//	4 — rewrite: mutate the CallNode to call __like_3f8a1c2d(body) instead of
//	             like(body, "%error%"), dropping the now-baked-in pattern arg.
func (p *o11yExprPatcher) rewriteLikeCall(n *ast.CallNode, c *ast.IdentifierNode) {
	// ── Arity check ───────────────────────────────────────────────────────
	if len(n.Arguments) != 2 {
		p.errs = append(p.errs, fmt.Errorf(
			"%s() requires exactly 2 arguments, got %d", c.Value, len(n.Arguments),
		))
		return
	}

	// ── Pattern type check ────────────────────────────────────────────────
	strNode, isStr := n.Arguments[1].(*ast.StringNode)
	if !isStr {
		p.errs = append(p.errs, fmt.Errorf(
			"%s() pattern (second argument) must be a string literal", c.Value,
		))
		return
	}

	// ── Pattern compilation + rewrite ─────────────────────────────────────
	pattern := strNode.Value
	var matcher func(string) bool
	var slotName string
	var compileErr error
	if c.Value == "like" {
		matcher, compileErr = compileLike(pattern)
		slotName = likeSlotName(pattern)
	} else {
		matcher, compileErr = compileILike(pattern)
		slotName = iLikeSlotName(pattern)
	}
	if compileErr != nil {
		p.errs = append(p.errs, fmt.Errorf(
			"%s() invalid pattern %q: %w", c.Value, pattern, compileErr,
		))
		return
	}
	p.compiledPatterns[slotName] = matcher // step 3
	c.Value = slotName                     // step 4a: rename callee
	n.Arguments = n.Arguments[:1]          // step 4b: drop pattern arg
}
