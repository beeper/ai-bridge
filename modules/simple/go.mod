module github.com/beeper/ai-bridge/modules/simple

go 1.24.0

require (
	github.com/beeper/ai-bridge v0.0.0-20260209155641-adfecbb4ed29
	github.com/beeper/ai-bridge/modules/runtime v0.0.0-20260209155641-adfecbb4ed29
	maunium.net/go/mautrix v0.26.3-0.20260129174719-d2364b382275
)

replace github.com/beeper/ai-bridge => ../..

replace github.com/beeper/ai-bridge/modules/runtime => ../runtime

replace github.com/beeper/ai-bridge/modules/contracts => ../contracts
