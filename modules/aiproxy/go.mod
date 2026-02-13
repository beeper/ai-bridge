module github.com/beeper/ai-bridge/modules/aiproxy

go 1.24.0

require github.com/beeper/ai-bridge v0.0.0-20260209155641-adfecbb4ed29

require (
	github.com/openai/openai-go/v3 v3.16.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
)

replace github.com/beeper/ai-bridge => ../..
