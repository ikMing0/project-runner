package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ProjectTemplate struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Project          Project `json:"project"`
	FrontendRelative string  `json:"frontendRelative"`
}
type TemplateBundle struct {
	Format    string            `json:"format"`
	Version   int               `json:"version"`
	Templates []ProjectTemplate `json:"templates"`
}

func cloneProject(p Project) Project {
	data, _ := json.Marshal(p)
	var result Project
	_ = json.Unmarshal(data, &result)
	return result
}

func templateFromProject(p Project, name string) (ProjectTemplate, error) {
	if p.Kind != "spring-maven" && p.Kind != "spring-gradle" && p.Kind != "node" {
		return ProjectTemplate{}, fmt.Errorf("模板启动类型无效")
	}
	template := ProjectTemplate{Name: strings.TrimSpace(name), Project: cloneProject(p)}
	if template.Name == "" || utf8.RuneCountInString(template.Name) > 160 {
		return template, fmt.Errorf("请输入 1–160 字符的模板名称")
	}
	if filepath.IsAbs(p.Module) {
		return template, fmt.Errorf("模板模块需要使用相对目录")
	}
	if p.Module != "" {
		if _, err := safeModule(p.Directory, p.Module); err != nil {
			return template, err
		}
	}
	if p.Frontend != nil {
		relative, err := filepath.Rel(p.Directory, p.Frontend.Directory)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return template, fmt.Errorf("保存模板时，配套前端需要位于项目目录内")
		}
		template.FrontendRelative = relative
		template.Project.Frontend.Directory = ""
	}
	template.Project.ID = ""
	template.Project.Directory = ""
	return template, nil
}

func (a *App) templatePath() (string, error) {
	a.mu.Lock()
	config := a.configPath
	a.mu.Unlock()
	if config == "" {
		return "", fmt.Errorf("无法确定本机模板目录")
	}
	return filepath.Join(filepath.Dir(config), "templates.json"), nil
}

func (a *App) ListTemplates() ([]ProjectTemplate, error) {
	path, err := a.templatePath()
	if err != nil {
		return nil, err
	}
	a.libraryMu.Lock()
	defer a.libraryMu.Unlock()
	var templates []ProjectTemplate
	err = readBoundedJSON(path, &templates, 2*1024*1024)
	if os.IsNotExist(err) {
		return []ProjectTemplate{}, nil
	}
	return templates, err
}

func (a *App) SaveTemplate(p Project, name string) (ProjectTemplate, error) {
	template, err := templateFromProject(p, name)
	if err != nil {
		return template, err
	}
	template.ID, err = newRecordID()
	if err != nil {
		return template, err
	}
	path, err := a.templatePath()
	if err != nil {
		return template, err
	}
	a.libraryMu.Lock()
	defer a.libraryMu.Unlock()
	var templates []ProjectTemplate
	if err = readBoundedJSON(path, &templates, 2*1024*1024); err != nil && !os.IsNotExist(err) {
		return template, err
	}
	if len(templates) >= 50 {
		return template, fmt.Errorf("最多保存 50 个模板，请先删除不再使用的模板")
	}
	templates = append(templates, template)
	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil || len(data) > 2*1024*1024 {
		return template, fmt.Errorf("模板内容超过本机 2 MiB 上限，请精简参数或减少模板")
	}
	return template, privateJSON(path, templates)
}

func (a *App) DeleteTemplate(id string) error {
	path, err := a.templatePath()
	if err != nil {
		return err
	}
	a.libraryMu.Lock()
	defer a.libraryMu.Unlock()
	var templates []ProjectTemplate
	if err = readBoundedJSON(path, &templates, 2*1024*1024); err != nil {
		return err
	}
	out := make([]ProjectTemplate, 0, len(templates))
	found := false
	for _, template := range templates {
		if template.ID == id {
			found = true
		} else {
			out = append(out, template)
		}
	}
	if !found {
		return fmt.Errorf("模板不存在")
	}
	return privateJSON(path, out)
}

// Sharing uses a strict allowlist. Local paths, all environment values, launch
// arguments and health URLs are excluded even if their names look harmless.
func shareTemplate(template ProjectTemplate) ProjectTemplate {
	p := template.Project
	project := Project{Name: p.Name, Kind: p.Kind, Module: p.Module, Port: p.Port,
		PackageManager: p.PackageManager, Script: p.Script, PortMode: p.PortMode,
		ConfigProperty: p.ConfigProperty, AutoOpen: p.AutoOpen, WaitForBackend: p.WaitForBackend}
	if p.Frontend != nil {
		f := p.Frontend
		project.Frontend = &FrontendConfig{Port: f.Port, PackageManager: f.PackageManager, Script: f.Script, PortMode: f.PortMode,
			AutoProxy: f.AutoProxy, ProxyVariable: f.ProxyVariable}
	}
	return ProjectTemplate{Name: template.Name, Project: project, FrontendRelative: template.FrontendRelative}
}

func templateBundle(template ProjectTemplate) TemplateBundle {
	return TemplateBundle{Format: "project-runner-templates", Version: 1, Templates: []ProjectTemplate{shareTemplate(template)}}
}

func (a *App) ExportProjectTemplate(p Project) (string, error) {
	template, err := templateFromProject(p, p.Name)
	if err != nil {
		return "", err
	}
	bundle := templateBundle(template)
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "导出脱敏分享模板", DefaultFilename: "project-runner-template.json", Filters: []runtime.FileFilter{{DisplayName: "JSON 模板", Pattern: "*.json"}}})
	if err != nil || path == "" {
		return "", err
	}
	return path, privateJSON(path, bundle)
}

func validateImportedTemplate(template ProjectTemplate) (ProjectTemplate, error) {
	p := template.Project
	if p.Kind != "spring-maven" && p.Kind != "spring-gradle" && p.Kind != "node" {
		return template, fmt.Errorf("模板启动类型无效")
	}
	if strings.TrimSpace(template.Name) == "" || utf8.RuneCountInString(template.Name) > 160 {
		return template, fmt.Errorf("模板名称无效")
	}
	// Ignore executable/private fields supplied by a manually edited JSON file.
	template = shareTemplate(template)
	template.ID = ""
	for _, relative := range []string{template.Project.Module, template.FrontendRelative} {
		clean := filepath.Clean(relative)
		if filepath.IsAbs(relative) || strings.Contains(relative, ":") || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return template, fmt.Errorf("模板目录必须位于选择的项目目录内")
		}
	}
	return template, nil
}

func readTemplateBundle(path string) ([]ProjectTemplate, error) {
	var bundle TemplateBundle
	if err := readBoundedJSON(path, &bundle, 1024*1024); err != nil {
		return nil, err
	}
	if bundle.Format != "project-runner-templates" || bundle.Version != 1 || len(bundle.Templates) == 0 || len(bundle.Templates) > 20 {
		return nil, fmt.Errorf("文件不是支持的运行台模板，或模板数量超过 20 个")
	}
	out := make([]ProjectTemplate, 0, len(bundle.Templates))
	for _, template := range bundle.Templates {
		validated, err := validateImportedTemplate(template)
		if err != nil {
			return nil, err
		}
		out = append(out, validated)
	}
	return out, nil
}

func (a *App) ReadTemplateFile() ([]ProjectTemplate, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择运行台模板（先预览，不自动保存）", Filters: []runtime.FileFilter{{DisplayName: "JSON 模板", Pattern: "*.json"}}})
	if err != nil || path == "" {
		return []ProjectTemplate{}, err
	}
	return readTemplateBundle(path)
}

func (a *App) ApplyTemplate(template ProjectTemplate, directory string) (Project, error) {
	if strings.TrimSpace(directory) == "" {
		return Project{}, fmt.Errorf("请选择模板对应的项目目录")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return Project{}, fmt.Errorf("项目目录不存在")
	}
	// Keep local tool/config references for a saved local template. Imported
	// templates have already passed through the sharing allowlist.
	p := cloneProject(template.Project)
	if strings.TrimSpace(p.Name) == "" {
		p.Name = template.Name
	}
	p.ID = ""
	p.Directory = directory
	if p.Module != "" {
		if _, err := safeModule(directory, p.Module); err != nil {
			return Project{}, err
		}
	}
	if p.Frontend != nil {
		relative := template.FrontendRelative
		if relative == "" {
			return Project{}, fmt.Errorf("模板缺少前端相对目录，请重新识别前端")
		}
		dir, err := safeModule(directory, relative)
		if err != nil {
			return Project{}, err
		}
		p.Frontend.Directory = dir
	}
	return p, nil
}
