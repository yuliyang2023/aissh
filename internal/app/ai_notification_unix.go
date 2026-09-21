//go:build !windows

package app

import "context"

func newAINotifier(context.Context) func(string, string) {
	return nil
}
