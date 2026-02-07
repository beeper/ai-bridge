package connector

import (
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func newMediaTestClient(meta *UserLoginMetadata, oc *OpenAIConnector) *AIClient {
	login := &database.UserLogin{
		ID:       networkid.UserLoginID("login"),
		Metadata: meta,
	}
	userLogin := &bridgev2.UserLogin{UserLogin: login, Log: zerolog.Nop()}
	return &AIClient{
		UserLogin: userLogin,
		connector: oc,
	}
}

func TestResolveMediaProviderAPIKeyOpenAIMagicProxyUsesLoginToken(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	meta := &UserLoginMetadata{
		Provider: ProviderMagicProxy,
		APIKey:   "tok",
		BaseURL:  "https://bai.bt.hn/team/proxy",
	}
	client := newMediaTestClient(meta, &OpenAIConnector{})

	if got := client.resolveMediaProviderAPIKey("openai", "", ""); got != "tok" {
		t.Fatalf("unexpected key: %q", got)
	}
}

func TestResolveOpenAIMediaBaseURLMagicProxyUsesOpenAIServicePath(t *testing.T) {
	meta := &UserLoginMetadata{
		Provider: ProviderMagicProxy,
		APIKey:   "tok",
		BaseURL:  "https://bai.bt.hn/team/proxy",
	}
	client := newMediaTestClient(meta, &OpenAIConnector{})

	if got := resolveOpenAIMediaBaseURL(client); got != "https://bai.bt.hn/team/proxy/openai/v1" {
		t.Fatalf("unexpected base url: %q", got)
	}
}

func TestResolveOpenAIMediaBaseURLBeeperUsesOpenAIServicePath(t *testing.T) {
	meta := &UserLoginMetadata{
		Provider: ProviderBeeper,
		APIKey:   "tok",
		BaseURL:  "https://matrix.example.com",
	}
	client := newMediaTestClient(meta, &OpenAIConnector{})

	want := "https://matrix.example.com/_matrix/client/unstable/com.beeper.ai/openai/v1"
	if got := resolveOpenAIMediaBaseURL(client); got != want {
		t.Fatalf("unexpected base url: got %q want %q", got, want)
	}
}
