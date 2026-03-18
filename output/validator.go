package output

import "context"

// ValidatorFunc validates and optionally transforms an output value.
// Return the (possibly modified) output and nil error on success.
// Return an error to trigger a model retry (use agent.NewModelRetry for retry feedback).
type ValidatorFunc[O any] func(ctx context.Context, output O) (O, error)

// RunValidators runs all validators in sequence on the output.
// Returns the final output and any error from the first failing validator.
func RunValidators[O any](ctx context.Context, output O, validators []ValidatorFunc[O]) (O, error) {
	var err error
	for _, v := range validators {
		output, err = v(ctx, output)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}
