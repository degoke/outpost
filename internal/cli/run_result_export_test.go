package cli

import "github.com/degoke/outpost/internal/mirror"

// FinishRunResultForTest exposes run result handling for tests.
func (app *App) FinishRunResultForTest(result mirror.RunResult, err error) (int, error) {
	return finishRunResult(app, result, err)
}
