package connector

import "github.com/openai/openai-go/v3/responses"

const ToolNameBetterWebSearch = "better_web_search"

func normalizeToolAlias(name string) string {
	switch name {
	case ToolNameBetterWebSearch:
		return ToolNameWebSearch
	default:
		return name
	}
}

func renameWebSearchToolParams(tools []responses.ToolUnionParam) []responses.ToolUnionParam {
	for i := range tools {
		if tools[i].OfFunction == nil {
			continue
		}
		if tools[i].OfFunction.Name == ToolNameWebSearch {
			tools[i].OfFunction.Name = ToolNameBetterWebSearch
		}
	}
	return tools
}
