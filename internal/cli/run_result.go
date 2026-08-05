package cli

import (
	"os"

	"github.com/degoke/outpost/internal/mirror"
)

func finishRunResult(app *App, result mirror.RunResult, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	if app.Out.JSON {
		if err := app.Out.PrintJSON(result); err != nil {
			return 0, err
		}
	}
	return result.ExitCode, nil
}

func exitRunResult(app *App, result mirror.RunResult, err error) error {
	code, err := finishRunResult(app, result, err)
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}
