package skill

type Skill struct {
	Name           string       `yaml:"name" json:"name"`
	Description    string       `yaml:"description" json:"description"`
	Pages          []string     `yaml:"page" json:"pages"`
	Type           string       `yaml:"type" json:"type"`
	Mode           string       `yaml:"mode" json:"mode"`
	Version        string       `yaml:"version" json:"version"`
	Trigger        string       `yaml:"trigger" json:"trigger"`
	Params         []SkillParam `yaml:"params" json:"params"`
	PromptTemplate string       `yaml:"prompt_template" json:"prompt_template"`

	MandatoryTool       string   `yaml:"mandatory_tool" json:"mandatory_tool,omitempty"`
	ForbiddenTools      []string `yaml:"forbidden_tools" json:"forbidden_tools,omitempty"`
	ForbiddenWithoutTool []string `yaml:"forbidden_without_tool" json:"forbidden_without_tool,omitempty"`
	ReplyGuide          string   `yaml:"reply_guide" json:"reply_guide,omitempty"`

	Source      string `yaml:"-" json:"source"`
	Status      string `yaml:"-" json:"status"`
	FileVersion string `yaml:"-" json:"-"`
	GrayRatio   int    `yaml:"-" json:"-"`
}

type SkillParam struct {
	Name              string `yaml:"name" json:"name"`
	Type              string `yaml:"type" json:"type"`
	Required          bool   `yaml:"required" json:"required"`
	Desc              string `yaml:"desc" json:"desc"`
	DefaultFromMemory bool   `yaml:"default_from_memory" json:"default_from_memory,omitempty"`
	Default           string `yaml:"default" json:"default,omitempty"`
}

func (s *Skill) IsWorkflow() bool {
	return s.Mode == "workflow" || s.MandatoryTool != ""
}

func (s *Skill) MatchesPage(page string) bool {
	for _, p := range s.Pages {
		if p == "*" || p == page {
			return true
		}
	}
	return false
}

func (s *Skill) ToSummary() string {
	desc := s.Description
	if desc == "" {
		desc = s.Trigger
	}
	return "- " + s.Name + "：" + desc
}
