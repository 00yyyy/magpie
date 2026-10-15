package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yetone/magpie/internal/provider"
)

// deepseekToolPatterns normalizes the ECMAScript null escape \0 in tool schemas
// to the Unicode escape \u0000. DeepSeek's schema validator rejects \0 with:
// "Invalid schema for function ... is not a 'regex'".
//
// To avoid re-encoding the entire payload or tool array (which reorders keys,
// escapes <, > and &, and breaks prompt caching), it finds only the offending
// "pattern" fields in tool schemas using gjson and splices the normalized string
// literals in place from the end of the body backwards.
func deepseekToolPatterns(p provider.Provider, to provider.Protocol, body []byte) []byte {
	if p.Preset != "deepseek" && provider.HostOf(p.Base(to)) != "api.deepseek.com" {
		return body
	}
	if !bytes.Contains(body, []byte(`\\0`)) {
		return body
	}
	return toolPatterns(body, unicodeNullEscape)
}

// toolPatterns is body with fix applied to every "pattern" of its tools'
// schemas, each spliced in place so the rest of the body stays byte for
// byte as it came.
func toolPatterns(body []byte, fix func(string) string) []byte {
	toolsRes := gjson.GetBytes(body, "tools")
	if !toolsRes.Exists() || toolsRes.Index <= 0 || !toolsRes.IsArray() {
		return body
	}

	var targets []patternTarget
	collectPatterns(toolsRes, false, fix, &targets)
	if len(targets) == 0 {
		return body
	}

	// Sort targets ascending by Index so we can copy cleanly forward into a
	// new buffer without modifying the caller's slice or its backing array.
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].index < targets[j].index
	})

	out := make([]byte, 0, len(body)+6*len(targets))
	last := 0
	for _, t := range targets {
		if t.index < last || t.index+t.rawLen > len(body) {
			continue
		}
		out = append(out, body[last:t.index]...)
		out = append(out, t.replacement...)
		last = t.index + t.rawLen
	}
	out = append(out, body[last:]...)
	return out
}

type patternTarget struct {
	index       int
	rawLen      int
	replacement string
}

func collectPatterns(node gjson.Result, inSchema bool, fix func(string) string, targets *[]patternTarget) {
	if !node.Exists() {
		return
	}
	if node.IsObject() {
		node.ForEach(func(key, value gjson.Result) bool {
			k := key.String()
			if inSchema {
				if k == "pattern" && value.Type == gjson.String && value.Index > 0 {
					var pattern string
					if err := json.Unmarshal([]byte(value.Raw), &pattern); err == nil {
						if norm := fix(pattern); norm != pattern {
							*targets = append(*targets, patternTarget{
								index:       value.Index,
								rawLen:      len(value.Raw),
								replacement: encodeJSONStringNoHTMLEscape(norm),
							})
						}
					}
				}
				switch k {
				case "properties", "$defs", "definitions", "dependentSchemas", "patternProperties":
					if value.IsObject() {
						value.ForEach(func(_, schema gjson.Result) bool {
							collectPatterns(schema, true, fix, targets)
							return true
						})
					}
				case "items", "additionalProperties", "contains", "propertyNames", "not", "if", "then", "else", "allOf", "anyOf", "oneOf", "prefixItems", "unevaluatedProperties", "unevaluatedItems", "contentSchema":
					collectPatterns(value, true, fix, targets)
				}
			} else {
				if k == "input_schema" || k == "parameters" {
					collectPatterns(value, true, fix, targets)
				} else if k == "function" {
					collectPatterns(value, false, fix, targets)
				}
			}
			return true
		})
	} else if node.IsArray() {
		node.ForEach(func(_, item gjson.Result) bool {
			collectPatterns(item, inSchema, fix, targets)
			return true
		})
	}
}

func encodeJSONStringNoHTMLEscape(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func unicodeNullEscape(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' || i+1 == len(pattern) {
			out.WriteByte(pattern[i])
			continue
		}
		next := pattern[i+1]
		// A pair of backslashes is literal; an octal escape has its own
		// meaning and must not be shortened to a null character.
		if next == '0' && (i+2 == len(pattern) || pattern[i+2] < '0' || pattern[i+2] > '9') {
			out.WriteString(`\u0000`)
		} else {
			out.WriteByte('\\')
			out.WriteByte(next)
		}
		i++
	}
	return out.String()
}
