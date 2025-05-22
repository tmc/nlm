package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/beprotojson"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// ChunkedResponseParser is a specialized parser for NotebookLM's chunked response format
// which parses the special format used for the ListRecentlyViewedProjects response
type ChunkedResponseParser struct {
	Raw         string
	Debug       bool
	rawChunks   []string
	cleanedData string
}

// NewChunkedResponseParser creates a new parser for the given raw response
func NewChunkedResponseParser(raw string) *ChunkedResponseParser {
	return &ChunkedResponseParser{
		Raw:   raw,
		Debug: false,
	}
}

// WithDebug enables debug output for this parser
func (p *ChunkedResponseParser) WithDebug(debug bool) *ChunkedResponseParser {
	p.Debug = debug
	return p
}

// logDebug logs a message if debug mode is enabled
func (p *ChunkedResponseParser) logDebug(format string, args ...interface{}) {
	if p.Debug {
		fmt.Printf("[ChunkedParser] "+format+"\n", args...)
	}
}

// ParseListProjectsResponse extracts projects from the raw response with fallback mechanisms
func (p *ChunkedResponseParser) ParseListProjectsResponse() ([]*pb.Project, error) {
	// Initialize chunks
	p.rawChunks = p.extractChunks()

	// Step 1: Try parsing using standard JSON techniques
	projects, err := p.parseStandardJSON()
	if err == nil && len(projects) > 0 {
		p.logDebug("Successfully parsed %d projects using standard JSON method", len(projects))
		return projects, nil
	}
	
	p.logDebug("Standard JSON parsing failed: %v, trying regex method", err)

	// Step 2: Try using regex-based extraction (legacy approach enhanced)
	projects, err = p.parseWithRegex()
	if err == nil && len(projects) > 0 {
		p.logDebug("Successfully parsed %d projects using regex method", len(projects))
		return projects, nil
	}
	
	p.logDebug("Regex parsing failed: %v, trying direct scan method", err)

	// Step 3: Try direct scanning for projects (most robust but less accurate)
	projects, err = p.parseDirectScan()
	if err == nil && len(projects) > 0 {
		p.logDebug("Successfully parsed %d projects using direct scan method", len(projects))
		return projects, nil
	}

	// If we get here, all methods failed
	return nil, fmt.Errorf("failed to parse projects: %w", err)
}

// extractChunks preprocesses the raw response into clean chunks
func (p *ChunkedResponseParser) extractChunks() []string {
	// Remove the typical chunked response header
	cleanedResponse := strings.TrimSpace(strings.TrimPrefix(p.Raw, ")]}'"))
	
	// Handle trailing digits (like "25") that might appear at the end of the response
	// This is a common issue we're seeing in the error message
	if len(cleanedResponse) > 0 {
		// Trim trailing numeric values that may represent chunk sizes
		re := regexp.MustCompile(`\n\d+$`)
		cleanedResponse = re.ReplaceAllString(cleanedResponse, "")
	}
	
	// Save the cleaned data for other methods to use
	p.cleanedData = cleanedResponse
	
	// Split by newline to get individual chunks
	chunks := strings.Split(cleanedResponse, "\n")
	
	// Filter out chunks that are just numbers (chunk size indicators)
	var filteredChunks []string
	for _, chunk := range chunks {
		if !isNumeric(strings.TrimSpace(chunk)) {
			filteredChunks = append(filteredChunks, chunk)
		}
	}
	
	return filteredChunks
}

// parseStandardJSON attempts to extract projects using JSON unmarshaling
func (p *ChunkedResponseParser) parseStandardJSON() ([]*pb.Project, error) {
	var jsonSection string
	
	// Look for the first chunk containing "wrb.fr" and "wXbhsf"
	for _, chunk := range p.rawChunks {
		if strings.Contains(chunk, "\"wrb.fr\"") && strings.Contains(chunk, "\"wXbhsf\"") {
			jsonSection = chunk
			break
		}
	}
	
	if jsonSection == "" {
		return nil, fmt.Errorf("failed to find JSON section containing 'wrb.fr'")
	}
	
	// Try to unmarshal the entire JSON section
	var wrbResponse []interface{}
	err := json.Unmarshal([]byte(jsonSection), &wrbResponse)
	if err != nil {
		// Try to extract just the array part
		arrayStart := strings.Index(jsonSection, "[[")
		if arrayStart >= 0 {
			arrayEnd := strings.LastIndex(jsonSection, "]]")
			if arrayEnd >= 0 && arrayEnd > arrayStart {
				arrayString := jsonSection[arrayStart : arrayEnd+2]
				err = json.Unmarshal([]byte(arrayString), &wrbResponse)
				if err != nil {
					return nil, fmt.Errorf("failed to parse array part: %w", err)
				}
			}
		}
		
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	}
	
	// Extract the projects data, which is typically at index 2
	if len(wrbResponse) < 3 {
		return nil, fmt.Errorf("unexpected response format: array too short (len=%d)", len(wrbResponse))
	}
	
	var projectsRaw string
	switch v := wrbResponse[2].(type) {
	case string:
		projectsRaw = v
	default:
		return nil, fmt.Errorf("unexpected type for project data: %T", wrbResponse[2])
	}
	
	// Unescape the JSON string (double-quoted)
	var unescaped string
	err = json.Unmarshal([]byte("\""+projectsRaw+"\""), &unescaped)
	if err != nil {
		return nil, fmt.Errorf("failed to unescape project data: %w", err)
	}
	
	// Try to parse as an array of arrays
	var projectsData []interface{}
	err = json.Unmarshal([]byte(unescaped), &projectsData)
	if err != nil {
		// Handle specific error case from the error message
		if strings.Contains(err.Error(), "cannot unmarshal object into Go value of type []interface {}") {
			// Try fallback approach for object-style response
			return p.parseAsObject(unescaped)
		}
		return nil, fmt.Errorf("failed to parse project data as array: %w", err)
	}
	
	// Now extract projects from the project list
	var projects []*pb.Project
	for _, item := range projectsData {
		projectArray, ok := item.([]interface{})
		if !ok || len(projectArray) < 3 {
			continue // Skip any non-array or too-short arrays
		}
		
		project := &pb.Project{}
		
		// Extract title (typically at index 0)
		if title, ok := projectArray[0].(string); ok {
			project.Title = title
		}
		
		// Extract ID (typically at index 2)
		if id, ok := projectArray[2].(string); ok {
			project.ProjectId = id
		}
		
		// Extract emoji (typically at index 3 if available)
		if len(projectArray) > 3 {
			if emoji, ok := projectArray[3].(string); ok {
				project.Emoji = emoji
			} else {
				project.Emoji = "📄" // Default emoji
			}
		} else {
			project.Emoji = "📄" // Default emoji
		}
		
		// Add to results if we have an ID and title
		if project.ProjectId != "" {
			projects = append(projects, project)
		}
	}
	
	if len(projects) == 0 {
		return nil, fmt.Errorf("parsed JSON but found no valid projects")
	}
	
	return projects, nil
}

// parseAsObject attempts to parse the data as a JSON object instead of an array
func (p *ChunkedResponseParser) parseAsObject(data string) ([]*pb.Project, error) {
	var projectMap map[string]interface{}
	err := json.Unmarshal([]byte(data), &projectMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse as object: %w", err)
	}
	
	var projects []*pb.Project
	
	// Look for project objects in the map
	for key, value := range projectMap {
		// Look for UUID-like keys
		if isUUIDLike(key) {
			// This might be a project ID
			proj := &pb.Project{
				ProjectId: key,
				Emoji:     "📄", // Default emoji
			}
			
			// Try to extract title from the value
			if projData, ok := value.(map[string]interface{}); ok {
				if title, ok := projData["title"].(string); ok {
					proj.Title = title
				} else if title, ok := projData["name"].(string); ok {
					proj.Title = title
				}
				
				// Try to extract emoji
				if emoji, ok := projData["emoji"].(string); ok {
					proj.Emoji = emoji
				}
			}
			
			// If we couldn't extract a title, use a placeholder
			if proj.Title == "" {
				proj.Title = "Project " + key[:8]
			}
			
			projects = append(projects, proj)
		}
	}
	
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found in object format")
	}
	
	return projects, nil
}

// parseWithRegex uses the enhanced regex-based approach
func (p *ChunkedResponseParser) parseWithRegex() ([]*pb.Project, error) {
	// Attempt to find the wrb.fr,wXbhsf section with project data
	wrbfrPattern := regexp.MustCompile(`\[\[\"wrb\.fr\",\"wXbhsf\",\"(.*?)\"\,`)
	matches := wrbfrPattern.FindStringSubmatch(p.cleanedData)
	
	// Try alternative quotes
	if len(matches) < 2 {
		wrbfrPattern = regexp.MustCompile(`\[\["wrb\.fr","wXbhsf","(.*?)",`)
		matches = wrbfrPattern.FindStringSubmatch(p.cleanedData)
	}
	
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find project data section in response")
	}
	
	// The project data is in the first capture group
	projectDataStr := matches[1]
	
	// Unescape the JSON string
	projectDataStr = strings.ReplaceAll(projectDataStr, "\\\"", "\"")
	projectDataStr = strings.ReplaceAll(projectDataStr, "\\\\", "\\")
	
	// Debugging info
	p.logDebug("Project data string (first 100 chars): %s", truncate(projectDataStr, 100))
	
	// Find projects with title, ID, and emoji
	var projects []*pb.Project
	
	// First try to identify project titles
	titlePattern := regexp.MustCompile(`\[\[\[\"([^\"]+?)\"`)
	titleMatches := titlePattern.FindAllStringSubmatch(projectDataStr, -1)
	
	for _, match := range titleMatches {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		
		title := match[1]
		// Look for project ID near the title
		idPattern := regexp.MustCompile(fmt.Sprintf(`\["%s"[^\]]*?,[^\]]*?,"([a-zA-Z0-9-]+)"`, regexp.QuoteMeta(title)))
		idMatch := idPattern.FindStringSubmatch(projectDataStr)
		
		projectID := ""
		if len(idMatch) > 1 {
			projectID = idMatch[1]
		}
		
		// If we couldn't find ID directly, try to extract the first UUID-like pattern nearby
		if projectID == "" {
			// Look for a UUID-like pattern
			uuidPattern := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
			// Find within reasonable distance of title
			searchStart := strings.Index(projectDataStr, title)
			if searchStart > 0 {
				searchEnd := min(searchStart+500, len(projectDataStr))
				uuidMatches := uuidPattern.FindStringSubmatch(projectDataStr[searchStart:searchEnd])
				if len(uuidMatches) > 0 {
					projectID = uuidMatches[0]
				}
			}
		}
		
		if projectID == "" {
			// Skip projects without ID
			continue
		}
		
		// Look for emoji (typically a short string within quotes)
		emoji := "📄" // Default emoji
		emojiPattern := regexp.MustCompile(`"([^"]{1,5})"`)
		// Look within reasonable distance after projectID
		searchStart := strings.Index(projectDataStr, projectID)
		if searchStart > 0 {
			searchEnd := min(searchStart+100, len(projectDataStr))
			emojiMatches := emojiPattern.FindAllStringSubmatch(projectDataStr[searchStart:searchEnd], -1)
			for _, emojiMatch := range emojiMatches {
				if len(emojiMatch) > 1 && len(emojiMatch[1]) <= 2 {
					// Most emojis are 1-2 characters
					emoji = emojiMatch[1]
					break
				}
			}
		}
		
		projects = append(projects, &pb.Project{
			Title:     title,
			ProjectId: projectID,
			Emoji:     emoji,
		})
	}
	
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found using regex patterns")
	}
	
	return projects, nil
}

// parseDirectScan directly scans for UUID patterns and tries to find titles nearby
func (p *ChunkedResponseParser) parseDirectScan() ([]*pb.Project, error) {
	// Scan the entire response for UUIDs (project IDs)
	uuidPattern := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
	uuidMatches := uuidPattern.FindAllString(p.cleanedData, -1)
	
	if len(uuidMatches) == 0 {
		return nil, fmt.Errorf("no UUID-like project IDs found in the response")
	}
	
	// Deduplicate project IDs
	seenIDs := make(map[string]bool)
	var uniqueIDs []string
	for _, id := range uuidMatches {
		if !seenIDs[id] {
			seenIDs[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	
	var projects []*pb.Project
	// For each ID, look for title nearby
	for _, id := range uniqueIDs {
		project := &pb.Project{
			ProjectId: id,
			Emoji:     "📄", // Default emoji
		}
		
		// Try to find a title near the ID
		idIndex := strings.Index(p.cleanedData, id)
		if idIndex > 0 {
			// Look before the ID for title (up to 500 chars before)
			beforeStart := max(0, idIndex-500)
			beforeText := p.cleanedData[beforeStart:idIndex]
			
			// Title pattern: typically in quotes and more than 3 chars
			titlePattern := regexp.MustCompile(`"([^"]{3,100})"`)
			titleMatches := titlePattern.FindAllStringSubmatch(beforeText, -1)
			
			if len(titleMatches) > 0 {
				// Take the title closest to the ID
				lastMatch := titleMatches[len(titleMatches)-1]
				if len(lastMatch) > 1 {
					project.Title = lastMatch[1]
				}
			}
			
			// Look after the ID for emoji (within 100 chars)
			afterEnd := min(len(p.cleanedData), idIndex+100)
			afterText := p.cleanedData[idIndex:afterEnd]
			
			// Emoji pattern: short string in quotes after ID
			emojiPattern := regexp.MustCompile(`"([^"]{1,2})"`)
			emojiMatches := emojiPattern.FindStringSubmatch(afterText)
			if len(emojiMatches) > 1 {
				project.Emoji = emojiMatches[1]
			}
		}
		
		// If we don't have a title, use a placeholder
		if project.Title == "" {
			project.Title = "Notebook " + id[:8]
		}
		
		projects = append(projects, project)
	}
	
	return projects, nil
}

// SanitizeResponse removes any trailing or invalid content from the response
// This is particularly useful for handling trailing digits like "25" in the error case
func (p *ChunkedResponseParser) SanitizeResponse(input string) string {
	// Remove chunked response prefix
	input = strings.TrimPrefix(input, ")]}'")
	
	// Process line by line to handle chunk sizes correctly
	lines := strings.Split(input, "\n")
	var result []string
	
	for i, line := range lines {
		line = strings.TrimSpace(line)
		
		// Skip empty lines
		if line == "" {
			continue
		}
		
		// Skip standalone numeric lines that are likely chunk sizes
		if isNumeric(line) {
			continue
		}
		
		// Check if this is a JSON array or object
		if (strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")) ||
		   (strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}")) {
			result = append(result, line)
			continue
		}
		
		// If line starts with [ but doesn't end with ], check if subsequent lines complete it
		if strings.HasPrefix(line, "[") && !strings.HasSuffix(line, "]") {
			// Try to find the end of this array/object
			var completeJson string
			completeJson = line
			
			for j := i + 1; j < len(lines); j++ {
				nextLine := strings.TrimSpace(lines[j])
				if nextLine == "" || isNumeric(nextLine) {
					continue
				}
				
				completeJson += nextLine
				
				// Check if we've completed the JSON structure
				if balancedBrackets(completeJson) {
					result = append(result, completeJson)
					break
				}
			}
		}
	}
	
	// Join lines back together
	return strings.Join(result, "\n")
}

// balancedBrackets checks if a string has balanced brackets ([], {})
func balancedBrackets(s string) bool {
	stack := []rune{}
	
	for _, char := range s {
		switch char {
		case '[', '{':
			stack = append(stack, char)
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return false
			}
			stack = stack[:len(stack)-1]
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	
	return len(stack) == 0
}

// TryParseAsJSONArray attempts to extract and parse a JSON array from the response
// This is a fallback approach for the specific error in the requirements
func (p *ChunkedResponseParser) TryParseAsJSONArray() ([]interface{}, error) {
	// First clean the response to remove any trailing characters
	cleanedResponse := p.SanitizeResponse(p.Raw)
	
	// Find JSON array patterns
	arrayPattern := regexp.MustCompile(`\[\[.*?\]\]`)
	matches := arrayPattern.FindAllString(cleanedResponse, -1)
	
	for _, potentialArray := range matches {
		var result []interface{}
		err := json.Unmarshal([]byte(potentialArray), &result)
		if err == nil && len(result) > 0 {
			return result, nil
		}
	}
	
	// If we can't find a valid JSON array, try more aggressively
	// Find the start and end of what looks like a JSON array
	start := strings.Index(cleanedResponse, "[[")
	if start >= 0 {
		// Find the balanced end of this array
		bracketCount := 0
		end := start
		for i := start; i < len(cleanedResponse); i++ {
			if cleanedResponse[i] == '[' {
				bracketCount++
			} else if cleanedResponse[i] == ']' {
				bracketCount--
				if bracketCount == 0 {
					end = i + 1
					break
				}
			}
		}
		
		if end > start {
			arrayStr := cleanedResponse[start:end]
			var result []interface{}
			err := json.Unmarshal([]byte(arrayStr), &result)
			if err == nil {
				return result, nil
			}
			// If still failing, try our special beprotojson parser
			result, err = beprotojson.UnmarshalArray(arrayStr)
			if err == nil {
				return result, nil
			}
			
			return nil, fmt.Errorf("failed to parse JSON array '%s': %w", truncate(arrayStr, 50), err)
		}
	}
	
	return nil, fmt.Errorf("no valid JSON array found in response")
}

// ParseJSONArray parses a JSON array from the response with robust error handling
func (p *ChunkedResponseParser) ParseJSONArray() ([]interface{}, error) {
	// Try standard JSON parsing first
	var result []interface{}
	jsonData := strings.TrimPrefix(p.Raw, ")]}'")
	
	// Try to find what looks like a chunk that contains JSON
	chunks := strings.Split(jsonData, "\n")
	var jsonChunk string
	
	for _, chunk := range chunks {
		if strings.HasPrefix(chunk, "[[") || strings.HasPrefix(chunk, "{") {
			jsonChunk = chunk
			break
		}
	}
	
	if jsonChunk == "" {
		jsonChunk = jsonData // If we can't find a specific chunk, use the whole data
	}
	
	// Handle the case where there are trailing digits (like "25")
	// These might be chunk size indicators
	if len(jsonChunk) > 0 {
		if i := strings.LastIndex(jsonChunk, "]}"); i > 0 {
			// Look for any trailing content after the last valid closing bracket
			trailingContent := jsonChunk[i+2:]
			if len(trailingContent) > 0 {
				// If there's trailing content that's not JSON, truncate it
				if !strings.HasPrefix(trailingContent, "[") && !strings.HasPrefix(trailingContent, "{") {
					jsonChunk = jsonChunk[:i+2]
				}
			}
		}
	}
	
	// Try standard JSON unmarshaling
	err := json.Unmarshal([]byte(jsonChunk), &result)
	if err != nil {
		p.logDebug("Standard JSON parsing failed: %v, trying fallback approach", err)
		
		// If the object unmarshal fails with the exact error we're targeting
		if strings.Contains(err.Error(), "cannot unmarshal object into Go value of type []interface {}") {
			// Try additional fallback approaches
			return p.TryParseAsJSONArray()
		}
		
		// Try to find just the array part
		arrayStart := strings.Index(jsonChunk, "[[")
		if arrayStart >= 0 {
			arrayEnd := strings.LastIndex(jsonChunk, "]]")
			if arrayEnd >= 0 && arrayEnd > arrayStart {
				arrayStr := jsonChunk[arrayStart : arrayEnd+2]
				err = json.Unmarshal([]byte(arrayStr), &result)
				if err == nil {
					return result, nil
				}
				
				// Try custom unmarshal with our specialized beprotojson package
				result, err = beprotojson.UnmarshalArray(arrayStr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse array portion: %w", err)
				}
				return result, nil
			}
		}
	}
	
	if len(result) == 0 {
		return nil, fmt.Errorf("parsed empty JSON array from response")
	}
	
	return result, nil
}

// Helper function to truncate text for display
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// isUUIDLike checks if a string looks like a UUID
func isUUIDLike(s string) bool {
	return regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(s)
}

// DebugPrint prints the chunked response analysis for debugging
func (p *ChunkedResponseParser) DebugPrint() {
	chunks := strings.Split(p.Raw, "\n")
	fmt.Println("=== Chunked Response Analysis ===")
	fmt.Printf("Total chunks: %d\n", len(chunks))
	
	for i, chunk := range chunks {
		truncated := truncate(chunk, 100)
		fmt.Printf("Chunk %d: %s\n", i, truncated)
		
		// Detect chunk size indicators
		if isNumeric(chunk) && i < len(chunks)-1 {
			nextChunkLen := len(chunks[i+1])
			fmt.Printf("  -> Possible chunk size: %s, next chunk len: %d\n", chunk, nextChunkLen)
		}
		
		// Try to identify the JSON section
		if strings.Contains(chunk, "\"wrb.fr\"") {
			fmt.Printf("  -> Contains wrb.fr, likely contains project data\n")
		}
	}
	
	// Try to check if the response ends with a number (like "25")
	lastChunk := chunks[len(chunks)-1]
	if isNumeric(lastChunk) {
		fmt.Printf("NOTE: Response ends with number %s, which may cause parsing issues\n", lastChunk)
	}
}