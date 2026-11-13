package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A workspace member is audited through its root's lockfile, so init must not
// demand an audit of its own — but a nested project that is NOT a member (its
// own lockfile, its own deps) still needs one.
func TestDetectStackFoldsWorkspaceMembersIntoTheirRoot(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("web/package.json", `{"name":"web","workspaces":["packages/*"]}`)
	write("web/packages/client/package.json", `{"name":"@devpro/client"}`)
	write("web/packages/ui/package.json", `{"name":"@devpro/ui"}`)
	write("web/tools/legacy/package.json", `{"name":"legacy"}`) // nested, not a member
	write("yarnish/package.json", `{"name":"y","workspaces":{"packages":["libs/*"]}}`)
	write("yarnish/libs/a/package.json", `{"name":"a"}`)

	var got []string
	for _, n := range detectStack(root).node {
		got = append(got, n.dir)
	}
	want := []string{"web", "web/tools/legacy", "yarnish"}
	if len(got) != len(want) {
		t.Fatalf("node projects = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != filepath.FromSlash(want[i]) {
			t.Fatalf("node projects = %v, want %v", got, want)
		}
	}
}
