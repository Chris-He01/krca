package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"knsight-go/internal/hub/user"
)

// GetSkillTool allows agents to fetch the full content of a skill by name.
type GetSkillTool struct {
	loader  *Loader
	sceneID string // when non-empty, scene-scoped skills are visible
}

func NewGetSkillTool(loader *Loader) *GetSkillTool {
	return &GetSkillTool{loader: loader}
}

// NewGetSkillToolWithScene creates a GetSkillTool that also exposes scene-scoped skills.
func NewGetSkillToolWithScene(loader *Loader, sceneID string) *GetSkillTool {
	return &GetSkillTool{loader: loader, sceneID: sceneID}
}

func (t *GetSkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_skill",
		Desc: `Fetch the full content of a skill by name.
Skills provide step-by-step diagnostic procedures, scripts, and expert knowledge for specific problem types.
Use this when the <skills> summary lists a relevant skill — call get_skill(name) to load the full instructions before executing.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {Type: schema.String, Desc: "Skill name as listed in the <skills> summary", Required: true},
		}),
	}, nil
}

func (t *GetSkillTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err), nil
	}
	if args.Name == "" {
		return "error: name is required", nil
	}

	u := user.FromContext(ctx)
	skill, ok := t.loader.GetSkill(args.Name)
	if !ok || !t.isVisible(skill.Scope, u.ID) {
		// Suggest visible skills only
		var names []string
		for _, s := range t.loader.ListSkills() {
			if t.isVisible(s.Scope, u.ID) {
				names = append(names, s.Name)
			}
		}
		return fmt.Sprintf("error: skill %q not found. Available skills: %s",
			args.Name, strings.Join(names, ", ")), nil
	}

	return fmt.Sprintf("# Skill: %s\nScope: %s\n\n%s", skill.Name, skill.Scope, skill.Content), nil
}

// isVisible returns true if the skill scope is accessible in the current context.
// A skill is visible if it passes user visibility OR scene visibility (when scene mode is active).
func (t *GetSkillTool) isVisible(scope, userID string) bool {
	if isVisibleToUser(scope, userID) {
		return true
	}
	if t.sceneID != "" && isVisibleToScene(scope, t.sceneID) {
		return true
	}
	return false
}
