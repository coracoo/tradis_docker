package system

import "time"

type navigationAIConfiguration struct {
	Enabled             bool
	APIKey              string
	BaseURL             string
	Model               string
	Temperature         float64
	AllowCreateCategory bool
	NavigationPrompt    string
}

type navigationAIResult struct {
	Body     []byte
	Duration time.Duration
	Attempts int
	TraceID  string
}
