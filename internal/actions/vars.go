package actions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
	"github.com/bytedance/sonic"
)

// {{ $.a.b }} — capture the JSON path inside the mustaches.
var varRe = regexp.MustCompile(`\{\{\s*(\$[^}]+?)\s*\}\}`)

// varResolver rewrites {{$...}} tokens in string values against the flow scope,
// so any input field can reference upstream data. It holds a per-call cache so
// each distinct path is fetched from the runtime only once, however many values
// reference it.
type varResolver struct {
	job   *sdkv1.Job
	cache map[string]string
}

func newVarResolver(job *sdkv1.Job) *varResolver {
	return &varResolver{job: job, cache: make(map[string]string)}
}

// resolve substitutes every {{$...}} token in text. Tokens the scope can't
// supply are left verbatim so nothing is silently dropped.
func (vr *varResolver) resolve(text string) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	return varRe.ReplaceAllStringFunc(text, func(tok string) string {
		path := strings.TrimSpace(varRe.FindStringSubmatch(tok)[1])
		v, ok := vr.cache[path]
		if !ok {
			v = vr.fetch(path)
			vr.cache[path] = v
		}
		return v
	})
}

// fetch reads a JSON path from the flow context. The reply is JSON: a JSON
// string is unwrapped to its value, anything else is returned raw so it can be
// inlined into the field.
func (vr *varResolver) fetch(jsonPath string) string {
	raw, ok := vr.job.CmdGetScope(jsonPath).([]byte)
	if !ok || len(raw) == 0 {
		return fmt.Sprintf("{{%s}}", jsonPath) // leave the token in place
	}
	var s string
	if err := sonic.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// resolveInputVars walks the decoded input and rewrites {{$...}} tokens in every
// string leaf, descending into nested objects and arrays. Non-string leaves are
// left untouched. Values with no token never hit the runtime, so this is safe to
// run over every action's input uniformly.
func resolveInputVars(job *sdkv1.Job, in map[string]any) map[string]any {
	vr := newVarResolver(job)
	walked, _ := walkResolve(vr, in).(map[string]any)
	if walked == nil {
		return in
	}
	return walked
}

func walkResolve(vr *varResolver, v any) any {
	switch t := v.(type) {
	case string:
		return vr.resolve(t)
	case map[string]any:
		for k, val := range t {
			t[k] = walkResolve(vr, val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = walkResolve(vr, val)
		}
		return t
	default:
		return v
	}
}
