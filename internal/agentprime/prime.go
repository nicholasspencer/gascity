// Package agentprime composes the non-hook prompt shown by `gc prime --strict`.
package agentprime

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/gastownhall/gascity/internal/agentutil"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/git"
	"github.com/gastownhall/gascity/internal/promptmeta"
	workdirutil "github.com/gastownhall/gascity/internal/workdir"
)

const defaultPrompt = `# Gas City Agent

You are an agent in a Gas City workspace. Check for available work
and execute it.

## Your tools

- ` + "`bd ready`" + ` — see available work items
- ` + "`bd show <id>`" + ` — see details of a work item
- ` + "`bd close <id>`" + ` — mark work as done

## How to work

1. Check for available work: ` + "`bd ready`" + `
2. Pick a bead and execute the work described in its title
3. When done, close it: ` + "`bd close <id>`" + `
4. Check for more work. Repeat until the queue is empty.
`

const (
	canonicalPromptTemplateSuffix = ".template.md"
	legacyPromptTemplateSuffix    = ".md.tmpl"
)

var (
	// ErrAgentNameRequired reports that a prime request omitted the agent name.
	ErrAgentNameRequired = errors.New("agent name required")
	// ErrAgentNotFound reports that a prime request named no configured agent.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrConfigRequired reports that prompt composition was called without city config.
	ErrConfigRequired = errors.New("city config required")
)

// AgentNotFoundError carries the missing agent name for ErrAgentNotFound checks.
type AgentNotFoundError struct {
	Name string
}

func (e AgentNotFoundError) Error() string {
	return fmt.Sprintf("agent %q not found in city config", e.Name)
}

// Is reports whether target is ErrAgentNotFound.
func (e AgentNotFoundError) Is(target error) bool {
	return target == ErrAgentNotFound
}

// PromptTemplateError wraps failures while reading or executing an agent prompt template.
type PromptTemplateError struct {
	Agent    string
	Template string
	Err      error
}

func (e PromptTemplateError) Error() string {
	return fmt.Sprintf("prompt_template %q for agent %q: %v", e.Template, e.Agent, e.Err)
}

func (e PromptTemplateError) Unwrap() error {
	return e.Err
}

// Request describes the city and agent prompt to compose.
type Request struct {
	FS        fsys.FS
	CityPath  string
	CityName  string
	Config    *config.City
	AgentName string
	Stderr    io.Writer
	Store     beads.Store
}

// Result is the composed prompt returned to API callers.
type Result struct {
	Agent  string
	Prompt string
	Bytes  int
}

type promptContext struct {
	CityRoot            string
	AgentName           string
	TemplateName        string
	BindingName         string
	BindingPrefix       string
	RigName             string
	RigRoot             string
	WorkDir             string
	IssuePrefix         string
	Branch              string
	DefaultBranch       string
	WorkQuery           string
	SlingQuery          string
	ProviderKey         string
	ProviderDisplayName string
	InstructionsFile    string
	Env                 map[string]string
}

type promptRenderResult struct {
	Text    string
	Version string
	SHA     string
}

// ComposeStrict composes the prompt that `gc prime --strict` would display for an agent.
func ComposeStrict(req Request) (Result, error) {
	fs := req.FS
	if fs == nil {
		fs = fsys.OSFS{}
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	cfg := req.Config
	if cfg == nil {
		return Result{}, ErrConfigRequired
	}
	agentName := strings.TrimSpace(req.AgentName)
	if agentName == "" {
		return Result{}, ErrAgentNameRequired
	}
	if cfg.Workspace.Suspended {
		return Result{Agent: agentName}, nil
	}

	agentCfg, ok := resolvePrimeAgent(cfg, agentName)
	if !ok {
		return Result{}, AgentNotFoundError{Name: agentName}
	}
	if isAgentEffectivelySuspended(cfg, &agentCfg) {
		return Result{Agent: agentCfg.QualifiedName()}, nil
	}
	if agentCfg.PromptTemplate != "" {
		if _, err := fs.ReadFile(promptTemplateSourcePath(req.CityPath, agentCfg.PromptTemplate)); err != nil {
			return Result{}, PromptTemplateError{Agent: agentName, Template: agentCfg.PromptTemplate, Err: err}
		}
	}

	ctx := buildPrimeContext(req.CityPath, req.CityName, &agentCfg, cfg.Rigs, stderr)
	ctx.ProviderKey, ctx.ProviderDisplayName = providerInfoForAgent(&agentCfg, &cfg.Workspace, cfg.Providers)
	ctx.InstructionsFile = instructionsFileForAgent(&agentCfg, &cfg.Workspace, cfg.Providers)

	if agentCfg.PromptTemplate != "" {
		fragments := effectivePromptFragments(
			cfg.Workspace.GlobalFragments,
			agentCfg.InjectFragments,
			agentCfg.AppendFragments,
			agentCfg.InheritedAppendFragments,
			cfg.AgentDefaults.AppendFragments,
		)
		prompt := renderPrompt(fs, req.CityPath, req.CityName, agentCfg.PromptTemplate, ctx, cfg.Workspace.SessionTemplate, stderr,
			cfg.PackDirsForRig(ctx.RigName), fragments, req.Store)
		if prompt != "" {
			return Result{Agent: ctx.AgentName, Prompt: prompt, Bytes: len(prompt)}, nil
		}
	}

	if agentCfg.PromptTemplate == "" {
		if prompt := builtinWorkerPrompt(fs, req.CityPath, cfg, agentCfg); prompt != "" {
			return Result{Agent: ctx.AgentName, Prompt: prompt, Bytes: len(prompt)}, nil
		}
	}

	return Result{Agent: ctx.AgentName, Prompt: defaultPrompt, Bytes: len(defaultPrompt)}, nil
}

func resolvePrimeAgent(cfg *config.City, name string) (config.Agent, bool) {
	if a, ok := agentutil.ResolveAgent(cfg, name, agentutil.ResolveOpts{AllowPoolMembers: true}); ok {
		return a, true
	}
	return findAgentByName(cfg, name)
}

func findAgentByName(cfg *config.City, name string) (config.Agent, bool) {
	for _, a := range cfg.Agents {
		if a.Name == name {
			return a, true
		}
	}
	for _, a := range cfg.Agents {
		if !a.SupportsInstanceExpansion() || a.UsesCanonicalSingletonPoolIdentity() {
			continue
		}
		prefix := a.Name + "-"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := name[len(prefix):]
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 1 {
			continue
		}
		sp := agentutil.ScaleParamsFor(&a)
		if sp.Max >= 0 && n > sp.Max {
			continue
		}
		return agentutil.DeepCopyAgent(&a, name, a.Dir), true
	}
	return config.Agent{}, false
}

func builtinWorkerPrompt(fs fsys.FS, cityPath string, cfg *config.City, a config.Agent) string {
	promptFile := ""
	if cfg.Daemon.FormulaV2 {
		promptFile = citylayout.SystemPacksRoot + "/core/assets/prompts/graph-worker.md"
	} else if a.SupportsInstanceExpansion() || isPoolInstance(cfg, a) {
		promptFile = citylayout.SystemPacksRoot + "/core/assets/prompts/pool-worker.md"
	}
	if promptFile == "" {
		return ""
	}
	content, err := fs.ReadFile(filepath.Join(cityPath, promptFile))
	if err != nil {
		return ""
	}
	return string(content)
}

func isPoolInstance(cfg *config.City, a config.Agent) bool {
	for _, ca := range cfg.Agents {
		if !ca.SupportsInstanceExpansion() || ca.Dir != a.Dir {
			continue
		}
		if strings.HasPrefix(a.Name, ca.Name+"-") {
			return true
		}
	}
	return false
}

func isAgentEffectivelySuspended(cfg *config.City, a *config.Agent) bool {
	if cfg.Workspace.Suspended || a.Suspended {
		return true
	}
	if a.Dir == "" {
		return false
	}
	for _, r := range cfg.Rigs {
		if r.Name == a.Dir && r.Suspended {
			return true
		}
	}
	return false
}

func buildPrimeContext(cityPath, cityName string, a *config.Agent, rigs []config.Rig, stderr io.Writer) promptContext {
	ctx := promptContext{
		CityRoot:      cityPath,
		AgentName:     a.QualifiedName(),
		TemplateName:  a.Name,
		BindingName:   a.BindingName,
		BindingPrefix: a.BindingPrefix(),
		Env:           a.Env,
	}
	if rigName := configuredRigName(cityPath, a, rigs); rigName != "" {
		ctx.RigName = rigName
		ctx.RigRoot = workdirutil.RigRootForName(rigName, rigs)
		ctx.IssuePrefix = findRigPrefix(rigName, rigs)
	}
	ctx.DefaultBranch = defaultBranchForRig(ctx.RigName, rigs, ctx.WorkDir)
	ctx.WorkQuery = expandAgentCommandTemplate(cityPath, cityName, a, rigs, "work_query", a.EffectiveWorkQuery(), stderr)
	ctx.SlingQuery = expandAgentCommandTemplate(cityPath, cityName, a, rigs, "sling_query", a.EffectiveSlingQuery(), stderr)
	return ctx
}

func configuredRigName(cityPath string, a *config.Agent, rigs []config.Rig) string {
	if a == nil || a.Dir == "" {
		return ""
	}
	return workdirutil.ConfiguredRigName(cityPath, *a, rigs)
}

func expandAgentCommandTemplate(cityPath, cityName string, agentCfg *config.Agent, rigs []config.Rig, fieldName, command string, stderr io.Writer) string {
	if agentCfg == nil || command == "" || !strings.Contains(command, "{{") {
		return command
	}
	expanded, err := workdirutil.ExpandCommandTemplate(command, cityPath, cityName, *agentCfg, rigs)
	if err != nil {
		if stderr != nil {
			if fieldName == "" {
				fieldName = "command"
			}
			fmt.Fprintf(stderr, "expandAgentCommandTemplate: agent %q field %q: %v (using raw command)\n", agentCfg.QualifiedName(), fieldName, err) //nolint:errcheck
		}
		return command
	}
	return expanded
}

func renderPrompt(fs fsys.FS, cityPath, cityName, templatePath string, ctx promptContext, sessionTemplate string, stderr io.Writer, packDirs []string, injectFragments []string, store beads.Store) string {
	return renderPromptWithMeta(fs, cityPath, cityName, templatePath, ctx, sessionTemplate, stderr, packDirs, injectFragments, store).Text
}

func renderPromptWithMeta(fs fsys.FS, cityPath, cityName, templatePath string, ctx promptContext, sessionTemplate string, stderr io.Writer, packDirs []string, injectFragments []string, store beads.Store) promptRenderResult {
	if templatePath == "" {
		return promptRenderResult{}
	}
	sourcePath := promptTemplateSourcePath(cityPath, templatePath)
	data, err := fs.ReadFile(sourcePath)
	if err != nil {
		return promptRenderResult{}
	}
	raw := string(data)
	fm, body := promptmeta.Parse(raw)

	if !isPromptTemplatePath(templatePath) {
		return promptRenderResult{
			Text:    body,
			Version: fm.Version,
			SHA:     promptmeta.SHA(body),
		}
	}

	var tmpl *template.Template
	tmpl = template.New("prompt").
		Funcs(promptFuncMap(cityName, sessionTemplate, store, func() *template.Template { return tmpl })).
		Option("missingkey=zero")

	for _, dir := range packDirs {
		loadSharedTemplates(fs, tmpl, filepath.Join(dir, "prompts", "shared"), stderr)
		loadSharedTemplates(fs, tmpl, filepath.Join(dir, "template-fragments"), stderr)
	}
	loadSharedTemplates(fs, tmpl, filepath.Join(cityPath, "prompts", "shared"), stderr)
	loadSharedTemplates(fs, tmpl, filepath.Join(cityPath, "template-fragments"), stderr)
	loadSharedTemplates(fs, tmpl, filepath.Join(filepath.Dir(sourcePath), "shared"), stderr)
	loadSharedTemplates(fs, tmpl, filepath.Join(filepath.Dir(sourcePath), "template-fragments"), stderr)

	tmpl, err = tmpl.Parse(body)
	if err != nil {
		fmt.Fprintf(stderr, "gc: prompt template %q: %v\n", templatePath, err) //nolint:errcheck
		return promptRenderResult{Text: body, Version: fm.Version, SHA: promptmeta.SHA(body)}
	}

	td := buildTemplateData(ctx)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		fmt.Fprintf(stderr, "gc: prompt template %q: %v\n", templatePath, err) //nolint:errcheck
		return promptRenderResult{Text: body, Version: fm.Version, SHA: promptmeta.SHA(body)}
	}
	for _, name := range injectFragments {
		frag := tmpl.Lookup(name)
		if frag == nil {
			fmt.Fprintf(stderr, "gc: inject_fragment %q: template not found\n", name) //nolint:errcheck
			continue
		}
		var fbuf bytes.Buffer
		if err := frag.Execute(&fbuf, td); err != nil {
			fmt.Fprintf(stderr, "gc: inject_fragment %q: %v\n", name, err) //nolint:errcheck
			continue
		}
		buf.WriteString("\n\n")
		buf.Write(fbuf.Bytes())
	}

	rendered := buf.String()
	return promptRenderResult{Text: rendered, Version: fm.Version, SHA: promptmeta.SHA(rendered)}
}

func promptTemplateSourcePath(cityPath, templatePath string) string {
	if filepath.IsAbs(templatePath) {
		return templatePath
	}
	return filepath.Join(cityPath, templatePath)
}

func isPromptTemplatePath(path string) bool {
	return strings.HasSuffix(path, canonicalPromptTemplateSuffix) || strings.HasSuffix(path, legacyPromptTemplateSuffix)
}

func sharedTemplateFileNames(entries []os.DirEntry) []string {
	legacy := make([]string, 0, len(entries))
	canonical := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch name := e.Name(); {
		case strings.HasSuffix(name, legacyPromptTemplateSuffix):
			legacy = append(legacy, name)
		case strings.HasSuffix(name, canonicalPromptTemplateSuffix):
			canonical = append(canonical, name)
		}
	}
	sort.Strings(legacy)
	sort.Strings(canonical)
	return append(legacy, canonical...)
}

func loadSharedTemplates(fs fsys.FS, tmpl *template.Template, dir string, stderr io.Writer) {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return
	}
	for _, name := range sharedTemplateFileNames(entries) {
		if sdata, err := fs.ReadFile(filepath.Join(dir, name)); err == nil {
			if _, err := tmpl.Parse(string(sdata)); err != nil {
				fmt.Fprintf(stderr, "gc: shared template %q: %v\n", name, err) //nolint:errcheck
			}
		}
	}
}

func mergeFragmentLists(global, perAgent []string) []string {
	if len(global) == 0 && len(perAgent) == 0 {
		return nil
	}
	merged := make([]string, 0, len(global)+len(perAgent))
	seen := make(map[string]struct{}, len(global)+len(perAgent))
	merged = append(merged, global...)
	for _, name := range global {
		seen[name] = struct{}{}
	}
	for _, name := range perAgent {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, name)
	}
	return merged
}

func effectivePromptFragments(global, inject, appendFragments, inherited, defaults []string) []string {
	fragments := mergeFragmentLists(global, inject)
	fragments = mergeFragmentLists(fragments, appendFragments)
	fragments = mergeFragmentLists(fragments, inherited)
	return mergeFragmentLists(fragments, defaults)
}

func buildTemplateData(ctx promptContext) map[string]string {
	m := make(map[string]string, len(ctx.Env)+16)
	for k, v := range ctx.Env {
		m[k] = v
	}
	m["CityRoot"] = ctx.CityRoot
	m["AgentName"] = ctx.AgentName
	m["TemplateName"] = ctx.TemplateName
	m["BindingName"] = ctx.BindingName
	m["BindingPrefix"] = ctx.BindingPrefix
	m["RigName"] = ctx.RigName
	m["RigRoot"] = ctx.RigRoot
	m["WorkDir"] = ctx.WorkDir
	m["IssuePrefix"] = ctx.IssuePrefix
	m["Branch"] = ctx.Branch
	m["DefaultBranch"] = ctx.DefaultBranch
	m["WorkQuery"] = ctx.WorkQuery
	m["SlingQuery"] = ctx.SlingQuery
	m["ProviderKey"] = ctx.ProviderKey
	m["ProviderDisplayName"] = ctx.ProviderDisplayName
	m["InstructionsFile"] = ctx.InstructionsFile
	return m
}

func findRigPrefix(rigName string, rigs []config.Rig) string {
	for i := range rigs {
		if rigs[i].Name == rigName {
			return rigs[i].EffectivePrefix()
		}
	}
	return ""
}

func defaultBranchForRig(rigName string, rigs []config.Rig, dir string) string {
	if rigName != "" {
		for i := range rigs {
			if rigs[i].Name == rigName {
				if branch := rigs[i].EffectiveDefaultBranch(); branch != "" {
					return branch
				}
				break
			}
		}
	}
	if dir == "" {
		return "main"
	}
	branch, _ := git.New(dir).DefaultBranch()
	if branch == "" {
		return "main"
	}
	return branch
}

func promptFuncMap(cityName, sessionTemplate string, store beads.Store, parentTmpl func() *template.Template) template.FuncMap {
	return template.FuncMap{
		"cmd": func() string {
			return filepath.Base(os.Args[0])
		},
		"session": func(agentName string) string {
			return agentutil.LookupSessionName(store, cityName, agentName, sessionTemplate)
		},
		"basename": func(qualifiedName string) string {
			_, name := config.ParseQualifiedName(qualifiedName)
			return name
		},
		"templateFirst": func(data any, names ...string) (string, error) {
			t := parentTmpl()
			if t == nil {
				return "", nil
			}
			for _, name := range names {
				if name == "" {
					continue
				}
				frag := t.Lookup(name)
				if frag == nil {
					continue
				}
				var buf bytes.Buffer
				if err := frag.Execute(&buf, data); err != nil {
					return "", err
				}
				return buf.String(), nil
			}
			return "", nil
		},
	}
}

func providerInfoForAgent(a *config.Agent, ws *config.Workspace, cityProviders map[string]config.ProviderSpec) (key, displayName string) {
	if a == nil {
		return "", ""
	}
	name := a.Provider
	if name == "" && ws != nil {
		name = ws.Provider
	}
	if name == "" {
		return "", ""
	}
	return name, providerDisplayNameFor(name, cityProviders)
}

func instructionsFileForAgent(a *config.Agent, ws *config.Workspace, cityProviders map[string]config.ProviderSpec) string {
	const defaultInstructionsFile = "AGENTS.md"
	if a == nil {
		return defaultInstructionsFile
	}
	name := a.Provider
	if name == "" && ws != nil {
		name = ws.Provider
	}
	if name == "" {
		return defaultInstructionsFile
	}
	if spec, ok := cityProviders[name]; ok && spec.InstructionsFile != "" {
		return spec.InstructionsFile
	}
	if spec, ok := config.BuiltinProviders()[name]; ok && spec.InstructionsFile != "" {
		return spec.InstructionsFile
	}
	if family := config.BuiltinFamily(name, cityProviders); family != "" && family != name {
		if spec, ok := config.BuiltinProviders()[family]; ok && spec.InstructionsFile != "" {
			return spec.InstructionsFile
		}
	}
	return defaultInstructionsFile
}

func providerDisplayNameFor(name string, cityProviders map[string]config.ProviderSpec) string {
	if name == "" {
		return ""
	}
	if spec, ok := cityProviders[name]; ok && spec.DisplayName != "" {
		return spec.DisplayName
	}
	if spec, ok := config.BuiltinProviders()[name]; ok && spec.DisplayName != "" {
		return spec.DisplayName
	}
	if family := config.BuiltinFamily(name, cityProviders); family != "" && family != name {
		if spec, ok := config.BuiltinProviders()[family]; ok && spec.DisplayName != "" {
			return spec.DisplayName
		}
	}
	return name
}
