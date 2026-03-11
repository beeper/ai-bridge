package codex

import "testing"

func TestIsHostAuthLogin_WithExplicitHostSource(t *testing.T) {
	meta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceHost}
	if !isHostAuthLogin(meta) {
		t.Fatal("expected host source to be treated as host-auth login")
	}
}

func TestIsHostAuthLogin_BackCompatLegacyHostMetadata(t *testing.T) {
	meta := &UserLoginMetadata{
		CodexAuthSource:  "",
		CodexHomeManaged: false,
		CodexHome:        "",
	}
	if !isHostAuthLogin(meta) {
		t.Fatal("expected legacy unmanaged empty-home login to be treated as host-auth login")
	}
}

func TestIsManagedAuthLogin_SourceManaged(t *testing.T) {
	meta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceManaged}
	if !isManagedAuthLogin(meta) {
		t.Fatal("expected managed source to be treated as managed login")
	}
}

func TestIsManagedAuthLogin_LegacyManagedFlag(t *testing.T) {
	meta := &UserLoginMetadata{
		CodexAuthSource:  "",
		CodexHomeManaged: true,
	}
	if !isManagedAuthLogin(meta) {
		t.Fatal("expected legacy managed flag to be treated as managed login")
	}
}

func TestShouldAttemptRemoteAccountLogout_HostAndManaged(t *testing.T) {
	hostMeta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceHost}
	if shouldAttemptRemoteAccountLogout(hostMeta) {
		t.Fatal("expected host-auth login to skip remote account/logout")
	}

	managedMeta := &UserLoginMetadata{CodexAuthSource: CodexAuthSourceManaged}
	if !shouldAttemptRemoteAccountLogout(managedMeta) {
		t.Fatal("expected managed login to call remote account/logout")
	}
}
