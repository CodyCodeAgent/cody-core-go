package output

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOutputTool(t *testing.T) {
	params, err := BuildParamsOneOf[BasicStruct]()
	require.NoError(t, err)

	tool := GenerateOutputTool[BasicStruct](params)
	require.NotNil(t, tool)

	info, err := tool.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, DefaultOutputToolName, info.Name)
	assert.Contains(t, info.Desc, "final response")
}

func TestGenerateOutputToolWithName(t *testing.T) {
	params, err := BuildParamsOneOf[BasicStruct]()
	require.NoError(t, err)

	tool := GenerateOutputToolWithName[BasicStruct]("final_result_BasicStruct", params)
	require.NotNil(t, tool)

	info, err := tool.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "final_result_BasicStruct", info.Name)
}

func TestIsOutputToolName(t *testing.T) {
	assert.True(t, IsOutputToolName("final_result"))
	assert.True(t, IsOutputToolName("final_result_MyType"))
	assert.True(t, IsOutputToolName("final_result_VulnFound"))
	assert.False(t, IsOutputToolName("search"))
	assert.False(t, IsOutputToolName("get_weather"))
	assert.False(t, IsOutputToolName("final"))
	assert.False(t, IsOutputToolName("final_results"))
}

func TestOutputToolInvokableRun_ReturnsError(t *testing.T) {
	params, err := BuildParamsOneOf[BasicStruct]()
	require.NoError(t, err)

	tool := GenerateOutputTool[BasicStruct](params)
	_, err = tool.InvokableRun(context.Background(), `{}`)
	assert.Error(t, err) // Output tool should not be invoked directly
}
