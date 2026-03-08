package providers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AvengeMedia/DankMaterialShell/core/internal/keybinds"
	"github.com/sblinch/kdl-go/document"
)

type NiriProvider struct {
	configDir        string
	dmsBindsIncluded bool
	parsed           bool
}

func NewNiriProvider(configDir string) *NiriProvider {
	if configDir == "" {
		configDir = defaultNiriConfigDir()
	}
	return &NiriProvider{
		configDir: configDir,
	}
}

func defaultNiriConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "niri")
}

func (n *NiriProvider) Name() string {
	return "niri"
}

func (n *NiriProvider) GetCheatSheet() (*keybinds.CheatSheet, error) {
	result, err := ParseNiriKeys(n.configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse niri config: %w", err)
	}

	n.dmsBindsIncluded = result.DMSBindsIncluded
	n.parsed = true

	categorizedBinds := make(map[string][]keybinds.Keybind)
	n.convertSection(result.Section, "", categorizedBinds, result.ConflictingConfigs)

	sheet := &keybinds.CheatSheet{
		Title:            "Niri Keybinds",
		Provider:         n.Name(),
		Binds:            categorizedBinds,
		DMSBindsIncluded: result.DMSBindsIncluded,
	}

	if result.DMSStatus != nil {
		sheet.DMSStatus = &keybinds.DMSBindsStatus{
			Exists:          result.DMSStatus.Exists,
			Included:        result.DMSStatus.Included,
			IncludePosition: result.DMSStatus.IncludePosition,
			TotalIncludes:   result.DMSStatus.TotalIncludes,
			BindsAfterDMS:   result.DMSStatus.BindsAfterDMS,
			Effective:       result.DMSStatus.Effective,
			OverriddenBy:    result.DMSStatus.OverriddenBy,
			StatusMessage:   result.DMSStatus.StatusMessage,
		}
	}

	return sheet, nil
}

func (n *NiriProvider) HasDMSBindsIncluded() bool {
	if n.parsed {
		return n.dmsBindsIncluded
	}

	result, err := ParseNiriKeys(n.configDir)
	if err != nil {
		return false
	}

	n.dmsBindsIncluded = result.DMSBindsIncluded
	n.parsed = true
	return n.dmsBindsIncluded
}

func (n *NiriProvider) convertSection(section *NiriSection, subcategory string, categorizedBinds map[string][]keybinds.Keybind, conflicts map[string]*NiriKeyBinding) {
	currentSubcat := subcategory
	if section.Name != "" {
		currentSubcat = section.Name
	}

	for _, kb := range section.Keybinds {
		category := n.categorizeByAction(kb.Action)
		bind := n.convertKeybind(&kb, currentSubcat, conflicts)
		categorizedBinds[category] = append(categorizedBinds[category], bind)
	}

	for _, child := range section.Children {
		n.convertSection(&child, currentSubcat, categorizedBinds, conflicts)
	}
}

func (n *NiriProvider) categorizeByAction(action string) string {
	switch {
	case action == "next-window" || action == "previous-window":
		return "Alt-Tab"
	case strings.Contains(action, "screenshot"):
		return "Screenshot"
	case action == "show-hotkey-overlay" || action == "toggle-overview":
		return "Overview"
	case action == "quit" ||
		action == "power-off-monitors" ||
		action == "power-on-monitors" ||
		action == "suspend" ||
		action == "do-screen-transition" ||
		action == "toggle-keyboard-shortcuts-inhibit" ||
		strings.Contains(action, "dpms"):
		return "System"
	case action == "spawn":
		return "Execute"
	case strings.Contains(action, "workspace"):
		return "Workspace"
	case strings.HasPrefix(action, "focus-monitor") ||
		strings.HasPrefix(action, "move-column-to-monitor") ||
		strings.HasPrefix(action, "move-window-to-monitor"):
		return "Monitor"
	case strings.Contains(action, "window") ||
		strings.Contains(action, "focus") ||
		strings.Contains(action, "move") ||
		strings.Contains(action, "swap") ||
		strings.Contains(action, "resize") ||
		strings.Contains(action, "column"):
		return "Window"
	default:
		return "Other"
	}
}

func (n *NiriProvider) convertKeybind(kb *NiriKeyBinding, subcategory string, conflicts map[string]*NiriKeyBinding) keybinds.Keybind {
	rawAction := n.formatRawAction(kb.Action, kb.Args)
	keyStr := n.formatKey(kb)

	source := "config"
	if strings.Contains(kb.Source, "dms/binds.kdl") {
		source = "dms"
	}

	bind := keybinds.Keybind{
		Key:             keyStr,
		Description:     kb.Description,
		Action:          rawAction,
		Subcategory:     subcategory,
		Source:          source,
		HideOnOverlay:   kb.HideOnOverlay,
		CooldownMs:      kb.CooldownMs,
		AllowWhenLocked: kb.AllowWhenLocked,
		AllowInhibiting: kb.AllowInhibiting,
		Repeat:          kb.Repeat,
	}

	if source == "dms" && conflicts != nil {
		if conflictKb, ok := conflicts[keyStr]; ok {
			bind.Conflict = &keybinds.Keybind{
				Key:         keyStr,
				Description: conflictKb.Description,
				Action:      n.formatRawAction(conflictKb.Action, conflictKb.Args),
				Source:      "config",
			}
		}
	}

	return bind
}

func (n *NiriProvider) formatRawAction(action string, args []string) string {
	if len(args) == 0 {
		return action
	}

	if action == "spawn" && len(args) >= 3 && args[1] == "-c" {
		switch args[0] {
		case "sh", "bash":
			cmd := strings.Join(args[2:], " ")
			return fmt.Sprintf("spawn %s -c \"%s\"", args[0], strings.ReplaceAll(cmd, "\"", "\\\""))
		}
	}

	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		if arg == "" {
			quotedArgs[i] = `""`
		} else {
			quotedArgs[i] = arg
		}
	}
	return action + " " + strings.Join(quotedArgs, " ")
}

func (n *NiriProvider) formatKey(kb *NiriKeyBinding) string {
	parts := make([]string, 0, len(kb.Mods)+1)
	parts = append(parts, kb.Mods...)
	parts = append(parts, kb.Key)
	return strings.Join(parts, "+")
}

func (n *NiriProvider) GetOverridePath() string {
	return filepath.Join(n.configDir, "dms", "binds.kdl")
}

func (n *NiriProvider) validateAction(action string) error {
	action = strings.TrimSpace(action)
	switch {
	case action == "":
		return fmt.Errorf("action cannot be empty")
	case action == "spawn" || action == "spawn ":
		return fmt.Errorf("spawn command requires arguments")
	case strings.HasPrefix(action, "spawn "):
		rest := strings.TrimSpace(strings.TrimPrefix(action, "spawn "))
		switch rest {
		case "":
			return fmt.Errorf("spawn command requires arguments")
		case "sh -c \"\"", "sh -c ''", "bash -c \"\"", "bash -c ''":
			return fmt.Errorf("shell command cannot be empty")
		}
	}
	return nil
}

// ---------- SURGICAL SetBind (preserves comments & formatting) ----------

func (n *NiriProvider) SetBind(key, action, description string, options map[string]any) error {
	if err := n.validateAction(action); err != nil {
		return err
	}

	overridePath := n.GetOverridePath()

	if err := os.MkdirAll(filepath.Dir(overridePath), 0o755); err != nil {
		return fmt.Errorf("failed to create dms directory: %w", err)
	}

	bind := &overrideBind{
		Key:         key,
		Action:      action,
		Description: description,
		Options:     options,
	}

	// Read existing file
	data, err := os.ReadFile(overridePath)
	if os.IsNotExist(err) {
		// Create new file with just this bind
		content := "binds {\n" + n.formatBindLine(bind, "    ") + "}\n"
		return n.validateAndWrite(overridePath, content)
	}
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	// Try to find and replace existing bind in-place
	idx := n.findBindLine(lines, key)
	if idx >= 0 {
		// Preserve the original indentation
		indent := n.getLineIndent(lines[idx])
		lines[idx] = strings.TrimRight(n.formatBindLine(bind, indent), "\n")
		return n.validateAndWrite(overridePath, strings.Join(lines, "\n"))
	}

	// New bind: insert into GUI BINDS section
	if n.isRecentWindowsAction(action) {
		insertIdx := n.findBlockClosing(lines, "recent-windows")
		if insertIdx < 0 {
			// No recent-windows block — append one at the end
			result := strings.TrimRight(string(data), "\n") + "\n\n"
			result += "recent-windows {\n"
			result += "    binds {\n"
			result += n.formatBindLine(bind, "        ")
			result += "    }\n"
			result += "}\n"
			return n.validateAndWrite(overridePath, result)
		}
		newLine := strings.TrimRight(n.formatBindLine(bind, "        "), "\n")
		lines = insertBeforeIndex(lines, insertIdx, newLine)
	} else {
		insertIdx := n.findBlockClosing(lines, "binds")
		if insertIdx < 0 {
			return fmt.Errorf("could not find binds block in %s", overridePath)
		}

		// Check if GUI BINDS section header exists
		guiSectionExists := false
		for _, line := range lines {
			if strings.Contains(line, "GUI BINDS") {
				guiSectionExists = true
				break
			}
		}

		if !guiSectionExists {
			// Create the GUI BINDS section header before the closing brace
			header := []string{
				"",
				"    // ===========================",
				"    // GUI BINDS",
				"    // ===========================",
				"",
			}
			newLine := strings.TrimRight(n.formatBindLine(bind, "    "), "\n")
			header = append(header, newLine)
			for i := len(header) - 1; i >= 0; i-- {
				lines = insertBeforeIndex(lines, insertIdx, header[i])
			}
		} else {
			// Insert before closing brace (after existing GUI binds)
			newLine := strings.TrimRight(n.formatBindLine(bind, "    "), "\n")
			lines = insertBeforeIndex(lines, insertIdx, newLine)
		}
	}

	return n.validateAndWrite(overridePath, strings.Join(lines, "\n"))
}

// ---------- SURGICAL RemoveBind (preserves comments & formatting) ----------

func (n *NiriProvider) RemoveBind(key string) error {
	overridePath := n.GetOverridePath()

	data, err := os.ReadFile(overridePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	idx := n.findBindLine(lines, key)
	if idx < 0 {
		return nil // not found, nothing to do
	}

	// Remove the line
	lines = append(lines[:idx], lines[idx+1:]...)

	return n.validateAndWrite(overridePath, strings.Join(lines, "\n"))
}

// ---------- HELPERS ----------

type overrideBind struct {
	Key         string
	Action      string
	Description string
	Options     map[string]any
}

// formatBindLine generates a single KDL bind line with the given indentation.
func (n *NiriProvider) formatBindLine(bind *overrideBind, indent string) string {
	var sb strings.Builder
	n.writeBindNode(&sb, bind, indent)
	return sb.String()
}

// findBindLine returns the line index containing the given key combo, or -1.
func (n *NiriProvider) findBindLine(lines []string, key string) int {
	pattern := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `[\s{]`)
	for i, line := range lines {
		if pattern.MatchString(line) {
			return i
		}
	}
	return -1
}

// getLineIndent returns the leading whitespace of a line.
func (n *NiriProvider) getLineIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// findBlockClosing finds the closing } of a named top-level block.
// For "recent-windows", returns the inner binds {} closing brace.
func (n *NiriProvider) findBlockClosing(lines []string, blockName string) int {
	inBlock := false
	depth := 0
	targetDepth := 1
	if blockName == "recent-windows" {
		targetDepth = 2
	}

	blockPattern := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(blockName) + `\s*\{`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inBlock && blockPattern.MatchString(line) {
			inBlock = true
			depth = 1
			continue
		}

		if !inBlock {
			continue
		}

		for _, ch := range trimmed {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				if depth == targetDepth {
					return i
				}
				depth--
				if depth <= 0 {
					return -1
				}
			}
		}
	}
	return -1
}

// validateAndWrite validates the KDL content with niri and writes it.
func (n *NiriProvider) validateAndWrite(path, content string) error {
	if err := n.validateBindsContent(content); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// insertBeforeIndex inserts a new element before the given index.
func insertBeforeIndex(lines []string, idx int, newLine string) []string {
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:idx]...)
	result = append(result, newLine)
	result = append(result, lines[idx:]...)
	return result
}

func (n *NiriProvider) isRecentWindowsAction(action string) bool {
	switch action {
	case "next-window", "previous-window":
		return true
	default:
		return false
	}
}

// ---------- KEPT: Line generation helpers ----------

func (n *NiriProvider) buildBindNode(bind *overrideBind) *document.Node {
	node := document.NewNode()
	node.SetName(bind.Key)

	if bind.Options != nil {
		if v, ok := bind.Options["repeat"]; ok && v == false {
			node.AddProperty("repeat", false, "")
		}
		if v, ok := bind.Options["cooldown-ms"]; ok {
			switch val := v.(type) {
			case int:
				node.AddProperty("cooldown-ms", val, "")
			case string:
				if ms, err := strconv.Atoi(val); err == nil {
					node.AddProperty("cooldown-ms", ms, "")
				}
			}
		}
		if v, ok := bind.Options["allow-when-locked"]; ok && v == true {
			node.AddProperty("allow-when-locked", true, "")
		}
		if v, ok := bind.Options["allow-inhibiting"]; ok && v == false {
			node.AddProperty("allow-inhibiting", false, "")
		}
	}

	if bind.Description != "" {
		node.AddProperty("hotkey-overlay-title", bind.Description, "")
	}

	actionNode := n.buildActionNode(bind.Action)
	node.AddNode(actionNode)

	return node
}

func (n *NiriProvider) buildActionNode(action string) *document.Node {
	action = strings.TrimSpace(action)
	node := document.NewNode()

	parts := n.parseActionParts(action)
	if len(parts) == 0 {
		node.SetName(action)
		return node
	}

	node.SetName(parts[0])
	for _, arg := range parts[1:] {
		if strings.Contains(arg, "=") {
			kv := strings.SplitN(arg, "=", 2)
			switch kv[1] {
			case "true":
				node.AddProperty(kv[0], true, "")
			case "false":
				node.AddProperty(kv[0], false, "")
			default:
				node.AddProperty(kv[0], kv[1], "")
			}
			continue
		}
		node.AddArgument(arg, "")
	}
	return node
}

func (n *NiriProvider) parseActionParts(action string) []string {
	var parts []string
	var current strings.Builder
	var inQuote, escaped, wasQuoted bool

	for _, r := range action {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			wasQuoted = true
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if current.Len() > 0 || wasQuoted {
				parts = append(parts, current.String())
				current.Reset()
				wasQuoted = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 || wasQuoted {
		parts = append(parts, current.String())
	}
	return parts
}

func (n *NiriProvider) writeBindNode(sb *strings.Builder, bind *overrideBind, indent string) {
	node := n.buildBindNode(bind)

	sb.WriteString(indent)
	sb.WriteString(node.Name.String())

	if node.Properties.Exist() {
		sb.WriteString(" ")
		sb.WriteString(strings.TrimLeft(node.Properties.String(), " "))
	}

	sb.WriteString(" { ")
	if len(node.Children) > 0 {
		child := node.Children[0]
		actionName := child.Name.String()
		sb.WriteString(actionName)
		forceQuote := actionName == "spawn"
		for _, arg := range child.Arguments {
			sb.WriteString(" ")
			n.writeArg(sb, arg.ValueString(), forceQuote)
		}
		if child.Properties.Exist() {
			sb.WriteString(" ")
			sb.WriteString(strings.TrimLeft(child.Properties.String(), " "))
		}
	}
	sb.WriteString("; }\n")
}

func (n *NiriProvider) writeArg(sb *strings.Builder, val string, forceQuote bool) {
	if !forceQuote && n.isNumericArg(val) {
		sb.WriteString(val)
		return
	}
	sb.WriteString("\"")
	sb.WriteString(strings.ReplaceAll(val, "\"", "\\\""))
	sb.WriteString("\"")
}

func (n *NiriProvider) isNumericArg(val string) bool {
	if val == "" {
		return false
	}
	start := 0
	if val[0] == '-' || val[0] == '+' {
		if len(val) == 1 {
			return false
		}
		start = 1
	}
	for i := start; i < len(val); i++ {
		if val[i] < '0' || val[i] > '9' {
			return false
		}
	}
	return true
}

func (n *NiriProvider) validateBindsContent(content string) error {
	tmpFile, err := os.CreateTemp("", "dms-binds-*.kdl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	cmd := exec.Command("niri", "validate", "-c", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("invalid config: %s", strings.TrimSpace(string(output)))
	}

	return nil
}
