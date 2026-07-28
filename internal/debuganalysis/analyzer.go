package debuganalysis

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	pathpkg "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ccLoad/internal/model"
)

const (
	MaxAnalysisImages          = 20
	MaxAnalysisImageBytes      = 20 * 1024 * 1024
	MaxAnalysisImageTotalBytes = 50 * 1024 * 1024
)

var (
	supportedImageMIMEs = map[string]bool{
		"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	}
	dataImageRE = regexp.MustCompile(`(?i)^data:(image/(?:png|jpeg|gif|webp));base64,([A-Za-z0-9+/]+={0,2})$`)
	pathRE      = regexp.MustCompile(`(?:^|[\s'"` + "`" + `])((?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.@()+,=:\\-]+)`)
	spaceRE     = regexp.MustCompile(`\s+`)
	fileArgKeys = map[string]bool{
		"file_path": true, "filepath": true, "path": true, "absolute_path": true,
		"relative_path": true, "notebook_path": true, "target_file": true,
		"target_path": true, "pattern": true, "glob": true,
	}
	toolNameHints = []string{
		"read", "write", "edit", "multiedit", "glob", "grep", "ls", "list", "find",
		"view", "open", "cat", "sed", "apply_patch",
	}
)

type UserQuestion struct {
	Index   int    `json:"index"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TextItem struct {
	Index   int    `json:"index"`
	Source  string `json:"source"`
	Content string `json:"content"`
}

type Image struct {
	Index    int    `json:"index"`
	Source   string `json:"source"`
	Location string `json:"location"`
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
	Bytes    int    `json:"bytes"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

type PathInfo struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Source string `json:"source"`
}

type ToolFileTree struct {
	Summary  string     `json:"summary"`
	Paths    []PathInfo `json:"paths"`
	TreeText string     `json:"tree_text"`
}

type Result struct {
	LogID         int64          `json:"log_id"`
	CreatedAt     int64          `json:"created_at"`
	AnalyzedAt    string         `json:"analyzed_at"`
	Source        map[string]any `json:"source"`
	UserQuestions []UserQuestion `json:"user_questions"`
	ToolFileTree  ToolFileTree   `json:"tool_file_tree"`
	AITexts       []TextItem     `json:"ai_texts"`
	FinalAIText   string         `json:"final_ai_text"`
	Images        []Image        `json:"images"`
	ToolCalls     []ToolCall     `json:"tool_calls"`
	Errors        []string       `json:"errors"`
}

func Analyze(entry *model.DebugLogEntry, sourceDir string) *Result {
	result := &Result{
		AnalyzedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Source:        map[string]any{"debug_log_dir": sourceDir, "format": "token-files-v2-gzip"},
		UserQuestions: []UserQuestion{},
		AITexts:       []TextItem{},
		Images:        []Image{},
		ToolCalls:     []ToolCall{},
		Errors:        []string{},
		ToolFileTree: ToolFileTree{
			Summary: "由 AI tool 调用参数推断出的文件/目录结构",
			Paths:   []PathInfo{},
		},
	}
	if entry == nil {
		result.Errors = append(result.Errors, "debug log entry is nil")
		return result
	}
	result.LogID = entry.LogID
	result.CreatedAt = entry.CreatedAt

	reqText := decodeBody(entry.ReqBody)
	respText := decodeBody(entry.RespBody)
	reqObj, reqOK := parseJSON(reqText)
	respObj, respOK := parseJSON(respText)
	var respEvents []any
	if !reqOK && reqText != "" {
		result.Errors = append(result.Errors, "req_body is not valid JSON")
	}
	if !respOK && respText != "" {
		respEvents = parseSSEJSONEvents(respText)
		if len(respEvents) == 0 {
			result.Errors = append(result.Errors, "resp_body is not valid JSON")
		}
	}

	if reqOK {
		result.UserQuestions = extractUserQuestions(reqObj)
		collectToolCalls(reqObj, &result.ToolCalls)
	}
	if respOK {
		collectToolCalls(respObj, &result.ToolCalls)
		result.AITexts = extractAITexts(respObj, result.AITexts)
	}
	if len(respEvents) > 0 {
		collectToolCalls(respEvents, &result.ToolCalls)
		result.AITexts = extractAITextsFromEvents(respEvents)
	}
	if len(result.AITexts) > 0 {
		result.FinalAIText = result.AITexts[len(result.AITexts)-1].Content
	}

	seenImages := make(map[string]bool)
	totalImageBytes := 0
	appendImages := func(obj any, source string) {
		remaining := MaxAnalysisImages - len(result.Images)
		if remaining <= 0 {
			return
		}
		for _, image := range ExtractBase64Images(obj, source, remaining) {
			digest := sha256.Sum256([]byte(image.Data))
			key := source + ":" + hex.EncodeToString(digest[:])
			if seenImages[key] || totalImageBytes+image.Bytes > MaxAnalysisImageTotalBytes {
				continue
			}
			seenImages[key] = true
			image.Index = len(result.Images)
			result.Images = append(result.Images, image)
			totalImageBytes += image.Bytes
		}
	}
	if reqOK {
		appendImages(reqObj, "input")
	}
	if respOK {
		appendImages(respObj, "output")
	}
	if len(respEvents) > 0 {
		appendImages(respEvents, "output")
	}

	paths := make(map[string]PathInfo)
	for _, call := range result.ToolCalls {
		for _, path := range pathsFromValue(call.Arguments) {
			paths[path] = PathInfo{Path: path, Type: inferType(path), Source: "tool_call"}
		}
	}
	pathNames := make([]string, 0, len(paths))
	for path := range paths {
		pathNames = append(pathNames, path)
	}
	sort.Strings(pathNames)
	for _, path := range pathNames {
		result.ToolFileTree.Paths = append(result.ToolFileTree.Paths, paths[path])
	}
	result.ToolFileTree.TreeText = buildTree(pathNames)
	return result
}

func decodeBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if utf8.Valid(body) {
		return string(body)
	}
	return base64.StdEncoding.EncodeToString(body)
}

func parseJSON(text string) (any, bool) {
	if text == "" {
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return value, true
}

func parseSSEJSONEvents(text string) []any {
	if text == "" || !strings.Contains(text, "data:") {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var events []any
	for _, block := range strings.Split(text, "\n\n") {
		var dataLines []string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimLeft(line[5:], " \t"))
			}
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if value, ok := parseJSON(data); ok {
			events = append(events, value)
		}
	}
	return events
}

func orderedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func asMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func asSlice(value any) ([]any, bool) {
	result, ok := value.([]any)
	return result, ok
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func walkMessages(value any, out *[]map[string]any) {
	switch current := value.(type) {
	case map[string]any:
		if messages, ok := asSlice(current["messages"]); ok {
			for _, item := range messages {
				if message, ok := asMap(item); ok {
					*out = append(*out, message)
				}
			}
		}
		if inputs, ok := asSlice(current["input"]); ok {
			for _, item := range inputs {
				message, ok := asMap(item)
				if ok && stringValue(message["type"]) == "message" {
					*out = append(*out, message)
				}
			}
		}
		if stringValue(current["type"]) == "message" && stringValue(current["role"]) != "" {
			*out = append(*out, current)
		}
		for _, key := range orderedKeys(current) {
			switch current[key].(type) {
			case map[string]any, []any:
				walkMessages(current[key], out)
			}
		}
	case []any:
		for _, item := range current {
			walkMessages(item, out)
		}
	}
}

func textFromContent(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			switch part := item.(type) {
			case string:
				parts = append(parts, part)
			case map[string]any:
				if text := stringValue(part["text"]); text != "" {
					parts = append(parts, text)
				} else if text := stringValue(part["content"]); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text := stringValue(value["text"]); text != "" {
			return text
		}
		return stringValue(value["content"])
	default:
		return ""
	}
}

func extractUserQuestions(value any) []UserQuestion {
	var messages []map[string]any
	walkMessages(value, &messages)
	questions := make([]UserQuestion, 0)
	seen := make(map[string]bool)
	for _, message := range messages {
		if stringValue(message["role"]) != "user" {
			continue
		}
		text := textFromContent(message["content"])
		if strings.TrimSpace(text) == "" || seen[text] {
			continue
		}
		seen[text] = true
		questions = append(questions, UserQuestion{Index: len(questions), Role: "user", Content: text})
	}
	return questions
}

type imageCandidate struct {
	mimeType string
	data     string
	bytes    int
}

func normalizeBase64Image(data any, mimeType any) *imageCandidate {
	mime := strings.ToLower(strings.TrimSpace(stringValue(mimeType)))
	dataString, ok := data.(string)
	if !ok || (mime != "" && !supportedImageMIMEs[mime]) {
		return nil
	}
	compact := spaceRE.ReplaceAllString(dataString, "")
	if compact == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil || len(decoded) == 0 || len(decoded) > MaxAnalysisImageBytes {
		return nil
	}
	if mime == "" {
		switch {
		case len(decoded) >= 8 && string(decoded[:8]) == "\x89PNG\r\n\x1a\n":
			mime = "image/png"
		case len(decoded) >= 3 && decoded[0] == 0xff && decoded[1] == 0xd8 && decoded[2] == 0xff:
			mime = "image/jpeg"
		case len(decoded) >= 6 && (string(decoded[:6]) == "GIF87a" || string(decoded[:6]) == "GIF89a"):
			mime = "image/gif"
		case len(decoded) >= 12 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WEBP":
			mime = "image/webp"
		default:
			return nil
		}
	}
	return &imageCandidate{mimeType: mime, data: base64.StdEncoding.EncodeToString(decoded), bytes: len(decoded)}
}

func imageFromDataURL(value any) *imageCandidate {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	match := dataImageRE.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) != 3 {
		return nil
	}
	return normalizeBase64Image(match[2], match[1])
}

func imageFromMapping(value map[string]any) *imageCandidate {
	var mime any
	for _, key := range []string{"mime_type", "media_type", "mimeType", "mediaType", "content_type", "contentType"} {
		if stringValue(value[key]) != "" {
			mime = value[key]
			break
		}
	}
	var data any
	for _, key := range []string{"data", "base64", "data_base64", "b64_json", "result"} {
		if _, ok := value[key].(string); ok {
			data = value[key]
			break
		}
	}
	if data != nil {
		if normalized := normalizeBase64Image(data, mime); normalized != nil {
			return normalized
		}
	}
	valueType := stringValue(value["type"])
	if valueType == "image_generation_call" || valueType == "image_generation" {
		generated := value["result"]
		if generated == nil {
			generated = value["b64_json"]
		}
		return normalizeBase64Image(generated, "image/png")
	}
	return nil
}

// ExtractBase64Images collects supported inline images and returns browser-safe payloads.
func ExtractBase64Images(value any, source string, limit int) []Image {
	if limit <= 0 {
		return []Image{}
	}
	images := make([]Image, 0)
	seen := make(map[string]bool)
	var walk func(any, string)
	appendImage := func(candidate *imageCandidate, location string) {
		if candidate == nil || len(images) >= limit {
			return
		}
		digest := sha256.Sum256([]byte(candidate.data))
		key := hex.EncodeToString(digest[:])
		if seen[key] {
			return
		}
		seen[key] = true
		images = append(images, Image{
			Index: len(images), Source: source, Location: location,
			MIMEType: candidate.mimeType, Data: candidate.data, Bytes: candidate.bytes,
		})
	}
	walk = func(current any, location string) {
		if len(images) >= limit {
			return
		}
		switch item := current.(type) {
		case string:
			appendImage(imageFromDataURL(item), location)
		case map[string]any:
			appendImage(imageFromMapping(item), location)
			for _, key := range orderedKeys(item) {
				walk(item[key], location+"."+key)
			}
		case []any:
			for i, child := range item {
				walk(child, fmt.Sprintf("%s[%d]", location, i))
			}
		}
	}
	walk(value, "$")
	return images
}

func appendUniqueText(out []TextItem, text, source string) []TextItem {
	text = strings.TrimSpace(text)
	if text == "" {
		return out
	}
	for _, item := range out {
		if item.Content == text {
			return out
		}
	}
	return append(out, TextItem{Index: len(out), Source: source, Content: text})
}

func extractAITexts(value any, out []TextItem) []TextItem {
	switch current := value.(type) {
	case map[string]any:
		if choices, ok := asSlice(current["choices"]); ok {
			for _, choiceValue := range choices {
				choice, ok := asMap(choiceValue)
				if !ok {
					continue
				}
				message, ok := asMap(choice["message"])
				if ok && stringValue(message["role"]) == "assistant" {
					out = appendUniqueText(out, textFromContent(message["content"]), "choices.message")
				}
			}
		}
		valueType := stringValue(current["type"])
		if valueType == "message" && stringValue(current["role"]) == "assistant" {
			out = appendUniqueText(out, textFromContent(current["content"]), "response.message")
		}
		if valueType == "output_text" {
			out = appendUniqueText(out, stringValue(current["text"]), "output_text")
		}
		if valueType == "response.output_text.done" {
			out = appendUniqueText(out, stringValue(current["text"]), "response.output_text.done")
		}
		if valueType == "response.output_item.done" {
			if item, ok := asMap(current["item"]); ok {
				out = extractAITexts(item, out)
			}
		}
		for _, key := range orderedKeys(current) {
			switch current[key].(type) {
			case map[string]any, []any:
				out = extractAITexts(current[key], out)
			}
		}
	case []any:
		for _, item := range current {
			out = extractAITexts(item, out)
		}
	}
	return out
}

func extractAITextsFromEvents(events []any) []TextItem {
	out := extractAITexts(events, []TextItem{})
	responseBuffers := make(map[string][]string)
	var responseOrder []string
	chatBuffers := make(map[string][]string)
	var chatOrder []string
	for _, eventValue := range events {
		event, ok := asMap(eventValue)
		if !ok {
			continue
		}
		if stringValue(event["type"]) == "response.output_text.delta" {
			if delta := stringValue(event["delta"]); delta != "" {
				itemID := stringValue(event["item_id"])
				if itemID == "" {
					itemID = stringValue(event["output_index"])
				}
				if itemID == "" || itemID == "0" {
					itemID = "default"
				}
				if _, exists := responseBuffers[itemID]; !exists {
					responseOrder = append(responseOrder, itemID)
				}
				responseBuffers[itemID] = append(responseBuffers[itemID], delta)
			}
		}
		if choices, ok := asSlice(event["choices"]); ok {
			for _, choiceValue := range choices {
				choice, ok := asMap(choiceValue)
				if !ok {
					continue
				}
				deltaObject, ok := asMap(choice["delta"])
				if !ok {
					continue
				}
				content := stringValue(deltaObject["content"])
				if content == "" {
					continue
				}
				choiceID := stringValue(choice["index"])
				if choiceID == "" {
					choiceID = "0"
				}
				if _, exists := chatBuffers[choiceID]; !exists {
					chatOrder = append(chatOrder, choiceID)
				}
				chatBuffers[choiceID] = append(chatBuffers[choiceID], content)
			}
		}
	}
	for _, id := range responseOrder {
		out = appendUniqueText(out, strings.Join(responseBuffers[id], ""), "response.output_text.delta:"+id)
	}
	for _, id := range chatOrder {
		out = appendUniqueText(out, strings.Join(chatBuffers[id], ""), "chat.completion.delta:"+id)
	}
	return out
}

func firstMeaningful(values ...any) any {
	for _, value := range values {
		switch current := value.(type) {
		case nil:
			continue
		case string:
			if current == "" {
				continue
			}
		}
		return value
	}
	return map[string]any{}
}

func safeJSONArg(value any) any {
	if text, ok := value.(string); ok {
		if parsed, valid := parseJSON(text); valid {
			return parsed
		}
	}
	return value
}

func collectToolCalls(value any, out *[]ToolCall) {
	switch current := value.(type) {
	case map[string]any:
		if toolCalls, ok := asSlice(current["tool_calls"]); ok {
			for _, toolValue := range toolCalls {
				tool, ok := asMap(toolValue)
				if !ok {
					continue
				}
				function, _ := asMap(tool["function"])
				name := stringValue(firstMeaningful(function["name"], tool["name"], tool["type"]))
				args := firstMeaningful(function["arguments"], tool["arguments"], tool["input"])
				*out = append(*out, ToolCall{Name: name, Arguments: safeJSONArg(args)})
			}
		}
		valueType := stringValue(current["type"])
		if valueType == "function_call" {
			name := stringValue(current["name"])
			if name == "" {
				name = "function_call"
			}
			*out = append(*out, ToolCall{Name: name, Arguments: safeJSONArg(firstMeaningful(current["arguments"]))})
		}
		if valueType == "response.output_item.done" {
			if item, ok := asMap(current["item"]); ok && stringValue(item["type"]) == "function_call" {
				name := stringValue(item["name"])
				if name == "" {
					name = "function_call"
				}
				*out = append(*out, ToolCall{Name: name, Arguments: safeJSONArg(firstMeaningful(item["arguments"]))})
			}
		}
		if valueType == "tool_use" || valueType == "server_tool_use" {
			name := stringValue(current["name"])
			if name == "" {
				name = valueType
			}
			*out = append(*out, ToolCall{Name: name, Arguments: firstMeaningful(current["input"])})
		}
		if name := stringValue(current["name"]); name != "" {
			if input, ok := asMap(current["input"]); ok && toolNameMatches(name) {
				*out = append(*out, ToolCall{Name: name, Arguments: input})
			}
		}
		for _, key := range orderedKeys(current) {
			switch current[key].(type) {
			case map[string]any, []any:
				collectToolCalls(current[key], out)
			}
		}
	case []any:
		for _, item := range current {
			collectToolCalls(item, out)
		}
	}
}

func toolNameMatches(name string) bool {
	name = strings.ToLower(name)
	for _, hint := range toolNameHints {
		if strings.Contains(name, hint) {
			return true
		}
	}
	return false
}

func normalizePath(path string) string {
	path = strings.Trim(path, " \t\r\n'\"` ,;:()[]{}")
	if path == "" || strings.Contains(path, "://") {
		return ""
	}
	if strings.HasPrefix(path, "/") {
		return pathpkg.Clean(path)
	}
	if !strings.Contains(path, "/") {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == ".." {
			return ""
		}
	}
	return path
}

func pathsFromValue(value any) []string {
	var paths []string
	switch current := value.(type) {
	case string:
		if !strings.ContainsAny(current, " \t\r\n|;&<>") {
			if path := normalizePath(current); path != "" {
				paths = append(paths, path)
			}
		}
		for _, match := range pathRE.FindAllStringSubmatch(current, -1) {
			if len(match) > 1 {
				if path := normalizePath(match[1]); path != "" {
					paths = append(paths, path)
				}
			}
		}
	case []any:
		for _, item := range current {
			paths = append(paths, pathsFromValue(item)...)
		}
	case map[string]any:
		for _, key := range orderedKeys(current) {
			child := current[key]
			if fileArgKeys[key] {
				paths = append(paths, pathsFromValue(child)...)
				continue
			}
			switch child.(type) {
			case map[string]any, []any, string:
				paths = append(paths, pathsFromValue(child)...)
			}
		}
	}
	return paths
}

func inferType(path string) string {
	base := pathpkg.Base(strings.TrimSuffix(path, "/"))
	if strings.HasSuffix(path, "/") || !strings.Contains(base, ".") {
		return "directory"
	}
	return "file"
}

type treeNode map[string]treeNode

func buildTree(paths []string) string {
	root := treeNode{}
	for _, path := range paths {
		clean := strings.TrimPrefix(path, "/")
		if clean == "" {
			clean = path
		}
		current := root
		for _, part := range strings.Split(clean, "/") {
			if current[part] == nil {
				current[part] = treeNode{}
			}
			current = current[part]
		}
	}
	var lines []string
	var walk func(treeNode, int)
	walk = func(node treeNode, indent int) {
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, strings.Repeat("  ", indent)+key)
			walk(node[key], indent+1)
		}
	}
	walk(root, 0)
	return strings.Join(lines, "\n")
}
