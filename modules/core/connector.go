package core

import (
	"github.com/beeper/ai-bridge/modules/simple"
	"github.com/beeper/ai-bridge/pkg/connector"
)

func NewConnector() *connector.OpenAIConnector {
	return simple.NewConnector()
}
