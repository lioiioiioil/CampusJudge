package protocol_test

import (
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
)

func TestArtifactsCannotEscapeOrAliasPaths(t *testing.T) {
	for _, filename := range []string{"", ".", "..", "../outside", "/absolute", "a/../b", "a//b", "a\\b", "a\x00b"} {
		t.Run(filename, func(t *testing.T) {
			if protocol.ValidateArtifacts([]protocol.Artifact{{Path: filename}}) == nil {
				t.Fatalf("unsafe path accepted: %q", filename)
			}
		})
	}
	if protocol.ValidateArtifacts([]protocol.Artifact{{Path: "Main.class"}, {Path: "Main.class"}}) == nil {
		t.Fatal("duplicate artifact paths accepted")
	}
	if err := protocol.ValidateArtifacts([]protocol.Artifact{{Path: "pkg/Main.class", Data: []byte("class")}}); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactPackageHasHardByteAndCountLimits(t *testing.T) {
	if protocol.ValidateArtifacts(nil) == nil {
		t.Fatal("empty artifacts accepted")
	}
	if protocol.ValidateArtifacts(make([]protocol.Artifact, protocol.MaxArtifactFiles+1)) == nil {
		t.Fatal("excessive artifact count accepted")
	}
	if protocol.ValidateArtifacts([]protocol.Artifact{{Path: "program", Data: []byte(strings.Repeat("x", protocol.MaxArtifactBytes+1))}}) == nil {
		t.Fatal("excessive artifact bytes accepted")
	}
}
