package upload

// QuoteSSHArgsForTest exposes rsync ssh arg quoting for tests.
func QuoteSSHArgsForTest(args []string) []string {
	return quoteSSHArgs(args)
}
