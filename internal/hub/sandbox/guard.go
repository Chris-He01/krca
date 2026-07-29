package sandbox

import (
	"fmt"
	"regexp"
	"strings"
)

var defaultDenyPatterns = []string{
	`rm\s+-[rf]`,
	`shutdown`,
	`reboot`,
	`poweroff`,
	`mkfs`,
	`dd\s+if=`,
	`format`,
	`:\(\)\{`,
}

type Guard struct {
	denyRegexps        []*regexp.Regexp
	restrictWorkspace  bool
}

func NewGuard(denyPatterns []string, restrictWorkspace bool) (*Guard, error) {
	if len(denyPatterns) == 0 {
		denyPatterns = defaultDenyPatterns
	}
	regexps := make([]*regexp.Regexp, 0, len(denyPatterns))
	for _, pat := range denyPatterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("invalid deny pattern %q: %w", pat, err)
		}
		regexps = append(regexps, re)
	}
	return &Guard{
		denyRegexps:       regexps,
		restrictWorkspace: restrictWorkspace,
	}, nil
}

func (g *Guard) CheckCommand(command string) error {
	for _, re := range g.denyRegexps {
		if re.MatchString(command) {
			return fmt.Errorf("command denied: matches blocked pattern %q", re.String())
		}
	}
	return nil
}

func (g *Guard) CheckPath(path string) error {
	if !g.restrictWorkspace {
		return nil
	}
	if strings.Contains(path, "../") {
		return fmt.Errorf("path denied: path traversal detected in %q", path)
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("path denied: absolute paths not allowed when restrict_to_workspace is true")
	}
	return nil
}
