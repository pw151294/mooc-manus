package llm

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}
