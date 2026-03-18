package output

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunValidators_NoValidators(t *testing.T) {
	result, err := RunValidators[int](context.Background(), 42, nil)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestRunValidators_SinglePassingValidator(t *testing.T) {
	validators := []ValidatorFunc[int]{
		func(_ context.Context, v int) (int, error) {
			return v * 2, nil
		},
	}
	result, err := RunValidators(context.Background(), 21, validators)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestRunValidators_FailingValidator(t *testing.T) {
	validators := []ValidatorFunc[int]{
		func(_ context.Context, v int) (int, error) {
			if v < 0 {
				return v, fmt.Errorf("value must be non-negative")
			}
			return v, nil
		},
	}
	_, err := RunValidators(context.Background(), -1, validators)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-negative")
}

func TestRunValidators_ChainedValidators(t *testing.T) {
	validators := []ValidatorFunc[int]{
		func(_ context.Context, v int) (int, error) { return v + 1, nil },
		func(_ context.Context, v int) (int, error) { return v * 2, nil },
	}
	result, err := RunValidators(context.Background(), 5, validators)
	require.NoError(t, err)
	assert.Equal(t, 12, result) // (5+1)*2 = 12
}

func TestRunValidators_FirstFailStops(t *testing.T) {
	called := false
	validators := []ValidatorFunc[int]{
		func(_ context.Context, v int) (int, error) { return v, fmt.Errorf("fail") },
		func(_ context.Context, v int) (int, error) { called = true; return v, nil },
	}
	_, err := RunValidators(context.Background(), 1, validators)
	assert.Error(t, err)
	assert.False(t, called, "second validator should not be called after first fails")
}
