package agent

import (
	"testing"

	"github.com/codycode/cody-core-go/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type VulnFound struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

type CodeSafe struct {
	Summary string `json:"summary"`
}

type ExtraType struct {
	Info string `json:"info"`
}

func TestOneOf2_Match(t *testing.T) {
	u := NewOneOf2A[VulnFound, CodeSafe](VulnFound{Type: "SQLi", Severity: "high"})

	aCalled := false
	bCalled := false
	u.Match(
		func(v VulnFound) {
			aCalled = true
			assert.Equal(t, "SQLi", v.Type)
		},
		func(s CodeSafe) {
			bCalled = true
		},
	)
	assert.True(t, aCalled)
	assert.False(t, bCalled)
}

func TestOneOf2_MatchB(t *testing.T) {
	u := NewOneOf2B[VulnFound, CodeSafe](CodeSafe{Summary: "all clear"})

	bCalled := false
	u.Match(
		func(v VulnFound) { t.Error("should not be called") },
		func(s CodeSafe) {
			bCalled = true
			assert.Equal(t, "all clear", s.Summary)
		},
	)
	assert.True(t, bCalled)
}

func TestOneOf2_Value(t *testing.T) {
	u := NewOneOf2A[VulnFound, CodeSafe](VulnFound{Type: "XSS"})
	v, ok := u.Value().(VulnFound)
	assert.True(t, ok)
	assert.Equal(t, "XSS", v.Type)
}

func TestOneOf3_Match(t *testing.T) {
	u := NewOneOf3C[VulnFound, CodeSafe, ExtraType](ExtraType{Info: "extra"})

	cCalled := false
	u.Match(
		func(v VulnFound) { t.Error("should not be called") },
		func(s CodeSafe) { t.Error("should not be called") },
		func(e ExtraType) {
			cCalled = true
			assert.Equal(t, "extra", e.Info)
		},
	)
	assert.True(t, cCalled)
}

func TestBuildOneOf2OutputTools(t *testing.T) {
	tools, infos, err := buildOneOf2OutputTools[VulnFound, CodeSafe]()
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Len(t, infos, 2)

	assert.Equal(t, output.DefaultOutputToolName+"_VulnFound", infos[0].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_CodeSafe", infos[1].toolName)
	assert.Equal(t, 0, infos[0].typeIndex)
	assert.Equal(t, 1, infos[1].typeIndex)
}

func TestBuildOneOf3OutputTools(t *testing.T) {
	tools, infos, err := buildOneOf3OutputTools[VulnFound, CodeSafe, ExtraType]()
	require.NoError(t, err)
	assert.Len(t, tools, 3)
	assert.Len(t, infos, 3)

	assert.Equal(t, output.DefaultOutputToolName+"_VulnFound", infos[0].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_CodeSafe", infos[1].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_ExtraType", infos[2].toolName)
}

func TestTypeName(t *testing.T) {
	assert.Equal(t, "VulnFound", typeName[VulnFound]())
	assert.Equal(t, "CodeSafe", typeName[CodeSafe]())
	assert.Equal(t, "int", typeName[int]())
	assert.Equal(t, "string", typeName[string]())
}
