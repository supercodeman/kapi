package skill

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/sangchenglong/kapi/internal/dao"
)

type Registry struct {
	mu        sync.RWMutex
	skills    map[string]*Skill
	fileDir   string
	skillDAO  *dao.SkillDAO
}

func NewRegistry(fileDir string, skillDAO *dao.SkillDAO) *Registry {
	return &Registry{
		skills:   make(map[string]*Skill),
		fileDir:  fileDir,
		skillDAO: skillDAO,
	}
}

func (r *Registry) Load(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.skills = make(map[string]*Skill)

	if err := r.loadFromFiles(); err != nil {
		log.Printf("warning: failed to load skills from files: %v", err)
	}

	if r.skillDAO != nil {
		if err := r.loadFromDB(ctx); err != nil {
			log.Printf("warning: failed to load skills from db: %v", err)
		}
	}

	log.Printf("skill registry loaded: %d skills", len(r.skills))
	return nil
}

func (r *Registry) loadFromFiles() error {
	if r.fileDir == "" {
		return nil
	}

	files, err := filepath.Glob(filepath.Join(r.fileDir, "*.yaml"))
	if err != nil {
		return err
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Printf("warning: failed to read skill file %s: %v", f, err)
			continue
		}
		var s Skill
		if err := yaml.Unmarshal(data, &s); err != nil {
			log.Printf("warning: failed to parse skill file %s: %v", f, err)
			continue
		}
		s.Source = "file"
		s.Status = "enabled"
		s.FileVersion = s.Version
		r.skills[s.Name] = &s
	}
	return nil
}

func (r *Registry) loadFromDB(ctx context.Context) error {
	dbSkills, err := r.skillDAO.ListEnabled(ctx)
	if err != nil {
		return err
	}

	for _, ds := range dbSkills {
		var pages []string
		if err := json.Unmarshal([]byte(ds.Page), &pages); err != nil {
			log.Printf("warning: failed to parse pages for skill %s: %v", ds.Name, err)
		}

		var params []SkillParam
		if err := json.Unmarshal([]byte(ds.ParamsSchema), &params); err != nil {
			log.Printf("warning: failed to parse params for skill %s: %v", ds.Name, err)
		}

		s := &Skill{
			Name:           ds.Name,
			Pages:          pages,
			Type:           ds.Type,
			Version:        ds.Version,
			Trigger:        ds.TriggerDesc,
			Params:         params,
			PromptTemplate: ds.PromptTemplate,
			Source:          "db",
			Status:         ds.Status,
			GrayRatio:      ds.GrayRatio,
		}

		if existing, ok := r.skills[s.Name]; ok {
			s.FileVersion = existing.FileVersion
		}
		r.skills[s.Name] = s
	}
	return nil
}

func (r *Registry) GetSkillsByPage(page string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Skill
	for _, s := range r.skills {
		if s.Status == "enabled" && s.MatchesPage(page) {
			result = append(result, *s)
		}
	}
	return result
}

// GetToolsForPage 收集指定页面所有 Skill 关联的 Tool 名称
func (r *Registry) GetToolsForPage(page string) map[string]bool {
	skills := r.GetSkillsByPage(page)
	tools := map[string]bool{
		"list_bills": true,
		"create_bill": true,
	}
	for _, s := range skills {
		if s.MandatoryTool != "" {
			tools[s.MandatoryTool] = true
		}
	}
	return tools
}

func (r *Registry) GetAllSkills() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		if s.Status == "enabled" {
			result = append(result, *s)
		}
	}
	return result
}

func (r *Registry) GetSkillByName(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	if !ok {
		return Skill{}, false
	}
	return *s, true
}

func (r *Registry) GenerateSummary(page string) string {
	skills := r.GetSkillsByPage(page)
	if len(skills) == 0 {
		return ""
	}
	var summary string
	for _, s := range skills {
		summary += s.ToSummary() + "\n"
	}
	return summary
}

func (r *Registry) Reload(ctx context.Context) error {
	return r.Load(ctx)
}
