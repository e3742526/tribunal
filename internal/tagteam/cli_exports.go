package tagteam

func ReadLatestForCLI(workdir string) (LatestRun, error) {
	return readLatest(workdir)
}

func ReadFinalForCLI(path string) (FinalRun, error) {
	return readFinal(path)
}

func UserConfigPathForCLI() (string, error) {
	return userConfigPath()
}

func EnsureGitignoreEntryForCLI(workdir, entry string) error {
	return ensureGitignoreEntry(workdir, entry)
}
