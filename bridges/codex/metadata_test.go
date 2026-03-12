package codex

import "testing"

func TestIsHostAuthLogin_WithExplicitHostSource(t *testing.T) {
	meta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceHost}
	if !isHostAuthLogin(meta) {
		t.Fatal("expected host source to be treated as host-auth login")
	}
}

func TestIsManagedAuthLogin_SourceManaged(t *testing.T) {
	meta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceManaged}
	if !isManagedAuthLogin(meta) {
		t.Fatal("expected managed source to be treated as managed login")
	}
}

func TestIsManagedAuthLogin_LegacyManagedFlag(t *testing.T) {
	meta := &UserLoginMetadata{CodexHomeManaged: true}
	if !isManagedAuthLogin(meta) {
		t.Fatal("expected legacy managed flag to be treated as managed login")
	}
}

func TestIsHostAuthLogin_DistinguishesManagedFromHost(t *testing.T) {
	hostMeta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceHost}
	if !isHostAuthLogin(hostMeta) {
		t.Fatal("expected host-auth login to be recognized")
	}

	managedMeta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceManaged}
	if isHostAuthLogin(managedMeta) {
		t.Fatal("expected managed login to not be host-auth")
	}
}
