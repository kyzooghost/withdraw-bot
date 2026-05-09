package app

import (
	"context"
	"fmt"
)

const errConfigCheckFailed = "config check %d failed"

type Check func(ctx context.Context) error

func RunChecks(ctx context.Context, checks []Check) error {
	for index, check := range checks {
		if err := check(ctx); err != nil {
			return fmt.Errorf(errConfigCheckFailed+": %w", index+1, err)
		}
	}
	return nil
}
