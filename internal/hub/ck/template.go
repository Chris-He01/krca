package ck

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RenderSQL substitutes ${name} placeholders in sql with values from
// params. Values are formatted by their Go type:
//
//   - time.Time → '2006-01-02 15:04:05' (ClickHouse DateTime literal, CST)
//   - string    → single-quoted, embedded ' doubled (CH/SQL escape)
//   - bool      → 1 or 0
//   - int / float / json.Number → bare number
//   - nil       → NULL
//   - []any     → comma-joined, each element re-formatted (for IN lists)
//
// In addition, a handful of well-known placeholders are auto-bound when
// the caller does not supply them explicitly:
//
//   - ${now}             current time (CST)
//   - ${today_start}     today 00:00:00 CST
//   - ${today_end}       today 23:59:59 CST
//   - ${yesterday_start} yesterday 00:00:00 CST
//   - ${yesterday_end}   yesterday 23:59:59 CST
//   - ${hour_ago}        now − 1h
//   - ${day_ago}         now − 24h
//   - ${week_ago}        now − 7d
//
// Time helpers operate in Asia/Shanghai (CST/UTC+8), matching the rest of
// the project's standard-time convention.
func RenderSQL(sql string, params map[string]any) (string, error) {
	if !strings.Contains(sql, "${") {
		return sql, nil
	}

	loc := cstLoc()
	now := time.Now().In(loc)
	builtins := map[string]any{
		"now":             now,
		"today_start":     time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc),
		"today_end":       time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc),
		"yesterday_start": time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, loc),
		"yesterday_end":   time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 59, 0, loc),
		"hour_ago":        now.Add(-time.Hour),
		"day_ago":         now.Add(-24 * time.Hour),
		"week_ago":        now.Add(-7 * 24 * time.Hour),
	}

	var firstErr error
	out := rePlaceholder.ReplaceAllStringFunc(sql, func(match string) string {
		name := match[2 : len(match)-1] // strip ${ and }
		// Caller params take precedence.
		if params != nil {
			if v, ok := params[name]; ok {
				lit, err := formatLiteral(v)
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("param %q: %w", name, err)
				}
				return lit
			}
		}
		if v, ok := builtins[name]; ok {
			lit, err := formatLiteral(v)
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("builtin %q: %w", name, err)
			}
			return lit
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("unresolved placeholder ${%s}", name)
		}
		return match
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

var rePlaceholder = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

func formatLiteral(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case time.Time:
		return "'" + x.In(cstLoc()).Format("2006-01-02 15:04:05") + "'", nil
	case string:
		return quoteString(x), nil
	case bool:
		if x {
			return "1", nil
		}
		return "0", nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32), nil
	case float64:
		// JSON numbers arrive as float64. Render integers without the
		// trailing ".0" to keep generated SQL tidy.
		if x == float64(int64(x)) && !isFloatExp(x) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case []any:
		if len(x) == 0 {
			return "", errors.New("empty list parameter")
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			lit, err := formatLiteral(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, lit)
		}
		return strings.Join(parts, ", "), nil
	case []string:
		if len(x) == 0 {
			return "", errors.New("empty list parameter")
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, quoteString(item))
		}
		return strings.Join(parts, ", "), nil
	default:
		return "", fmt.Errorf("unsupported parameter type %T", v)
	}
}

// QuoteString returns a CH/SQL single-quoted literal. Exported wrapper
// around quoteString so callers building bespoke SQL (e.g. the node-health
// endpoint) get the same escaping rules as RenderSQL.
func QuoteString(s string) string { return quoteString(s) }

// QuoteStringList renders a slice as a comma-separated list of quoted
// strings, suitable for SQL IN clauses.
func QuoteStringList(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = quoteString(s)
	}
	return strings.Join(parts, ", ")
}

// quoteString returns a CH/SQL single-quoted literal. Backslashes are
// escaped (ClickHouse supports C-style escapes inside string literals)
// and embedded single quotes are doubled, which is also accepted.
func quoteString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'':
			b.WriteString("''")
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// isFloatExp reports whether x falls outside the range where casting to
// int64 round-trips losslessly. Anything past ±2^53 stops being safe in
// float64.
func isFloatExp(x float64) bool {
	const lim = 1 << 53
	return x > float64(lim) || x < -float64(lim)
}

var cstZone = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}()

func cstLoc() *time.Location { return cstZone }
