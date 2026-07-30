package mirror

import "context"

// WrapCommandPathForTest exposes command PATH wrapping for tests.
func WrapCommandPathForTest(cmd string, pathPrefixes []string) string {
	return wrapCommandPath(cmd, pathPrefixes)
}

// PackageInstallScriptForTest exposes package install scripts for tests.
func PackageInstallScriptForTest(packages []string) string {
	return packageInstallScript(packages)
}

// YumPackagesForTest exposes yum package mapping for tests.
func YumPackagesForTest(packages []string) []string {
	return yumPackages(packages)
}

// PlanFingerprintForTest exposes toolchain plan fingerprints for tests.
func PlanFingerprintForTest(plan ToolchainPlan) string {
	return planFingerprint(plan)
}

// EnsureToolchainForRunForTest exposes mirror run toolchain wrapping for tests.
func (r *Runner) EnsureToolchainForRunForTest(ctx context.Context, command string, skip bool) (string, error) {
	return r.ensureToolchainForRun(ctx, command, skip)
}
